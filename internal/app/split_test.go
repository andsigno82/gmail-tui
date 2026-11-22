package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/config"
	"github.com/andsigno82/gmail-tui/internal/ui/inbox"
)

// splitModel prepara il radice a due colonne: layout split, misura data, inbox
// caricata con i due messaggi di test. Nessun timer è eseguito: l'anteprima
// parte solo quando il test la fa scattare (feedPreview).
func splitModel(t *testing.T, width, height int) Model {
	t.Helper()
	refs := testRefs(t)
	m, _ := newTestModelMock(t, fullMessage(refs[0]), fullMessage(refs[1]))
	m.SetPrefs(config.LayoutSplit, config.PreviewMarkReadDelay)
	mm, _ := step(m, tea.WindowSizeMsg{Width: width, Height: height})
	mm, _ = step(mm, msgsLoaded{refs: refs})
	return mm
}

// feedPreview simula lo scadere del timer di 150 ms e il fetch dell'anteprima,
// senza eseguire le tea.Tick (che dormono davvero) né lo spinner.
func feedPreview(t *testing.T, m Model) Model {
	t.Helper()
	tok := m.prevTok
	mm, _ := step(m, previewDue{tok: tok})
	mm, _ = step(mm, fetchPreview(mm.store, tok, mm.prevRef.ID)())
	return mm
}

// TestSplitActivation: due colonne solo con layout split e larghezza ≥ 120.
func TestSplitActivation(t *testing.T) {
	cases := []struct {
		name   string
		layout string
		width  int
		want   bool
	}{
		{"split 119", config.LayoutSplit, 119, false},
		{"split 120", config.LayoutSplit, 120, true},
		{"split 140", config.LayoutSplit, 140, true},
		{"single 140", config.LayoutSingle, 140, false},
		{"vuoto 140 (default single)", "", 140, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := splitModel(t, c.width, 30)
			m.SetPrefs(c.layout, config.PreviewMarkReadDelay)
			m = m.relayoutForTest()
			if got := m.splitOn(); got != c.want {
				t.Errorf("splitOn() = %v, voglio %v", got, c.want)
			}
		})
	}
}

// relayoutForTest ricalcola le dimensioni come farebbe un cambio di layout.
func (m Model) relayoutForTest() Model {
	m.relayout()
	return m
}

// TestSplitWidthsProportion: lista floor(35%), anteprima il resto meno separatore.
func TestSplitWidthsProportion(t *testing.T) {
	cases := []struct{ width, list, preview int }{
		{120, 42, 77},
		{140, 49, 90},
		{200, 70, 129},
	}
	for _, c := range cases {
		list, preview := splitWidths(c.width)
		if list != c.list || preview != c.preview {
			t.Errorf("splitWidths(%d) = %d/%d, voglio %d/%d", c.width, list, preview, c.list, c.preview)
		}
		if list+preview+splitSepWidth != c.width {
			t.Errorf("splitWidths(%d): colonne + separatore non coprono la larghezza", c.width)
		}
	}
}

// TestSplitViewRendersTwoColumns: la vista occupa la larghezza del terminale e
// ogni riga della lista sta nella sua colonna (nessuna riga la sfonda).
func TestSplitViewRendersTwoColumns(t *testing.T) {
	m := splitModel(t, 120, 30)
	view := m.splitView()
	list, _ := splitWidths(120)
	for i, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > 120 {
			t.Fatalf("riga %d larga %d > 120: %q", i, w, line)
		}
	}
	first := strings.Split(m.inbox.View(), "\n")[0]
	if w := lipgloss.Width(first); w > list {
		t.Errorf("riga lista larga %d, colonna %d", w, list)
	}
	if !strings.Contains(view, "│") {
		t.Error("separatore verticale assente")
	}
}

