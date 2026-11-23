package cache

// Istantanee locali di lista e thread (Piano 16, Task 4; D16-A1, D16-T2, D16-T3).
//
// Bucket "snap:list" (chiave = nome mailbox, "" = INBOX) e "snap:thread" (chiave =
// thread id, 8 byte big-endian); valore JSON con Version, SavedAt, Mailbox,
// UIDValidity e righe o ref. Le scrive lo store IMAP (write-through); le leggono
// gli accessi Cached* senza rete. Sono dati PROVVISORI (R1): i MessageID che
// contengono non si passano mai a MailStore finché il server non li conferma.
//
// Regola d'epoca (R3): un'istantanea vale solo se la sua UIDVALIDITY coincide con
// quella registrata per i corpi della stessa mailbox (meta:<mailbox>), o se la
// mailbox non ha ancora corpi (meta assente). Altrimenti è un miss con motivo
// LookupEpoch. Una voce illeggibile è un miss, mai un panic né un errore.
//
// Le istantanee sono nell'LRU con i namespace "list" (MaxListSnapshots) e "thread"
// (MaxThreadSnapshots); gli hit toccano l'LRU solo in memoria (D16-T6).

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

const (
	bucketSnapList   = "snap:list"
	bucketSnapThread = "snap:thread"

	// snapVersion è la versione del formato delle istantanee: una versione diversa
	// è un miss.
	snapVersion = 1
)

// Lookup è l'esito di una lettura di istantanea: serve alla traccia (D16-T14).
type Lookup int

const (
	LookupMiss  Lookup = iota // assente o illeggibile
	LookupHit                 // valida
	LookupEpoch               // presente ma scartata dalla regola d'epoca (R3)
)

type listSnap struct {
	Version     int
	SavedAt     time.Time
	Mailbox     string
	UIDValidity uint32
	Limited     bool
	Rows        []mail.Conversation
}

type threadSnap struct {
	Version     int
	SavedAt     time.Time
	Mailbox     string
	UIDValidity uint32
	Refs        []msg.MessageRef
}

// errNoChange annulla una transazione che non ha nulla da scrivere (evita il commit,
// e l'fsync, quando una patch non trova corrispondenze).
var errNoChange = errors.New("nessuna modifica")

// snapMailbox normalizza la mailbox: "" è INBOX (una chiave bbolt non può essere vuota).
func snapMailbox(m string) string {
	if m == "" {
		return "INBOX"
	}
	return m
}

func threadKey(id uint64) []byte { return seqBytes(id) }

func threadUnit(id uint64) string { return strconv.FormatUint(id, 10) }

// epochMatches applica R3: l'epoca dell'istantanea contro quella dei corpi.
func epochMatches(tx *bolt.Tx, mailbox string, snapEpoch uint32) bool {
	meta := tx.Bucket([]byte("meta:" + mailbox))
	if meta == nil {
		return true
	}
	v := meta.Get([]byte("uidvalidity"))
	return v == nil || binaryUint32(v) == snapEpoch
}

// removeSnap toglie una voce di istantanea e la sua unità LRU.
func removeSnap(tx *bolt.Tx, ns, bucket string, key []byte, unit string) error {
	if b := tx.Bucket([]byte(bucket)); b != nil {
		if err := b.Delete(key); err != nil {
			return err
		}
	}
	return lruRemove(tx, ns, unit)
}

// putSnap scrive una voce di istantanea, la tocca nell'LRU ed espelle oltre max.
func putSnap(tx *bolt.Tx, ns, bucket string, key []byte, unit string, data []byte, max uint64) error {
	b, err := tx.CreateBucketIfNotExists([]byte(bucket))
	if err != nil {
		return err
	}
	if err := b.Put(key, data); err != nil {
		return err
	}
	if err := lruTouch(tx, ns, unit, []lruEntry{{Bucket: bucket, Key: append([]byte(nil), key...)}}); err != nil {
		return err
	}
	return lruEvict(tx, ns, max, dropEntry(tx))
}

