package thread

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/trace"
)

func bodyEvents(t *testing.T, buf *bytes.Buffer) []trace.Event {
	t.Helper()
	var out []trace.Event
	for _, ln := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var e trace.Event
		if err := json.Unmarshal([]byte(ln), &e); err != nil {
			t.Fatal(err)
		}
		if e.K == "body" {
			out = append(out, e)
		}
	}
	return out
}

// Piano 16 (traccia): il rendering del corpo emette FromHTML, render e build con
// durata e byte del sorgente; "sync" nel loop, "async" nella Cmd. Nessun testo
// del messaggio finisce nella traccia.
func TestTracciaRenderingCorpoSincrono(t *testing.T) {
	var buf bytes.Buffer
	trace.Install(trace.New(&buf, ""))
	defer trace.Uninstall()

	html := "<p>contenuto-riservato <a href=\"https://example.com/x\">link</a></p>"
	_ = buildBody(&msg.Message{BodyHTML: html, BodyText: "x"}, 80, testStyles(t))

	ops := map[string]trace.Event{}
	for _, e := range bodyEvents(t, &buf) {
		ops[e.Op] = e
		if e.Mode != "sync" {
			t.Errorf("%s: mode %q, atteso sync", e.Op, e.Mode)
		}
	}
	for _, op := range []string{"FromHTML", "render", "build"} {
		if _, ok := ops[op]; !ok {
			t.Fatalf("manca l'evento body %s: %s", op, buf.String())
		}
	}
	if ops["FromHTML"].Bytes != len(html) || ops["FromHTML"].N < 1 {
		t.Errorf("FromHTML: %+v", ops["FromHTML"])
	}
	for _, secret := range []string{"riservato", "example.com"} {
		if strings.Contains(buf.String(), secret) {
			t.Errorf("la traccia contiene %q", secret)
		}
	}
}

func TestTracciaRenderingCorpoAsincrono(t *testing.T) {
	var buf bytes.Buffer
	trace.Install(trace.New(&buf, ""))
	defer trace.Uninstall()

	m := bigModel(t, bigHTML())
	_ = m.View()
	if got := len(bodyEvents(t, &buf)); got != 0 {
		t.Fatalf("il fallback non deve emettere eventi body, ho %d", got)
	}
	_ = rendered(t, m.RenderCmd()) // esegue la Cmd in background
	ev := bodyEvents(t, &buf)
	if len(ev) == 0 {
		t.Fatal("nessun evento body dal rendering in background")
	}
	for _, e := range ev {
		if e.Mode != "async" {
			t.Errorf("%s: mode %q, atteso async", e.Op, e.Mode)
		}
	}
}

func TestSenzaTracciaNessunEventoBody(t *testing.T) {
	trace.Uninstall()
	_ = buildBody(&msg.Message{BodyHTML: "<p>x</p>"}, 80, testStyles(t)) // nessun panic, nessun costo
}
