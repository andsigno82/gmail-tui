package imap

// Piano 11, Task 2: Change.Refs porta le intestazioni dei nuovi messaggi INBOX
// (per la notifica desktop) senza segnarli come letti.

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// recheckSetup connette lo store, seleziona INBOX e fissa la baseline come Watch:
// il ricontrollo successivo è lo stesso codice della riconnessione.
func recheckSetup(t *testing.T, ts *testServer) (*Store, *watchState, chan mail.Change) {
	t.Helper()
	store := connectStore(t, ts)
	c, err := store.connected()
	if err != nil {
		t.Fatal(err)
	}
	sel, err := c.Select("INBOX", nil).Wait()
	if err != nil {
		t.Fatal(err)
	}
	last, err := store.highestUID(t.Context(), c, "INBOX")
	if err != nil {
		t.Fatal(err)
	}
	return store, &watchState{lastUID: last, uidValid: sel.UIDValidity, mailbox: "INBOX"}, make(chan mail.Change, 4)
}

// recheckRun esegue recheckNewMail sulla connessione corrente dello store e
// restituisce il Change emesso.
func recheckRun(t *testing.T, store *Store, st *watchState, ch chan mail.Change) mail.Change {
	t.Helper()
	c, err := store.connected()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.recheckNewMail(t.Context(), c, ch, st); err != nil {
		t.Fatalf("recheckNewMail: %v", err)
	}
	select {
	case got := <-ch:
		return got
	default:
		t.Fatal("nessun Change emesso")
		return mail.Change{}
	}
}

// Piano 11, review punto 8: due messaggi, ordine dal più recente, conteggio.
func TestRecheckRefsNewestFirstAndCountsAll(t *testing.T) {
	ts := startTestServer(t)
	store, st, ch := recheckSetup(t, ts)
	ts.seedMail("Ada <ada@example.org>", "Primo", "c", false, false)
	ts.seedMail("Bob <bob@example.org>", "Secondo", "c", false, false)
	got := recheckRun(t, store, st, ch)
	if len(got.IDs) != 2 || got.Unread != 2 {
		t.Fatalf("IDs=%d Unread=%d, voglio 2 e 2", len(got.IDs), got.Unread)
	}
	if len(got.Refs) != 2 || got.Refs[0].Subject != "Secondo" || got.Refs[1].Subject != "Primo" {
		t.Fatalf("Refs = %+v, voglio [Secondo, Primo] (dal più recente)", got.Refs)
	}
}

// Piano 11, review punto 2: le intestazioni sono solo degli ultimi 5 UID, il
// conteggio resta quello reale.
func TestRecheckHeadersCappedAtLastFive(t *testing.T) {
	ts := startTestServer(t)
	store, st, ch := recheckSetup(t, ts)
	for i := 1; i <= 7; i++ {
		ts.seedMail("Ada <ada@example.org>", "M"+string(rune('0'+i)), "c", false, false)
	}
	got := recheckRun(t, store, st, ch)
	if len(got.IDs) != 7 || got.Unread != 7 {
		t.Fatalf("IDs=%d Unread=%d, voglio 7 e 7", len(got.IDs), got.Unread)
	}
	if len(got.Refs) != 5 {
		t.Fatalf("Refs = %d, voglio 5 (cappa)", len(got.Refs))
	}
	if got.Refs[0].Subject != "M7" || got.Refs[4].Subject != "M3" {
		t.Fatalf("Refs da %q a %q, voglio da M7 a M3", got.Refs[0].Subject, got.Refs[4].Subject)
	}
}

// Piano 11, review punto 3: il conteggio considera solo i messaggi non letti.
func TestRecheckCountsOnlyUnread(t *testing.T) {
	ts := startTestServer(t)
	store, st, ch := recheckSetup(t, ts)
	ts.seedMail("A <a@example.org>", "Letta1", "c", true, false)
	ts.seedMail("B <b@example.org>", "NonLetta", "c", false, false)
	ts.seedMail("C <c@example.org>", "Letta2", "c", true, false)
	got := recheckRun(t, store, st, ch)
	if len(got.IDs) != 3 || got.Unread != 1 {
		t.Fatalf("IDs=%d Unread=%d, voglio 3 e 1", len(got.IDs), got.Unread)
	}
}