// dropSnapshots scarta le istantanee legate a mailbox: la sua lista e i thread salvati
// su quella mailbox (per Tutti i messaggi: tutti). Chiamata quando l'epoca cambia o
// la mailbox viene svuotata.
func dropSnapshots(tx *bolt.Tx, mailbox string) error {
	if err := removeSnap(tx, lruList, bucketSnapList, []byte(mailbox), mailbox); err != nil {
		return err
	}
	tb := tx.Bucket([]byte(bucketSnapThread))
	if tb == nil {
		return nil
	}
	var ids []uint64
	err := tb.ForEach(func(k, v []byte) error {
		var s threadSnap
		if len(k) == 8 && json.Unmarshal(v, &s) == nil && s.Mailbox == mailbox {
			ids = append(ids, binary.BigEndian.Uint64(k))
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := removeSnap(tx, lruThread, bucketSnapThread, threadKey(id), threadUnit(id)); err != nil {
			return err
		}
	}
	return nil
}

// PutList salva l'istantanea della lista di mailbox ("" = INBOX): le prime
// SnapshotRows righe, con la UIDVALIDITY della SELECT e l'istante at. limited è il
// flag mail.ErrScanLimit al momento del salvataggio. Righe vuote = cancella.
func (c *Cache) PutList(mailbox string, uidValidity uint32, rows []mail.Conversation, limited bool, at time.Time) error {
	mailbox = snapMailbox(mailbox)
	if len(rows) == 0 {
		return c.update(func(tx *bolt.Tx) error {
			return removeSnap(tx, lruList, bucketSnapList, []byte(mailbox), mailbox)
		})
	}
	if len(rows) > SnapshotRows {
		rows = rows[:SnapshotRows]
	}
	data, err := json.Marshal(listSnap{
		Version: snapVersion, SavedAt: at, Mailbox: mailbox,
		UIDValidity: uidValidity, Limited: limited, Rows: rows,
	})
	if err != nil {
		return err
	}
	return c.update(func(tx *bolt.Tx) error {
		return putSnap(tx, lruList, bucketSnapList, []byte(mailbox), mailbox, data, MaxListSnapshots)
	})
}

// LookupList legge l'istantanea della lista con l'esito (hit, miss, scartata per
// epoca). Solo con LookupHit le righe sono valorizzate.
func (c *Cache) LookupList(mailbox string) ([]mail.Conversation, time.Time, bool, Lookup) {
	mailbox = snapMailbox(mailbox)
	var s listSnap
	res := LookupMiss
	_ = c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketSnapList))
		if b == nil {
			return nil
		}
		data := b.Get([]byte(mailbox))
		if data == nil || json.Unmarshal(data, &s) != nil || s.Version != snapVersion || len(s.Rows) == 0 {
			return nil
		}
		if epochMatches(tx, mailbox, s.UIDValidity) {
			res = LookupHit
		} else {
			res = LookupEpoch
		}
		return nil
	})
	if res != LookupHit {
		return nil, time.Time{}, false, res
	}
	c.noteTouch(lruList, mailbox)
	return s.Rows, s.SavedAt, s.Limited, LookupHit
}

// GetList è LookupList ridotta a ok (miss e scarto per epoca sono entrambi !ok).
func (c *Cache) GetList(mailbox string) (rows []mail.Conversation, at time.Time, limited, ok bool) {
	rows, at, limited, res := c.LookupList(mailbox)
	return rows, at, limited, res == LookupHit
}

// PutThread salva l'istantanea dei ref del thread threadID (X-GM-THRID, 0 = non
// Gmail: ignorato). mailbox e uidValidity sono quelli della SELECT di Tutti i
// messaggi. Ref vuoti = cancella.
func (c *Cache) PutThread(threadID uint64, mailbox string, uidValidity uint32, refs []msg.MessageRef, at time.Time) error {
	if threadID == 0 {
		return nil
	}
	mailbox = snapMailbox(mailbox)
	if len(refs) == 0 {
		return c.update(func(tx *bolt.Tx) error {
			return removeSnap(tx, lruThread, bucketSnapThread, threadKey(threadID), threadUnit(threadID))
		})
	}
	data, err := json.Marshal(threadSnap{
		Version: snapVersion, SavedAt: at, Mailbox: mailbox, UIDValidity: uidValidity, Refs: refs,
	})
	if err != nil {
		return err
	}
	return c.update(func(tx *bolt.Tx) error {
		return putSnap(tx, lruThread, bucketSnapThread, threadKey(threadID), threadUnit(threadID), data, MaxThreadSnapshots)
	})
}

