package cache

// Indice LRU in bbolt (Piano 16, Task 3, D16-T5).
//
// Per ogni namespace ns (body, thread, list) esiste il bucket "lru:<ns>" con:
//   - sotto-bucket "seq": chiave = numero di sequenza (uint64 big-endian da
//     NextSequence), valore = chiave dell'unità. L'ordine delle chiavi è l'ordine di
//     uso: Cursor.First() è la meno usata di recente;
//   - sotto-bucket "unit": chiave = chiave dell'unità, valore = sequenza (8 byte) +
//     JSON dell'elenco delle voci (bucket, chiave) che l'unità occupa;
//   - chiave "n": numero di unità (uint64 big-endian).
//
// Una unità può avere più voci: le copie dello stesso messaggio Gmail in mailbox
// diverse (D16-T4) occupano un posto solo e si rimuovono insieme.

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"

	bolt "go.etcd.io/bbolt"
)

// Namespace dell'indice LRU.
const (
	lruBody   = "body"
	lruThread = "thread"
	lruList   = "list"
)

// lruEntry è una voce occupata da una unità: chiave Key nel bucket Bucket.
type lruEntry struct {
	Bucket string
	Key    []byte
}

// pendingTouch è un tocco in lettura non ancora scritto su disco (D16-T6).
type pendingTouch struct {
	ns, unit string
}

// maxPendingTouches limita la memoria dei tocchi: oltre, si perdono i più vecchi
// (LRU approssimato, come per un crash).
const maxPendingTouches = 4096

// bodyUnit è la chiave LRU di un corpo: il messaggio Gmail (g<GmailID>), oppure,
// senza GmailID, la copia mailbox+chiave (D16-T4).
func bodyUnit(mailbox string, key []byte, gmailID uint64) string {
	if gmailID != 0 {
		return "g" + strconv.FormatUint(gmailID, 10)
	}
	return "i" + mailbox + "\x00" + string(key)
}

