package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/theme"
	"github.com/andsigno82/gmail-tui/internal/ui/toast"
)

// recorder è uno store wrapper che registra le chiamate ListMessages/Search:
// il Mock espone solo metodi (risultati filtrati), il piano richiede un
// wrapper che registri le chiamate per gli assert sui parametri. Il campo Err
// inietta un errore SOLO su Search (per i test di feedback in status bar).
type recorder struct {
	mail.MailStore
	lists  []msg.Query
	pages  []mail.Page // Piano 7, Task 3: Page di ogni ListMessages (paginazione)
	search []msg.Query
	labels []labelCall // Task 7: ApplyLabel (il Mock è no-op, solo registrazione)
	err    error
}

// ListConversations registra le letture di lista (lists, pages) e le ricerche
// (search): l'app le distingue dal tetto di pagina searchLimit (Piano 8, Task 9).
func (r *recorder) ListConversations(ctx context.Context, q msg.Query, pg mail.Page) ([]mail.Conversation, error) {
	if pg.Limit == searchLimit {
		r.search = append(r.search, q)
		if r.err != nil {
			return nil, r.err
		}
		return r.MailStore.ListConversations(ctx, q, pg)
	}
	r.lists = append(r.lists, q)
	r.pages = append(r.pages, pg)
	return r.MailStore.ListConversations(ctx, q, pg)
}

// searchFixture costruisce il corpus di ricerca: due messaggi da bob, uno da
// carol (date crescenti per l'ordinamento data-desc del Mock).
func searchFixture() []*msg.Message {
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	mk := func(n int, from, subj string, unread bool) *msg.Message {
		return &msg.Message{
			Ref: msg.MessageRef{
				ID: msg.NewID("INBOX", uint32(n)), From: from, Subject: subj,
				Date: base.Add(time.Duration(n) * time.Hour), Unread: unread,
			},
		}
	}
	return []*msg.Message{
		mk(1, "bob@example.com", "prima di bob", true),
		mk(2, "carol@example.com", "da carol", false),
		mk(3, "bob@example.com", "seconda di bob", false),
	}
}

