package thread

// Corpo del messaggio a blocchi (Piano 15, Task 14): BodyHTML se presente, altrimenti
// BodyText, passano dal modello di internal/msg/richtext e dai renderer di
// internal/ui/render. I corpi piccoli si rendono subito; quelli grandi in una Cmd
// fuori dal loop (D15-T5): finche il risultato non arriva si mostra il testo
// semplice con un indicatore, e i risultati stantii (generazione diversa) si
// scartano, come da AGENTS.md. Il server MCP non importa questo pacchetto: legge
// solo BodyText.

import (
	"fmt"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/msg/richtext"
	"github.com/andsigno82/gmail-tui/internal/ui/render"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

const (
	// syncRenderLimit: sorgente (BodyHTML o BodyText) fino a questa dimensione si
	// rende nel loop Update; oltre, in una Cmd (D15-T5). Da tarare in Task 19.
	syncRenderLimit = 32 << 10
	// fallbackBytes: quanto del testo semplice si mostra mentre il rendering e in volo.
	fallbackBytes = 4 << 10
	// defaultBodyWidth e la larghezza del corpo prima del primo SetSize.
	defaultBodyWidth = 80
)

// RenderedMsg e il risultato di un rendering in background (Piano 15, Task 14,
// D15-T5). La radice lo inoltra al thread (Update); Gen e la generazione del
// thread che lo ha chiesto: se nel frattempo il thread e cambiato (altra larghezza,
// messaggio chiuso o sostituito) il risultato e scartato.
type RenderedMsg struct {
	ID      msg.MessageID
	Width   int
	Gen     int
	Variant string
	Out     render.Out
}

// genSeq numera le generazioni: ogni thread nuovo e ogni cambio di larghezza ne
// prende una nuova, quindi un risultato di un thread sostituito non coincide mai.
var genSeq atomic.Int64

func nextGen() int { return int(genSeq.Add(1)) }

// themeKey e l'impronta degli stili usata nella chiave della cache: cambia con il
// tema, senza che thread conosca il nome del tema.
func themeKey(st styles.Styles) string {
	return st.LinkDashed.Render("x") + st.Quoted.Render("x") + st.Button.Render("x") + st.TableHeader.Render("x")
}

// variantOf distingue il corpo da HTML da quello da testo semplice.
func variantOf(full *msg.Message) string {
	if full.BodyHTML != "" {
		return "html"
	}
	return "text"
}

// sourceLen e la dimensione del sorgente che il rendering deve leggere.
func sourceLen(full *msg.Message) int {
	if full.BodyHTML != "" {
		return len(full.BodyHTML)
	}
	return len(full.BodyText)
}

// blankOut e una riga vuota senza zone (separatore tra blocchi).
func blankOut() render.Out {
	return render.Out{Lines: []string{""}, Zones: [][]render.Zone{nil}}
}

// stackBlocks rende ogni blocco con render.Dispatch e li impila separati da una
// riga vuota; i blocchi che non producono righe si saltano.
func stackBlocks(blocks []richtext.Block, c render.Ctx) render.Out {
	var parts []render.Out
	for _, b := range blocks {
		o := render.Dispatch(b, c)
		if len(o.Lines) == 0 {
			continue
		}
		if len(parts) > 0 {
			parts = append(parts, blankOut())
		}
		parts = append(parts, o)
	}
	return render.Stack(parts...)
}

// buildBody costruisce il corpo reso: richtext.FromHTML su BodyHTML se presente
// (e se produce blocchi), altrimenti FromText su BodyText. Se l'HTML supera il
// limite di richtext, in coda compare "[messaggio troncato: N KiB]" con N i KiB
// tagliati (D15-T4). Pura: gira anche fuori dal loop.
func buildBody(full *msg.Message, width int, st styles.Styles) render.Out {
	c := render.Ctx{Width: width, St: st}
	var blocks []richtext.Block
	truncated := false
	if full.BodyHTML != "" {
		blocks, truncated = richtext.FromHTML(full.BodyHTML)
	}
	if len(blocks) == 0 {
		blocks = richtext.FromText(full.BodyText)
	}
	o := stackBlocks(blocks, c)
	if truncated {
		cut := (len(full.BodyHTML) - richtext.MaxInputBytes + 1023) / 1024
		note := richtext.Block{Runs: []richtext.Run{{Text: fmt.Sprintf("[messaggio troncato: %d KiB]", cut)}}}
		o = render.Stack(o, blankOut(), render.Dispatch(note, c))
	}
	return o
}

// fallbackBody e cio che si vede mentre il rendering a blocchi e in volo: un
// indicatore e l'inizio del testo semplice (BodyText), reso con lo stesso motore.
func (m Model) fallbackBody(full *msg.Message, width int) render.Out {
	text := full.BodyText
	if len(text) > fallbackBytes {
		cut := fallbackBytes
		for cut > 0 && text[cut]&0xC0 == 0x80 {
			cut--
		}
		text = text[:cut]
	}
	wait := render.Out{
		Lines: []string{m.st.Spinner.Render("⠋") + " " + m.st.ListDate.Render("rendering in corso…")},
		Zones: [][]render.Zone{nil},
	}
	c := render.Ctx{Width: width, St: m.st}
	return render.Stack(wait, blankOut(), stackBlocks(richtext.FromText(text), c))
}

// bodyWidth e la larghezza del corpo (prima del primo SetSize, un default).
func (m Model) bodyWidth() int {
	if m.width > 0 {
		return m.width
	}
	return defaultBodyWidth
}

// bodyOut ritorna il corpo reso di full: dalla cache, subito se il sorgente e
// piccolo, altrimenti il fallback e una Cmd in coda (renderQ, svuotata da
// RenderCmd). Ricevitore puntatore: accoda la Cmd e usa la cache condivisa.
func (m *Model) bodyOut(full *msg.Message) render.Out {
	w := m.bodyWidth()
	key := cacheKey{id: full.Ref.ID, width: w, theme: m.theme, variant: variantOf(full)}
	m.cache.use(key)
	if sourceLen(full) <= syncRenderLimit {
		return m.cache.lookup(key, func() render.Out { return buildBody(full, w, m.st) })
	}
	if o, ok := m.cache.get(key); ok {
		return o
	}
	if m.cache.markPending(key) {
		st, gen := m.st, m.gen
		m.renderQ = append(m.renderQ, func() tea.Msg {
			return RenderedMsg{ID: key.id, Width: w, Gen: gen, Variant: key.variant, Out: buildBody(full, w, st)}
		})
	}
	return m.fallbackBody(full, w)
}

// RenderCmd svuota e ritorna le Cmd di rendering in background accodate dagli
// ultimi refresh (nil se non ce ne sono). La radice la chiama dopo ogni
// aggiornamento del thread e la accoda alla Cmd di ritorno (D15-T5). Ricevitore
// puntatore: svuota la coda.
func (m *Model) RenderCmd() tea.Cmd {
	if len(m.renderQ) == 0 {
		return nil
	}
	cmd := m.renderQ[0]
	if len(m.renderQ) > 1 {
		cmd = tea.Batch(m.renderQ...)
	}
	m.renderQ = nil
	return cmd
}

// applyRendered accoglie un RenderedMsg: scartato se la generazione non e quella
// corrente (larghezza cambiata, thread sostituito o chiuso); altrimenti entra
// nella cache e il contenuto si ricalcola.
func (m *Model) applyRendered(r RenderedMsg) {
	if m.cache == nil || r.Gen != m.gen {
		return
	}
	key := cacheKey{id: r.ID, width: r.Width, theme: m.theme, variant: r.Variant}
	m.cache.donePending(key)
	m.cache.put(key, r.Out)
	m.refresh()
}
