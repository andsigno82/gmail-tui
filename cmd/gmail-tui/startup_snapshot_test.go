package main

// Misura end to end (Piano 16, Task 13): tempo dall'avvio alla prima lista mostrata
// dall'istantanea e all'apertura di una conversazione gia vista, con la cache bbolt
// VERA (stesso percorso di openCache), la pila reale (mail.Guard + tracestore + store
// IMAP come Snapshotter) e un tea.Program vero senza terminale. Connect e la lista
// fresca simulano la latenza di Gmail (misura del 2026-11-23: Connect 28,9 s, lista
// fino a 77 s). Obiettivo del piano: prima lista < 1 s e thread < 1 s con istantanea.
//
// Latenze di default corte (suite veloce); per i valori reali:
//
//	P16_CONNECT=29s P16_LIST=77s go test ./cmd/gmail-tui -run TestStartupFirstListFromSnapshot -v

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/app"
	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/mail/imap"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/trace"
	"github.com/andsigno82/gmail-tui/internal/trace/tracestore"
	"github.com/andsigno82/gmail-tui/internal/trace/tracetea"
)

const (
	startupAddr = "ada@example.com"
	startupAll  = "[Gmail]/All Mail"
	startupEp   = 7
)

// slowStore e un Mock con la latenza di Gmail su Connect e ListConversations; le
// istantanee vengono dallo store IMAP vero (mai connesso, solo cache su disco).
type slowStore struct {
	*mail.Mock
	snap    mail.Snapshotter
	connect time.Duration
	list    time.Duration
}

func wait(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *slowStore) Connect(ctx context.Context) error {
	if err := wait(ctx, s.connect); err != nil {
		return err
	}
	return s.Mock.Connect(ctx)
}

func (s *slowStore) ListConversations(ctx context.Context, q msg.Query, pg mail.Page) ([]mail.Conversation, error) {
	if err := wait(ctx, s.list); err != nil {
		return nil, err
	}
	return s.Mock.ListConversations(ctx, q, pg)
}

func (s *slowStore) CachedConversations(box string) (mail.ConversationsSnapshot, bool) {
	return s.snap.CachedConversations(box)
}
func (s *slowStore) CachedThread(id uint64) (mail.ThreadSnapshot, bool) {
	return s.snap.CachedThread(id)
}
func (s *slowStore) CachedMessage(id msg.MessageID) (*msg.Message, bool) {
	return s.snap.CachedMessage(id)
}

// stamp registra l'istante della prima View che contiene ciascun testo cercato.
type stamp struct {
	tea.Model
	t0      time.Time
	needles []string
	mu      *sync.Mutex
	seen    map[string]time.Duration
	seenCh  chan string
}

func (s stamp) Update(ev tea.Msg) (tea.Model, tea.Cmd) {
	n, c := s.Model.Update(ev)
	s.Model = n
	return s, c
}

func (s stamp) View() tea.View {
	v := s.Model.View()
	now := time.Since(s.t0)
	s.mu.Lock()
	for _, n := range s.needles {
		if _, ok := s.seen[n]; !ok && strings.Contains(v.Content, n) {
			s.seen[n] = now
			s.seenCh <- n
		}
	}
	s.mu.Unlock()
	return v
}