// LookupThread legge l'istantanea del thread con l'esito (come LookupList).
func (c *Cache) LookupThread(threadID uint64) ([]msg.MessageRef, time.Time, Lookup) {
	var s threadSnap
	res := LookupMiss
	_ = c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketSnapThread))
		if b == nil {
			return nil
		}
		data := b.Get(threadKey(threadID))
		if data == nil || json.Unmarshal(data, &s) != nil || s.Version != snapVersion || len(s.Refs) == 0 {
			return nil
		}
		if epochMatches(tx, s.Mailbox, s.UIDValidity) {
			res = LookupHit
		} else {
			res = LookupEpoch
		}
		return nil
	})
	if res != LookupHit {
		return nil, time.Time{}, res
	}
	c.noteTouch(lruThread, threadUnit(threadID))
	return s.Refs, s.SavedAt, LookupHit
}

// GetThread è LookupThread ridotta a ok.
func (c *Cache) GetThread(threadID uint64) ([]msg.MessageRef, time.Time, bool) {
	refs, at, res := c.LookupThread(threadID)
	return refs, at, res == LookupHit
}

// snapWrite è una voce di istantanea da riscrivere.
type snapWrite struct {
	bucket    string
	key, data []byte
}

// applyWrites riscrive le voci raccolte da una scansione, senza alterarne la posizione
// LRU (una patch non è un uso); se non c'è nulla da scrivere annulla la transazione.
func applyWrites(tx *bolt.Tx, todo []snapWrite) error {
	if len(todo) == 0 {
		return errNoChange
	}
	for _, w := range todo {
		b := tx.Bucket([]byte(w.bucket))
		if b == nil {
			continue
		}
		if err := b.Put(w.key, w.data); err != nil {
			return err
		}
	}
	return nil
}

// PatchSnapshotRefs applica mutate ai ref con GmailID in gmailIDs, nelle istantanee di
// thread e sull'ultimo messaggio (Latest) delle righe di lista (D16-T3). mutate
// riporta se ha cambiato il ref: solo le istantanee cambiate si riscrivono. Le righe
// toccate ricalcolano Unread e Labels: dall'istantanea del thread se c'è (Unread = almeno
// un ref non letto; Labels = unione in ordine di apparizione), altrimenti dal Latest.
func (c *Cache) PatchSnapshotRefs(gmailIDs []uint64, mutate func(*msg.MessageRef) bool) error {
	set := make(map[uint64]bool, len(gmailIDs))
	for _, g := range gmailIDs {
		if g != 0 {
			set[g] = true
		}
	}
	if len(set) == 0 {
		return nil
	}
	err := c.update(func(tx *bolt.Tx) error {
		var todo []snapWrite
		threadRefs := map[uint64][]msg.MessageRef{}
		patchedThreads := map[uint64]bool{}
		if tb := tx.Bucket([]byte(bucketSnapThread)); tb != nil {
			err := tb.ForEach(func(k, v []byte) error {
				var s threadSnap
				if len(k) != 8 || json.Unmarshal(v, &s) != nil || s.Version != snapVersion {
					return nil
				}
				id := binary.BigEndian.Uint64(k)
				changed := false
				for i := range s.Refs {
					if set[s.Refs[i].GmailID] && mutate(&s.Refs[i]) {
						changed = true
					}
				}
				threadRefs[id] = s.Refs
				if !changed {
					return nil
				}
				data, err := json.Marshal(s)
				if err != nil {
					return err
				}
				patchedThreads[id] = true
				todo = append(todo, snapWrite{bucket: bucketSnapThread, key: append([]byte(nil), k...), data: data})
				return nil
			})
			if err != nil {
				return err
			}
		}
		if lb := tx.Bucket([]byte(bucketSnapList)); lb != nil {
			err := lb.ForEach(func(k, v []byte) error {
				var s listSnap
				if json.Unmarshal(v, &s) != nil || s.Version != snapVersion {
					return nil
				}
				changed := false
				for i := range s.Rows {
					row := &s.Rows[i]
					touched := set[row.Latest.GmailID] && mutate(&row.Latest)
					if row.ThreadID != 0 && patchedThreads[row.ThreadID] {
						touched = true
					}
					if touched {
						recomputeRow(row, threadRefs[row.ThreadID])
						changed = true
					}
				}
				if !changed {
					return nil
				}
				data, err := json.Marshal(s)
				if err != nil {
					return err
				}
				todo = append(todo, snapWrite{bucket: bucketSnapList, key: append([]byte(nil), k...), data: data})
				return nil
			})
			if err != nil {
				return err
			}
		}
		return applyWrites(tx, todo)
	})
	if errors.Is(err, errNoChange) {
		return nil
	}
	return err
}

