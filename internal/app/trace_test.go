package app

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/theme"
	"github.com/andsigno82/gmail-tui/internal/trace"
	"github.com/andsigno82/gmail-tui/internal/trace/tracestore"
	"github.com/andsigno82/gmail-tui/internal/trace/tracetea"
	"github.com/andsigno82/gmail-tui/internal/ui/wizard"
)

// pump esegue le Cmd con un tetto di tempo (le Cmd che bloccano, come il feed
// Watch, si abbandonano) e rialimenta il modello con i messaggi prodotti, come
// farebbe il runtime Bubble Tea. Ad ogni messaggio chiama anche View.
func pump(t *testing.T, m tea.Model, cmd tea.Cmd, depth int) tea.Model {
	t.Helper()
	if cmd == nil || depth > 6 {
		return m
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	var res tea.Msg
	select {
	case res = <-ch:
	case <-time.After(100 * time.Millisecond):
		return m
	}
	if b, ok := res.(tea.BatchMsg); ok {
		for _, c := range b {
			m = pump(t, m, c, depth+1)
		}
		return m
	}
	if res == nil {
		return m
	}
	next, c := m.Update(res)
	_ = next.View()
	return pump(t, next, c, depth+1)
}

// Piano 16 (traccia), PRIVACY: una sessione demo completa (avvio, lista, apertura
// di un messaggio, ricerca con un indirizzo digitato, cartelle, composer) produce
// una traccia che non contiene nessun indirizzo, oggetto, nome, corpo, label,
// cartella utente o URL dei dati demo, ne il testo digitato.
func TestTracciaDemoNonContieneDatiPersonali(t *testing.T) {
	var buf bytes.Buffer
	trace.Install(trace.New(&buf, "dev"))
	defer trace.Uninstall()

	th, err := theme.Load("default")
	if err != nil {
		t.Fatal(err)
	}
	mk := DemoStore()
	st := tracestore.Wrap(mk)
	m := New(st, tracestore.WrapSender(mk), th)
	m.SetAddressBook(nil, nil)
	w := tracetea.Wrap(m)

	_ = w.View() // il runtime rende subito la prima View (splash)
	w = pump(t, w, w.Init(), 0)
	for _, k := range []tea.Msg{
		tea.WindowSizeMsg{Width: 140, Height: 40},
		pressKey("j"), pressKey("j"),
		tea.KeyPressMsg{Code: tea.KeyEnter}, // apre il messaggio
	} {
		var c tea.Cmd
		w, c = w.Update(k)
		_ = w.View()
		w = pump(t, w, c, 0)
	}
	for _, k := range []tea.Msg{tea.KeyPressMsg{Code: tea.KeyEscape}, pressKey("/")} {
		var c tea.Cmd
		w, c = w.Update(k)
		_ = w.View()
		w = pump(t, w, c, 0)
	}
	for _, r := range "mario@budgeting.io" { // ricerca digitata: mai nella traccia
		var c tea.Cmd
		w, c = w.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		w = pump(t, w, c, 0)
	}
	for _, k := range []tea.Msg{tea.KeyPressMsg{Code: tea.KeyEnter}, pressKey("m"), tea.KeyPressMsg{Code: tea.KeyEscape}} {
		var c tea.Cmd
		w, c = w.Update(k)
		_ = w.View()
		w = pump(t, w, c, 0)
	}

	// Contenuti da non trovare: tutto cio che e dato utente del seed demo.
	var forbidden []string
	for _, mm := range mk.Msgs {
		forbidden = append(forbidden, mm.Ref.From, mm.Ref.Subject)
		forbidden = append(forbidden, mm.To...)
		forbidden = append(forbidden, mm.Ref.Labels...)
		for _, a := range mm.Attachments {
			forbidden = append(forbidden, a.Filename)
		}
		if f := strings.Fields(mm.BodyText); len(f) > 3 {
			forbidden = append(forbidden, strings.Join(f[:3], " "))
		}
	}
	boxes, _ := mk.ListMailboxes(t.Context())
	for _, b := range boxes {
		if b.Role == "" || strings.Contains(b.Name, "/") {
			forbidden = append(forbidden, b.Name) // label utente e cartelle localizzate
		}
	}
	forbidden = append(forbidden, "mario", "budgeting", "http", "example", "gmail-tui.dev", "@")
	out := strings.ToLower(buf.String())
	for _, f := range forbidden {
		if f != "" && strings.Contains(out, strings.ToLower(f)) {
			t.Errorf("la traccia contiene %q", f)
		}
	}

	// La traccia non e vuota e ha tutte le famiglie di eventi.
	kinds := map[string]int{}
	screens := map[string]bool{}
	ops := map[string]bool{}
	for _, ln := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var e trace.Event
		if err := json.Unmarshal([]byte(ln), &e); err != nil {
			t.Fatalf("riga non JSON: %v", err)
		}
		kinds[e.K]++
		if e.Screen != "" {
			screens[e.Screen] = true
		}
		if e.K == "store" {
			ops[e.Op] = true
		}
	}
	for _, k := range []string{"meta", "store", "msg", "phase", "body"} {
		if kinds[k] == 0 {
			t.Errorf("nessun evento %q: %v", k, kinds)
		}
	}
	for _, op := range []string{"Connect", "ListConversations", "FetchMessage", "ListMailboxes"} {
		if !ops[op] {
			t.Errorf("manca l'operazione store %s: %v", op, ops)
		}
	}
	for _, s := range []string{"inbox", "thread", "search"} {
		if !screens[s] {
			t.Errorf("manca la vista %q: %v", s, screens)
		}
	}
	var ph []string
	for _, ln := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var e trace.Event
		_ = json.Unmarshal([]byte(ln), &e)
		if e.K == "phase" {
			ph = append(ph, e.Op)
		}
	}
	if got, want := strings.Join(ph, ","), "first_view,connected,list_loaded,first_list_view"; got != want {
		t.Errorf("fasi = %s, attese %s", got, want)
	}
}

// Piano 16: il demo scelto dal wizard (`s`) e misurato come lo store del main: la
// prova in demo da first-run deve produrre eventi store.
func TestTracciaDemoDaWizardMisuraLoStore(t *testing.T) {
	var buf bytes.Buffer
	trace.Install(trace.New(&buf, ""))
	defer trace.Uninstall()

	m, _ := newWizardTestModel(t, wizard.Deps{})
	m, cmd := step(m, pressKey("s"))
	m, cmds := deliver(m, cmd)
	var w tea.Model = m
	for _, c := range cmds {
		w = pump(t, w, c, 0)
	}
	if !strings.Contains(buf.String(), `"op":"Connect"`) {
		t.Errorf("nessun evento store dal demo del wizard:\n%s", buf.String())
	}
}
