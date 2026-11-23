package app

// Finestre sovrapposte e barra di stato a segmenti (Piano 15, Task 16; D15-11, D15-12).
// Menu `,`, pannello label `l` e selettore cartelle `m` mantengono contenuto, tasti e
// comportamento: cambia solo il disegno, che passa da internal/ui/overlay (riquadro
// con ombra, centrato sopra la vista corrente). La barra di stato passa da
// internal/ui/statusline; le conferme modali restano barre colorate (model.go).

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/ui/overlay"
	"github.com/andsigno82/gmail-tui/internal/ui/statusline"
)

// window rende la finestra aperta in questo momento ("" se nessuna): stesso ordine
// di precedenza di prima (menu, pannello label, selettore cartelle).
func (m Model) window() string {
	switch {
	case m.menu.open:
		return m.settingsView()
	case m.labelPickOpen:
		return m.labelPickView()
	case m.folderOpen:
		return m.foldersView()
	}
	return ""
}

// windowFor incornicia content in un overlay.Window con titolo, largo quanto il
// contenuto piu largo (almeno minW colonne interne) e alto quanto le sue righe;
// larghezza e altezza non superano il corpo meno l'ombra.
func (m Model) windowFor(content, title string, minW int) string {
	lines := strings.Split(content, "\n")
	inner := max(minW, lipgloss.Width(title))
	for _, l := range lines {
		inner = max(inner, lipgloss.Width(l))
	}
	w, h := inner+4, len(lines)+2 // bordo + padding orizzontale; bordo verticale
	if title != "" {
		h++
	}
	if m.width > 0 {
		w = min(w, m.width-2)
	}
	if m.height > 1 {
		h = min(h, m.height-1-1) // corpo (senza barra) meno la riga d'ombra
	}
	return overlay.Window(content, title, m.styles, w, h)
}

// overlayWindow sovrappone win, centrata, alla vista base: la base si porta alla
// dimensione del corpo cosi il centro e quello dello schermo (senza barra). Senza
// size nota (prima del primo WindowSizeMsg) resta solo la finestra.
func (m Model) overlayWindow(base, win string) string {
	if m.width <= 0 {
		return win
	}
	h := max(m.height-1, 1)
	base = lipgloss.NewStyle().Width(m.width).Height(h).MaxWidth(m.width).MaxHeight(h).Render(base)
	x, y := overlay.Center(base, win)
	return overlay.Overlay(base, win, x, y, m.styles)
}

// modeName e il segmento "modo" della barra di stato.
func (m Model) modeName() string {
	switch {
	case m.menu.open:
		return "IMPOSTAZIONI"
	case m.labelPickOpen:
		return "ETICHETTE"
	case m.folderOpen:
		return "CARTELLE"
	}
	switch m.view {
	case viewThread:
		return "LETTURA"
	case viewSearch:
		return "RICERCA"
	case viewComposer:
		return "SCRIVI"
	case viewWizard:
		return "SETUP"
	}
	return "LISTA"
}

// segmentBar compone la barra di stato a segmenti: modo, cartella (nelle viste che
// ne hanno una), stato (selezione, spinner, testo persistente) e aiuto breve.
func (m Model) segmentBar() string {
	segs := []statusline.Segment{{Text: m.modeName(), Kind: statusline.SegMode}}
	switch m.view {
	case viewInbox, viewThread, viewSearch:
		name := "Posta in arrivo"
		if m.folder.Name != "" {
			name = folderName(m.folder)
		}
		segs = append(segs, statusline.Segment{Text: name, Kind: statusline.SegFolder})
	}
	var status []string
	if m.pending > 0 {
		// Spinner senza stile: il segmento porta il suo (il troncamento e per rune).
		sp := m.spinner
		sp.Style = lipgloss.NewStyle()
		status = append(status, strings.TrimSpace(sp.View()))
	}
	if n := len(m.sel); n > 0 {
		// Piano 10, Task 2: con selezione attiva la barra dice quante righe sono marcate.
		status = append(status, fmt.Sprintf("%d selezionate", n))
	}
	if m.status.text != "" {
		status = append(status, m.status.text)
	}
	segs = append(segs, statusline.Segment{Text: strings.Join(status, " "), Kind: statusline.SegStatus})
	width := m.width
	if width <= 0 {
		width = 200 // size non nota (prima del primo WindowSizeMsg): nessun troncamento
	}
	used := 0
	for _, s := range segs {
		if s.Text != "" {
			used += lipgloss.Width(s.Text) + 2 // testo e margine di una cella per lato
		}
	}
	segs = append(segs, statusline.Segment{Text: m.plainHelp(width - used - 2), Kind: statusline.SegHelp})
	return statusline.Line(segs, width, m.styles)
}

// plainHelp e l'help breve della vista corrente come testo semplice "tasto azione",
// separato da " · " e tagliato a voci intere in avail colonne (con "…" se ne
// restano fuori): lo stile lo da il segmento della barra.
func (m Model) plainHelp(avail int) string {
	var parts []string
	for _, b := range m.shortHelpFor(m.labelOpen, m.view) {
		if !b.Enabled() {
			continue
		}
		h := b.Help()
		if h.Key == "" && h.Desc == "" {
			continue
		}
		parts = append(parts, strings.TrimSpace(h.Key+" "+h.Desc))
	}
	const sep, ell = " · ", " …"
	out := ""
	for _, p := range parts {
		next := p
		if out != "" {
			next = out + sep + p
		}
		if lipgloss.Width(next) > avail {
			if out != "" && lipgloss.Width(out+ell) <= avail {
				return out + ell
			}
			return out
		}
		out = next
	}
	return out
}
