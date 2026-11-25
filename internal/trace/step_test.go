package trace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// ultimoEvento legge l'ultimo evento scritto in buf (il primo e sempre la riga meta).
func ultimoEvento(t *testing.T, buf *bytes.Buffer) Event {
	t.Helper()
	ev := parseLines(t, buf.Bytes())
	return ev[len(ev)-1]
}

// Piano 12a, Task 4: un passo porta operazione, nome, ruolo e misura; l'operazione
// viene dal ctx (WithSpan per le chiamate allo store, WithOp per il Watch).
func TestEventoStepPortaOperazionePassoRuolo(t *testing.T) {
	var buf bytes.Buffer
	Install(New(&buf, ""))
	defer Uninstall()

	ctx, _ := WithSpan(context.Background(), "ListConversations")
	BeginStep(ctx, StepSelect).End("inbox", 12, 0, nil)
	if e := ultimoEvento(t, &buf); e.K != "step" || e.Op != "ListConversations" || e.Step != "select" || e.Role != "inbox" || e.N != 12 || e.Err {
		t.Errorf("evento step = %+v", e)
	}

	BeginStep(WithOp(context.Background(), "Watch"), StepSearch).End("", 0, 0, context.Canceled)
	if e := ultimoEvento(t, &buf); e.Op != "Watch" || e.Step != "search" || !e.Err {
		t.Errorf("passo del Watch = %+v", e)
	}
}

// Piano 12a, revisione del Task 4: gli eventi step portano solo Op, Step, Role,
// conteggi, byte e durate. L'errore entra solo come booleano, mai il suo testo
// (stessa regola di TestEventiCacheSoloCampiAmmessi).
func TestEventiStepSoloCampiAmmessi(t *testing.T) {
	var buf bytes.Buffer
	Install(New(&buf, ""))
	defer Uninstall()

	BeginStep(WithOp(context.Background(), "Connect"), StepLogin).End("", 0, 0, errors.New("login fallito per mario@example.com"))
	BeginStep(WithOp(context.Background(), "FetchMessage"), StepFetch).End("all", 1, 2048, nil)

	var passi []map[string]any
	for _, ln := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(ln), &m); err != nil {
			t.Fatal(err)
		}
		if m["k"] == "step" {
			passi = append(passi, m)
		}
	}
	if len(passi) != 2 {
		t.Fatalf("attesi 2 eventi step, ho %d: %s", len(passi), buf.String())
	}
	ammessi := map[string]bool{"t": true, "k": true, "op": true, "step": true, "dur_us": true, "n": true, "bytes": true, "role": true, "err": true}
	for _, m := range passi {
		for k := range m {
			if !ammessi[k] {
				t.Errorf("campo %q non ammesso nell'evento step: %v", k, m)
			}
		}
	}
	if m := passi[0]; m["op"] != "Connect" || m["step"] != StepLogin || m["err"] != true {
		t.Errorf("passo fallito sbagliato: %v", m)
	}
	if m := passi[1]; m["op"] != "FetchMessage" || m["step"] != StepFetch || m["role"] != "all" || m["n"] != float64(1) || m["bytes"] != float64(2048) {
		t.Errorf("passo del corpo sbagliato: %v", m)
	}
	if strings.Contains(buf.String(), "mario") {
		t.Error("il testo dell'errore e finito nella traccia")
	}
}

// Senza traccia installata nessun costo: WithSpan non tocca il ctx, e un passo aperto
// a traccia spenta non emette nulla nemmeno se End arriva dopo l'installazione.
func TestStepSenzaTracciaNonFaNulla(t *testing.T) {
	Uninstall()
	ctx, sp := WithSpan(context.Background(), "ListConversations")
	if sp != nil || OpFrom(ctx) != "" {
		t.Fatal("senza traccia WithSpan non deve toccare il ctx")
	}
	passo := BeginStep(ctx, StepFetch)

	var buf bytes.Buffer
	Install(New(&buf, ""))
	defer Uninstall()
	passo.End("inbox", 1, 10, nil)
	if ev := parseLines(t, buf.Bytes()); len(ev) != 1 || ev[0].K != "meta" {
		t.Errorf("un passo aperto a traccia spenta non deve emettere nulla, ho: %s", buf.String())
	}
}

// WithSpan porta nel ctx anche il nome dell'operazione, oltre allo Span del gate.
func TestWithSpanPortaIlNomeDellOperazione(t *testing.T) {
	var buf bytes.Buffer
	Install(New(&buf, ""))
	defer Uninstall()
	ctx, sp := WithSpan(context.Background(), "FetchMessage")
	if sp == nil || OpFrom(ctx) != "FetchMessage" {
		t.Fatalf("WithSpan: span=%v op=%q", sp, OpFrom(ctx))
	}
}
