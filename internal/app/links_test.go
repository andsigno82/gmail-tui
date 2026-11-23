package app

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/config"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/thread"
	"github.com/andsigno82/gmail-tui/internal/ui/toast"
)

// linkMessage: messaggio la cui prima riga e un URL (Richiesta di Andrea, 2026-11-23).
func linkMessage(ref msg.MessageRef) *msg.Message {
	m := fullMessage(ref)
	m.BodyText = "https://example.com/x qui\nsecondo rigo"
	return m
}

// splitLinkModel: due colonne 140x30 con il messaggio dell'anteprima che ha un link
// nella prima riga del corpo.
func splitLinkModel(t *testing.T) Model {
	t.Helper()
	refs := testRefs(t)
	m, _ := newTestModelMock(t, linkMessage(refs[0]), fullMessage(refs[1]))
	m.SetPrefs(config.LayoutSplit, config.PreviewMarkReadDelay)
	mm, _ := step(m, tea.WindowSizeMsg{Width: 140, Height: 30})
	mm, _ = step(mm, msgsLoaded{refs: refs})
	return feedPreview(t, mm)
}

// TestSplitClickSuLinkApreSenzaFocus: nell'anteprima il clic su un link produce la
// Cmd di apertura e NON sposta il focus; la colonna di padding (subito a sinistra
// del testo) resta un clic normale che sposta il focus.
func TestSplitClickSuLinkApreSenzaFocus(t *testing.T) {
	m := splitLinkModel(t)
	list, _ := splitWidths(140)
	origin := list + splitSepWidth + splitPreviewPad

	mm, cmd := step(m, tea.MouseClickMsg{X: origin, Y: 5, Button: tea.MouseLeft})
	if mm.focusPreview {
		t.Error("clic su link: il focus si e spostato sull'anteprima")
	}
	if cmd == nil {
		t.Fatal("clic su link: nessuna Cmd di apertura")
	}
	// La Cmd non va eseguita: aprirebbe il browser vero.

	pad, cmdPad := step(m, tea.MouseClickMsg{X: origin - 1, Y: 5, Button: tea.MouseLeft})
	if !pad.focusPreview {
		t.Error("clic sul padding: il focus non e andato sull'anteprima")
	}
	if cmdPad != nil {
		t.Error("clic sul padding: Cmd inattesa")
	}
}

// TestLinkOpenedMsgToast: esito di apertura riuscito e fallito come toast.
func TestLinkOpenedMsgToast(t *testing.T) {
	m := newTestModel(t)
	m, _ = step(m, msgsLoaded{})
	mm, _ := step(m, thread.LinkOpenedMsg{Target: "example.com"})
	assertToast(t, mm, toast.Success, "aperto nel browser: example.com")

	mm, _ = step(m, thread.LinkOpenedMsg{Target: "example.com", Err: errors.New("open assente")})
	if !toastIs(mm, toast.Error) || !strings.HasPrefix(toastText(mm), "apertura link: ") {
		t.Errorf("toast d'errore = %q (kind %v)", toastText(mm), mm.toast.Kinds())
	}
}
