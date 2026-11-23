package cache

// Piano 16, Task 13 (dubbio d): all'uscita la TUI chiude la cache mentre Watch e comandi
// in volo possono ancora usarla. bbolt su un database chiuso restituisce errore, non
// panico; i tocchi LRU letti prima della chiusura finiscono su disco e il file si riapre.

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestUseAfterCloseIsErrorNotPanic(t *testing.T) {
	c, err := Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetMessage("INBOX", 1, gm("INBOX", 1, 1)); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, _, err := c.GetMessage("INBOX", 1); err == nil {
		t.Error("GetMessage su cache chiusa deve dare errore")
	}
	if err := c.SetMessage("INBOX", 1, gm("INBOX", 2, 2)); err == nil {
		t.Error("SetMessage su cache chiusa deve dare errore")
	}
	if _, _, _, ok := c.GetList("INBOX"); ok {
		t.Error("GetList su cache chiusa deve essere un miss")
	}
	if _, _, ok := c.GetThread(1); ok {
		t.Error("GetThread su cache chiusa deve essere un miss")
	}
	if err := c.PutList("INBOX", 1, nil, false, time.Now()); err == nil {
		t.Error("PutList su cache chiusa deve dare errore")
	}
}

func TestCloseWhileInUseKeepsFileValid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	c, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := uint32(1); i <= 20; i++ {
		if err := c.SetMessage("INBOX", 1, gm("INBOX", i, uint64(i))); err != nil {
			t.Fatal(err)
		}
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := uint32(1); ; i = i%20 + 1 {
				select {
				case <-stop:
					return
				default:
				}
				_, _, _ = c.GetMessage("INBOX", i)
				if w == 0 {
					_ = c.SetMessage("INBOX", 1, gm("INBOX", 100+i, uint64(100+i)))
				}
			}
		}(w)
	}
	time.Sleep(30 * time.Millisecond)
	if err := c.Close(); err != nil {
		t.Errorf("Close con uso concorrente: %v", err)
	}
	close(stop)
	wg.Wait()

	c2, err := Open(path)
	if err != nil {
		t.Fatalf("riapertura dopo chiusura concorrente: %v", err)
	}
	defer func() { _ = c2.Close() }()
	if !has(t, c2, "INBOX", 1) {
		t.Error("un corpo scritto prima della chiusura e sparito")
	}
}