func envDur(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func TestStartupFirstListFromSnapshot(t *testing.T) {
	connect := envDur("P16_CONNECT", 2*time.Second)
	list := envDur("P16_LIST", 2*time.Second)
	t.Setenv("HOME", t.TempDir()) // UserCacheDir sotto la HOME di prova (darwin)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	// 1) Sessione precedente: la cache si popola come la scrive lo store IMAP.
	c := cacheFor(startupAddr, "pw")
	if c == nil {
		t.Fatal("cache non aperta")
	}
	base := time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC)
	rows := make([]mail.Conversation, 50)
	for i := range rows {
		r := msg.MessageRef{
			ID: msg.NewID("INBOX", uint32(i+1)), From: "mittente@example.org", Subject: fmt.Sprintf("SNAP-%02d", i),
			Date: base.Add(-time.Duration(i) * time.Hour), GmailID: uint64(1000 + i), ThreadID: uint64(500 + i), Unread: i%3 == 0,
		}
		rows[i] = mail.Conversation{Latest: r, ThreadID: r.ThreadID, Count: 1, Participants: []string{r.From}, Unread: r.Unread}
	}
	if err := c.PutList("INBOX", startupEp, rows, false, base); err != nil {
		t.Fatal(err)
	}
	// il thread della prima riga: 3 messaggi in Tutti i messaggi, corpi da ~20 KB in cache
	var trefs []msg.MessageRef
	for k := 0; k < 3; k++ {
		r := msg.MessageRef{
			ID: msg.NewID(startupAll, uint32(900+k)), From: "mittente@example.org", Subject: "SNAP-00",
			Date: base.Add(time.Duration(k) * time.Minute), GmailID: uint64(2000 + k), ThreadID: 500, Unread: k == 2,
		}
		trefs = append(trefs, r)
		body := fmt.Sprintf("CORPO-SNAP-%d ", k) + strings.Repeat("testo del messaggio in cache. ", 700)
		if err := c.SetMessage(startupAll, startupEp+2, &msg.Message{Ref: r, To: []string{startupAddr}, BodyText: body}); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.PutThread(500, startupAll, startupEp+2, trefs, base); err != nil {
		t.Fatal(err)
	}
	closeCache(c)

	// 2) Avvio a caldo: apertura della cache (come runTUI), pila reale, Program vero.
	t0 := time.Now()
	c2 := cacheFor(startupAddr, "pw")
	if c2 == nil {
		t.Fatal("riapertura della cache")
	}
	openCacheTime := time.Since(t0)
	defer closeCache(c2)

	baseMock := &mail.Mock{}
	for i := 0; i < 50; i++ {
		baseMock.Msgs = append(baseMock.Msgs, &msg.Message{
			Ref: msg.MessageRef{
				ID: msg.NewID("INBOX", uint32(100+i)), From: "server@example.org", Subject: fmt.Sprintf("FRESCO-%02d", i),
				Date: base.Add(time.Hour).Add(-time.Duration(i) * time.Minute), GmailID: uint64(7000 + i), ThreadID: uint64(8000 + i),
			},
			BodyText: "corpo fresco",
		})
	}
	// La traccia (D16-T14) deve riassumere la fase: snapshot_list_view e hit di snap_list.
	// Va installata prima di tracestore.Wrap (no-op senza traccia).
	var tb bytes.Buffer
	trace.Install(trace.New(&tb, "test"))
	defer trace.Uninstall()
	inner := imap.NewStore(imap.Options{Addr: imapAddr, Username: startupAddr, Password: "pw", Cache: c2})
	slow := &slowStore{Mock: baseMock, snap: inner, connect: connect, list: list}
	store := mail.Guard(tracestore.Wrap(slow)) // stessa pila di realStore

	th, _ := loadTheme("", "")
	m := app.New(store, baseMock, th)
	m.SetVersion("test")
	m.SetIdentity(startupAddr, "", "")

	needles := []string{"SNAP-00", "CORPO-SNAP-2", "FRESCO-00"}
	st := stamp{Model: tracetea.Wrap(m), t0: t0, needles: needles, mu: &sync.Mutex{}, seen: map[string]time.Duration{}, seenCh: make(chan string, 8)}
	p := tea.NewProgram(st, tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithWindowSize(120, 35), tea.WithoutSignalHandler())

	var openedAt atomic.Int64 // ns dall'avvio al momento del tasto invio
	go func() {
		defer p.Quit()
		timeout := time.After(connect + list + 10*time.Second)
		deadline := map[string]bool{}
		for len(deadline) < 3 {
			select {
			case n := <-st.seenCh:
				deadline[n] = true
				if n == "SNAP-00" {
					openedAt.Store(int64(time.Since(t0)))
					p.Send(tea.KeyPressMsg{Code: tea.KeyEnter}) // apre la prima conversazione
				}
				// con le latenze di default non si aspetta la lista fresca
				if n == "CORPO-SNAP-2" && os.Getenv("P16_CONNECT") == "" {
					return
				}
			case <-timeout:
				return
			}
		}
	}()
	if _, err := p.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	trace.Uninstall()
	var sum bytes.Buffer
	if err := trace.Summarize(&tb, &sum); err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	for _, want := range []string{"snapshot_list_view", "snap_list"} {
		if !strings.Contains(sum.String(), want) {
			t.Errorf("il riepilogo della traccia non contiene %q\n%s", want, sum.String())
		}
	}
	if !regexp.MustCompile(`snap_list\s+1\s+0\s+0`).MatchString(sum.String()) {
		t.Errorf("il riepilogo deve contare 1 hit e 0 miss di snap_list\n%s", sum.String())
	}
	t.Logf("riepilogo traccia:\n%s", sum.String())

	st.mu.Lock()
	defer st.mu.Unlock()
	firstList, ok := st.seen["SNAP-00"]
	if !ok {
		t.Fatal("la lista da istantanea non e mai comparsa")
	}
	t.Logf("MISURA latenze simulate: Connect %v, lista fresca %v", connect, list)
	t.Logf("MISURA apertura cache (open+migrate+bind): %v", openCacheTime)
	t.Logf("MISURA prima lista da istantanea dall'avvio (cache compresa): %v", firstList)
	if d, ok := st.seen["CORPO-SNAP-2"]; ok {
		t.Logf("MISURA thread gia visto dal tasto invio: %v", d-time.Duration(openedAt.Load()))
		if d-time.Duration(openedAt.Load()) >= time.Second {
			t.Errorf("thread da istantanea oltre 1 s")
		}
	} else {
		t.Error("il thread da istantanea non e mai comparso")
	}
	if d, ok := st.seen["FRESCO-00"]; ok {
		t.Logf("MISURA lista fresca (Connect + lista): %v", d)
	}
	if firstList >= time.Second {
		t.Errorf("prima lista da istantanea = %v, voglio < 1 s", firstList)
	}
}
