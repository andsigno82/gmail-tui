package app

// Suggerimenti dei contatti (Piano 9, Task 12-13, D9-F/G/H): il composer chiede
// la query; la rubrica locale risponde subito (in Update, è in memoria); il
// comando esterno (address_book_cmd) gira in una cmd con timeout 2 s e il suo
// esito torna come suggestionsLoaded. I risultati stanti li scarta il composer.
// La rubrica si alimenta dai mittenti in lista, dai destinatari dei messaggi
// aperti e dagli invii riusciti.

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/contacts"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/composer"
	"github.com/andsigno82/gmail-tui/internal/ui/toast"
)

const (
	// suggestLimit è il numero di suggerimenti chiesti alla rubrica (il composer ne mostra al più 5).
	suggestLimit = 5
	// addressLookupTimeout è il budget del comando esterno dei contatti (D9-G).
	addressLookupTimeout = 2 * time.Second
	// lookupDebounce ritarda il comando esterno rispetto all'ultimo tasto (Piano 9, review G).
	lookupDebounce = 150 * time.Millisecond
)

// suggestDueMsg è lo scadere del debounce di una query: il comando esterno parte
// solo se seq è ancora l'ultimo (Piano 9, review G).
type suggestDueMsg struct {
	seq   int
	field composer.Field
	query string
}

// suggestionsLoaded è l'esito del comando esterno per una query del composer.
type suggestionsLoaded struct {
	field composer.Field
	query string
	items []composer.Suggestion
	err   error
}

// SetAddressBook collega la rubrica (file della cache o memoria) e il comando
// esterno (config address_book_cmd, nil = solo locale). Seam del main e della demo.
func (m *Model) SetAddressBook(book *contacts.Book, argv []string) {
	if book != nil {
		m.book = book
	}
	m.addrCmd = append([]string(nil), argv...)
}

// localSuggestions interroga la rubrica (nil-safe).
func localSuggestions(b *contacts.Book, query string) []composer.Suggestion {
	if b == nil {
		return nil
	}
	var out []composer.Suggestion
	for _, s := range b.Suggest(query, suggestLimit) {
		out = append(out, composer.Suggestion{Addr: s.Addr, Name: s.Name})
	}
	return out
}

// mergeSuggestions unisce locali ed esterni: prima i locali, poi gli esterni non
// già presenti (confronto case insensitive sull'indirizzo, il locale vince).
func mergeSuggestions(local, ext []composer.Suggestion) []composer.Suggestion {
	seen := make(map[string]bool, len(local)+len(ext))
	var out []composer.Suggestion
	for _, list := range [][]composer.Suggestion{local, ext} {
		for _, s := range list {
			key := strings.ToLower(s.Addr)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, s)
		}
	}
	return out
}

// suggestQuery risponde a una richiesta del composer: locali subito; il comando
// esterno (se configurato) parte dopo il debounce, con il tick della query.
func (m *Model) suggestQuery(q composer.SuggestQueryMsg) tea.Cmd {
	m.composer.SetSuggestions(q.Field, q.Query, localSuggestions(m.book, q.Query))
	if len(m.addrCmd) == 0 {
		return nil
	}
	m.suggestSeq++
	seq := m.suggestSeq
	return tea.Tick(lookupDebounce, func(time.Time) tea.Msg {
		return suggestDueMsg{seq: seq, field: q.Field, query: q.Query}
	})
}

// dueLookup avvia il comando esterno per l'ultima query (Piano 9, review G): un
// tick stantio (seq superato da un tasto successivo) non fa nulla.
func (m *Model) dueLookup(ev suggestDueMsg) tea.Cmd {
	if ev.seq != m.suggestSeq {
		return nil
	}
	argv := append([]string(nil), m.addrCmd...)
	q := composer.SuggestQueryMsg{Field: ev.field, Query: ev.query}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), addressLookupTimeout)
		defer cancel()
		got, err := contacts.Lookup(ctx, argv, q.Query)
		items := make([]composer.Suggestion, 0, len(got))
		for _, s := range got {
			items = append(items, composer.Suggestion{Addr: s.Addr, Name: s.Name})
		}
		return suggestionsLoaded{field: q.Field, query: q.Query, items: items, err: err}
	}
}

// applySuggestions porta l'esito del comando esterno nel composer (merge con i
// locali). Un errore del comando avvisa una volta per sessione (D9-G).
func (m *Model) applySuggestions(ev suggestionsLoaded) tea.Cmd {
	var cmd tea.Cmd
	if ev.err != nil && !m.addrWarned {
		m.addrWarned = true
		cmd = m.toast.Push(toast.Info, fmt.Sprintf("comando contatti: %v — solo suggerimenti locali", ev.err))
	}
	m.composer.SetSuggestions(ev.field, ev.query, mergeSuggestions(localSuggestions(m.book, ev.query), ev.items))
	return cmd
}

// feedBook registra persone nella rubrica (D9-F): bump per destinatari e invii,
// senza incremento per i mittenti della lista. Gli errori di scrittura non
// bloccano la vista: la rubrica resta utilizzabile in memoria.
func (m Model) feedBook(bump bool, people ...string) {
	if m.book == nil || len(people) == 0 {
		return
	}
	_ = m.book.Observe(m.clock(), bump, people...)
}

// addresses estrae gli indirizzi (come stringhe RFC 5322 semplici) di più liste.
func addresses(lists ...[]string) []string {
	var out []string
	for _, l := range lists {
		out = append(out, l...)
	}
	return out
}

// refSenders estrae i mittenti delle righe di lista.
func refSenders(refs []msg.MessageRef) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		if r.From != "" {
			out = append(out, r.From)
		}
	}
	return out
}