func seqBytes(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

// lruBuckets restituisce root, seq e unit del namespace; con create li crea.
// Senza create e con indice assente restituisce nil.
func lruBuckets(tx *bolt.Tx, ns string, create bool) (root, seq, unit *bolt.Bucket, err error) {
	name := []byte("lru:" + ns)
	if create {
		if root, err = tx.CreateBucketIfNotExists(name); err != nil {
			return nil, nil, nil, err
		}
		if seq, err = root.CreateBucketIfNotExists([]byte("seq")); err != nil {
			return nil, nil, nil, err
		}
		if unit, err = root.CreateBucketIfNotExists([]byte("unit")); err != nil {
			return nil, nil, nil, err
		}
		return root, seq, unit, nil
	}
	root = tx.Bucket(name)
	if root == nil {
		return nil, nil, nil, nil
	}
	seq, unit = root.Bucket([]byte("seq")), root.Bucket([]byte("unit"))
	if seq == nil || unit == nil {
		return nil, nil, nil, nil
	}
	return root, seq, unit, nil
}

func lruCount(root *bolt.Bucket) uint64 {
	if v := root.Get([]byte("n")); len(v) == 8 {
		return binary.BigEndian.Uint64(v)
	}
	return 0
}

func lruSetCount(root *bolt.Bucket, n uint64) error {
	return root.Put([]byte("n"), seqBytes(n))
}

// decodeUnit separa la sequenza dalle voci di un valore del bucket "unit". Un valore
// illeggibile dà elenco vuoto: l'unità si rimuove senza toccare altro.
func decodeUnit(v []byte) (seq []byte, entries []lruEntry) {
	if len(v) < 8 {
		return nil, nil
	}
	seq = append([]byte(nil), v[:8]...)
	_ = json.Unmarshal(v[8:], &entries)
	return seq, entries
}

func encodeUnit(seq []byte, entries []lruEntry) ([]byte, error) {
	data, err := json.Marshal(entries)
	if err != nil {
		return nil, err
	}
	return append(append([]byte(nil), seq...), data...), nil
}

// touch segna l'unità come la più recente e vi aggiunge le voci entries (senza
// duplicati). Crea l'unità se manca.
func lruTouch(tx *bolt.Tx, ns, unit string, entries []lruEntry) error {
	root, seq, units, err := lruBuckets(tx, ns, true)
	if err != nil {
		return err
	}
	var merged []lruEntry
	if old := units.Get([]byte(unit)); old != nil {
		oldSeq, oldEntries := decodeUnit(old)
		if oldSeq != nil {
			if err := seq.Delete(oldSeq); err != nil {
				return err
			}
		}
		merged = oldEntries
	} else if err := lruSetCount(root, lruCount(root)+1); err != nil {
		return err
	}
	for _, e := range entries {
		if !hasEntry(merged, e) {
			merged = append(merged, e)
		}
	}
	return lruPutSeq(seq, units, unit, merged)
}

func hasEntry(list []lruEntry, e lruEntry) bool {
	for _, x := range list {
		if x.Bucket == e.Bucket && string(x.Key) == string(e.Key) {
			return true
		}
	}
	return false
}

// lruPutSeq assegna all'unità la sequenza successiva.
func lruPutSeq(seq, units *bolt.Bucket, unit string, entries []lruEntry) error {
	n, err := seq.NextSequence()
	if err != nil {
		return err
	}
	key := seqBytes(n)
	val, err := encodeUnit(key, entries)
	if err != nil {
		return fmt.Errorf("lru encode %s: %w", unit, err)
	}
	if err := seq.Put(key, []byte(unit)); err != nil {
		return err
	}
	return units.Put([]byte(unit), val)
}

// lruTouchExisting riporta in cima l'unità solo se esiste ancora (i tocchi in
// memoria possono riferirsi a unità già espulse: non vanno ricreate).
func lruTouchExisting(tx *bolt.Tx, ns, unit string) error {
	_, seq, units, err := lruBuckets(tx, ns, false)
	if err != nil || units == nil {
		return err
	}
	old := units.Get([]byte(unit))
	if old == nil {
		return nil
	}
	oldSeq, entries := decodeUnit(old)
	if oldSeq != nil {
		if err := seq.Delete(oldSeq); err != nil {
			return err
		}
	}
	return lruPutSeq(seq, units, unit, entries)
}

// lruRemoveUnit toglie l'unità dall'indice (non le sue voci).
func lruRemoveUnit(root, seq, units *bolt.Bucket, unit []byte) error {
	if old := units.Get(unit); old != nil {
		if oldSeq, _ := decodeUnit(old); oldSeq != nil {
			if err := seq.Delete(oldSeq); err != nil {
				return err
			}
		}
		if err := units.Delete(unit); err != nil {
			return err
		}
		if n := lruCount(root); n > 0 {
			return lruSetCount(root, n-1)
		}
	}
	return nil
}

// lruRemove toglie l'unità dall'indice del namespace, se c'è (non le sue voci).
func lruRemove(tx *bolt.Tx, ns, unit string) error {
	root, seq, units, err := lruBuckets(tx, ns, false)
	if err != nil || units == nil {
		return err
	}
	return lruRemoveUnit(root, seq, units, []byte(unit))
}

// lruEvict rimuove le unità meno usate finché ce ne sono più di max; per ogni voce
// delle unità espulse chiama drop.
func lruEvict(tx *bolt.Tx, ns string, max uint64, drop func(lruEntry) error) error {
	root, seq, units, err := lruBuckets(tx, ns, false)
	if err != nil || units == nil {
		return err
	}
	for lruCount(root) > max {
		k, unit := seq.Cursor().First()
		if k == nil {
			return lruSetCount(root, 0) // contatore incoerente: nessuna unità da espellere
		}
		k, unit = append([]byte(nil), k...), append([]byte(nil), unit...)
		_, entries := decodeUnit(units.Get(unit))
		for _, e := range entries {
			if err := drop(e); err != nil {
				return err
			}
		}
		if err := seq.Delete(k); err != nil {
			return err
		}
		if units.Get(unit) != nil {
			if err := lruRemoveUnit(root, seq, units, unit); err != nil {
				return err
			}
		} else if n := lruCount(root); n > 0 {
			if err := lruSetCount(root, n-1); err != nil {
				return err
			}
		}
	}
	return nil
}

// lruDropBucket toglie dall'indice del namespace tutte le voci del bucket indicato
// (la mailbox svuotata); le unità rimaste senza voci spariscono.
func lruDropBucket(tx *bolt.Tx, ns, bucket string) error {
	root, seq, units, err := lruBuckets(tx, ns, false)
	if err != nil || units == nil {
		return err
	}
	type change struct {
		unit    []byte
		seq     []byte
		entries []lruEntry
	}
	var todo []change
	err = units.ForEach(func(k, v []byte) error {
		sq, entries := decodeUnit(v)
		kept := make([]lruEntry, 0, len(entries))
		for _, e := range entries {
			if e.Bucket != bucket {
				kept = append(kept, e)
			}
		}
		if len(kept) != len(entries) || sq == nil {
			todo = append(todo, change{append([]byte(nil), k...), sq, kept})
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, ch := range todo {
		if len(ch.entries) == 0 {
			if err := lruRemoveUnit(root, seq, units, ch.unit); err != nil {
				return err
			}
			continue
		}
		val, err := encodeUnit(ch.seq, ch.entries)
		if err != nil {
			return err
		}
		if err := units.Put(ch.unit, val); err != nil {
			return err
		}
	}
	return nil
}

// dropEntry cancella dal database la voce di un'unità espulsa.
func dropEntry(tx *bolt.Tx) func(lruEntry) error {
	return func(e lruEntry) error {
		if b := tx.Bucket([]byte(e.Bucket)); b != nil {
			return b.Delete(e.Key)
		}
		return nil
	}
}

// noteTouch registra in memoria un tocco in lettura (D16-T6): nessuna transazione
// di scrittura per un hit.
func (c *Cache) noteTouch(ns, unit string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending = append(c.pending, pendingTouch{ns: ns, unit: unit})
	if len(c.pending) > maxPendingTouches {
		c.pending = append([]pendingTouch(nil), c.pending[len(c.pending)-maxPendingTouches/2:]...)
	}
}

// takePending prende i tocchi accumulati (e li toglie dalla memoria).
func (c *Cache) takePending() []pendingTouch {
	c.mu.Lock()
	defer c.mu.Unlock()
	p := c.pending
	c.pending = nil
	return p
}

// restorePending rimette in testa i tocchi di una transazione fallita.
func (c *Cache) restorePending(p []pendingTouch) {
	if len(p) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending = append(append([]pendingTouch(nil), p...), c.pending...)
}

// applyPending scrive nella transazione i tocchi in memoria, nell'ordine in cui
// sono avvenuti.
func applyPending(tx *bolt.Tx, p []pendingTouch) error {
	for _, t := range p {
		if err := lruTouchExisting(tx, t.ns, t.unit); err != nil {
			return err
		}
	}
	return nil
}

// update esegue fn in una transazione di scrittura preceduta dalla scrittura dei
// tocchi in memoria (Piano 16, Task 3, D16-T6): ogni scrittura porta con sé i tocchi
// accumulati dalle letture. Se la transazione fallisce i tocchi tornano in memoria.
func (c *Cache) update(fn func(tx *bolt.Tx) error) error {
	p := c.takePending()
	err := c.db.Update(func(tx *bolt.Tx) error {
		if err := applyPending(tx, p); err != nil {
			return err
		}
		return fn(tx)
	})
	if err != nil {
		c.restorePending(p)
	}
	return err
}

// flush scrive i tocchi in memoria, se ce ne sono.
func (c *Cache) flush() error {
	p := c.takePending()
	if len(p) == 0 {
		return nil
	}
	err := c.db.Update(func(tx *bolt.Tx) error { return applyPending(tx, p) })
	if err != nil {
		c.restorePending(p)
	}
	return err
}
