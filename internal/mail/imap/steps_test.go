package imap

// Piano 12a, Task 5: i comandi del percorso di lettura lasciano un evento step
// per comando, col ruolo della cartella e mai col suo nome.

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/trace"
	"github.com/andsigno82/gmail-tui/internal/trace/tracestore"
)

// syncBuf e un buffer di traccia leggibile mentre altre goroutine (il Watch)
// potrebbero ancora scrivere: senza il mutex il race detector segnalerebbe la lettura.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// stepTrail ritorna "operazione passo ruolo" per ogni evento step della traccia.
func stepTrail(t *testing.T, buf *syncBuf) []string {
	t.Helper()
	var out []string
	for riga := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		var e trace.Event
		if err := json.Unmarshal([]byte(riga), &e); err != nil {
			t.Fatalf("riga di traccia non valida: %v", err)
		}
		if e.K == "step" {
			out = append(out, strings.TrimSpace(e.Op+" "+e.Step+" "+e.Role))
		}
	}
	return out
}

// traceRole non guarda mai il nome della cartella.
func TestTraceRoleNeverUsesMailboxName(t *testing.T) {
	s := NewStore(Options{})
	s.noteRoles([]mail.Mailbox{
		{Name: "INBOX", Role: "inbox"},
		{Name: "[Gmail]/Tutti i messaggi", Role: "all"},
		{Name: "Lavoro"},
	})
	cases := map[string]string{
		"": "inbox", "INBOX": "inbox",
		"[Gmail]/Tutti i messaggi": "all",
		"Lavoro":                   "label",
		"MaiVista":                 "label",
	}
	for mailbox, want := range cases {
		if got := s.traceRole(mailbox); got != want {
			t.Errorf("traceRole(%q) = %q, voglio %q", mailbox, got, want)
		}
	}
}

// Una lista Gmail, comando per comando: LIST per trovare Tutti i messaggi, poi
// SELECT, SEARCH e FETCH in INBOX e SELECT, SEARCH e FETCH su Tutti i messaggi.
func TestListConversationsEmitsOneStepPerCommand(t *testing.T) {
	var buf syncBuf
	trace.Install(trace.New(&buf, ""))
	defer trace.Uninstall()
	inbox := []gmailFetch{
		{uid: 1, thread: 100, login: "ann", date: "01-Nov-2026"},
		{uid: 2, thread: 200, login: "bob", date: "02-Nov-2026"},
	}
	addr := scriptedGmailServer(t, map[string][]gmailFetch{"INBOX": inbox, "[Gmail]/All Mail": inbox})
	inner := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true})
	st := tracestore.Wrap(inner)
	ctx := context.Background()
	if err := st.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = inner.Close() }()
	if _, err := st.ListConversations(ctx, msg.Query{}, mail.Page{}); err != nil {
		t.Fatalf("ListConversations: %v", err)
	}

	want := []string{
		"Connect tcp", "Connect login",
		"ListConversations list",
		"ListConversations select inbox",
		"ListConversations search inbox",
		"ListConversations fetch inbox",
		"ListConversations select all",
		"ListConversations search all",
		"ListConversations fetch all",
	}
	if got := stepTrail(t, &buf); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("passi = %v, voglio %v", got, want)
	}
	for _, nome := range []string{"INBOX", "All Mail"} {
		if strings.Contains(buf.String(), nome) {
			t.Errorf("la traccia contiene il nome di una cartella: %q", nome)
		}
	}
}

// La base del Watch (SELECT e SEARCH di INBOX) si misura come passo del Watch.
// Si guardano solo i passi del Watch: prima ci sono quelli del Connect, e il round
// di IDLE che segue rifà la SELECT.
func TestWatchBaselineEmitsSteps(t *testing.T) {
	var buf syncBuf
	trace.Install(trace.New(&buf, ""))
	defer trace.Uninstall()
	ts := startTestServer(t)
	s := connectStore(t, ts)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if _, err := s.Watch(ctx); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	got := stepTrail(t, &buf)
	var delWatch []string
	for _, passo := range got {
		if strings.HasPrefix(passo, "Watch ") {
			delWatch = append(delWatch, passo)
		}
	}
	if len(delWatch) < 2 || delWatch[0] != "Watch select inbox" || delWatch[1] != "Watch search inbox" {
		t.Errorf("passi = %v, voglio che i passi del Watch inizino con Watch select inbox, Watch search inbox", got)
	}
}