// Piano 11, review punto 3: tutti letti (es. letti da un altro client) → nessun non letto.
func TestRecheckAllReadCountsZero(t *testing.T) {
	ts := startTestServer(t)
	store, st, ch := recheckSetup(t, ts)
	ts.seedMail("A <a@example.org>", "L1", "c", true, false)
	ts.seedMail("B <b@example.org>", "L2", "c", true, false)
	got := recheckRun(t, store, st, ch)
	if len(got.IDs) != 2 || got.Unread != 0 {
		t.Fatalf("IDs=%d Unread=%d, voglio 2 e 0", len(got.IDs), got.Unread)
	}
}

// Piano 11, review punto 3 e 8: riconnessione. Il ricontrollo post-riconnessione
// usa la baseline di prima con un client nuovo: stessi conteggi.
func TestRecheckAfterReconnectUsesOldBaseline(t *testing.T) {
	ts := startTestServer(t)
	_, st, ch := recheckSetup(t, ts)
	ts.seedMail("A <a@example.org>", "Offline1", "c", true, false)
	ts.seedMail("B <b@example.org>", "Offline2", "c", false, false)
	store2 := connectStore(t, ts) // riconnessione: client nuovo, stessa baseline
	got := recheckRun(t, store2, st, ch)
	if len(got.IDs) != 2 || got.Unread != 1 {
		t.Fatalf("riconnessione: IDs=%d Unread=%d, voglio 2 e 1", len(got.IDs), got.Unread)
	}
}

// Piano 11, review punto 8: fetch delle intestazioni fallito → ID e conteggio
// restano, Refs vuoti.
func TestRecheckHeaderFailureKeepsIDsAndCount(t *testing.T) {
	ts := startTestServer(t)
	store, st, ch := recheckSetup(t, ts)
	store.headersFn = func(string, []imap.UID) ([]msg.MessageRef, error) {
		return nil, errors.New("fetch fallito")
	}
	ts.seedMail("A <a@example.org>", "X", "c", false, false)
	ts.seedMail("B <b@example.org>", "Y", "c", false, false)
	got := recheckRun(t, store, st, ch)
	if len(got.IDs) != 2 || got.Unread != 2 || got.Refs != nil {
		t.Fatalf("IDs=%d Unread=%d Refs=%v, voglio 2, 2, nil", len(got.IDs), got.Unread, got.Refs)
	}
}

func TestWatchNewMailCarriesHeadersWithoutSeen(t *testing.T) {
	ts := startTestServer(t)
	ts.seedMail("Base <base@example.org>", "Base", "corpo base", false, false)
	store := connectStore(t, ts)

	ch, err := store.Watch(t.Context())
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	settleIdle()

	uid := ts.seedMail("Ada <ada@example.org>", "Oggetto di prova", "corpo nuovo", false, false)

	c := recvChange(t, ch, 5*time.Second)
	if c.Kind != mail.ChangeNewMail {
		t.Fatalf("Kind = %d, voglio ChangeNewMail", c.Kind)
	}
	if len(c.Refs) != 1 {
		t.Fatalf("Refs = %d, voglio 1 (una per ID nuovo)", len(c.Refs))
	}
	r := c.Refs[0]
	if r.ID != msg.NewID("INBOX", uid) {
		t.Errorf("Refs[0].ID = %s, voglio %s", r.ID, msg.NewID("INBOX", uid))
	}
	if !strings.Contains(r.From, "ada@example.org") {
		t.Errorf("Refs[0].From = %q, voglio il mittente Ada", r.From)
	}
	if r.Subject != "Oggetto di prova" {
		t.Errorf("Refs[0].Subject = %q, voglio %q", r.Subject, "Oggetto di prova")
	}

	// Il fetch delle intestazioni usa ENVELOPE/FLAGS senza BODY: il messaggio
	// resta non letto (nessun \Seen aggiunto dal Watch).
	list, err := store.ListMessages(t.Context(), msg.Query{}, mail.Page{})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	for _, l := range list {
		if l.ID == r.ID && !l.Unread {
			t.Errorf("il messaggio nuovo risulta letto: il Watch non deve usare \\Seen")
		}
	}
}
