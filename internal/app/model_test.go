package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"gmail-tui/internal/mail"
	"gmail-tui/internal/msg"
	"gmail-tui/internal/theme"
	"gmail-tui/internal/ui/inbox"
	"gmail-tui/internal/ui/thread"
)

// newTestModel costruisce il Model radice contro il Mock e il tema default.
func newTestModel(tb testing.TB, msgs ...*msg.Message) Model {
	tb.Helper()
	th, err := theme.Load("default")
	if err != nil {
		tb.Fatalf("tema default: %v", err)
	}
	return New(&mail.Mock{Msgs: msgs}, th)
}

// newTestModelMock come newTestModel ma restituisce anche il Mock (per gli
// assert sulle chiamate store, es. auto MarkRead).
func newTestModelMock(tb testing.TB, msgs ...*msg.Message) (Model, *mail.Mock) {
	tb.Helper()
	th, err := theme.Load("default")
	if err != nil {
		tb.Fatalf("tema default: %v", err)
	}
	mock := &mail.Mock{Msgs: msgs}
	return New(mock, th), mock
}

// fullMessage costruisce un messaggio completo per il Mock: corpo lungo
// (per lo scroll), una riga citata, un allegato. ref decide Unread/star.
func fullMessage(ref msg.MessageRef) *msg.Message {
	return &msg.Message{
		Ref:      ref,
		To:       []string{"bob@example.com"},
		BodyText: strings.TrimSuffix(strings.Repeat("riga corpo\n", 40), "\n") + "\n> riga citata",
		Attachments: []msg.Attachment{
			{PartIndex: 0, Filename: "f.pdf", ContentType: "application/pdf", Size: 1536},
		},
	}
}

func testRefs(tb testing.TB) []msg.MessageRef {
	tb.Helper()
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	return []msg.MessageRef{
		{ID: msg.NewID("INBOX", 1), From: "a@x.dev", Subject: "prima", Date: base, Unread: true},
		{ID: msg.NewID("INBOX", 2), From: "b@x.dev", Subject: "seconda", Date: base.Add(time.Hour)},
	}
}

// step applica un msg al modello e restituisce il nuovo modello + cmd.
func step(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

// runCmd esegue una cmd e restituisce tutti i Msg prodotti, srotolando
// ricorsivamente i tea.BatchMsg (il runtime li esegue nello stesso modo).
func runCmd(c tea.Cmd) []tea.Msg {
	if c == nil {
		return nil
	}
	res := c()
	batch, ok := res.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{res}
	}
	var out []tea.Msg
	for _, c := range batch {
		out = append(out, runCmd(c)...)
	}
	return out
}

func pressKey(k string) tea.KeyPressMsg {
	switch k {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	default:
		return tea.KeyPressMsg{Code: rune(k[0]), Text: k}
	}
}

// --- New / Init -----------------------------------------------------------

func TestNewStartsInInbox(t *testing.T) {
	m := newTestModel(t)
	if got := m.currentView(); got != viewInbox {
		t.Errorf("vista iniziale = %v, voglio viewInbox", got)
	}
}

func TestInitEmitsLoadAndSpinner(t *testing.T) {
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	m := newTestModel(t, &msg.Message{
		Ref: msg.MessageRef{ID: msg.NewID("INBOX", 1), Subject: "prima", Date: base},
	})
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init deve emettere una Cmd non-nil (loadRefs)")
	}
	res := cmd()
	batch, ok := res.(tea.BatchMsg)
	if !ok {
		t.Fatalf("Init cmd = %T, voglio tea.BatchMsg", res)
	}
	var loaded bool
	var tick bool
	for _, c := range batch {
		switch msg := c().(type) {
		case msgsLoaded:
			loaded = true
			if msg.err != nil {
				t.Errorf("loadRefs err = %v, voglio nil", msg.err)
			}
			if len(msg.refs) != 1 || msg.refs[0].Subject != "prima" {
				t.Errorf("loadRefs refs = %v, voglio il ref dal Mock", msg.refs)
			}
		case spinner.TickMsg:
			tick = true
		}
	}
	if !loaded {
		t.Error("Init non emette loadRefs (msgsLoaded)")
	}
	if !tick {
		t.Error("Init non avvia lo spinner")
	}
}

