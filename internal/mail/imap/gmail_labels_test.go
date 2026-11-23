package imap

// Piano 10, Task 5: AllMailIDs e ModifyLabels (D10-T4, D10-T5). Con X-GM-EXT-1 il
// memserver non conosce X-GM-*: i test Gmail usano il server scriptato, quelli
// senza Gmail il memserver.

import (
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// scriptedLog registra i comandi ricevuti dal server scriptato (goroutine-safe).
type scriptedLog struct {
	mu   sync.Mutex
	cmds []string
}

func (l *scriptedLog) add(cmd string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cmds = append(l.cmds, cmd)
}

func (l *scriptedLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.cmds...)
}

// TestAllMailIDsScripted: per ogni ID in INBOX legge X-GM-MSGID e cerca lo stesso
// messaggio in Tutti i messaggi con SEARCH X-GM-MSGID.
func TestAllMailIDsScripted(t *testing.T) {
	log := &scriptedLog{}
	box := ""
	respond := func(cmd string) []string {
		log.add(cmd)
		switch {
		case strings.HasPrefix(cmd, "LIST"):
			return []string{`* LIST (\HasNoChildren) "/" INBOX`, `* LIST (\HasNoChildren \All) "/" "[Gmail]/All Mail"`}
		case strings.HasPrefix(cmd, "SELECT"):
			box = strings.Trim(strings.TrimPrefix(cmd, "SELECT "), `"`)
			return []string{"* 2 EXISTS"}
		case strings.HasPrefix(cmd, "UID FETCH"):
			return []string{"* 1 FETCH (UID 1 X-GM-MSGID 555)", "* 2 FETCH (UID 2 X-GM-MSGID 777)"}
		case strings.HasPrefix(cmd, "UID SEARCH") && box == "[Gmail]/All Mail":
			switch {
			case strings.Contains(cmd, "X-GM-MSGID 555"):
				return []string{"* SEARCH 9"}
			case strings.Contains(cmd, "X-GM-MSGID 777"):
				return []string{"* SEARCH 10"}
			}
		}
		return nil
	}
	addr := scriptedIMAP(t, "IMAP4rev1 X-GM-EXT-1 UIDPLUS", respond)

	s := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true})
	if err := s.Connect(t.Context()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = s.Close() }()

	got, err := s.AllMailIDs(t.Context(), []msg.MessageID{msg.NewID("INBOX", 1), msg.NewID("INBOX", 2)})
	if err != nil {
		t.Fatalf("AllMailIDs: %v", err)
	}
	want := []msg.MessageID{msg.NewID("[Gmail]/All Mail", 9), msg.NewID("[Gmail]/All Mail", 10)}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("AllMailIDs = %v, voglio %v", got, want)
	}
}

// TestAllMailIDsNoGmailErrors: senza X-GM-EXT-1 errore, nessun comando al server.
func TestAllMailIDsNoGmailErrors(t *testing.T) {
	log := &scriptedLog{}
	addr := scriptedIMAP(t, "IMAP4rev1 UIDPLUS", func(cmd string) []string {
		log.add(cmd)
		return nil
	})
	s := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true})
	if err := s.Connect(t.Context()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = s.Close() }()

	if _, err := s.AllMailIDs(t.Context(), []msg.MessageID{msg.NewID("INBOX", 1)}); err == nil {
		t.Fatal("AllMailIDs senza X-GM-EXT-1: err = nil, voglio errore")
	}
	if cmds := log.all(); len(cmds) != 0 {
		t.Errorf("comandi inviati senza Gmail: %v, voglio nessuno", cmds)
	}
}

// TestModifyLabelsGmailStoreScripted: con Gmail add e remove sono STORE ±X-GM-LABELS
// (SILENT) per ogni mailbox degli ID; l'add è codificato in modified UTF-7.
func TestModifyLabelsGmailStoreScripted(t *testing.T) {
	log := &scriptedLog{}
	respond := func(cmd string) []string {
		log.add(cmd)
		switch {
		case strings.HasPrefix(cmd, "LIST"):
			return []string{`* LIST (\HasNoChildren \All) "/" "[Gmail]/All Mail"`}
		case strings.HasPrefix(cmd, "SELECT"):
			return []string{"* 1 EXISTS"}
		}
		return nil
	}
	addr := scriptedIMAP(t, "IMAP4rev1 X-GM-EXT-1 UIDPLUS", respond)

	s := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true})
	if err := s.Connect(t.Context()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = s.Close() }()

	ids := []msg.MessageID{msg.NewID("INBOX", 1), msg.NewID("[Gmail]/All Mail", 4)}
	if err := s.ModifyLabels(t.Context(), ids, []string{"Lavoro Ü"}, []string{"Vecchia"}); err != nil {
		t.Fatalf("ModifyLabels: %v", err)
	}
	adds := 0
	dels := 0
	for _, c := range log.all() {
		if strings.Contains(c, `+X-GM-LABELS.SILENT ("Lavoro &ANw-")`) {
			adds++
		}
		if strings.Contains(c, `-X-GM-LABELS.SILENT ("Vecchia")`) {
			dels++
		}
	}
	if adds != 2 || dels != 2 {
		t.Errorf("STORE add = %d, remove = %d (voglio 2 e 2, una per mailbox): %v", adds, dels, log.all())
	}
}

