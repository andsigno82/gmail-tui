package app

// Splash d'avvio e stati vuoti (Piano 15, Task 16b; richiesta di Andrea,
// 2026-11-23). Finché non arriva il primo caricamento della lista (dopo il wizard,
// se c'è) il corpo mostra al centro il titolo a blocchi con la sfumatura sobria
// primary->accent (D15-13), lo spinner con la fase corrente e la versione. NESSUN
// ritardo artificiale: sparisce appena msgsLoaded non stantio arriva. Se il Connect
// fallisce, al posto dello spinner stanno l'errore sanificato e "R per riprovare".
// Colori solo da slot del tema (stili di Styles); la sfumatura sta solo sulle
// lettere del titolo, come il titolo delle finestre (overlay).

import (
	"fmt"
	"image/color"
	"os"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// Soglie del titolo (Piano 15, Task 16b): da splashBigMin colonne il titolo grande
// (glifi a larghezza doppia), da splashCompactMin il compatto (glifi semplici),
// sotto solo testo. Il minimo dell'app (80 colonne) rientra sempre nel grande.
const (
	splashBigMin     = 78
	splashCompactMin = 37
	splashErrMaxW    = 70 // larghezza massima del testo d'errore
	splashErrMaxLen  = 300
)

// splashGlyphs è il carattere a blocchi 5 righe per ogni lettera di "gmail-tui".
var splashGlyphs = map[rune][5]string{
	'g': {"███", "█  ", "█ █", "█ █", "███"},
	'm': {"█▀▄▀█", "█ █ █", "█ █ █", "█   █", "█   █"},
	'a': {"███", "█ █", "███", "█ █", "█ █"},
	'i': {"███", " █ ", " █ ", " █ ", "███"},
	'l': {"█  ", "█  ", "█  ", "█  ", "███"},
	'-': {"   ", "   ", "███", "   ", "   "},
	't': {"███", " █ ", " █ ", " █ ", " █ "},
	'u': {"█ █", "█ █", "█ █", "█ █", "███"},
}

const splashWord = "gmail-tui"

// splashRows costruisce le 5 righe del titolo; wide raddoppia ogni cella (titolo
// grande, spazio fra le lettere di 2 colonne invece di 1).
func splashRows(wide bool) []string {
	gap := " "
	if wide {
		gap = "  "
	}
	rows := make([]string, 5)
	for i, r := range splashWord {
		g := splashGlyphs[r]
		for y := range rows {
			cell := g[y]
			if wide {
				var b strings.Builder
				for _, c := range cell {
					b.WriteRune(c)
					b.WriteRune(c)
				}
				cell = b.String()
			}
			if i > 0 {
				rows[y] += gap
			}
			rows[y] += cell
		}
	}
	return rows
}

// splashTitle rende il titolo per la larghezza data: grande, compatto o solo testo,
// con la sfumatura primary->accent lungo le colonne (primary da OverlayBorder,
// accent da Spinner, come il titolo delle finestre).
func splashTitle(st styles.Styles, width int) string {
	primary, accent := st.OverlayBorder.GetForeground(), st.Spinner.GetForeground()
	var rows []string
	switch {
	case width >= splashBigMin:
		rows = splashRows(true)
	case width >= splashCompactMin:
		rows = splashRows(false)
	default:
		return gradientRow(splashWord, primary, accent, true)
	}
	out := make([]string, len(rows))
	cols := lipgloss.Blend1D(lipgloss.Width(rows[0]), primary, accent)
	for y, row := range rows {
		var sb strings.Builder
		for x, r := range []rune(row) {
			if r == ' ' {
				sb.WriteRune(r)
				continue
			}
			sb.WriteString(lipgloss.NewStyle().Foreground(cols[x]).Render(string(r)))
		}
		out[y] = sb.String()
	}
	return strings.Join(out, "\n")
}

// gradientRow colora ogni lettera di s lungo la sfumatura a->b.
func gradientRow(s string, a, b color.Color, bold bool) string {
	runes := []rune(s)
	cols := lipgloss.Blend1D(len(runes), a, b)
	var sb strings.Builder
	for i, r := range runes {
		sb.WriteString(lipgloss.NewStyle().Foreground(cols[i]).Bold(bold).Render(string(r)))
	}
	return sb.String()
}

// SetVersion imposta la versione mostrata sotto lo spinner dello splash (il main
// passa quella iniettata a build time).
func (m *Model) SetVersion(v string) { m.version = v }

// splashOn dice se il corpo mostra lo splash: la lista non è ancora arrivata e
// non c'è il wizard.
func (m *Model) splashOn() bool {
	return !m.listLoaded && !m.wizardActive && m.view == viewInbox
}

// splashErrText rende l'errore del Connect sicuro da mostrare: la password App
// dell'ambiente mascherata, controlli, ANSI e bidi tolti, lunghezza limitata
// (AGENTS.md: i segreti restano mascherati in errori e log).
func splashErrText(err error) string {
	s := err.Error()
	if pw := os.Getenv("GMAIL_APP_PASSWORD"); pw != "" {
		s = strings.ReplaceAll(s, pw, "****")
		s = strings.ReplaceAll(s, strings.ReplaceAll(pw, " ", ""), "****")
	}
	return sanitizeNotifyText(s, splashErrMaxLen)
}

// splashView rende lo splash al centro del corpo (altezza = schermo - barra).
func (m *Model) splashView() string {
	var phase string
	switch {
	case m.connErr != "":
		w := splashErrMaxW
		if m.width > 0 {
			w = min(w, max(m.width-8, 10))
		}
		msg := m.styles.ErrText.Width(w).Align(lipgloss.Center).Render(m.connErr)
		phase = msg + "\n" + m.styles.ListDate.Render("R per riprovare")
	case !m.connected:
		phase = m.spinner.View() + " " + m.styles.ListDate.Render("connessione a imap.gmail.com…")
	default:
		phase = m.spinner.View() + " " + m.styles.ListDate.Render("caricamento posta…")
	}
	parts := []string{splashTitle(m.styles, m.width), "", phase}
	if m.version != "" {
		parts = append(parts, "", m.styles.ListDate.Render(m.version))
	}
	block := lipgloss.JoinVertical(lipgloss.Center, parts...)
	return m.center(block)
}

// center posiziona s al centro del corpo; senza size nota (prima del primo
// WindowSizeMsg) lo restituisce com'è.
func (m *Model) center(s string) string {
	if m.width <= 0 || m.height <= 1 {
		return s
	}
	return lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, s)
}

// folderLabel è il nome leggibile della cartella mostrata.
func (m *Model) folderLabel() string {
	if m.folder.Name == "" {
		return "Posta in arrivo"
	}
	return folderName(m.folder)
}

// listBody rende la lista nella colonna w x h: le righe, oppure se il caricamento
// è finito e la lista è davvero vuota un messaggio neutro centrato (mai il
// "No items." della lista né un testo che sembri un errore).
func (m *Model) listBody(w, h int) string {
	if !m.listLoaded || len(m.refs) > 0 {
		return m.inbox.View()
	}
	text := fmt.Sprintf("Nessun messaggio in %s", m.folderLabel())
	if m.queryRaw != "" {
		text = "Nessun messaggio corrisponde alla ricerca"
	}
	text = m.styles.ListDate.Render(text)
	if w <= 0 || h <= 0 {
		return text
	}
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, text)
}