// --- Routing --------------------------------------------------------------

func TestQAndEscInInboxQuit(t *testing.T) {
	for _, key := range []string{"q", "esc"} {
		m := newTestModel(t)
		_, cmd := step(m, pressKey(key))
		if cmd == nil {
			t.Fatalf("%s in inbox: cmd nil, voglio quit", key)
		}
		res := cmd()
		if _, ok := res.(tea.QuitMsg); !ok {
			t.Errorf("%s in inbox: cmd = %T, voglio tea.QuitMsg", key, res)
		}
	}
}

func TestEscAndQInThreadReturnToInbox(t *testing.T) {
	// Task 5: esc/q nel thread sono delegati al modello thread, che risponde
	// con BackMsg; la radice torna all'inbox al messaggio (lista preservata).
	for _, key := range []string{"esc", "q"} {
		m := newTestModel(t)
		m.view = viewThread
		mm, cmd := step(m, pressKey(key))
		if cmd == nil {
			t.Fatalf("%s in thread: cmd nil, voglio BackMsg", key)
		}
		if _, ok := cmd().(thread.BackMsg); !ok {
			t.Fatalf("%s in thread: msg = %T, voglio BackMsg", key, cmd())
		}
		mm, cmd2 := step(mm, thread.BackMsg{})
		if cmd2 != nil {
			t.Errorf("%s: cmd dopo BackMsg = %v, voglio nil", key, cmd2)
		}
		if mm.currentView() != viewInbox {
			t.Errorf("%s in thread: dopo BackMsg vista = %v, voglio inbox", key, mm.currentView())
		}
	}
}

func TestSlashOpensSearchFromInbox(t *testing.T) {
	m := newTestModel(t)
	mm, _ := step(m, pressKey("/"))
	if mm.currentView() != viewSearch {
		t.Errorf("vista dopo / = %v, voglio viewSearch", mm.currentView())
	}
}

func TestEscInSearchReturnsToInbox(t *testing.T) {
	m := newTestModel(t)
	m.view = viewSearch
	mm, cmd := step(m, pressKey("esc"))
	if cmd != nil {
		t.Errorf("esc in search: cmd = %v, voglio nil (la ricerca vera è Task 6)", cmd)
	}
	if mm.currentView() != viewInbox {
		t.Errorf("vista dopo esc in search = %v, voglio inbox", mm.currentView())
	}
}

func TestCShowsComposerPlaceholder(t *testing.T) {
	m := newTestModel(t)
	mm, _ := step(m, pressKey("c"))
	if got := mm.statusText(); got != "composer in Piano 4" {
		t.Errorf("status dopo c = %q, voglio \"composer in Piano 4\"", got)
	}
}

// --- msgsLoaded / pending / spinner ---------------------------------------

func TestMsgsLoadedStoresRefs(t *testing.T) {
	m := newTestModel(t)
	refs := testRefs(t)
	mm, _ := step(m, msgsLoaded{refs: refs})
	if len(mm.refs) != 2 || mm.refs[0].ID != refs[0].ID {
		t.Errorf("refs = %v, voglio %v", mm.refs, refs)
	}
}

func TestMsgsLoadedErrorShowsStatusErr(t *testing.T) {
	m := newTestModel(t)
	mm, _ := step(m, msgsLoaded{err: errors.New("boom")})
	if !mm.status.isErr {
		t.Error("status.isErr = false, voglio true per un errore di list")
	}
	if !strings.Contains(mm.statusText(), "boom") {
		t.Errorf("statusText = %q, voglio contenga \"boom\"", mm.statusText())
	}
	if mm.pending != 0 {
		t.Errorf("pending = %d, voglio 0 dopo il risultato", mm.pending)
	}
}