// TestEncodeModifiedUTF7: codifica dei nomi label non ASCII (RFC 3501 §5.1.3).
func TestEncodeModifiedUTF7(t *testing.T) {
	cases := map[string]string{
		"Lavoro":   "Lavoro",
		"Ü":        "&ANw-",
		"a&b":      "a&-b",
		"Lavoro Ü": "Lavoro &ANw-",
	}
	for in, want := range cases {
		if got := encodeModifiedUTF7(in); got != want {
			t.Errorf("encodeModifiedUTF7(%q) = %q, voglio %q", in, got, want)
		}
	}
}

// TestModifyLabelsNoGmailAddCopies: senza Gmail add copia il messaggio nella label
// (creandola se manca) e lascia l'originale in INBOX.
func TestModifyLabelsNoGmailAddCopies(t *testing.T) {
	ts := startTestServerCaps(t, gmailCaps)
	ts.createMailbox("Work")
	ts.seedMail("A <a@example.org>", "Uno", "corpo", false, false)
	store := connectStore(t, ts)

	if err := store.ModifyLabels(t.Context(), []msg.MessageID{msg.NewID("INBOX", 1)}, []string{"Work"}, nil); err != nil {
		t.Fatalf("ModifyLabels: %v", err)
	}
	if got := mailboxSubjects(t, store, "Work"); len(got) != 1 {
		t.Errorf("Work = %v, voglio 1 copia", got)
	}
	if got := mailboxSubjects(t, store, ""); len(got) != 1 {
		t.Errorf("INBOX = %v, voglio l'originale ancora lì", got)
	}
}

// TestModifyLabelsNoGmailRemoveOutsideFolderRefused: senza Gmail la rimozione da una
// label richiede ID che stanno in quella label; altrimenti rifiuto senza azioni.
func TestModifyLabelsNoGmailRemoveOutsideFolderRefused(t *testing.T) {
	ts := startTestServerCaps(t, gmailCaps)
	ts.createMailbox("Work")
	ts.seedMail("A <a@example.org>", "Uno", "corpo", false, false)
	store := connectStore(t, ts)

	if err := store.ModifyLabels(t.Context(), []msg.MessageID{msg.NewID("INBOX", 1)}, nil, []string{"Work"}); err == nil {
		t.Fatal("rimozione da Work di un ID in INBOX: err = nil, voglio rifiuto")
	}
	if got := mailboxSubjects(t, store, ""); len(got) != 1 {
		t.Errorf("INBOX dopo rifiuto = %v, voglio 1 messaggio", got)
	}

	if err := store.ModifyLabels(t.Context(), []msg.MessageID{msg.NewID("INBOX", 1)}, []string{"Work"}, nil); err != nil {
		t.Fatalf("add a Work: %v", err)
	}
	if err := store.ModifyLabels(t.Context(), []msg.MessageID{msg.NewID("Work", 1)}, nil, []string{"Work"}); err != nil {
		t.Fatalf("remove da Work: %v", err)
	}
	if got := mailboxSubjects(t, store, "Work"); len(got) != 0 {
		t.Errorf("Work dopo remove = %v, voglio vuota", got)
	}
	if got := mailboxSubjects(t, store, ""); len(got) != 1 {
		t.Errorf("INBOX dopo remove = %v, voglio 1 messaggio", got)
	}
}

// TestModifyLabelsGmailPartialFailureKeepsCache: con Gmail lo STORE +X-GM-LABELS riesce
// e lo STORE -X-GM-LABELS fallisce. Il server ha applicato l'aggiunta: la cache deve
// seguire la parte riuscita (Piano 10, review punto 4), non restare indietro.
func TestModifyLabelsGmailPartialFailureKeepsCache(t *testing.T) {
	respond := func(cmd string) []string {
		switch {
		case strings.HasPrefix(cmd, "LIST"):
			return []string{`* LIST (\HasNoChildren \All) "/" "[Gmail]/All Mail"`}
		case strings.HasPrefix(cmd, "SELECT"):
			return []string{"* 1 EXISTS"}
		case strings.Contains(cmd, "-X-GM-LABELS"):
			return []string{"NO rimozione rifiutata"}
		}
		return nil
	}
	addr := scriptedIMAP(t, "IMAP4rev1 X-GM-EXT-1 UIDPLUS", respond)
	c := openTestCache(t)
	if err := c.SetMessage("INBOX", 1, &msg.Message{Ref: msg.MessageRef{ID: msg.NewID("INBOX", 1), GmailID: 77}}); err != nil {
		t.Fatalf("SetMessage: %v", err)
	}

	s := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true, Cache: c})
	if err := s.Connect(t.Context()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = s.Close() }()

	if err := s.ModifyLabels(t.Context(), []msg.MessageID{msg.NewID("INBOX", 1)}, []string{"Lavoro"}, []string{"Vecchia"}); err == nil {
		t.Fatal("ModifyLabels con STORE rifiutato: err = nil, voglio errore")
	}
	got, ok, err := c.GetMessage("INBOX", 1)
	if err != nil || !ok {
		t.Fatalf("GetMessage: ok=%v err=%v", ok, err)
	}
	if !slices.Contains(got.Ref.Labels, "Lavoro") {
		t.Errorf("label in cache = %v, voglio Lavoro: l'aggiunta è riuscita sul server", got.Ref.Labels)
	}
}