// recomputeRow ricalcola Unread e Labels della riga dopo una patch: dai ref del
// thread se ci sono, altrimenti dall'ultimo messaggio.
func recomputeRow(row *mail.Conversation, refs []msg.MessageRef) {
	if len(refs) == 0 {
		row.Unread = row.Latest.Unread
		row.Labels = append([]string(nil), row.Latest.Labels...)
		return
	}
	row.Unread = false
	var labels []string
	seen := map[string]bool{}
	for _, r := range refs {
		if r.Unread {
			row.Unread = true
		}
		for _, l := range r.Labels {
			if !seen[l] {
				seen[l] = true
				labels = append(labels, l)
			}
		}
	}
	row.Labels = labels
}

// RemoveListRows toglie dall'istantanea della lista di mailbox le righe con uno dei
// threadIDs (0 ignorato) o il cui ultimo messaggio ha uno degli ids (D16-T3:
// Archive, Trash, ApplyLabel off, DeleteDraft). Se non resta nessuna riga
// l'istantanea sparisce.
func (c *Cache) RemoveListRows(mailbox string, threadIDs []uint64, ids []msg.MessageID) error {
	mailbox = snapMailbox(mailbox)
	tset := make(map[uint64]bool, len(threadIDs))
	for _, t := range threadIDs {
		if t != 0 {
			tset[t] = true
		}
	}
	iset := make(map[msg.MessageID]bool, len(ids))
	for _, id := range ids {
		iset[id] = true
	}
	if len(tset) == 0 && len(iset) == 0 {
		return nil
	}
	err := c.update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketSnapList))
		if b == nil {
			return errNoChange
		}
		data := b.Get([]byte(mailbox))
		var s listSnap
		if data == nil || json.Unmarshal(data, &s) != nil || s.Version != snapVersion {
			return errNoChange
		}
		kept := make([]mail.Conversation, 0, len(s.Rows))
		for _, row := range s.Rows {
			if (row.ThreadID != 0 && tset[row.ThreadID]) || iset[row.Latest.ID] {
				continue
			}
			kept = append(kept, row)
		}
		if len(kept) == len(s.Rows) {
			return errNoChange
		}
		if len(kept) == 0 {
			return removeSnap(tx, lruList, bucketSnapList, []byte(mailbox), mailbox)
		}
		s.Rows = kept
		out, err := json.Marshal(s)
		if err != nil {
			return err
		}
		return b.Put([]byte(mailbox), out)
	})
	if errors.Is(err, errNoChange) {
		return nil
	}
	return err
}

// RemoveThreadRefs toglie i messaggi con GmailID in gmailIDs da ogni istantanea di
// thread (D16-T3: Trash). Un thread rimasto senza ref sparisce.
func (c *Cache) RemoveThreadRefs(gmailIDs []uint64) error {
	set := make(map[uint64]bool, len(gmailIDs))
	for _, g := range gmailIDs {
		if g != 0 {
			set[g] = true
		}
	}
	if len(set) == 0 {
		return nil
	}
	err := c.update(func(tx *bolt.Tx) error {
		tb := tx.Bucket([]byte(bucketSnapThread))
		if tb == nil {
			return errNoChange
		}
		var todo []snapWrite
		var empty []uint64
		err := tb.ForEach(func(k, v []byte) error {
			var s threadSnap
			if len(k) != 8 || json.Unmarshal(v, &s) != nil || s.Version != snapVersion {
				return nil
			}
			kept := make([]msg.MessageRef, 0, len(s.Refs))
			for _, r := range s.Refs {
				if !set[r.GmailID] {
					kept = append(kept, r)
				}
			}
			if len(kept) == len(s.Refs) {
				return nil
			}
			if len(kept) == 0 {
				empty = append(empty, binary.BigEndian.Uint64(k))
				return nil
			}
			s.Refs = kept
			data, err := json.Marshal(s)
			if err != nil {
				return err
			}
			todo = append(todo, snapWrite{bucket: bucketSnapThread, key: append([]byte(nil), k...), data: data})
			return nil
		})
		if err != nil {
			return err
		}
		if len(todo) == 0 && len(empty) == 0 {
			return errNoChange
		}
		for _, w := range todo {
			if err := tb.Put(w.key, w.data); err != nil {
				return err
			}
		}
		for _, id := range empty {
			if err := removeSnap(tx, lruThread, bucketSnapThread, threadKey(id), threadUnit(id)); err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, errNoChange) {
		return nil
	}
	return err
}
