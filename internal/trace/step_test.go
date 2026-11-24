package trace

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// ultimoEvento legge l'ultimo evento scritto in buf (il primo e sempre la riga meta).
func ultimoEvento(t *testing.T, buf *bytes.Buffer) Event {
	t.Helper()
	righe := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var e Event
	if err := json.Unmarshal([]byte(righe[len(righe)-1]), &e); err != nil {
		t.Fatalf("evento di traccia non valido: %v (%q)", err, buf.String())
	}
	return e
}

// Piano 12a, Task 4: un passo porta operazione, nome, ruolo e misura; l'operazione
// viene dal ctx (WithSpan per le chiamate allo store, WithOp per il Watch).
func TestStepEventCarriesOperationStepAndRole(t *testing.T) {
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

// Senza traccia installata nessun costo: WithSpan non tocca il ctx e il timer e vuoto.
func TestStepWithoutTraceIsNoOp(t *testing.T) {
	Uninstall()
	ctx, sp := WithSpan(context.Background(), "ListConversations")
	if sp != nil || OpFrom(ctx) != "" {
		t.Fatal("senza traccia WithSpan non deve toccare il ctx")
	}
	BeginStep(ctx, StepFetch).End("inbox", 1, 10, nil)
}

// WithSpan porta nel ctx anche il nome dell'operazione, oltre allo Span del gate.
func TestWithSpanNamesTheOperation(t *testing.T) {
	var buf bytes.Buffer
	Install(New(&buf, ""))
	defer Uninstall()
	ctx, sp := WithSpan(context.Background(), "FetchMessage")
	if sp == nil || OpFrom(ctx) != "FetchMessage" {
		t.Fatalf("WithSpan: span=%v op=%q", sp, OpFrom(ctx))
	}
}