func TestPendingCounterAndSpinner(t *testing.T) {
	m := newTestModel(t)
	if m.pending != 1 {
		t.Fatalf("pending iniziale = %d, voglio 1 (load di Init in flight)", m.pending)
	}
	if !strings.Contains(m.View().Content, spinner.MiniDot.Frames[0]) {
		t.Error("lo spinner deve comparire in View quando pending > 0")
	}

	mm, cmd := step(m, spinner.TickMsg{})
	if cmd == nil {
		t.Error("con pending > 0 il tick dello spinner deve ri-armarsi")
	}
	if !strings.Contains(mm.View().Content, spinner.MiniDot.Frames[1]) {
		t.Errorf("frame spinner dopo tick = %q, voglio %q", mm.spinner.View(), spinner.MiniDot.Frames[1])
	}

	mm, _ = step(mm, msgsLoaded{refs: testRefs(t)})
	if mm.pending != 0 {
		t.Errorf("pending dopo msgsLoaded = %d, voglio 0", mm.pending)
	}
	if strings.Contains(mm.View().Content, spinner.MiniDot.Frames[0]) {
		t.Error("lo spinner non deve comparire quando pending = 0")
	}

	_, cmd = step(mm, spinner.TickMsg{})
	if cmd != nil {
		t.Error("con pending = 0 il tick dello spinner non deve ri-armarsi")
	}
}

// --- status bar ------------------------------------------------------------

func TestStatusBarTextChanges(t *testing.T) {
	m := newTestModel(t)
	mm, _ := step(m, statusMsg{text: "demo mode"})
	if got := mm.statusText(); got != "demo mode" {
		t.Errorf("statusText = %q, voglio \"demo mode\"", got)
	}
	// il prossimo evento sostituisce lo status (timeout logico)
	mm, _ = step(mm, statusMsg{text: "fatto"})
	if got := mm.statusText(); got != "fatto" {
		t.Errorf("statusText = %q, voglio \"fatto\"", got)
	}
}

func TestOpDoneFeedback(t *testing.T) {
	m := newTestModel(t)
	mm, _ := step(m, opDone{label: "archiviata"})
	if got := mm.statusText(); got != "archiviata" {
		t.Errorf("statusText = %q, voglio \"archiviata\"", got)
	}
	mm, _ = step(mm, opDone{label: "archivia", err: errors.New("nope")})
	if !mm.status.isErr || !strings.Contains(mm.statusText(), "nope") {
		t.Errorf("statusText = %q (isErr=%v), voglio errore \"nope\"", mm.statusText(), mm.status.isErr)
	}
}

// --- inbox (Task 4) ---------------------------------------------------------

