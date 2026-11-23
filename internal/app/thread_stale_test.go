package app

import (
	"strings"
	"testing"
)

// TestOpeningNextMessageNeverShowsThePrevious riproduce la segnalazione di Andrea
// (2026-11-23): A aperto (o in anteprima), poi si apre B prima che sia caricato.
// Finché B non arriva la vista non deve mostrare A e le azioni (r, #, a…) non
// devono avere come bersaglio A: il thread è condiviso con l'anteprima.
func TestOpeningNextMessageNeverShowsThePrevious(t *testing.T) {
	refs := testRefs(t)
	a, b := refs[0], refs[1]
	m, _ := newTestModelMock(t, fullMessage(a), fullMessage(b))
	mm := openThread(t, m, a)
	if !strings.Contains(mm.body(), a.Subject) {
		t.Fatalf("A aperto ma il corpo non lo mostra:\n%s", mm.body())
	}

	// Apertura di B: la richiesta parte, il risultato non è ancora arrivato.
	mm, _ = step(mm, inboxSelected(b))
	if strings.Contains(mm.body(), a.Subject) {
		t.Errorf("in attesa di B la vista mostra ancora A:\n%s", mm.body())
	}
	if !strings.Contains(mm.body(), "Caricamento") {
		t.Errorf("in attesa di B manca il caricamento:\n%s", mm.body())
	}
	if ref, ok := mm.currentRef(); ok {
		t.Errorf("in attesa di B le azioni hanno come bersaglio %v", ref.ID)
	}
}
