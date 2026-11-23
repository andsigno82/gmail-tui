package cache

// Piano 16, Task 3: LRU dei corpi (D16-T4, D16-T5, D16-T6). L'unità è il messaggio
// Gmail: le copie in più mailbox occupano un posto solo.

import (
	"path/filepath"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

const allMail = "[Gmail]/All Mail"

// gm è un messaggio con GmailID gid in box, uid = uid.
func gm(box string, uid uint32, gid uint64) *msg.Message {
	return &msg.Message{Ref: msg.MessageRef{
		ID:      msg.NewID(box, uid),
		GmailID: gid,
		Subject: "g",
		Date:    time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC).Add(time.Duration(gid) * time.Minute),
	}, BodyText: "corpo"}
}

// fill salva in INBOX (epoca 1) le unità from..to (uid = gid).
func fill(t *testing.T, c *Cache, from, to uint64) {
	t.Helper()
	for g := from; g <= to; g++ {
		if err := c.SetMessage("INBOX", 1, gm("INBOX", uint32(g), g)); err != nil {
			t.Fatalf("SetMessage %d: %v", g, err)
		}
	}
}

func has(t *testing.T, c *Cache, box string, uid uint32) bool {
	t.Helper()
	_, ok, err := c.GetMessage(box, uid)
	if err != nil {
		t.Fatalf("GetMessage %s/%d: %v", box, uid, err)
	}
	return ok
}

func TestLRUEvictsLeastRecentlyUsed(t *testing.T) {
	c := newCache(t)
	fill(t, c, 1, MaxBodies)
	// una lettura dell'unità 1 la rende la più recente: vale alla scrittura successiva
	if !has(t, c, "INBOX", 1) {
		t.Fatal("unità 1 assente prima dell'espulsione")
	}
	fill(t, c, MaxBodies+1, MaxBodies+1) // 201a unità
	if has(t, c, "INBOX", 2) {
		t.Error("l'unità 2 (la meno usata) doveva essere espulsa")
	}
	if !has(t, c, "INBOX", 1) {
		t.Error("l'unità 1, toccata in lettura, doveva sopravvivere")
	}
	if !has(t, c, "INBOX", MaxBodies+1) {
		t.Error("l'unità appena scritta manca")
	}
	n := 0
	for g := uint32(1); g <= MaxBodies+1; g++ {
		if has(t, c, "INBOX", g) {
			n++
		}
	}
	if n != MaxBodies {
		t.Errorf("unità in cache = %d, voglio %d", n, MaxBodies)
	}
}

func TestLRUCopiesShareOneSlot(t *testing.T) {
	c := newCache(t)
	// stesso messaggio Gmail (gid 1) in INBOX e in Tutti i messaggi: un posto solo
	if err := c.SetMessage("INBOX", 1, gm("INBOX", 1, 1)); err != nil {
		t.Fatal(err)
	}
	if err := c.SetMessage(allMail, 1, gm(allMail, 900, 1)); err != nil {
		t.Fatal(err)
	}
	if !has(t, c, "INBOX", 1) || !has(t, c, allMail, 900) {
		t.Fatal("le due copie dell'unità 1 devono stare in cache")
	}
	fill(t, c, 2, MaxBodies) // 200 unità in tutto, 201 copie
	for g := uint32(2); g <= MaxBodies; g++ {
		if !has(t, c, "INBOX", g) {
			t.Fatalf("unità %d espulsa: le copie non devono contare doppio", g)
		}
	}
	// la lettura sopra ha toccato 2..200 e non la 1: la 1 (con le sue due copie) è la
	// meno usata
	fill(t, c, MaxBodies+1, MaxBodies+1)
	if has(t, c, "INBOX", 1) || has(t, c, allMail, 900) {
		t.Error("l'espulsione deve togliere TUTTE le copie dell'unità")
	}
}

func TestLRUTouchesFlushedOnClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	c, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c.db.NoSync = true
	fill(t, c, 1, MaxBodies)
	if !has(t, c, "INBOX", 1) { // tocco in sola lettura, solo in memoria
		t.Fatal("unità 1 assente")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	c2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c2.db.NoSync = true
	defer func() { _ = c2.Close() }()
	fill(t, c2, MaxBodies+1, MaxBodies+1)
	if !has(t, c2, "INBOX", 1) {
		t.Error("il tocco in lettura non è stato salvato a Close: l'unità 1 è stata espulsa")
	}
	if has(t, c2, "INBOX", 2) {
		t.Error("l'unità 2 doveva essere espulsa")
	}
}

// D16-T6: un hit non apre transazioni di scrittura (su darwin ogni commit è un
// F_FULLFSYNC).
func TestGetMessageHitDoesNotWrite(t *testing.T) {
	c := newCache(t)
	fill(t, c, 1, 3)
	before := txID(c)
	for i := 0; i < 50; i++ {
		has(t, c, "INBOX", 2)
	}
	if after := txID(c); after != before {
		t.Errorf("gli hit hanno scritto: txid %d -> %d", before, after)
	}
}

// txID è l'id dell'ultima transazione di scrittura confermata (cresce a ogni commit).
func txID(c *Cache) int {
	var id int
	_ = c.db.View(func(tx *bolt.Tx) error { id = int(tx.ID()); return nil })
	return id
}

// Una voce non decodificabile è un miss, non un errore che blocca l'apertura.
func TestGetMessageCorruptEntryIsMiss(t *testing.T) {
	c := newCache(t)
	fill(t, c, 1, 2)
	err := c.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("msgs:INBOX")).Put(uint32Bytes(1), []byte("{non json"))
	})
	if err != nil {
		t.Fatal(err)
	}
	m, ok, err := c.GetMessage("INBOX", 1)
	if err != nil || ok || m != nil {
		t.Fatalf("voce corrotta: m=%v ok=%v err=%v, voglio miss", m, ok, err)
	}
}

func BenchmarkGetMessageHit(b *testing.B) {
	c, err := Open(filepath.Join(b.TempDir(), "cache.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if err := c.SetMessage("INBOX", 1, gm("INBOX", 7, 7)); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok, _ := c.GetMessage("INBOX", 7); !ok {
			b.Fatal("miss")
		}
	}
}
