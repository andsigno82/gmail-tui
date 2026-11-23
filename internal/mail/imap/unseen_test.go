package imap

// Test di UnreadCounts (Piano 15, Task 11): STATUS (UNSEEN) per INBOX e label
// utente sotto gate, LIST-STATUS solo se il server lo annuncia, nessun STATUS
// sulla mailbox selezionata, best-effort sulle mailbox che falliscono.

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/emersion/go-imap/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// listStatusCaps sono le capability di gmailCaps più LIST-STATUS (RFC 5819).
var listStatusCaps = func() imap.CapSet {
	caps := imap.CapSet{imap.CapListStatus: {}}
	for c := range gmailCaps {
		caps[c] = struct{}{}
	}
	return caps
}()

// seedUnreadCorpus prepara INBOX (2 non letti, 1 letto), Work (1 non letto),
// Work/Sub (letto), Empty (vuota), più cartelle che NON devono comparire:
// il contenitore \Noselect e le speciali [Gmail]/* (con non letti).
func seedUnreadCorpus(ts *testServer) {
	ts.listAs("[Gmail]", imap.MailboxAttrNoSelect)
	ts.createSpecialMailbox("[Gmail]/Trash", imap.MailboxAttrTrash)
	ts.createSpecialMailbox("[Gmail]/All Mail", imap.MailboxAttrAll)
	ts.createMailbox("Work")
	ts.createMailbox("Work/Sub")
	ts.createMailbox("Empty")
	ts.seedMail("a@example.org", "I1", "x", false, false)
	ts.seedMail("a@example.org", "I2", "x", false, false)
	ts.seedMail("a@example.org", "I3", "x", true, false)
	ts.seedInMailbox("Work", "a@example.org", "W1", "x", false, false)
	ts.seedInMailbox("Work/Sub", "a@example.org", "S1", "x", true, false)
	ts.seedInMailbox("[Gmail]/Trash", "a@example.org", "T1", "x", false, false)
	ts.seedInMailbox("[Gmail]/All Mail", "a@example.org", "A1", "x", false, false)
}

var wantUnread = map[string]int{"INBOX": 2, "Work": 1, "Work/Sub": 0, "Empty": 0}

func equalCounts(got, want map[string]int) bool {
	if len(got) != len(want) {
		return false
	}
	for k, v := range want {
		if gv, ok := got[k]; !ok || gv != v {
			return false
		}
	}
	return true
}

