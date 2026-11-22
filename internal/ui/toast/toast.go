// Package toast contiene le notifiche effimere sovrapposte alla TUI (Piano 7.3,
// Task 1): una pila di al più 3 riquadri in alto a destra, composta con il
// compositor nativo di lipgloss v2 (nessuna dipendenza esterna, decisione T1).
// Successo e info scadono da soli dopo Duration; un errore resta finché un
// tasto non lo chiude (DismissErrors, decisione T3).
package toast

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// Duration è la vita di un toast di successo o info (decisione T3).
// È una variabile solo perché il pacchetto app la abbassa nei test (che
// eseguono le cmd in modo sincrono): nessuna attesa reale (Piano 7.3, Task 2).
var Duration = 3 * time.Second

// maxToasts è il numero massimo di notifiche in pila (decisione T4): oltre,
// esce la più vecchia non d'errore (evict).
const maxToasts = 3

// Kind è il tipo di notifica (colore del bordo).
type Kind int

const (
	Info Kind = iota
	Success
	Error
)

// ExpiredMsg scade il toast con quell'ID (non gli altri).
type ExpiredMsg struct{ ID int }

type item struct {
	id   int
	kind Kind
	text string
}

// Model è la pila dei toast. Il valore va riassegnato al modello radice
// (gotcha value receiver): i metodi mutanti hanno puntatore.
type Model struct {
	items  []item
	nextID int
	st     styles.Styles
}

// New costruisce una pila vuota con gli stili del tema.
func New(st styles.Styles) Model { return Model{st: st} }

// SetStyles riapplica gli stili (cambio tema).
func (m *Model) SetStyles(st styles.Styles) { m.st = st }

// Push aggiunge un toast; oltre maxToasts toglie il più vecchio che non è un
// errore (evict). Gli errori non armano timer; gli altri tipi sì, con un ID che
// rende lo ExpiredMsg riconoscibile anche se il toast è già uscito dalla pila.
func (m *Model) Push(k Kind, text string) tea.Cmd {
	if len(m.items) >= maxToasts {
		m.evict()
	}
	m.nextID++
	id := m.nextID
	m.items = append(m.items, item{id: id, kind: k, text: text})
	if k == Error {
		return nil
	}
	return tea.Tick(Duration, func(time.Time) tea.Msg { return ExpiredMsg{ID: id} })
}

// evict toglie il toast più vecchio che NON è un errore (Piano 7.3, fix review):
// un errore sopravvive ai successi; solo se la pila è tutta di errori esce il
// più vecchio.
func (m *Model) evict() {
	for i, it := range m.items {
		if it.kind != Error {
			m.items = append(m.items[:i], m.items[i+1:]...)
			return
		}
	}
	m.items = m.items[1:]
}

// Update gestisce ExpiredMsg: rimuove quel toast se è ancora in pila. Gli altri
// messaggi non la riguardano.
func (m *Model) Update(msg tea.Msg) {
	ev, ok := msg.(ExpiredMsg)
	if !ok {
		return
	}
	for i, it := range m.items {
		// Un errore non arma timer: un ExpiredMsg con il suo ID è stantio.
		if it.id == ev.ID && it.kind != Error {
			m.items = append(m.items[:i], m.items[i+1:]...)
			break
		}
	}
}

// DismissErrors chiude i toast d'errore; gli altri restano. La app la chiama a
// ogni tasto premuto (il tasto viene poi eseguito normalmente).
func (m *Model) DismissErrors() {
	kept := m.items[:0]
	for _, it := range m.items {
		if it.kind != Error {
			kept = append(kept, it)
		}
	}
	m.items = kept
}

// Texts è il testo dei toast in pila, dal più vecchio.
func (m Model) Texts() []string {
	out := make([]string, 0, len(m.items))
	for _, it := range m.items {
		out = append(out, it.text)
	}
	return out
}

// Kinds è il tipo di ogni toast in pila, dal più vecchio.
func (m Model) Kinds() []Kind {
	out := make([]Kind, 0, len(m.items))
	for _, it := range m.items {
		out = append(out, it.kind)
	}
	return out
}

// Layers rende i toast come layer lipgloss in alto a destra per una vista larga
// width: margine di 1 colonna dal bordo destro, di 1 riga dall'alto, impilati
// verso il basso. Il box non supera ~40% della larghezza e il testo va a capo.
// Z=1: sopra il corpo (Z=0, il layer base della app).
func (m Model) Layers(width int) []*lipgloss.Layer {
	if len(m.items) == 0 {
		return nil
	}
	maxBox := max(width*2/5, 12)
	layers := make([]*lipgloss.Layer, 0, len(m.items))
	y := 1
	for _, it := range m.items {
		box := m.render(it, maxBox)
		l := lipgloss.NewLayer(box).X(width - lipgloss.Width(box) - 1).Y(y).Z(1)
		layers = append(layers, l)
		y += lipgloss.Height(box)
	}
	return layers
}

// render compone il riquadro di un toast entro maxBox colonne (bordo e padding
// compresi): il testo va a capo sulla larghezza interna disponibile.
func (m Model) render(it item, maxBox int) string {
	const chrome = 4 // bordo (2) + padding orizzontale (2)
	inner := min(lipgloss.Width(it.text), maxBox-chrome)
	inner = max(inner, 1)
	wrapped := lipgloss.NewStyle().Width(inner).Render(it.text)
	return m.style(it.kind).Render(strings.TrimRight(wrapped, "\n"))
}

// style sceglie lo stile del tipo di notifica.
func (m Model) style(k Kind) lipgloss.Style {
	switch k {
	case Success:
		return m.st.ToastSuccess
	case Error:
		return m.st.ToastError
	default:
		return m.st.ToastInfo
	}
}
