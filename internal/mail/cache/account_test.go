package cache

// Piano 16, Task 3: legame con l'account (D16-T9) ed epoca (R3 lato cache).

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestBindAccountAdoptsUnbound(t *testing.T) {
	c := newCache(t)
	fill(t, c, 1, 3) // cache dello schema 3: nessun account
	if err := c.BindAccount("Andrea@Gmail.com"); err != nil {
		t.Fatalf("BindAccount: %v", err)
	}
	if !has(t, c, "INBOX", 2) {
		t.Fatal("la cache senza account va adottata, non svuotata (Q16-3)")
	}
	// lo stesso indirizzo (anche con maiuscole diverse) non svuota
	if err := c.BindAccount("andrea@gmail.com"); err != nil {
		t.Fatal(err)
	}
	if !has(t, c, "INBOX", 2) {
		t.Fatal("stesso account: la cache non deve svuotarsi")
	}
}

func TestBindAccountDifferentWipes(t *testing.T) {
	c := newCache(t)
	if err := c.BindAccount("a@gmail.com"); err != nil {
		t.Fatal(err)
	}
	fill(t, c, 1, 3)
	if err := c.SetMessage("Label", 1, gm("Label", 9, 9)); err != nil {
		t.Fatal(err)
	}
	if err := c.BindAccount("b@gmail.com"); err != nil {
		t.Fatalf("BindAccount: %v", err)
	}
	if has(t, c, "INBOX", 2) || has(t, c, "Label", 9) {
		t.Error("cambio account: la posta dell'altro account deve sparire")
	}
	if _, ok, _ := c.UIDValidity("INBOX"); ok {
		t.Error("cambio account: anche le epoche vanno svuotate")
	}
	// la cache è usabile e legata al nuovo account
	fill(t, c, 4, 4)
	if err := c.BindAccount("b@gmail.com"); err != nil {
		t.Fatal(err)
	}
	if !has(t, c, "INBOX", 4) {
		t.Error("dopo lo svuotamento la cache deve restare scrivibile e legata a b@")
	}
	// e l'indice LRU è ripartito pulito: 199 unità nuove + la 4 = 200 stanno tutte
	fill(t, c, 10, 10+MaxBodies-2)
	if !has(t, c, "INBOX", 4) {
		t.Error("indice LRU non azzerato dallo svuotamento: voce fantasma nel conteggio")
	}
}

func TestAccountStoredHashed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	c, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c.db.NoSync = true
	if err := c.BindAccount("segreto.unico@gmail.com"); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("segreto.unico")) || bytes.Contains(raw, []byte("gmail.com")) {
		t.Error("l'indirizzo compare in chiaro nel file di cache")
	}
	// e il legame sopravvive alla riapertura
	c2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c2.db.NoSync = true
	defer func() { _ = c2.Close() }()
	fill(t, c2, 1, 1)
	if err := c2.BindAccount("segreto.unico@gmail.com"); err != nil {
		t.Fatal(err)
	}
	if !has(t, c2, "INBOX", 1) {
		t.Error("stesso account dopo riapertura: nessuno svuotamento")
	}
	// un altro account dopo la riapertura svuota (il legame è su disco)
	if err := c2.BindAccount("altro@gmail.com"); err != nil {
		t.Fatal(err)
	}
	if has(t, c2, "INBOX", 1) {
		t.Error("account diverso dopo riapertura: la cache doveva svuotarsi")
	}
}

func TestNoteUIDValidityChangeDropsMailbox(t *testing.T) {
	c := newCache(t)
	fill(t, c, 1, 1) // INBOX epoca 1
	if err := c.SetMessage("Label", 7, gm("Label", 9, 9)); err != nil {
		t.Fatal(err)
	}
	changed, err := c.NoteUIDValidity("INBOX", 1)
	if err != nil || changed {
		t.Fatalf("stessa epoca: changed=%v err=%v, voglio false", changed, err)
	}
	if !has(t, c, "INBOX", 1) {
		t.Fatal("stessa epoca: il corpo deve restare")
	}
	changed, err = c.NoteUIDValidity("INBOX", 2)
	if err != nil || !changed {
		t.Fatalf("epoca diversa: changed=%v err=%v, voglio true", changed, err)
	}
	if has(t, c, "INBOX", 1) {
		t.Error("epoca diversa: i corpi della mailbox vanno scartati")
	}
	if !has(t, c, "Label", 9) {
		t.Error("le altre mailbox non vanno toccate")
	}
	if v, ok, _ := c.UIDValidity("INBOX"); !ok || v != 2 {
		t.Errorf("UIDValidity = %d ok=%v, voglio 2", v, ok)
	}
	// l'indice LRU non conserva l'unità scartata: 199 unità nuove + Label (1) = 200
	for g := uint64(10); g <= 10+MaxBodies-2; g++ {
		if err := c.SetMessage("INBOX", 2, gm("INBOX", uint32(g), g)); err != nil {
			t.Fatal(err)
		}
	}
	if !has(t, c, "Label", 9) {
		t.Error("voce LRU fantasma: l'unità scartata contava ancora nel limite")
	}
}

func TestNoteUIDValidityFirstSeenIsRecordedNotChange(t *testing.T) {
	c := newCache(t)
	changed, err := c.NoteUIDValidity("INBOX", 9)
	if err != nil || changed {
		t.Fatalf("prima epoca vista: changed=%v err=%v, voglio false", changed, err)
	}
	if v, ok, _ := c.UIDValidity("INBOX"); !ok || v != 9 {
		t.Errorf("UIDValidity = %d ok=%v, voglio 9", v, ok)
	}
}

// Come per gli hit: confermare la stessa epoca non scrive (nessun fsync per lista).
func TestNoteUIDValiditySameEpochDoesNotWrite(t *testing.T) {
	c := newCache(t)
	if _, err := c.NoteUIDValidity("INBOX", 9); err != nil {
		t.Fatal(err)
	}
	before := txID(c)
	for i := 0; i < 10; i++ {
		if _, err := c.NoteUIDValidity("INBOX", 9); err != nil {
			t.Fatal(err)
		}
	}
	if after := txID(c); after != before {
		t.Errorf("NoteUIDValidity con epoca invariata ha scritto: %d -> %d", before, after)
	}
}
