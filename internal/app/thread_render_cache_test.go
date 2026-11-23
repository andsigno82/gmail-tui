package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/ui/inbox"
	"github.com/andsigno82/gmail-tui/internal/ui/thread"
)

// Revisione Piano 15, punto 6: la cache di rendering vale tra messaggi. Riaprire
// un messaggio grande gia reso mostra subito il corpo, senza fallback ne nuovo
// rendering in background.
func TestRiaperturaCorpoGrandeUsaLaCacheCondivisa(t *testing.T) {
	ref := testRefs(t)[1]
	full := fullMessage(ref)
	full.BodyHTML = "<p>" + strings.Repeat("parola ", 6000) + "</p>"
	full.BodyText = "testo di riserva"
	m, _ := newTestModelMock(t, full)
	mm, _ := step(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	mm, _ = step(mm, msgsLoaded{refs: testRefs(t)})
	mm, _ = step(mm, inbox.SelectedMsg{Ref: ref})

	mm, cmd := step(mm, msgLoaded{m: full, requested: ref.ID})
	for _, ms := range runCmd(cmd) {
		if r, ok := ms.(thread.RenderedMsg); ok {
			mm, _ = step(mm, r)
		}
	}
	if strings.Contains(mm.thread.View(), "rendering in corso") {
		t.Fatal("primo rendering non arrivato")
	}

	// Riapertura: nuovo thread, stesso messaggio.
	mm, _ = step(mm, inbox.SelectedMsg{Ref: ref})
	mm, cmd = step(mm, msgLoaded{m: full, requested: ref.ID})
	if v := mm.thread.View(); strings.Contains(v, "rendering in corso") || !strings.Contains(v, "parola parola") {
		t.Errorf("riapertura senza cache condivisa:\n%s", v)
	}
	for _, ms := range runCmd(cmd) {
		if _, ok := ms.(thread.RenderedMsg); ok {
			t.Errorf("riapertura: rendering in background rilanciato")
		}
	}
}