func TestMsgsLoadedUpdatesInbox(t *testing.T) {
	m := newTestModel(t)
	refs := testRefs(t)
	// Il terminale invia la size all'avvio: senza, la lista ha altezza 0 e
	// mostra una sola riga per pagina.
	mm, _ := step(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	mm, _ = step(mm, msgsLoaded{refs: refs})
	body := mm.body()
	i1, i2 := strings.Index(body, "prima"), strings.Index(body, "seconda")
	if i1 < 0 || i2 < 0 {
		t.Fatalf("body inbox = %q, voglio le righe dei ref", body)
	}
	if i1 > i2 {
		t.Errorf("ordine inbox alterato: prima=%d seconda=%d (ordine di arrivo deve restare)", i1, i2)
	}
}

// TestSelectedMsgFetchesThenBuildsThread: selezionare un messaggio passa alla
// thread view, incrementa pending (spinner) ed emette la Cmd di fetch; il
// risultato (msgLoaded) costruisce la thread view vera.
func TestSelectedMsgFetchesThenBuildsThread(t *testing.T) {
	ref := testRefs(t)[1] // già letto: qui niente markread
	m, _ := newTestModelMock(t, fullMessage(ref))
	mm, _ := step(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	mm, _ = step(mm, msgsLoaded{refs: testRefs(t)})

	mm, cmd := step(mm, inbox.SelectedMsg{Ref: ref})
	if mm.currentView() != viewThread {
		t.Errorf("vista dopo SelectedMsg = %v, voglio viewThread", mm.currentView())
	}
	if mm.pendingRef.ID != ref.ID {
		t.Errorf("pendingRef = %v, voglio %v", mm.pendingRef.ID, ref.ID)
	}
	if mm.pending != 1 {
		t.Errorf("pending dopo SelectedMsg = %d, voglio 1 (fetch in flight)", mm.pending)
	}
	if cmd == nil {
		t.Fatal("SelectedMsg: cmd nil, voglio fetchMsg")
	}
	if !strings.Contains(mm.body(), "Caricamento") {
		t.Errorf("body durante il fetch = %q, voglio il placeholder", mm.body())
	}

	// La cmd è Batch(fetch, spinner.Tick): si srotola come fa il runtime.
	var loaded msgLoaded
	found := false
	for _, ms := range runCmd(cmd) {
		if l, ok := ms.(msgLoaded); ok {
			loaded, found = l, true
		}
	}
	if !found {
		t.Fatalf("cmd di SelectedMsg: msg = %T dentro il batch, voglio msgLoaded", cmd())
	}
	if loaded.err != nil {
		t.Fatalf("msgLoaded.err = %v, voglio nil", loaded.err)
	}
	if loaded.m == nil || loaded.m.BodyText == "" {
		t.Errorf("msgLoaded.m = %v, voglio il messaggio dal Mock", loaded.m)
	}

	mm, cmd2 := step(mm, loaded)
	if cmd2 != nil {
		t.Errorf("msgLoaded di un messaggio letto: cmd = %v, voglio nil", cmd2)
	}
	if mm.pending != 0 {
		t.Errorf("pending dopo msgLoaded = %d, voglio 0", mm.pending)
	}
	body := mm.body()
	if !strings.Contains(body, ref.Subject) {
		t.Errorf("body thread = %q, voglio l'oggetto nell'header", body)
	}
	if !strings.Contains(body, "riga corpo") {
		t.Errorf("body thread = %q, voglio il corpo nella viewport", body)
	}
	if !strings.Contains(body, "📎 f.pdf (1.5 KiB)") {
		t.Errorf("body thread = %q, voglio l'allegato in coda", body)
	}
}

func TestEnterInInboxOpensSelected(t *testing.T) {
	m := newTestModel(t)
	refs := testRefs(t)
	mm, _ := step(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	mm, _ = step(mm, msgsLoaded{refs: refs})
	mm.inbox.Select(1)
	mm, cmd := step(mm, pressKey("enter"))
	if cmd == nil {
		t.Fatal("enter in inbox: cmd nil, voglio SelectedMsg dalla delega")
	}
	sel, ok := cmd().(inbox.SelectedMsg)
	if !ok {
		t.Fatalf("enter in inbox: msg = %T, voglio inbox.SelectedMsg", cmd())
	}
	if sel.Ref.ID != refs[1].ID {
		t.Errorf("SelectedMsg.Ref = %v, voglio la riga selezionata %v", sel.Ref.ID, refs[1].ID)
	}
	if mm.currentView() != viewInbox {
		t.Errorf("vista dopo enter = %v, voglio ancora inbox (il cambio avviene al msg)", mm.currentView())
	}
}

func TestMouseClickForwardedToInbox(t *testing.T) {
	m := newTestModel(t)
	refs := testRefs(t)
	mm, _ := step(m, msgsLoaded{refs: refs})
	mm, _ = step(mm, tea.WindowSizeMsg{Width: 100, Height: 30})
	mm, cmd := step(mm, tea.MouseClickMsg{Y: 1, Button: tea.MouseLeft})
	if cmd != nil {
		t.Errorf("singolo click: cmd = %v, voglio nil", cmd)
	}
	sel, ok := mm.inbox.SelectedRef()
	if !ok || sel.ID != refs[1].ID {
		t.Errorf("ref selezionato dopo click = %v (ok=%v), voglio la riga 1", sel.ID, ok)
	}
}

// TestMouseWheelScrollsThread: nel thread la rotella è delegata alla
// viewport, che scorre il corpo (cmd nil, vista invariata).
func TestMouseWheelScrollsThread(t *testing.T) {
	ref := testRefs(t)[0]
	m, _ := newTestModelMock(t, fullMessage(ref))
	mm, _ := step(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	mm, _ = step(mm, msgsLoaded{refs: testRefs(t)})
	mm, cmd := step(mm, inbox.SelectedMsg{Ref: ref})
	for _, ms := range runCmd(cmd) { // msgLoaded costruisce la thread view
		mm, _ = step(mm, ms)
	}

	mm, cmd = step(mm, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if cmd != nil {
		t.Errorf("wheel in thread: cmd = %v, voglio nil", cmd)
	}
	if mm.thread.YOffset() != 3 {
		t.Errorf("YOffset thread dopo wheel = %d, voglio 3", mm.thread.YOffset())
	}
	if mm.currentView() != viewThread {
		t.Errorf("vista dopo wheel = %v, voglio thread", mm.currentView())
	}
}

// TestMouseIgnoredInSearch: fuori da inbox e thread il mouse resta noop.
func TestMouseIgnoredInSearch(t *testing.T) {
	m := newTestModel(t)
	m.view = viewSearch
	mm, cmd := step(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if cmd != nil {
		t.Errorf("wheel in search: cmd = %v, voglio nil", cmd)
	}
	if mm.currentView() != viewSearch {
		t.Errorf("vista dopo wheel in search = %v, voglio search", mm.currentView())
	}
}

// TestMouseIgnoredUnderHelpOverlay: con l'overlay help aperto il mouse è
// ignorato come i tasti (fix merge-gate): un doppio click NON seleziona righe
// sotto l'overlay né apre messaggi invisibili dietro l'overlay.
func TestMouseIgnoredUnderHelpOverlay(t *testing.T) {
	m := newTestModel(t)
	refs := testRefs(t)
	mm, _ := step(m, msgsLoaded{refs: refs})
	mm, _ = step(mm, tea.WindowSizeMsg{Width: 100, Height: 30})
	mm, _ = step(mm, pressKey("?"))
	if !mm.showHelp {
		t.Fatal("l'overlay help non è aperto")
	}

	// doppio click sulla riga 1 (stesso Y entro la finestra del doppio click)
	mm, cmd1 := step(mm, tea.MouseClickMsg{Y: 1, Button: tea.MouseLeft})
	mm, cmd2 := step(mm, tea.MouseClickMsg{Y: 1, Button: tea.MouseLeft})
	if cmd1 != nil || cmd2 != nil {
		t.Errorf("click sotto l'help: cmd = %v / %v, voglio nil / nil", cmd1, cmd2)
	}
	if !mm.showHelp {
		t.Error("il click non deve chiudere l'overlay")
	}
	sel, ok := mm.inbox.SelectedRef()
	if !ok || sel.ID != refs[0].ID {
		t.Errorf("ref selezionato dopo il doppio click = %v (ok=%v), voglio %v (cursore immobile)",
			sel.ID, ok, refs[0].ID)
	}
	if mm.currentView() != viewInbox {
		t.Errorf("vista dopo il doppio click = %v, voglio inbox (nessuna apertura)", mm.currentView())
	}
}

// TestStaleFetchResultIgnoredWhenNewerOpenRendered: open A (fetch lento) →
// esc → open B (fetch veloce) → B si renderizza; quando il risultato di A
// arriva PER ULTIMO è scartato: nessun rebuild della thread (il corpo di A
// sotto l'header di B) e nessun auto MarkRead su A. L'ordine dei fetch è
// controllato dal test: le Cmd girano solo quando vengono eseguite (runCmd),
// quindi il test stesso è lo scheduler dell'interleaving (deterministico,
// senza goroutine né wrapper di gating).
func TestStaleFetchResultIgnoredWhenNewerOpenRendered(t *testing.T) {
	refs := testRefs(t) // [0] "prima" unread, [1] "seconda" read
	msgA := fullMessage(refs[0])
	msgA.BodyText = "corpo messaggio A lento\n"
	msgB := fullMessage(refs[1])
	msgB.BodyText = "corpo messaggio B veloce\n"
	mock := &mail.Mock{Msgs: []*msg.Message{msgA, msgB}}
	th, err := theme.Load("default")
	if err != nil {
		t.Fatalf("tema default: %v", err)
	}
	m := New(mock, th)
	mm, _ := step(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	mm, _ = step(mm, msgsLoaded{refs: refs})

	// open A: fetch "lento" — la cmd NON viene eseguita subito
	mm, cmdA := step(mm, inbox.SelectedMsg{Ref: refs[0]})
	// esc: back all'inbox col fetch ancora in volo
	mm, _ = step(mm, thread.BackMsg{})
	// open B: fetch "veloce"
	mm, cmdB := step(mm, inbox.SelectedMsg{Ref: refs[1]})

	// il fetch di B completa per primo: B si renderizza
	var loadedB msgLoaded
	for _, ms := range runCmd(cmdB) {
		if l, ok := ms.(msgLoaded); ok {
			loadedB = l
		}
	}
	mm, _ = step(mm, loadedB)
	if mm.thread.Ref().ID != refs[1].ID {
		t.Fatalf("thread dopo il fetch di B = %v, voglio %v", mm.thread.Ref().ID, refs[1].ID)
	}
	if body := mm.body(); !strings.Contains(body, "corpo messaggio B veloce") {
		t.Fatalf("body dopo il fetch di B = %q, voglio il corpo di B", body)
	}

	// il risultato di A arriva PER ULTIMO: deve essere ignorato
	var loadedA msgLoaded
	for _, ms := range runCmd(cmdA) {
		if l, ok := ms.(msgLoaded); ok {
			loadedA = l
		}
	}
	mm, cmdLate := step(mm, loadedA)
	if cmdLate != nil {
		t.Errorf("risultato stante di A: cmd = %v, voglio nil (niente auto MarkRead su A)", cmdLate)
	}
	if mm.thread.Ref().ID != refs[1].ID {
		t.Errorf("thread dopo il risultato stante = %v, voglio ancora %v (nessun rebuild)",
			mm.thread.Ref().ID, refs[1].ID)
	}
	if body := mm.body(); !strings.Contains(body, "corpo messaggio B veloce") ||
		strings.Contains(body, "corpo messaggio A lento") {
		t.Errorf("body dopo il risultato stante = %q, voglio ancora il corpo di B (non A)", body)
	}
	if mm.pending != 0 {
		t.Errorf("pending dopo entrambi i risultati = %d, voglio 0", mm.pending)
	}
}

// TestFetchResultAfterBackIgnored: esc durante il fetch — il risultato che
// arriva dopo NON riapre la thread: nessun auto MarkRead, nessuna thread view
// costruita, la vista resta inbox e lo status intatto.
func TestFetchResultAfterBackIgnored(t *testing.T) {
	ref := testRefs(t)[0] // unread: il risultato stante emetterebbe markRead
	mock := &mail.Mock{Msgs: []*msg.Message{fullMessage(ref)}}
	th, err := theme.Load("default")
	if err != nil {
		t.Fatalf("tema default: %v", err)
	}
	m := New(mock, th)
	mm, _ := step(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	mm, _ = step(mm, msgsLoaded{refs: testRefs(t)})

	mm, cmd := step(mm, inbox.SelectedMsg{Ref: ref})
	mm, _ = step(mm, thread.BackMsg{}) // esc mentre il fetch è in volo
	if mm.currentView() != viewInbox {
		t.Fatalf("vista dopo esc = %v, voglio inbox", mm.currentView())
	}

	var loaded msgLoaded
	for _, ms := range runCmd(cmd) {
		if l, ok := ms.(msgLoaded); ok {
			loaded = l
		}
	}
	mm, cmdLate := step(mm, loaded)
	if cmdLate != nil {
		t.Errorf("risultato dopo esc: cmd = %v, voglio nil (nessun MarkRead)", cmdLate)
	}
	if mm.currentView() != viewInbox {
		t.Errorf("vista dopo il risultato tardo = %v, voglio inbox", mm.currentView())
	}
	if mm.thread.Loaded() {
		t.Error("il risultato tardo non deve costruire la thread view")
	}
	if got := mm.statusText(); got != "" {
		t.Errorf("status = %q, voglio vuoto (risultato ignorato)", got)
	}
	if mm.pending != 0 {
		t.Errorf("pending = %d, voglio 0 (l'op in flight si chiude lo stesso)", mm.pending)
	}
}

// --- thread: auto MarkRead, errori, back, open ------------------------------

// TestSelectedMsgAutoMarkRead: il fetch di un messaggio NON letto emette la
// Cmd di MarkRead in cascata (errore silenzioso in status, non fatale); la
// copia locale del ref nel thread passa a letta e il Mock registra la chiamata.
func TestSelectedMsgAutoMarkRead(t *testing.T) {
	ref := testRefs(t)[0] // Unread: true
	mock := &mail.Mock{Msgs: []*msg.Message{fullMessage(ref)}}
	th, err := theme.Load("default")
	if err != nil {
		t.Fatalf("tema default: %v", err)
	}
	m := New(mock, th)
	mm, _ := step(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	mm, _ = step(mm, msgsLoaded{refs: testRefs(t)})
	mm, cmd := step(mm, inbox.SelectedMsg{Ref: ref})
	var loaded msgLoaded
	for _, ms := range runCmd(cmd) { // msgLoaded costruisce la thread view
		if l, ok := ms.(msgLoaded); ok {
			loaded = l
		}
	}
	mm, cmd2 := step(mm, loaded)

	if cmd2 == nil {
		t.Fatal("fetch di un messaggio non letto: cmd nil, voglio markRead")
	}
	// La copia locale del ref nel thread è già segnata letta (nessun re-list).
	if mm.thread.Ref().Unread {
		t.Error("copia locale del ref dopo il fetch: Unread = true, voglio false")
	}
	// La cmd concatenata chiama MarkRead sullo store e dà feedback in status.
	res := cmd2()
	od, ok := res.(opDone)
	if !ok {
		t.Fatalf("cmd di markread: msg = %T, voglio opDone", res)
	}
	if od.label != "letta" || od.err != nil {
		t.Errorf("opDone = %+v, voglio {letta nil}", od)
	}
	// Il Mock ha ricevuto la chiamata: il messaggio non è più unread.
	if mock.Msgs[0].Ref.Unread {
		t.Error("Mock dopo markRead: Unread = true, voglio false")
	}
	mm, _ = step(mm, od)
	if mm.statusText() != "letta" {
		t.Errorf("status dopo opDone = %q, voglio \"letta\"", mm.statusText())
	}
}

// TestMsgLoadedErrorShowsStatusErr: un fetch fallito finisce in status bar
// (isErr) e chiude l'op in flight, senza costruire thread view. Il fetch è in
// volo solo dopo SelectedMsg (vista thread): è lo stato che il handler si
// aspetta — un risultato con altra vista o altro ID è scartato come stante
// (fix merge-gate anti-race).
func TestMsgLoadedErrorShowsStatusErr(t *testing.T) {
	m := newTestModel(t)
	m.view = viewThread // fetch in flight (SelectedMsg ha già impostato la vista)
	mm, cmd := step(m, msgLoaded{err: errors.New("rete giù")})
	if cmd != nil {
		t.Errorf("cmd = %v, voglio nil", cmd)
	}
	if !mm.status.isErr || !strings.Contains(mm.statusText(), "rete giù") {
		t.Errorf("status = %q (isErr=%v), voglio l'errore del fetch", mm.statusText(), mm.status.isErr)
	}
	if mm.pending != 0 {
		t.Errorf("pending = %d, voglio 0 dopo il risultato", mm.pending)
	}
	if mm.thread.Loaded() {
		t.Error("a fetch fallito non deve esistere una thread view caricata")
	}
}

// TestBackMsgPreservesInboxList: il ritorno dal thread NON re-lista — la
// lista conserva ref, cursore e stato (refresh solo al back esplicito via
// eventi Watch, Task 8).
func TestBackMsgPreservesInboxList(t *testing.T) {
	m := newTestModel(t)
	mm, _ := step(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	mm, _ = step(mm, msgsLoaded{refs: testRefs(t)})
	mm.inbox.Select(1)
	mm.view = viewThread

	mm, cmd := step(mm, thread.BackMsg{})
	if cmd != nil {
		t.Errorf("cmd dopo BackMsg = %v, voglio nil (nessun re-list)", cmd)
	}
	if mm.currentView() != viewInbox {
		t.Errorf("vista dopo BackMsg = %v, voglio inbox", mm.currentView())
	}
	sel, ok := mm.inbox.SelectedRef()
	if !ok || sel.ID != testRefs(t)[1].ID {
		t.Errorf("ref selezionato dopo il back = %v (ok=%v), voglio %v (lista preservata)",
			sel.ID, ok, testRefs(t)[1].ID)
	}
}

// TestOpenRequestedWiresExecCmd: la radice risponde a OpenRequested con la
// Cmd di exec (tea.ExecProcess, NON eseguita nei test) e a OpenedMsg col
// feedback in status bar; un errore di preparazione finisce in status isErr.
func TestOpenRequestedWiresExecCmd(t *testing.T) {
	m := newTestModel(t)
	mm, cmd := step(m, thread.OpenRequested{Path: "/tmp/msg.html"})
	if cmd == nil {
		t.Fatal("OpenRequested: cmd nil, voglio la exec cmd del browser")
	}
	if mm.currentView() != viewInbox {
		t.Errorf("vista = %v, deve restare invariata", mm.currentView())
	}

	mm, cmd = step(m, thread.OpenRequested{Err: errors.New("disco pieno")})
	if cmd != nil {
		t.Errorf("OpenRequested err: cmd = %v, voglio nil", cmd)
	}
	if !mm.status.isErr || !strings.Contains(mm.statusText(), "disco pieno") {
		t.Errorf("status = %q (isErr=%v), voglio l'errore", mm.statusText(), mm.status.isErr)
	}

	mm, _ = step(m, thread.OpenedMsg{Err: nil})
	if mm.statusText() != "aperto nel browser" {
		t.Errorf("status = %q, voglio \"aperto nel browser\"", mm.statusText())
	}
	mm, _ = step(m, thread.OpenedMsg{Err: errors.New("open assente")})
	if !mm.status.isErr || !strings.Contains(mm.statusText(), "open assente") {
		t.Errorf("status = %q (isErr=%v), voglio l'errore del browser", mm.statusText(), mm.status.isErr)
	}
}

// --- WindowSize ------------------------------------------------------------

func TestWindowSizeMsgStoresSize(t *testing.T) {
	m := newTestModel(t)
	mm, _ := step(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if mm.width != 100 || mm.height != 30 {
		t.Errorf("size = %dx%d, voglio 100x30", mm.width, mm.height)
	}
	// la status bar occupa l'ultima riga del terminale
	content := mm.View().Content
	if got := strings.Count(content, "\n"); got != 29 {
		t.Errorf("righe renderizzate = %d, voglio 30 (altezza terminale)", got+1)
	}
}

// --- conformance -----------------------------------------------------------

func TestModelImplementsTeaModel(t *testing.T) {
	var _ tea.Model = Model{}
}