// searchModel costruisce il Model radice su uno store che registra le
// ricerche, già dimensionato e con il load iniziale consolidato (pending 0).
func searchModel(tb testing.TB, refs []msg.MessageRef) (Model, *recorder) {
	tb.Helper()
	rec := &recorder{MailStore: &mail.Mock{Msgs: searchFixture()}}
	th, err := theme.Load("default")
	if err != nil {
		tb.Fatalf("tema default: %v", err)
	}
	m := New(rec, noopSender{}, th)
	mm, _ := step(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	mm, _ = step(mm, msgsLoaded{refs: refs})
	return mm, rec
}

// typeKeys digita s un carattere alla volta nella vista attiva.
func typeKeys(m Model, s string) Model {
	for _, r := range s {
		next, _ := step(m, pressKey(string(r)))
		m = next
	}
	return m
}

// --- vista ricerca: focus e digitazione ------------------------------------

// TestSlashOpensSearchAndFocusesInput: `/` apre la vista ricerca, focalizza
// il textinput (cmd di blink non-nil) e la vista rende prompt, placeholder e
// hint dei tasti.
func TestSlashOpensSearchAndFocusesInput(t *testing.T) {
	m := newTestModel(t)
	mm, cmd := step(m, pressKey("/"))
	if mm.currentView() != viewSearch {
		t.Fatalf("vista dopo / = %v, voglio viewSearch", mm.currentView())
	}
	if !mm.search.Focused() {
		t.Error("textinput ricerca non focalizzato dopo /")
	}
	if cmd == nil {
		t.Error("cmd dopo / = nil, voglio la blink cmd del cursore")
	}
	body := mm.body()
	if !strings.Contains(body, "/ ") {
		t.Errorf("body ricerca = %q, voglio il prompt \"/ \"", body)
	}
	if !strings.Contains(body, "has:attachment") {
		t.Errorf("body ricerca = %q, voglio il placeholder con la sintassi", body)
	}
	if !strings.Contains(body, "invio cerca · esc annulla") {
		t.Errorf("body ricerca = %q, voglio l'hint dei tasti", body)
	}
}

// TestTypingInSearchGoesToInput: con la vista ricerca attiva i tasti fluiscono
// nel textinput e compongono la query.
func TestTypingInSearchGoesToInput(t *testing.T) {
	mm, _ := searchModel(t, nil)
	mm, _ = step(mm, pressKey("/"))
	mm = typeKeys(mm, "from:bob is:unread")
	if got := mm.search.Value(); got != "from:bob is:unread" {
		t.Errorf("query = %q, voglio %q", got, "from:bob is:unread")
	}
	if mm.currentView() != viewSearch {
		t.Errorf("vista dopo digitazione = %v, voglio search", mm.currentView())
	}
}

// TestQTypesIntoSearchInput: in ricerca "q" è testo della query, NON quit
// (solo enter/esc sono intercettati dalla radice).
func TestQTypesIntoSearchInput(t *testing.T) {
	mm, _ := searchModel(t, nil)
	mm, _ = step(mm, pressKey("/"))
	mm, cmd := step(mm, pressKey("q"))
	if mm.currentView() != viewSearch {
		t.Errorf("vista dopo q = %v, voglio search (q è testo qui)", mm.currentView())
	}
	if got := mm.search.Value(); got != "q" {
		t.Errorf("query dopo q = %q, voglio \"q\"", got)
	}
	if cmd != nil {
		if res := cmd(); isQuit(res) {
			t.Errorf("q in ricerca produce %T, voglio niente quit", res)
		}
	}
}

func isQuit(m tea.Msg) bool {
	_, ok := m.(tea.QuitMsg)
	return ok
}

// --- enter: la ricerca vera --------------------------------------------------

// TestSearchEnterQueriesStoreWithParsedQuery: enter con "from:bob" chiama
// MailStore.Search col Query parsato (assert sulla registrazione del wrapper,
// conferma sui risultati filtrati del Mock), torna all'inbox e al risultato
// aggiorna lista + status col conteggio.
func TestSearchEnterQueriesStoreWithParsedQuery(t *testing.T) {
	mm, rec := searchModel(t, nil)
	mm, _ = step(mm, pressKey("/"))
	mm = typeKeys(mm, "from:bob")

	mm, cmd := step(mm, pressKey("enter"))
	if mm.currentView() != viewInbox {
		t.Errorf("vista dopo enter = %v, voglio inbox (i risultati finiscono lì)", mm.currentView())
	}
	if mm.pending != 1 {
		t.Errorf("pending dopo enter = %d, voglio 1 (ricerca in flight)", mm.pending)
	}

	var loaded msgsLoaded
	for _, ms := range runCmd(cmd) {
		if l, ok := ms.(msgsLoaded); ok {
			loaded = l
		}
	}
	if loaded.query != "from:bob" {
		t.Errorf("msgsLoaded.query = %q, voglio \"from:bob\"", loaded.query)
	}
	if err := loaded.err; err != nil {
		t.Fatalf("msgsLoaded.err = %v, voglio nil", err)
	}
	if len(loaded.refs) != 2 {
		t.Fatalf("Search refs = %d, voglio 2 (solo bob — Mock ha filtrato)", len(loaded.refs))
	}
	if len(rec.search) != 1 {
		t.Fatalf("chiamate Search = %d, voglio 1", len(rec.search))
	}
	if want := (msg.Query{From: "bob"}); rec.search[0] != want {
		t.Errorf("Search chiamata con %+v, voglio %+v", rec.search[0], want)
	}

	mm, _ = step(mm, loaded)
	if got := mm.statusText(); got != `2 risultati per "from:bob"` {
		t.Errorf("status = %q, voglio %q", got, `2 risultati per "from:bob"`)
	}
	if len(mm.refs) != 2 {
		t.Errorf("refs inbox = %d, voglio 2", len(mm.refs))
	}
	body := mm.body()
	if !strings.Contains(body, "seconda di bob") || !strings.Contains(body, "prima di bob") {
		t.Errorf("body inbox = %q, voglio i messaggi di bob", body)
	}
	if strings.Contains(body, "da carol") {
		t.Errorf("body inbox = %q, carol non deve comparire", body)
	}
	if mm.pending != 0 {
		t.Errorf("pending dopo risultato = %d, voglio 0", mm.pending)
	}
}

// TestSearchEmptyQueryRelistsAll: enter con input vuoto NON chiama Search ma
// re-lista tutto (ListMessages Query{}, come il load di Init); il badge di una
// ricerca precedente viene AZZERATO dal re-list (nessun badge stante).
func TestSearchEmptyQueryRelistsAll(t *testing.T) {
	mm, rec := searchModel(t, testRefs(t))

	// prima una ricerca vera col badge: il re-list successivo deve azzerarlo
	mm, _ = step(mm, pressKey("/"))
	mm = typeKeys(mm, "prima")
	mm, cmdSearch := step(mm, pressKey("enter"))
	var searched msgsLoaded
	for _, ms := range runCmd(cmdSearch) {
		if l, ok := ms.(msgsLoaded); ok {
			searched = l
		}
	}
	mm, _ = step(mm, searched)
	if got := mm.statusText(); got != `1 risultati per "prima"` {
		t.Fatalf("badge dopo ricerca = %q, voglio %q", got, `1 risultati per "prima"`)
	}

	mm, _ = step(mm, pressKey("/"))

	mm, cmd := step(mm, pressKey("enter"))
	if mm.currentView() != viewInbox {
		t.Errorf("vista dopo enter = %v, voglio inbox", mm.currentView())
	}

	var loaded msgsLoaded
	for _, ms := range runCmd(cmd) {
		if l, ok := ms.(msgsLoaded); ok {
			loaded = l
		}
	}
	if loaded.query != "" {
		t.Errorf("msgsLoaded.query = %q, voglio vuoto (re-list, non ricerca)", loaded.query)
	}
	if len(loaded.refs) != 3 {
		t.Errorf("refs re-list = %d, voglio 3 (tutti i messaggi del Mock)", len(loaded.refs))
	}
	if len(rec.search) != 1 {
		t.Errorf("chiamate Search = %d, voglio 1 (solo la ricerca con badge)", len(rec.search))
	}
	if len(rec.lists) != 1 || rec.lists[0] != (msg.Query{}) {
		t.Errorf("ListMessages = %+v, voglio [Query{}] (re-list completo)", rec.lists)
	}

	mm, _ = step(mm, loaded)
	if mm.status.text != "" {
		t.Errorf("status dopo re-list = %q, voglio vuoto (badge di ricerca azzerato)", mm.status.text)
	}
	if len(mm.refs) != 3 {
		t.Errorf("refs inbox = %d, voglio 3", len(mm.refs))
	}
	if body := mm.body(); !strings.Contains(body, "da carol") {
		t.Errorf("body inbox = %q, voglio il re-list completo (anche carol)", body)
	}
}

// TestSubmitQueryWhitespaceOnlyRelists: query di soli spazi = vuota →
// re-list, e il Query trasmesso a Search è il risultato di ParseQuery.
func TestSubmitQueryWhitespaceOnlyRelists(t *testing.T) {
	rec := &recorder{MailStore: &mail.Mock{}}
	for _, raw := range []string{"", "   "} {
		out := runCmd(submitQuery(rec, raw, "", 0)) // Piano 7, Task 4: cartella di default = INBOX
		if len(out) != 1 {
			t.Fatalf("submitQuery(%q): msg = %d, voglio 1", raw, len(out))
		}
		loaded, ok := out[0].(msgsLoaded)
		if !ok {
			t.Fatalf("submitQuery(%q): msg = %T, voglio msgsLoaded", raw, out[0])
		}
		if loaded.query != "" || loaded.err != nil {
			t.Errorf("submitQuery(%q) = %+v, voglio re-list senza query", raw, loaded)
		}
	}
	if len(rec.lists) != 2 || len(rec.search) != 0 {
		t.Errorf("chiamate: lists=%d search=%d, voglio lists=2 search=0", len(rec.lists), len(rec.search))
	}

	out := runCmd(submitQuery(rec, "from:bob is:unread", "", 0))
	loaded := out[0].(msgsLoaded)
	if loaded.query != "from:bob is:unread" {
		t.Errorf("query riportata = %q, voglio la stringa digitata", loaded.query)
	}
	if want := (msg.Query{From: "bob", Unread: true}); rec.search[0] != want {
		t.Errorf("Search chiamata con %+v, voglio %+v (ParseQuery)", rec.search[0], want)
	}
}

// --- esc: annulla senza cercare ----------------------------------------------

// TestSearchEscCancelsWithoutSearch: esc torna all'inbox SENZA chiamare
// Search, la lista resta com'era e la bozza della query sopravvive.
func TestSearchEscCancelsWithoutSearch(t *testing.T) {
	mm, rec := searchModel(t, testRefs(t))
	mm, _ = step(mm, pressKey("/"))
	mm = typeKeys(mm, "from:bob")

	mm, cmd := step(mm, pressKey("esc"))
	if cmd != nil {
		t.Errorf("esc in ricerca: cmd = %v, voglio nil (nessuna ricerca)", cmd)
	}
	if mm.currentView() != viewInbox {
		t.Errorf("vista dopo esc = %v, voglio inbox", mm.currentView())
	}
	if len(rec.search) != 0 {
		t.Errorf("chiamate Search dopo esc = %d, voglio 0", len(rec.search))
	}
	if mm.search.Focused() {
		t.Error("textinput ancora focalizzato dopo esc")
	}
	if len(mm.refs) != 2 {
		t.Errorf("refs inbox dopo esc = %d, voglio 2 (lista invariata)", len(mm.refs))
	}

	// La bozza resta: un nuovo / ripropone la query non inviata.
	mm, _ = step(mm, pressKey("/"))
	if got := mm.search.Value(); got != "from:bob" {
		t.Errorf("bozza dopo esc e nuovo / = %q, voglio \"from:bob\"", got)
	}
}

// --- errori ------------------------------------------------------------------

// TestSearchErrorShowsStatus: una ricerca fallita finisce in un toast d'errore
// (errore) con la query, chiude l'op in flight e NON tocca la lista.
func TestSearchErrorShowsStatus(t *testing.T) {
	rec := &recorder{MailStore: &mail.Mock{Msgs: searchFixture()}, err: errors.New("rete giù")}
	th, err := theme.Load("default")
	if err != nil {
		t.Fatalf("tema default: %v", err)
	}
	m := New(rec, noopSender{}, th)
	mm, _ := step(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	mm, _ = step(mm, msgsLoaded{}) // consolida il load iniziale (pending 0)
	mm, _ = step(mm, pressKey("/"))
	mm = typeKeys(mm, "from:bob")
	mm, cmd := step(mm, pressKey("enter"))

	var loaded msgsLoaded
	for _, ms := range runCmd(cmd) {
		if l, ok := ms.(msgsLoaded); ok {
			loaded = l
		}
	}
	if loaded.err == nil || loaded.err.Error() != "rete giù" {
		t.Fatalf("msgsLoaded.err = %v, voglio \"rete giù\"", loaded.err)
	}

	mm, cmd2 := step(mm, loaded)
	if cmd2 != nil {
		t.Errorf("cmd dopo errore ricerca = %v, voglio nil", cmd2)
	}
	if !toastIs(mm, toast.Error) {
		t.Error("toast non di errore, voglio true per un errore di ricerca")
	}
	if got := toastText(mm); !strings.Contains(got, "ricerca") || !strings.Contains(got, "rete giù") {
		t.Errorf("status = %q, voglio \"ricerca … rete giù\"", got)
	}
	if mm.pending != 0 {
		t.Errorf("pending = %d, voglio 0 dopo il risultato", mm.pending)
	}
}
