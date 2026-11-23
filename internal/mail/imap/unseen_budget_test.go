package imap

// Revisione Piano 15, punto 5: il budget di UnreadCounts vale anche DENTRO le
// chiamate LIST/STATUS/SEARCH: un server lento non deve tenere il gate (e quindi
// il Watch) oltre il budget.

import (
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
)

func TestUnreadCountsBudgetDentroIComandi(t *testing.T) {
	for name, tc := range map[string]struct {
		caps  imap.CapSet
		delay func(ts *testServer)
	}{
		"STATUS lento":                 {gmailCaps, func(ts *testServer) { ts.statusDelay = 1500 * time.Millisecond }},
		"LIST lento (con LIST-STATUS)": {listStatusCaps, func(ts *testServer) { ts.listDelay = 1500 * time.Millisecond }},
	} {
		t.Run(name, func(t *testing.T) {
			old := unseenBudget
			unseenBudget = 200 * time.Millisecond
			t.Cleanup(func() { unseenBudget = old })

			ts := startTestServerCaps(t, tc.caps)
			seedUnreadCorpus(ts)
			store := connectStore(t, ts)
			ts.mu.Lock()
			tc.delay(ts)
			ts.mu.Unlock()

			start := time.Now()
			got, err := store.UnreadCounts(t.Context())
			if el := time.Since(start); el > time.Second {
				t.Errorf("UnreadCounts ha impiegato %v con budget 200ms", el)
			}
			if err != nil {
				t.Fatalf("UnreadCounts: %v (voglio la mappa parziale senza errore)", err)
			}
			if len(got) >= len(wantUnread) {
				t.Errorf("mappa non parziale: %v", got)
			}

			// Il gate e' libero.
			done := make(chan struct{})
			go func() { store.gate.acquire()(); close(done) }()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("gate ancora tenuto dopo UnreadCounts")
			}
		})
	}
}
