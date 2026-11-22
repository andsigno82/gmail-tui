package app

// Bugfix TDD 2026-11-22 (segnalato dall'utente con credenziali reali):
// beginSession lanciava loadRefs/Watch senza MAI chiamare store.Connect.
// Con il Mock Connect è un no-op, quindi la suite non lo vedeva; con lo
// store IMAP reale il primo load falliva con
// "imap store non connesso: chiamare Connect prima di questa operazione".
// Il contratto di mail.MailStore richiede Connect prima delle letture.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/theme"
)

// requiresConnectStore incapsula un Mock e replica il contratto dello store
// reale: le letture falliscono finché Connect non è stato chiamato.
type requiresConnectStore struct {
	*mail.Mock
	mu        sync.Mutex
	connected bool
}

func (s *requiresConnectStore) Connect(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connected = true
	return s.Mock.Connect(ctx)
}

func (s *requiresConnectStore) ListMessages(ctx context.Context, q msg.Query, pg mail.Page) ([]msg.MessageRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.connected {
		return nil, errors.New("imap store non connesso: chiamare Connect prima di questa operazione")
	}
	return s.Mock.ListMessages(ctx, q, pg)
}

// flakyConnectStore fallisce i primi n Connect, poi riesce (per il retry).
type flakyConnectStore struct {
	requiresConnectStore
	failures int
}

func (s *flakyConnectStore) Connect(ctx context.Context) error {
	s.mu.Lock()
	if s.failures > 0 {
		s.failures--
		s.mu.Unlock()
		return errors.New("boom rete")
	}
	s.mu.Unlock()
	return s.requiresConnectStore.Connect(ctx)
}

func (s *flakyConnectStore) connectedNow() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connected
}

func seedOne(m *mail.Mock) {
	m.Msgs = append(m.Msgs, &msg.Message{Ref: msg.MessageRef{
		ID: msg.NewID("INBOX", 1), From: "ada@example.com", Subject: "benvenuta",
		Date: time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC),
	}})
}

// Il percorso atteso: Init → connectCmd → connResult(ok) → loadRefs →
// msgsLoaded SENZA errore, con lo store effettivamente connesso.
func TestInitConnectsStoreBeforeFirstLoad(t *testing.T) {
	mock := &mail.Mock{}
	seedOne(mock)
	store := &requiresConnectStore{Mock: mock}
	th, err := theme.Load("default")
	if err != nil {
		t.Fatalf("tema default: %v", err)
	}
	m := New(store, &mail.Mock{}, th)

	var connRes tea.Msg
	for _, msg := range runCmd(m.Init()) { // Init → batch {connectCmd, tick}
		if cr, ok := msg.(connResult); ok {
			connRes = cr
		}
	}
	if connRes == nil {
		t.Fatal("Init non emette connectCmd: il load parte senza Connect")
	}
	var c2 tea.Cmd
	_, c2 = step(m, connRes)
	if c2 == nil {
		t.Fatal("dopo connResult ok atteso il batch di load/watch")
	}
	var loadedErr error
	var refs int
	for _, msg := range runCmd(c2) {
		if ml, ok := msg.(msgsLoaded); ok {
			loadedErr, refs = ml.err, len(ml.refs)
		}
	}
	if loadedErr != nil {
		t.Fatalf("msgsLoaded err = %v (il connect deve precedere il load)", loadedErr)
	}
	if refs != 1 {
		t.Fatalf("refs = %d, atteso 1 dal seed", refs)
	}
	if !store.connected {
		t.Fatal("Connect non è mai stato chiamato sullo store")
	}
}

// Connect fallito → status d'errore con l'invito a R e NESSUN load/watch;
// il tasto R rilancia la sequenza, che stavolta riesce.
func TestConnectFailureShowsStatusAndRRetries(t *testing.T) {
	mock := &mail.Mock{}
	seedOne(mock)
	store := &flakyConnectStore{requiresConnectStore: requiresConnectStore{Mock: mock}, failures: 1}
	th, err := theme.Load("default")
	if err != nil {
		t.Fatalf("tema default: %v", err)
	}
	m := New(store, &mail.Mock{}, th)

	var connRes tea.Msg
	for _, msg := range runCmd(m.Init()) {
		if cr, ok := msg.(connResult); ok {
			connRes = cr
		}
	}
	if connRes == nil {
		t.Fatal("Init non emette connectCmd")
	}
	var c2 tea.Cmd
	m, c2 = step(m, connRes)
	if c2 != nil {
		t.Fatal("dopo un connect fallito non deve partire load/watch")
	}
	if st := toastText(m); !strings.Contains(st, "connessione fallita") || !strings.Contains(st, "boom rete") {
		t.Fatalf("status = %q, atteso 'connessione fallita: boom rete'", st)
	}
	if !m.connectFailed {
		t.Fatal("connectFailed deve essere true dopo un fallimento")
	}

	// Retry con R: Connect stavolta riesce → load → msgsLoaded pulito.
	// Il flag si spegne al dispatch e pending risale (spinner): un secondo
	// R durante il retry non lancia Connect concorrenti.
	var c3 tea.Cmd
	m, c3 = step(m, pressKey("R"))
	if c3 == nil {
		t.Fatal("R non rilancia la sequenza di connessione")
	}
	if m.connectFailed {
		t.Fatal("connectFailed deve spegnersi al dispatch del retry")
	}
	if m.pending != 1 {
		t.Fatalf("pending dopo R = %d, voglio 1 (connect in flight)", m.pending)
	}
	var connRes2 tea.Msg
	for _, msg := range runCmd(c3) {
		if cr, ok := msg.(connResult); ok {
			connRes2 = cr
		}
	}
	if connRes2 == nil {
		t.Fatal("il retry non emette connectCmd")
	}
	var c4 tea.Cmd
	m, c4 = step(m, connRes2)
	if c4 == nil {
		t.Fatal("dopo il retry riuscito atteso il batch di load/watch")
	}
	var loadedErr error
	for _, msg := range runCmd(c4) {
		if ml, ok := msg.(msgsLoaded); ok {
			loadedErr = ml.err
		}
	}
	if loadedErr != nil {
		t.Fatalf("msgsLoaded dopo retry: %v", loadedErr)
	}
	if !store.connectedNow() {
		t.Fatal("il retry non ha connesso lo store")
	}
	if m.connectFailed {
		t.Fatal("connectFailed deve essere false dopo il retry riuscito")
	}
}

// startSession esegue l'avvio completo del modello (Init → connect →
// load/watch) restituendo i messaggi. La catena è seguita ESPPLICITAMENTE:
// connResult e load/watch batch; i cmd di secondo livello NON vengono
// eseguiti (waitForChange blocca sul canale del feed — il feed resta armato
// fuori dai test di sessione). I tick dello spinner non vengono consegnati
// (come in deliver: non ri-armano la catena).
func startSession(t *testing.T, m Model) (Model, []tea.Msg) {
	t.Helper()
	var got []tea.Msg
	for _, msg := range runCmd(m.Init()) {
		got = append(got, msg)
		if cr, ok := msg.(connResult); ok {
			var c tea.Cmd
			m, c = step(m, cr)
			if c != nil {
				got = append(got, runCmd(c)...)
			}
		}
	}
	return m, got
}
