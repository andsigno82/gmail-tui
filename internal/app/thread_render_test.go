package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/ui/inbox"
	"github.com/andsigno82/gmail-tui/internal/ui/thread"
)

// Piano 15, Task 14 (D15-T5): un corpo grande si rende fuori dal loop; la radice
// accoda la Cmd del thread e gli inoltra il risultato (thread.RenderedMsg).
func TestCorpoGrandeSiRendeInBackgroundEArrivaAlThread(t *testing.T) {
	ref := testRefs(t)[1]
	full := fullMessage(ref)
	full.BodyHTML = "<p>" + strings.Repeat("parola ", 6000) + "</p>"
	full.BodyText = "testo di riserva"
	m, _ := newTestModelMock(t, full)
	mm, _ := step(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	mm, _ = step(mm, msgsLoaded{refs: testRefs(t)})
	mm, _ = step(mm, inbox.SelectedMsg{Ref: ref})

	mm, cmd := step(mm, msgLoaded{m: full, requested: ref.ID})
	if !strings.Contains(mm.thread.View(), "rendering in corso") {
		t.Fatalf("prima del risultato serve il fallback:\n%s", mm.thread.View())
	}
	var rendered *thread.RenderedMsg
	for _, ms := range runCmd(cmd) {
		if r, ok := ms.(thread.RenderedMsg); ok {
			rendered = &r
		}
	}
	if rendered == nil {
		t.Fatal("la Cmd di apertura non contiene il rendering in background del thread")
	}
	mm, _ = step(mm, *rendered)
	if v := mm.thread.View(); strings.Contains(v, "rendering in corso") || !strings.Contains(v, "parola parola") {
		t.Errorf("il RenderedMsg non e arrivato al thread:\n%s", v)
	}
}
