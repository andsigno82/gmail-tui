package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

func newCache(t *testing.T) *Cache {
	t.Helper()
	c, err := Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Piano 16, Task 3: i test non provano la durabilità, solo la logica; senza fsync
	// per transazione le suite con 200+ scritture restano veloci.
	c.db.NoSync = true
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// writeFile scrive data in path (helper dei test di file corrotto).
func writeFile(path string, data []byte) error { return os.WriteFile(path, data, 0o600) }

func sample(uid uint32) *msg.Message {
	return &msg.Message{Ref: msg.MessageRef{
		ID:      msg.NewID("INBOX", uid),
		Subject: "ciao",
		Date:    time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC),
	}, BodyText: "corpo"}
}

func TestSetGetRoundtrip(t *testing.T) {
	c := newCache(t)
	if err := c.SetMessage("INBOX", 1, sample(7)); err != nil {
		t.Fatalf("SetMessage: %v", err)
	}
	got, ok, err := c.GetMessage("INBOX", 7)
	if err != nil || !ok {
		t.Fatalf("GetMessage: ok=%v err=%v", ok, err)
	}
	if got.Ref.Subject != "ciao" || got.BodyText != "corpo" {
		t.Fatalf("roundtrip rotto: %+v", got)
	}
}

func TestGetMiss(t *testing.T) {
	c := newCache(t)
	_, ok, err := c.GetMessage("INBOX", 999)
	if err != nil || ok {
		t.Fatalf("miss atteso: ok=%v err=%v", ok, err)
	}
}

func TestUIDValidityInvalidation(t *testing.T) {
	c := newCache(t)
	if err := c.SetMessage("INBOX", 1, sample(7)); err != nil {
		t.Fatal(err)
	}
	// il server ha rigenerato le mailbox: uidValidity scatta da 1 a 2
	if err := c.SetMessage("INBOX", 2, sample(8)); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := c.GetMessage("INBOX", 7); ok {
		t.Fatal("vecchio UID deve essere invalidato dal cambio uidValidity")
	}
	got, ok, _ := c.GetMessage("INBOX", 8)
	if !ok || got == nil {
		t.Fatal("nuovo UID dopo invalidazione non trovato")
	}
	v, ok, _ := c.UIDValidity("INBOX")
	if !ok || v != 2 {
		t.Fatalf("UIDValidity = %d ok=%v, atteso 2", v, ok)
	}
}

func TestDropMailbox(t *testing.T) {
	c := newCache(t)
	_ = c.SetMessage("Label", 1, sample(7))
	if err := c.DropMailbox("Label"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := c.GetMessage("Label", 7); ok {
		t.Fatal("DropMailbox non ha svuotato")
	}
}
