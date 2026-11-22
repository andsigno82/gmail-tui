package composer

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Piano 9, Task 11 (D9-H): suggerimenti dei contatti in A, Cc e Ccn mentre si
// digita l'ultimo indirizzo (testo dopo l'ultima virgola).

// typed porta il testo di una pressione (un carattere) e restituisce la cmd.
func typed(t *testing.T, m Model, s string) (Model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, r := range s {
		m, cmd = step(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m, cmd
}

// TestTypingEmitsSuggestQuery: digitare emette SuggestQueryMsg con il frammento.
func TestTypingEmitsSuggestQuery(t *testing.T) {
	m := newModel(t, ComposerConfig{})
	_, cmd := typed(t, m, "an")
	if cmd == nil {
		t.Fatal("digitazione in A: nessuna cmd, voglio SuggestQueryMsg")
	}
	var got SuggestQueryMsg
	found := false
	for _, msg := range runCmds(cmd) {
		if q, ok := msg.(SuggestQueryMsg); ok {
			got, found = q, true
		}
	}
	if !found || got.Field != FieldTo || got.Query != "an" {
		t.Errorf("SuggestQueryMsg = %+v (trovato=%v), voglio {FieldTo, an}", got, found)
	}
}

// TestSetSuggestionsIgnoresStaleQuery: un risultato per una query superata è scartato.
func TestSetSuggestionsIgnoresStaleQuery(t *testing.T) {
	m := newModel(t, ComposerConfig{})
	m, _ = typed(t, m, "an")
	items := []Suggestion{{Addr: "anna@x.dev", Name: "Anna"}}
	m.SetSuggestions(FieldTo, "a", items) // query superata
	if len(m.suggest) != 0 {
		t.Errorf("risultato stantio accettato: %v", m.suggest)
	}
	m.SetSuggestions(FieldTo, "an", items)
	if len(m.suggest) != 1 {
		t.Errorf("risultato della query corrente non accettato: %v", m.suggest)
	}
}

// TestEnterInsertsSelectedSuggestion: invio sostituisce il frammento con "Nome <addr>, ".
func TestEnterInsertsSelectedSuggestion(t *testing.T) {
	m := newModel(t, ComposerConfig{To: "bob@y.dev, "})
	m.focusField(FieldTo)
	m, _ = typed(t, m, "an")
	m.SetSuggestions(FieldTo, "an", []Suggestion{{Addr: "anna@x.dev", Name: "Anna"}})
	m, _ = step(m, pressKey("ctrl+n")) // seleziona la riga (indice 0 → 1 con una sola riga: resta)
	m, _ = step(m, pressKey("enter"))
	if got := m.to.Value(); got != "bob@y.dev, Anna <anna@x.dev>, " {
		t.Errorf("campo A = %q, voglio il frammento sostituito", got)
	}
	if len(m.suggest) != 0 {
		t.Error("dopo l'inserimento la lista deve sparire")
	}
}

// TestEnterWithoutSuggestionsKeepsField: senza suggerimenti invio non cambia il campo.
func TestEnterWithoutSuggestionsKeepsField(t *testing.T) {
	m := newModel(t, ComposerConfig{To: "bob@y.dev"})
	m.focusField(FieldTo)
	before := m.to.Value()
	m, _ = step(m, pressKey("enter"))
	if m.to.Value() != before {
		t.Errorf("campo A = %q, voglio %q invariato", m.to.Value(), before)
	}
}

// TestCtrlNPSelectsRows: ctrl+n/ctrl+p scorrono le righe con wrap.
func TestCtrlNPSelectsRows(t *testing.T) {
	m := newModel(t, ComposerConfig{})
	m.focusField(FieldTo)
	m, _ = typed(t, m, "x")
	m.SetSuggestions(FieldTo, "x", []Suggestion{{Addr: "a@x.dev"}, {Addr: "b@x.dev"}})
	m, _ = step(m, pressKey("ctrl+n"))
	if m.suggSel != 1 {
		t.Errorf("sel dopo ctrl+n = %d, voglio 1", m.suggSel)
	}
	m, _ = step(m, pressKey("ctrl+n"))
	if m.suggSel != 0 {
		t.Errorf("sel dopo il wrap = %d, voglio 0", m.suggSel)
	}
	m, _ = step(m, pressKey("ctrl+p"))
	if m.suggSel != 1 {
		t.Errorf("sel dopo ctrl+p dal primo = %d, voglio 1 (wrap)", m.suggSel)
	}
}

// TestSuggestionRowsFitLayout: con cinque suggerimenti la view non supera l'altezza.
func TestSuggestionRowsFitLayout(t *testing.T) {
	m := newModel(t, ComposerConfig{})
	m.SetSize(80, 20)
	m.focusField(FieldTo)
	m, _ = typed(t, m, "x")
	var items []Suggestion
	for _, a := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		items = append(items, Suggestion{Addr: a + "@x.dev"})
	}
	m.SetSuggestions(FieldTo, "x", items)
	if len(m.suggest) > maxSuggestRows {
		t.Errorf("righe = %d, voglio al più %d", len(m.suggest), maxSuggestRows)
	}
	if h := strings.Count(m.View(), "\n") + 1; h > 20 {
		t.Errorf("view alta %d righe con suggerimenti, voglio ≤ 20", h)
	}
}