// TestSplitPreviewLoadsAfterTimerOnly: l'anteprima non si carica prima dello
// scadere del timer di 150 ms; il timer è armato dal cursore.
func TestSplitPreviewLoadsAfterTimerOnly(t *testing.T) {
	m := splitModel(t, 120, 30)
	if !m.prevHas {
		t.Fatal("cursore su una riga: anteprima non agganciata")
	}
	if m.previewReady() {
		t.Fatal("anteprima pronta prima del timer")
	}
	if !strings.Contains(m.previewView(77, 10), "Caricamento") {
		t.Error("prima del caricamento manca il testo di attesa")
	}
	mm := feedPreview(t, m)
	if !mm.previewReady() || mm.thread.Ref().ID != testRefs(t)[0].ID {
		t.Errorf("anteprima dopo il timer: pronta=%v ref=%v", mm.previewReady(), mm.thread.Ref().ID)
	}
	if strings.Contains(mm.previewView(77, 10), "Caricamento") {
		t.Error("anteprima caricata ma il testo di attesa resta")
	}
}

// TestSplitStalePreviewDiscarded: dopo un movimento rapido il risultato del
// messaggio precedente è scartato, e il timer stantio non fa fetch.
func TestSplitStalePreviewDiscarded(t *testing.T) {
	m := splitModel(t, 120, 30)
	staleTok := m.prevTok
	staleID := m.prevRef.ID
	mm, _ := step(m, pressKey("j")) // il cursore va sul secondo messaggio: token nuovo
	if mm.prevTok == staleTok {
		t.Fatal("il movimento del cursore non ha cambiato token")
	}
	mm, cmd := step(mm, previewDue{tok: staleTok})
	if cmd != nil {
		t.Errorf("timer stantio: cmd = %v, voglio nil", cmd)
	}
	mm, _ = step(mm, previewLoaded{tok: staleTok, id: staleID, m: fullMessage(testRefs(t)[0])})
	if mm.previewReady() {
		t.Error("risultato stantio mostrato nell'anteprima")
	}
}

// TestSplitPreviewMarkReadImmediate: "subito" segna come letto al caricamento.
func TestSplitPreviewMarkReadImmediate(t *testing.T) {
	m := splitModel(t, 120, 30)
	m.SetPrefs(config.LayoutSplit, config.PreviewMarkReadImmediate)
	tok := m.prevTok
	mm, _ := step(m, previewDue{tok: tok})
	mm, cmd := step(mm, fetchPreview(mm.store, tok, mm.prevRef.ID)())
	if mm.thread.Ref().Unread {
		t.Error("subito: anteprima ancora non letta dopo il caricamento")
	}
	if !hasOpDone(runCmd(cmd), "letta") {
		t.Errorf("subito: nessun MarkRead al caricamento, msgs=%v", runCmd(cmd))
	}
}

// TestSplitPreviewMarkReadDelay: "dopo 2 secondi" arma un timer e segna come letto
// solo se il messaggio è ancora in anteprima con lo stesso token.
func TestSplitPreviewMarkReadDelay(t *testing.T) {
	m := splitModel(t, 120, 30)
	tok := m.prevTok
	mm, _ := step(m, previewDue{tok: tok})
	mm, cmd := step(mm, fetchPreview(mm.store, tok, mm.prevRef.ID)())
	if cmd == nil {
		t.Fatal("dopo 2 secondi: nessun timer armato")
	}
	if !mm.thread.Ref().Unread {
		t.Fatal("dopo 2 secondi: segnato come letto prima dello scadere")
	}
	mm, cmd = step(mm, previewReadDue{tok: tok, id: mm.prevRef.ID})
	if !hasOpDone(runCmd(cmd), "letta") || mm.thread.Ref().Unread {
		t.Error("scadenza di 2 secondi: messaggio non segnato come letto")
	}
}

// TestSplitPreviewMarkReadDelayStaleDiscarded: se il cursore si è spostato
// prima dei 2 secondi, il timer non segna nulla.
func TestSplitPreviewMarkReadDelayStaleDiscarded(t *testing.T) {
	m := splitModel(t, 120, 30)
	tok := m.prevTok
	mm := feedPreview(t, m)
	mm, _ = step(mm, pressKey("j"))
	_, cmd := step(mm, previewReadDue{tok: tok, id: testRefs(t)[0].ID})
	if cmd != nil {
		t.Errorf("timer di 2 secondi stantio: cmd = %v, voglio nil", cmd)
	}
}

