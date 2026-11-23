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

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
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

// gmailAware è l'interfaccia opzionale con cui lo store dichiara la modalità
// Gmail (Piano 8, Task 11). Il Mock non la implementa: è un server non Gmail.
type gmailAware interface {
	GmailExt() bool
}

// searchQuery costruisce la Query di una ricerca digitata (Piano 8, Task 11):
// con Gmail la stringa passa intera a Query.Raw (X-GM-RAW), senza riscriverla;
// senza Gmail resta il parsing ParseQuery di prima.
func searchQuery(st mail.MailStore, raw string) msg.Query {
	if g, ok := st.(gmailAware); ok && g.GmailExt() {
		return msg.Query{Raw: raw}
	}
	return msg.ParseQuery(raw)
}

// scopeQuery applica l'ambito della cartella corrente a una ricerca (Piano 8,
// Task 11, D8-4): una query Raw non ha cartella, cerca in Tutti i messaggi come
// Gmail web (in:/label: dentro la stringa li ha già il server); le altre restano
// nella cartella salvo label: esplicita.
func scopeQuery(q msg.Query, box string) msg.Query {
	if q.Raw != "" {
		return q
	}
	return inFolder(q, box)
}

// submitQuery traduce la query digitata nella Cmd giusta nella cartella box
// (Piano 7, Task 4): stringa vuota (o solo spazi) → re-list completo (come il
// load di Init), altrimenti Search con la Query di searchQuery.
func submitQuery(st mail.MailStore, raw string, box string, gen int) tea.Cmd {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return loadRefs(st, box, gen)
	}
	return searchCmd(st, searchQuery(st, raw), raw, box, gen)
}

// searchCmd esegue la ricerca fuori dal loop UI. Il risultato è msgsLoaded
// con la stringa digitata: la radice la usa per il badge
// "N risultati per …" in status bar (una ricerca non deve sostituire il
// badge anche al re-list, che viaggia senza query).
func searchCmd(st mail.MailStore, q msg.Query, raw string, box string, gen int) tea.Cmd {
	return func() tea.Msg {
		convs, err := st.ListConversations(context.Background(), scopeQuery(q, box), mail.Page{Limit: searchLimit})
		lim, err := scanLimited(err)
		return msgsLoaded{refs: latestRefs(convs), convs: convs, err: err, query: raw, limited: lim, mailbox: box, gen: gen}
	}
}

// searchBody rende il corpo della vista ricerca: riga input e hint dei tasti
// (l'help contestuale vero arriva in Task 9).
func (m *Model) searchBody() string {
	return m.search.View() + "\n\n" + m.styles.HelpView.Render("invio cerca · esc annulla")
}
