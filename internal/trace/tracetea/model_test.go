package tracetea

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/trace"
)

// fake e un modello minimo: Update e View impiegano un tempo noto.
type fake struct {
	content string
	screen  string
}

type connResult struct{} // stessi nomi dei messaggi dell'app: la fase si riconosce dal tipo
type msgsLoaded struct{}
type customMsg struct{ secret string }

func (f fake) Init() tea.Cmd { return nil }
func (f fake) Update(m tea.Msg) (tea.Model, tea.Cmd) {
	time.Sleep(2 * time.Millisecond)
	return f, nil
}
func (f fake) View() tea.View {
	time.Sleep(time.Millisecond)
	return tea.NewView(f.content)
}
func (f fake) TraceScreen() string    { return f.screen }
func (f fake) ExitMessages() []string { return []string{"ok"} }

func evs(t *testing.T, buf *bytes.Buffer, kind string) []trace.Event {
	t.Helper()
	var out []trace.Event
	for _, ln := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var e trace.Event
		if err := json.Unmarshal([]byte(ln), &e); err != nil {
			t.Fatal(err)
		}
		if e.K == kind {
			out = append(out, e)
		}
	}
	return out
}

func TestSenzaTracciaNonAvvolge(t *testing.T) {
	trace.Uninstall()
	f := fake{}
	if Wrap(f) != tea.Model(f) {
		t.Error("senza traccia Wrap deve ritornare il modello cosi com'e")
	}
}

func TestUnEventoPerMessaggioConUpdateViewEByte(t *testing.T) {
	var buf bytes.Buffer
	trace.Install(trace.New(&buf, ""))
	defer trace.Uninstall()

	m := Wrap(fake{content: strings.Repeat("x", 1234), screen: "inbox"})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	_ = m.View()

	ms := evs(t, &buf, "msg")
	if len(ms) != 1 {
		t.Fatalf("atteso 1 evento msg, ho %d: %s", len(ms), buf.String())
	}
	e := ms[0]
	if e.Op != "tea.KeyPressMsg" || e.Key != "down" || e.Screen != "inbox" {
		t.Errorf("evento sbagliato: %+v", e)
	}
	if e.Upd < 2000 || e.View < 1000 || e.ViewBytes != 1234 {
		t.Errorf("misure sbagliate: %+v", e)
	}
}

func TestUpdateSenzaViewVieneComunqueScritto(t *testing.T) {
	var buf bytes.Buffer
	trace.Install(trace.New(&buf, ""))
	defer trace.Uninstall()

	m := Wrap(fake{})
	m, _ = m.Update(customMsg{})
	m, _ = m.Update(customMsg{})
	_ = m.View()

	ms := evs(t, &buf, "msg")
	if len(ms) != 2 || ms[0].View != 0 || ms[1].View == 0 {
		t.Fatalf("attesi 2 eventi (il primo senza View): %+v", ms)
	}
}

func TestTastiMaiIlTestoDigitato(t *testing.T) {
	var buf bytes.Buffer
	trace.Install(trace.New(&buf, ""))
	defer trace.Uninstall()

	m := Wrap(fake{})
	for _, r := range "mario@example.com" {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		_ = m.View()
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'x'}) // protocollo Kitty: codice stampabile senza Text
	m, _ = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = m.Update(customMsg{secret: "oggetto-riservato"})
	_ = m

	out := buf.String()
	for _, secret := range []string{"mario", "example", "riservato", "@"} {
		if strings.Contains(out, secret) {
			t.Errorf("la traccia contiene %q", secret)
		}
	}
	keys := map[string]int{}
	for _, e := range evs(t, &buf, "msg") {
		keys[e.Key]++
	}
	if keys["char"] != len("mario@example.com")+1 || keys["ctrl+c"] != 1 || keys["enter"] != 1 {
		t.Errorf("chiavi inattese: %v", keys)
	}
}

func TestFasiDAvvio(t *testing.T) {
	var buf bytes.Buffer
	trace.Install(trace.New(&buf, ""))
	defer trace.Uninstall()

	m := Wrap(fake{})
	_ = m.View() // prima View (splash)
	m, _ = m.Update(connResult{})
	_ = m.View()
	m, _ = m.Update(msgsLoaded{})
	_ = m.View()                  // prima View con la lista
	m, _ = m.Update(msgsLoaded{}) // le successive non ripetono le fasi
	_ = m.View()

	var ph []string
	for _, e := range evs(t, &buf, "phase") {
		ph = append(ph, e.Op)
	}
	want := []string{"first_view", "connected", "list_loaded", "first_list_view"}
	if strings.Join(ph, ",") != strings.Join(want, ",") {
		t.Errorf("fasi = %v, attese %v", ph, want)
	}
}

func TestExitMessagesInoltrato(t *testing.T) {
	var buf bytes.Buffer
	trace.Install(trace.New(&buf, ""))
	defer trace.Uninstall()
	w := Wrap(fake{})
	em, ok := w.(interface{ ExitMessages() []string })
	if !ok || len(em.ExitMessages()) != 1 {
		t.Error("ExitMessages deve passare attraverso il wrapper")
	}
}