// TestSplitPreviewMarkReadNever: "mai" non segna nulla.
func TestSplitPreviewMarkReadNever(t *testing.T) {
	m := splitModel(t, 120, 30)
	m.SetPrefs(config.LayoutSplit, config.PreviewMarkReadNever)
	tok := m.prevTok
	mm, _ := step(m, previewDue{tok: tok})
	mm, cmd := step(mm, fetchPreview(mm.store, tok, mm.prevRef.ID)())
	if !mm.thread.Ref().Unread {
		t.Error("mai: messaggio segnato come letto")
	}
	if cmd != nil {
		t.Errorf("mai: cmd = %v, voglio nil", cmd)
	}
}

// TestSplitTabAndEscFocus: Tab alterna il focus; con focus sull'anteprima j
// scorre il messaggio e non muove la lista; esc torna alla lista.
func TestSplitTabAndEscFocus(t *testing.T) {
	m := feedPreview(t, splitModel(t, 120, 30))
	mm, _ := step(m, pressKey("tab"))
	if !mm.previewFocused() {
		t.Fatal("Tab: focus non sull'anteprima")
	}
	before := mm.inbox.SelectedIndex()
	mm, _ = step(mm, pressKey("j"))
	if mm.inbox.SelectedIndex() != before {
		t.Error("j con focus sull'anteprima ha mosso la lista")
	}
	mm, _ = step(mm, pressKey("esc"))
	if mm.previewFocused() || mm.currentView() != viewInbox {
		t.Error("esc con focus sull'anteprima: focus non tornato alla lista (o uscita dall'inbox)")
	}
	mm, _ = step(mm, pressKey("tab"))
	mm, _ = step(mm, pressKey("tab"))
	if mm.previewFocused() {
		t.Error("secondo Tab: focus ancora sull'anteprima")
	}
	_, cmd := step(mm, pressKey("esc"))
	if !hasQuit(cmd) {
		t.Error("esc con focus sulla lista: deve uscire come oggi")
	}
}

// TestSplitTabIgnoredInSingleColumn: senza due colonne Tab non sposta il focus.
func TestSplitTabIgnoredInSingleColumn(t *testing.T) {
	m := splitModel(t, 100, 30)
	mm, _ := step(m, pressKey("tab"))
	if mm.focusPreview || mm.previewFocused() {
		t.Error("Tab a una colonna ha spostato il focus")
	}
}

// TestSplitActionsActOnPreview: le azioni (qui la stella) agiscono sul messaggio
// in anteprima, anche con il focus sulla lista.
func TestSplitActionsActOnPreview(t *testing.T) {
	m := feedPreview(t, splitModel(t, 120, 30))
	mm, _ := step(m, pressKey("s"))
	if !mm.thread.Ref().Starred || !mm.prevRef.Starred {
		t.Errorf("stella sull'anteprima: thread=%v prev=%v, voglio entrambi on", mm.thread.Ref().Starred, mm.prevRef.Starred)
	}
	ref, ok := mm.currentRef()
	if !ok || ref.ID != testRefs(t)[0].ID {
		t.Errorf("currentRef con due colonne = %v, voglio il messaggio in anteprima", ref.ID)
	}
}

// TestSplitEnterOpensFullThread: invio con il focus sull'anteprima apre il thread
// a schermo intero del messaggio in anteprima.
func TestSplitEnterOpensFullThread(t *testing.T) {
	m := feedPreview(t, splitModel(t, 120, 30))
	mm, _ := step(m, pressKey("tab"))
	_, cmd := step(mm, pressKey("enter"))
	var sel inbox.SelectedMsg
	found := false
	for _, ms := range runCmd(cmd) {
		if s, ok := ms.(inbox.SelectedMsg); ok {
			sel, found = s, true
		}
	}
	if !found || sel.Ref.ID != testRefs(t)[0].ID {
		t.Fatalf("invio sull'anteprima: nessun SelectedMsg del messaggio in anteprima (trovato=%v)", found)
	}
}

