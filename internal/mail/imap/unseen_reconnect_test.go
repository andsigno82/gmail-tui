package imap

// Dopo un budget scaduto dentro un comando UnreadCounts chiude la connessione:
// lo Store deve riconnettersi subito, prima di rilasciare il gate, cosi le op
// successive funzionano (o, se la riconnessione e' impossibile, danno
// mail.ErrNotConnected come per ogni store non connesso).

import (
	"errors"
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// stalledStore: server con STATUS lento (1,5 s) e budget di 200 ms.
func stalledStore(t *testing.T) (*testServer, *Store) {
	t.Helper()
	old := unseenBudget
	unseenBudget = 200 * time.Millisecond
	t.Cleanup(func() { unseenBudget = old })
	ts := startTestServerCaps(t, gmailCaps)
	seedUnreadCorpus(ts)
	store := connectStore(t, ts)
	ts.mu.Lock()
	ts.statusDelay = 1500 * time.Millisecond
	ts.mu.Unlock()
	return ts, store
}

func (ts *testServer) sessionCount() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.sessions
}

func TestUnreadCountsStallRiconnetteSubito(t *testing.T) {
	ts, store := stalledStore(t)
	got, err := store.UnreadCounts(t.Context())
	if err != nil || len(got) >= len(wantUnread) {
		t.Fatalf("UnreadCounts = %v, %v: voglio la mappa parziale senza errore", got, err)
	}
	refs, err := store.ListMessages(t.Context(), msg.Query{}, mail.Page{})
	if err != nil || len(refs) == 0 {
		t.Fatalf("ListMessages dopo lo stallo = %d righe, %v: la connessione non e' stata ristabilita", len(refs), err)
	}
	if n := ts.sessionCount(); n != 2 {
		t.Errorf("connessioni = %d, voglio 2 (iniziale + una riconnessione)", n)
	}
}

func TestUnreadCountsStallRiconnessioneImpossibile(t *testing.T) {
	ts, store := stalledStore(t)
	_ = ts.ln.Close() // nuovi dial rifiutati
	if _, err := store.UnreadCounts(t.Context()); err != nil {
		t.Fatalf("UnreadCounts: %v", err)
	}
	_, err := store.ListMessages(t.Context(), msg.Query{}, mail.Page{})
	if !errors.Is(err, mail.ErrNotConnected) {
		t.Fatalf("ListMessages = %v, voglio mail.ErrNotConnected", err)
	}
}

// Con un Watch attivo: nessun panic, nessuna seconda connessione dal Watch, e il
// feed continua a consegnare la posta nuova sulla connessione ristabilita.
func TestUnreadCountsStallConWatchAttivo(t *testing.T) {
	ts, store := stalledStore(t)
	ch, err := store.Watch(t.Context())
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	settleIdle()
	if _, err := store.UnreadCounts(t.Context()); err != nil {
		t.Fatalf("UnreadCounts: %v", err)
	}
	if _, err := store.ListMessages(t.Context(), msg.Query{}, mail.Page{}); err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	settleIdle() // il Watch rientra in IDLE sulla nuova connessione
	ts.seedMail("n@example.org", "nuova", "x", false, false)
	c := recvChange(t, ch, 5*time.Second)
	if c.Kind != mail.ChangeNewMail {
		t.Fatalf("Change = %+v, voglio ChangeNewMail (nessun Disconnected: lo Store si e' riconnesso da solo)", c)
	}
	time.Sleep(300 * time.Millisecond)
	if n := ts.sessionCount(); n != 2 {
		t.Errorf("connessioni = %d, voglio 2: il Watch non deve riconnettere di nuovo", n)
	}
}
