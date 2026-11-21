// Ricerca (Task 6): la vista `/` è un textinput per la query stile Gmail
// (`from:x is:unread testo`), parsato con msg.ParseQuery (Piano 2). Enter
// esegue la ricerca fuori dal loop UI; esc annulla; query vuota = re-list
// completo. I risultati tornano come msgsLoaded e finiscono nell'inbox.
package app

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"gmail-tui/internal/mail"
	"gmail-tui/internal/msg"
	"gmail-tui/internal/ui/styles"
)

// newPromptInput costruisce un textinput da mini-prompt (ricerca, label):
// prompt in Accent (slot SearchInput), placeholder attenuato (slot ListDate),
// testo col colore di default del terminale (la variante dark/light si
// risolve al rendering), cursore blocco lampeggiante. Nasce NON focalizzato:
// il focus arriva quando il prompt viene aperto. CharLimit è del chiamante.
func newPromptInput(prompt, placeholder string, st styles.Styles) textinput.Model {
	ti := textinput.New()
	ti.Prompt = prompt
	ti.Placeholder = placeholder
	var s textinput.Styles
	s.Focused.Prompt = st.SearchInput
	s.Focused.Placeholder = st.ListDate
	s.Blurred.Prompt = st.SearchInput
	s.Blurred.Placeholder = st.ListDate
	s.Cursor = textinput.CursorStyle{Shape: tea.CursorBlock, Blink: true}
	ti.SetStyles(s)
	return ti
}

// newSearchInput costruisce il textinput della ricerca: prompt "/ " e
// placeholder con la sintassi dei token riconosciuti da ParseQuery.
func newSearchInput(st styles.Styles) textinput.Model {
	ti := newPromptInput("/ ", "from:x to:y is:unread is:starred has:attachment label:x testo", st)
	ti.CharLimit = 256
	return ti
}

// submitQuery traduce la query digitata nella Cmd giusta: stringa vuota (o
// solo spazi) → re-list completo (ListMessages Query{}, prima pagina da 50,
// come il load di Init), altrimenti Search col Query parsato da ParseQuery.
func submitQuery(st mail.MailStore, raw string) tea.Cmd {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return loadRefs(st)
	}
	return searchCmd(st, msg.ParseQuery(raw), raw)
}

// searchCmd esegue la ricerca fuori dal loop UI. Il risultato è msgsLoaded
// con la stringa digitata: la radice la usa per il badge
// "N risultati per …" in status bar (una ricerca non deve sostituire il
// badge anche al re-list, che viaggia senza query).
func searchCmd(st mail.MailStore, q msg.Query, raw string) tea.Cmd {
	return func() tea.Msg {
		refs, err := st.Search(context.Background(), q)
		return msgsLoaded{refs: refs, err: err, query: raw}
	}
}

// searchBody rende il corpo della vista ricerca: riga input e hint dei tasti
// (l'help contestuale vero arriva in Task 9).
func (m Model) searchBody() string {
	return m.search.View() + "\n\n" + m.styles.HelpView.Render("invio cerca · esc annulla")
}
