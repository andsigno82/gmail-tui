package cache

// Piano 16, Task 3: compattazione a Close (D16-T8) e permessi (D16-A4).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCloseCompactsBloatedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	c, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c.db.NoSync = true
	for i := uint32(1); i <= 10; i++ {
		m := gm("Big", i, uint64(i))
		m.BodyText = strings.Repeat("x", 1<<20)
		if err := c.SetMessage("Big", 1, m); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.DropMailbox("Big"); err != nil { // 10 MiB liberati
		t.Fatal(err)
	}
	// una scrittura piccola fa rilasciare le pagine in sospeso
	if err := c.SetMessage("INBOX", 1, gm("INBOX", 1, 1)); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if before.Size() < 10<<20 {
		t.Fatalf("setup: file di %d byte, voglio >= 10 MiB", before.Size())
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() >= before.Size()/2 {
		t.Errorf("file dopo Close = %d byte, prima %d: compattazione non avvenuta", after.Size(), before.Size())
	}
	if perm := after.Mode().Perm(); perm != 0o600 {
		t.Errorf("permessi dopo la compattazione = %v, voglio 0600", perm)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("il file temporaneo deve sparire: %v", err)
	}
	// i dati sopravvivono e il file si riapre
	c2, err := Open(path)
	if err != nil {
		t.Fatalf("riapertura dopo compattazione: %v", err)
	}
	defer func() { _ = c2.Close() }()
	if !has(t, c2, "INBOX", 1) {
		t.Error("il corpo rimasto è sparito con la compattazione")
	}
}

func TestCloseSkipsCompactionSmallFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	c, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c.db.NoSync = true
	m := gm("Big", 1, 1)
	m.BodyText = strings.Repeat("x", 300<<10)
	if err := c.SetMessage("Big", 1, m); err != nil {
		t.Fatal(err)
	}
	if err := c.DropMailbox("Big"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetMessage("INBOX", 1, gm("INBOX", 1, 1)); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(path)
	if !os.SameFile(before, after) {
		t.Error("file sotto 4 MiB: nessuna compattazione (il rename cambia l'inode)")
	}
}

func TestOpenCreatesPrivateDirAndFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "gmail-tui")
	path := filepath.Join(dir, "cache.db")
	c, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	di, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := di.Mode().Perm(); perm != 0o700 {
		t.Errorf("cartella = %v, voglio 0700", perm)
	}
	fi, _ := os.Stat(path)
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("file = %v, voglio 0600", perm)
	}
}

// Una cartella nata 0755 (versioni precedenti) e un file troppo aperto vengono
// riportati a 0700/0600 all'apertura.
func TestOpenTightensExistingPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "gmail-tui")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "cache.db")
	c, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	c2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c2.Close() }()
	di, _ := os.Stat(dir)
	fi, _ := os.Stat(path)
	if di.Mode().Perm() != 0o700 || fi.Mode().Perm() != 0o600 {
		t.Errorf("permessi = %v/%v, voglio 0700/0600", di.Mode().Perm(), fi.Mode().Perm())
	}
}
