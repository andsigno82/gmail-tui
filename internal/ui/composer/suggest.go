package composer

// Suggerimenti dei contatti in A, Cc e Ccn (Piano 9, Task 11, D9-H): mentre si
// digita l'ultimo indirizzo (il testo dopo l'ultima virgola) il composer emette
// SuggestQueryMsg; la radice risponde con SetSuggestions. ctrl+n/ctrl+p
// scelgono la riga, invio inserisce "Nome <indirizzo>, " al posto del frammento.

import (
	"net/mail"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// maxSuggestRows è il numero massimo di suggerimenti mostrati sotto il campo.
const maxSuggestRows = 5

// Suggestion è un contatto proposto (dalla rubrica o dal comando esterno).
type Suggestion struct {
	Addr string
	Name string
}

// SuggestQueryMsg chiede i suggerimenti per il frammento corrente del campo.
type SuggestQueryMsg struct {
	Field Field
	Query string
}

// isRecipient dice se il campo accetta indirizzi (A, Cc, Ccn).
func isRecipient(f Field) bool {
	return f == FieldTo || f == FieldCc || f == FieldBcc
}

// recipientInput è l'input del campo destinatari f, nil per gli altri campi.
func (m *Model) recipientInput(f Field) *textinput.Model {
	switch f {
	case FieldTo:
		return &m.to
	case FieldCc:
		return &m.cc
	case FieldBcc:
		return &m.bcc
	}
	return nil
}

// fragment è il testo dopo l'ultima virgola, senza spazi: l'indirizzo in corso.
func fragment(v string) string {
	if i := strings.LastIndex(v, ","); i >= 0 {
		v = v[i+1:]
	}
	return strings.TrimSpace(v)
}

// suggestCmd ricalcola il frammento del campo focalizzato dopo una modifica:
// azzera la lista e chiede i suggerimenti (nessuna richiesta se il frammento è vuoto).
func (m *Model) suggestCmd() tea.Cmd {
	in := m.recipientInput(m.focus)
	if in == nil {
		return nil
	}
	q := fragment(in.Value())
	m.suggField, m.suggQuery = m.focus, q
	m.suggest, m.suggSel = nil, 0
	m.relayout()
	if q == "" {
		return nil
	}
	f := m.focus
	return func() tea.Msg { return SuggestQueryMsg{Field: f, Query: q} }
}

// SetSuggestions accoglie il risultato di una SuggestQueryMsg. Il risultato è
// scartato se il campo o il frammento non sono più quelli della richiesta.
func (m *Model) SetSuggestions(f Field, query string, items []Suggestion) {
	in := m.recipientInput(f)
	if in == nil || f != m.suggField || query != m.suggQuery || fragment(in.Value()) != query {
		return
	}
	if len(items) > maxSuggestRows {
		items = items[:maxSuggestRows]
	}
	m.suggest, m.suggSel = items, 0
	m.relayout()
}

// acceptSuggestion inserisce la riga scelta al posto del frammento: "Nome <addr>, "
// (o "addr, " senza nome). Chiude la lista.
func (m *Model) acceptSuggestion() {
	in := m.recipientInput(m.focus)
	if in == nil || len(m.suggest) == 0 {
		return
	}
	s := m.suggest[m.suggSel]
	name := s.Addr
	if s.Name != "" {
		// Nome semplice: "Nome <addr>"; con caratteri speciali RFC 5322 lo quota mail.Address.
		name = s.Name + " <" + s.Addr + ">"
		if strings.ContainsAny(s.Name, `,;:"<>()[]@\`) {
			name = (&mail.Address{Name: s.Name, Address: s.Addr}).String()
		}
	}
	v := in.Value()
	prefix := ""
	if i := strings.LastIndex(v, ","); i >= 0 {
		prefix = v[:i+1] + " "
	}
	in.SetValue(prefix + name + ", ")
	in.CursorEnd()
	m.suggest, m.suggQuery, m.suggSel = nil, "", 0
	m.relayout()
}

// moveSuggestion scorre le righe con wrap (delta +1 o -1).
func (m *Model) moveSuggestion(delta int) {
	n := len(m.suggest)
	if n == 0 {
		return
	}
	m.suggSel = (m.suggSel + delta + n) % n
}

// relayout riapplica il layout: le righe dei suggerimenti occupano altezza.
func (m *Model) relayout() {
	if m.height > 0 {
		m.SetSize(m.width, m.height)
	}
}

// suggestRows rende le righe dei suggerimenti se la lista appartiene al campo f.
func (m Model) suggestRows(f Field) string {
	if f != m.suggField || len(m.suggest) == 0 {
		return ""
	}
	var b strings.Builder
	maxW := max(0, m.width-6)
	for i, s := range m.suggest {
		label := s.Addr
		if s.Name != "" {
			label = s.Name + " <" + s.Addr + ">"
		}
		if r := []rune(label); len(r) > maxW {
			label = string(r[:maxW])
		}
		marker, st := "  ", m.st.ListNormal
		if i == m.suggSel {
			marker, st = "> ", m.st.ListSelected
		}
		b.WriteString("    " + st.Render(marker+label) + "\n")
	}
	return b.String()
}

// Suggestions restituisce i suggerimenti mostrati (vuoti se la lista è chiusa).
func (m Model) Suggestions() []Suggestion { return m.suggest }