// STATUS singoli (server senza LIST-STATUS): conteggi giusti, 0 per la
// mailbox vuota, nessuna speciale né \Noselect, uno STATUS per cartella.
func TestUnreadCountsStatusPerMailbox(t *testing.T) {
	ts := startTestServerCaps(t, gmailCaps)
	seedUnreadCorpus(ts)
	store := connectStore(t, ts)

	got, err := store.UnreadCounts(t.Context())
	if err != nil {
		t.Fatalf("UnreadCounts: %v", err)
	}
	if !equalCounts(got, wantUnread) {
		t.Fatalf("UnreadCounts = %v, voglio %v", got, wantUnread)
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.listStatusReqs != 0 {
		t.Errorf("LIST RETURN (STATUS) usato senza capability: %d", ts.listStatusReqs)
	}
	log := slices.Clone(ts.statusLog)
	slices.Sort(log)
	// Nessun SELECT prima: STATUS su tutte e quattro.
	if want := []string{"Empty", "INBOX", "Work", "Work/Sub"}; !slices.Equal(log, want) {
		t.Errorf("STATUS inviati su %v, voglio %v", log, want)
	}
}

// Con LIST-STATUS annunciato basta un LIST: zero comandi STATUS, stessi conteggi.
func TestUnreadCountsListStatus(t *testing.T) {
	ts := startTestServerCaps(t, listStatusCaps)
	seedUnreadCorpus(ts)
	store := connectStore(t, ts)

	got, err := store.UnreadCounts(t.Context())
	if err != nil {
		t.Fatalf("UnreadCounts: %v", err)
	}
	if !equalCounts(got, wantUnread) {
		t.Fatalf("UnreadCounts = %v, voglio %v", got, wantUnread)
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.listStatusReqs != 1 || len(ts.statusLog) != 0 {
		t.Errorf("LIST-STATUS = %d, STATUS singoli = %v: voglio 1 LIST e nessuno STATUS", ts.listStatusReqs, ts.statusLog)
	}
}

// La mailbox selezionata non riceve STATUS (RFC 3501 §6.3.10): il conteggio
// viene da SEARCH UNSEEN, e la selezione resta quella di prima.
func TestUnreadCountsSelectedMailboxNoStatus(t *testing.T) {
	for name, caps := range map[string]imap.CapSet{"status": gmailCaps, "liststatus": listStatusCaps} {
		t.Run(name, func(t *testing.T) {
			ts := startTestServerCaps(t, caps)
			seedUnreadCorpus(ts)
			store := connectStore(t, ts)
			// Seleziona Work (1 non letto) come farebbe una ListMessages.
			if _, err := store.ListMessages(t.Context(), msg.Query{Label: "Work"}, mail.Page{}); err != nil {
				t.Fatalf("ListMessages: %v", err)
			}

			got, err := store.UnreadCounts(t.Context())
			if err != nil {
				t.Fatalf("UnreadCounts: %v", err)
			}
			if !equalCounts(got, wantUnread) {
				t.Fatalf("UnreadCounts = %v, voglio %v", got, wantUnread)
			}
			ts.mu.Lock()
			log := slices.Clone(ts.statusLog)
			ts.mu.Unlock()
			if slices.Contains(log, "Work") {
				t.Errorf("STATUS inviato sulla mailbox selezionata: %v", log)
			}
			if sel := store.client.Mailbox(); sel == nil || sel.Name != "Work" {
				t.Errorf("mailbox selezionata dopo UnreadCounts = %+v, voglio Work", sel)
			}
		})
	}
}

// Best-effort: una mailbox listata ma con STATUS che fallisce (qui inesistente)
// è saltata, le altre restano.
func TestUnreadCountsSkipsFailingMailbox(t *testing.T) {
	for name, caps := range map[string]imap.CapSet{"status": gmailCaps, "liststatus": listStatusCaps} {
		t.Run(name, func(t *testing.T) {
			ts := startTestServerCaps(t, caps)
			seedUnreadCorpus(ts)
			ts.listAs("Ghost") // nel LIST, ma non esiste: STATUS -> NO
			store := connectStore(t, ts)

			got, err := store.UnreadCounts(t.Context())
			if err != nil {
				t.Fatalf("UnreadCounts: %v", err)
			}
			if _, ok := got["Ghost"]; ok {
				t.Errorf("Ghost presente: %v", got)
			}
			if !equalCounts(got, wantUnread) {
				t.Fatalf("UnreadCounts = %v, voglio %v", got, wantUnread)
			}
		})
	}
}

// Errore di connessione: si restituisce (non è best-effort).
func TestUnreadCountsConnectionError(t *testing.T) {
	ts := startTestServerCaps(t, gmailCaps)
	seedUnreadCorpus(ts)
	store := connectStore(t, ts)
	_ = ts.srv.Close() // chiude anche le connessioni attive
	if _, err := store.UnreadCounts(t.Context()); err == nil {
		t.Fatal("UnreadCounts su connessione chiusa: voglio un errore")
	}
}

func TestUnreadCountsNotConnected(t *testing.T) {
	s := NewStore(Options{Addr: "127.0.0.1:1", Username: "u", Password: "p"})
	_, err := s.UnreadCounts(t.Context())
	if !errors.Is(err, mail.ErrNotConnected) {
		t.Fatalf("errore = %v, atteso mail.ErrNotConnected", err)
	}
}

func TestUnreadCountsCanceledContext(t *testing.T) {
	ts := startTestServerCaps(t, gmailCaps)
	seedUnreadCorpus(ts)
	store := connectStore(t, ts)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.UnreadCounts(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("errore = %v, atteso context.Canceled", err)
	}
}
