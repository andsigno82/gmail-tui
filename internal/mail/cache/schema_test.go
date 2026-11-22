package cache

// Piano 8, Task 8: le entry scritte con lo schema precedente (senza GmailID,
// ThreadID, Labels in msg.MessageRef) vengono scartate all'apertura.

import (
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

func TestOpenDropsMessagesOfOlderSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	c, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	m := &msg.Message{Ref: msg.MessageRef{ID: msg.NewID("INBOX", 7), Subject: "vecchio"}}
	if err := c.SetMessage("INBOX", 1, m); err != nil {
		t.Fatalf("SetMessage: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// simula un database creato dalla versione precedente dello schema
	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		t.Fatalf("bolt open: %v", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("schema")).Put([]byte("version"), uint32Bytes(1))
	})
	if err != nil {
		t.Fatalf("set version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("bolt close: %v", err)
	}

	c2, err := Open(path)
	if err != nil {
		t.Fatalf("Open dopo schema vecchio: %v", err)
	}
	defer func() { _ = c2.Close() }()
	if _, ok, err := c2.GetMessage("INBOX", 7); err != nil || ok {
		t.Errorf("entry di schema vecchio ancora presente: ok=%v err=%v", ok, err)
	}

	// con lo schema corrente le entry restano valide tra due aperture
	if err := c2.SetMessage("INBOX", 1, m); err != nil {
		t.Fatalf("SetMessage: %v", err)
	}
	if err := c2.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	c3, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = c3.Close() }()
	if _, ok, _ := c3.GetMessage("INBOX", 7); !ok {
		t.Error("entry dello schema corrente persa alla riapertura")
	}
}