// TestSplitMouseWheelScrollsPreview: la rotella sulla colonna destra scorre
// l'anteprima; un click sulla destra non muove la lista.
func TestSplitMouseWheelScrollsPreview(t *testing.T) {
	m := feedPreview(t, splitModel(t, 120, 30))
	list, _ := splitWidths(120)
	mm, _ := step(m, tea.MouseWheelMsg{X: list + 5, Y: 3, Button: tea.MouseWheelDown})
	if mm.thread.YOffset() != 3 {
		t.Errorf("rotella a destra: YOffset = %d, voglio 3", mm.thread.YOffset())
	}
	before := mm.inbox.SelectedIndex()
	mm, _ = step(mm, tea.MouseClickMsg{X: list + 5, Y: 1, Button: tea.MouseLeft})
	if mm.inbox.SelectedIndex() != before {
		t.Error("click sulla colonna destra ha mosso la lista")
	}
	mm, _ = step(mm, tea.MouseWheelMsg{X: 3, Y: 3, Button: tea.MouseWheelDown})
	if mm.thread.YOffset() != 3 {
		t.Errorf("rotella a sinistra ha scorso l'anteprima: YOffset = %d", mm.thread.YOffset())
	}
}

// TestSplitResizeBelow120ReturnsToSingle: ridimensionare sotto 120 colonne torna
// a una colonna (nessun errore), e tornando sopra la soglia l'anteprima si riapre.
func TestSplitResizeBelow120ReturnsToSingle(t *testing.T) {
	m := feedPreview(t, splitModel(t, 140, 30))
	mm, _ := step(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if mm.splitOn() {
		t.Fatal("100 colonne: due colonne ancora attive")
	}
	if mm.inbox.View() == "" || lipgloss.Width(strings.Split(mm.body(), "\n")[0]) > 100 {
		t.Error("100 colonne: vista a una colonna non corretta")
	}
	mm, _ = step(mm, tea.WindowSizeMsg{Width: 140, Height: 30})
	if !mm.splitOn() || mm.prevHas == false {
		t.Error("tornando a 140 colonne l'anteprima deve riaprirsi dal cursore")
	}
}

// TestSplitEmptyListEmptiesPreview: lista vuota → anteprima vuota con testo
// attenuato; con un messaggio di nuovo, l'anteprima ricomincia dal cursore.
func TestSplitEmptyListEmptiesPreview(t *testing.T) {
	m := feedPreview(t, splitModel(t, 120, 30))
	mm, _ := step(m, msgsLoaded{refs: nil})
	if mm.prevHas {
		t.Error("lista vuota: anteprima ancora agganciata")
	}
	if !strings.Contains(mm.previewView(77, 5), "nessun messaggio") {
		t.Error("lista vuota: manca il testo dell'anteprima vuota")
	}
}

// TestSplitRowsFitListWidth: a 35% le righe della lista restano nella colonna
// (la data non esce dal bordo).
func TestSplitRowsFitListWidth(t *testing.T) {
	m := splitModel(t, 120, 30)
	list, _ := splitWidths(120)
	for _, line := range strings.Split(m.inbox.View(), "\n") {
		if line == "" {
			continue
		}
		if w := lipgloss.Width(line); w > list {
			t.Errorf("riga lista larga %d > %d: %q", w, list, line)
		}
	}
}

// TestSplitHelpShowsFocusKey: la barra breve mostra Tab solo con due colonne.
func TestSplitHelpShowsFocusKey(t *testing.T) {
	m := splitModel(t, 120, 30)
	if !bindingDescs(m.shortHelpFor(false, viewInbox))["lista/anteprima"] {
		t.Error("due colonne: barra breve senza Tab")
	}
	single := splitModel(t, 100, 30)
	if bindingDescs(single.shortHelpFor(false, viewInbox))["lista/anteprima"] {
		t.Error("una colonna: Tab nella barra breve")
	}
}

// hasOpDone dice se tra i Msg c'è un esito di mutazione con quell'etichetta.
func hasOpDone(ms []tea.Msg, label string) bool {
	for _, m := range ms {
		if o, ok := m.(opDone); ok && o.label == label {
			return true
		}
	}
	return false
}

// hasQuit dice se la cmd produce l'uscita del programma.
func hasQuit(c tea.Cmd) bool {
	if c == nil {
		return false
	}
	for _, m := range runCmd(c) {
		if _, ok := m.(tea.QuitMsg); ok {
			return true
		}
	}
	return false
}
