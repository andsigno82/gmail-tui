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

func pressKey(k string) tea.KeyPressMsg {
	switch k {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
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
	for _, key := range []string{"esc", "q"} {
		m := newTestModel(t)
		m.view = viewThread
		mm, cmd := step(m, pressKey(key))
		if cmd != nil {
			t.Errorf("%s in thread: cmd = %v, voglio nil", key, cmd)
		}
		if mm.currentView() != viewInbox {
			t.Errorf("%s in thread: vista = %v, voglio inbox", key, mm.currentView())
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
