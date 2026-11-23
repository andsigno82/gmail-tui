package render

import (
	"fmt"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/msg/richtext"
)

// Paragraph rende i Runs di un paragrafo (KindParagraph) o di un blocco
// preformattato (KindPre) a capo sulla larghezza (Piano 15, Task 14, lavoro A).
// E il punto d'ingresso del motore unico dei Runs: citazioni, elenchi e tabelle
// usano lo stesso percorso (wrapRuns/preRuns), quindi gli stessi controlli sui
// link (href sicuro, dominio sospetto, OSC 8, zone) e la stessa sanificazione.
// I Children (blocchi annidati sotto il testo, come gli elementi di elenco HTML)
// si rendono sotto il testo con Dispatch.
func Paragraph(b richtext.Block, c Ctx) Out { return paragraphWith(b, c, lipgloss.NewStyle()) }

// paragraphWith e Paragraph con lo stile base del testo semplice scelto dal
// chiamante (Quoted dentro una citazione).
func paragraphWith(b richtext.Block, c Ctx, base lipgloss.Style) Out {
	if c.Width <= 0 {
		return Out{}
	}
	var o Out
	if b.Kind == richtext.KindPre {
		o = preRuns(toRuns(b.Runs), c.Width, base, c)
	} else {
		o = wrapRuns(toRuns(b.Runs), c.Width, base, c)
	}
	if len(b.Children) == 0 {
		return o
	}
	parts := []Out{o}
	for _, ch := range b.Children {
		parts = append(parts, Dispatch(ch, c))
	}
	return Stack(parts...)
}

// ---- testo in linea: a capo, link e zone (Piano 15, Task 8) ----
//
// Motore unico dei Runs (Piano 15, Task 14, lavoro A): Paragraph, Quote, List e
// Table lo usano per il testo in linea. I link passano da prepLink (link.go, la
// stessa logica di Link: href sicuro, dominio sospetto con marcatore, OSC 8): se
// sospetti, l'id OSC 8 porta il prefisso idPrefixWarn e lo stile e LinkWarn. Le
// zone non si calcolano da indici logici ma si leggono dalle righe gia rese
// (scanZones), cosi restano giuste dopo a capo, bordi, rientri e barre.

// irun e un tratto di testo con il suo stile e, se link, href sanificato e id OSC 8.
type irun struct {
	text, href, id string
	st             *lipgloss.Style // stile del solo testo semplice; nil = base
}

// word e una parola non spezzabile (uno o piu tratti senza spazio in mezzo);
// gap = c'era spazio prima.
type word struct {
	gap  bool
	segs []irun
}

func (w word) width() int {
	n := 0
	for _, s := range w.segs {
		n += lipgloss.Width(s.text)
	}
	return n
}

// toRuns converte i Runs del modello (Piano 15, Task 14): un href non apribile
// (schema vietato, senza host, troppo lungo) lascia testo semplice senza zona;
// altrimenti l'href e ripulito (non puo chiudere o iniettare sequenze OSC 8,
// D15-T13), il testo vuoto mostra l'URL e un dominio sospetto prende il
// marcatore. L'id OSC 8 e comune a tutte le porzioni dello stesso tratto.
func toRuns(rs []richtext.Run) []irun {
	out := make([]irun, 0, len(rs))
	for i, r := range rs {
		if r.Href == "" {
			out = append(out, irun{text: r.Text})
			continue
		}
		li, ok := prepLink(r.Text, r.Href)
		if !ok {
			out = append(out, irun{text: r.Text})
			continue
		}
		text := r.Text
		if cleanText(text) == "" {
			text = li.display
		}
		if li.warn {
			text = warnMarker + text
		}
		out = append(out, irun{text: text, href: li.target, id: fmt.Sprintf("%s.%d", linkID(li), i)})
	}
	return out
}

// isWide dice se la runa occupa due colonne (CJK): tra due di esse si puo andare
// a capo anche senza spazi.
func isWide(r rune) bool {
	return r >= 0x1100 && lipgloss.Width(string(r)) >= 2
}

// tokenize divide i tratti in parole: spazi collassati, controlli scartati (un ESC
// nel contenuto non deve iniettare sequenze), ogni carattere largo e una parola.
func tokenize(runs []irun) []word {
	var words []word
	var cur word
	pending := false
	flush := func() {
		if len(cur.segs) > 0 {
			words = append(words, cur)
			cur = word{}
		}
	}
	for _, r := range runs {
		var buf strings.Builder
		emit := func() {
			if buf.Len() > 0 {
				cur.segs = append(cur.segs, irun{text: buf.String(), href: r.href, id: r.id, st: r.st})
				buf.Reset()
			}
		}
		for _, ch := range r.text {
			switch {
			case unicode.IsSpace(ch):
				emit()
				flush()
				pending = true
			case unicode.IsControl(ch), unicode.Is(unicode.Cf, ch): // bidi e zero-width: D15-T13
			case isWide(ch):
				emit()
				flush()
				words = append(words, word{gap: pending, segs: []irun{{text: string(ch), href: r.href, id: r.id, st: r.st}}})
				pending = false
			default:
				if len(cur.segs) == 0 && buf.Len() == 0 {
					cur.gap = pending
					pending = false
				}
				buf.WriteRune(ch)
			}
		}
		emit()
	}
	flush()
	return words
}

// hardSplit spezza i tratti in righe di al piu width colonne, almeno una runa
// per riga (parole piu larghe della riga, testo preformattato).
func hardSplit(segs []irun, width int) [][]irun {
	width = max(1, width)
	var lines [][]irun
	var cur []irun
	curW := 0
	for _, s := range segs {
		var buf strings.Builder
		for _, r := range s.text {
			rw := lipgloss.Width(string(r))
			if curW+rw > width && (curW > 0 || buf.Len() > 0) {
				if buf.Len() > 0 {
					cur = append(cur, irun{text: buf.String(), href: s.href, id: s.id, st: s.st})
					buf.Reset()
				}
				lines = append(lines, cur)
				cur, curW = nil, 0
			}
			buf.WriteRune(r)
			curW += rw
		}
		if buf.Len() > 0 {
			cur = append(cur, irun{text: buf.String(), href: s.href, id: s.id, st: s.st})
		}
	}
	return append(lines, cur)
}

func segsWidth(segs []irun) int {
	n := 0
	for _, s := range segs {
		n += lipgloss.Width(s.text)
	}
	return n
}

// wrapWords riempie le righe in modo goloso; lo spazio tra due parole dello
// stesso link fa parte del link.
func wrapWords(words []word, width int) [][]irun {
	width = max(1, width)
	var lines [][]irun
	var cur []irun
	curW := 0
	for _, w := range words {
		ww := w.width()
		gap := 0
		if w.gap && curW > 0 {
			gap = 1
		}
		if curW > 0 && curW+gap+ww > width {
			lines = append(lines, cur)
			cur, curW, gap = nil, 0, 0
		}
		if ww > width {
			parts := hardSplit(w.segs, width)
			lines = append(lines, parts[:len(parts)-1]...)
			cur = parts[len(parts)-1]
			curW = segsWidth(cur)
			continue
		}
		if gap == 1 {
			sp := irun{text: " "}
			if last, first := cur[len(cur)-1], w.segs[0]; last.href != "" && last.id == first.id {
				sp.href, sp.id = last.href, last.id
			}
			cur = append(cur, sp)
			curW++
		}
		cur = append(cur, w.segs...)
		curW += ww
	}
	if len(cur) > 0 {
		lines = append(lines, cur)
	}
	return lines
}

// osc8Open e osc8Close sono in link.go (Task 6): stessa sequenza OSC 8 con ST.

// renderSegs compone una riga: testo semplice in base (o nello stile del tratto),
// link in St.Link avvolto in OSC 8.
func renderSegs(line []irun, base lipgloss.Style, c Ctx) string {
	var b strings.Builder
	for i := 0; i < len(line); {
		s := line[i]
		j := i + 1
		text := s.text
		for j < len(line) && line[j].href == s.href && line[j].id == s.id && line[j].st == s.st {
			text += line[j].text
			j++
		}
		switch {
		case s.href != "":
			b.WriteString(osc8Open(s.href, s.id) + linkStyle(c.St, strings.HasPrefix(s.id, idPrefixWarn)).Render(text) + osc8Close)
		case s.st != nil:
			b.WriteString(s.st.Render(text))
		default:
			b.WriteString(base.Render(text))
		}
		i = j
	}
	return b.String()
}

func outOf(lines [][]irun, base lipgloss.Style, c Ctx) Out {
	strs := make([]string, len(lines))
	for i, l := range lines {
		strs[i] = renderSegs(l, base, c)
	}
	return Out{Lines: strs, Zones: scanZones(strs)}
}

// wrapRuns rende i tratti a capo su width colonne (Piano 15, Task 8).
func wrapRuns(runs []irun, width int, base lipgloss.Style, c Ctx) Out {
	var lines [][]irun
	hard := splitHard(runs)
	if len(hard) == 1 && len(hard[0]) == 0 {
		return Out{} // nessun testo: nessuna riga
	}
	for _, ln := range hard {
		wrapped := wrapWords(tokenize(ln), width)
		if len(wrapped) == 0 {
			wrapped = [][]irun{nil} // riga vuota voluta
		}
		lines = append(lines, wrapped...)
	}
	return outOf(lines, base, c)
}

// splitHard divide i tratti ai "\n" (a capo voluti: <br> in HTML, righe del testo
// semplice, Piano 15, Task 14); gli a capo finali si scartano. Ogni pezzo tiene
// href e id del tratto da cui viene.
func splitHard(runs []irun) [][]irun {
	lines := [][]irun{nil}
	for _, r := range runs {
		for i, part := range strings.Split(r.text, "\n") {
			if i > 0 {
				lines = append(lines, nil)
			}
			if part != "" {
				p := r
				p.text = part
				lines[len(lines)-1] = append(lines[len(lines)-1], p)
			}
		}
	}
	for len(lines) > 1 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// preRuns rende testo preformattato: a capo del modello conservati, spazi
// intatti, righe lunghe spezzate sulla larghezza.
func preRuns(runs []irun, width int, base lipgloss.Style, c Ctx) Out {
	var lines [][]irun
	var cur []irun
	for _, r := range runs {
		var buf strings.Builder
		for _, ch := range r.text {
			switch {
			case ch == '\n':
				if buf.Len() > 0 {
					cur = append(cur, irun{text: buf.String(), href: r.href, id: r.id, st: r.st})
					buf.Reset()
				}
				lines = append(lines, hardSplit(cur, width)...)
				cur = nil
			case ch == '\t':
				buf.WriteString("    ")
			case unicode.IsControl(ch):
			default:
				buf.WriteRune(ch)
			}
		}
		if buf.Len() > 0 {
			cur = append(cur, irun{text: buf.String(), href: r.href, id: r.id, st: r.st})
		}
	}
	if len(cur) > 0 {
		lines = append(lines, hardSplit(cur, width)...)
	}
	return outOf(lines, base, c)
}

// scanZones legge le zone dei link dalle righe gia rese: le colonne sono celle
// del terminale (lipgloss.Width), quindi sono giuste dopo bordi, rientri, barre e
// a capo. Una porzione di link ha X0 < X1; un link aperto a fine riga si chiude
// alla fine della riga (Piano 15, Task 8).
func scanZones(lines []string) [][]Zone {
	const open = "\x1b]8;"
	zones := make([][]Zone, len(lines))
	for i, l := range lines {
		col, start := 0, -1
		var cur Zone
		rest := l
		for {
			k := strings.Index(rest, open)
			if k < 0 {
				col += lipgloss.Width(rest)
				break
			}
			col += lipgloss.Width(rest[:k])
			rest = rest[k+len(open):]
			end, stLen := strings.Index(rest, "\x1b\\"), 2
			if end < 0 {
				if end, stLen = strings.IndexByte(rest, 7), 1; end < 0 {
					break
				}
			}
			params, uri, _ := strings.Cut(rest[:end], ";")
			rest = rest[end+stLen:]
			if start >= 0 && col > start {
				cur.X0, cur.X1 = start, col
				zones[i] = append(zones[i], cur)
			}
			start = -1
			if uri != "" {
				cur = Zone{Kind: ZoneLink, URL: uri}
				for _, p := range strings.Split(params, ":") {
					if v, ok := strings.CutPrefix(p, "id="); ok {
						cur.ID = v
						cur.Warn = strings.HasPrefix(v, idPrefixWarn)
					}
				}
				start = col
			}
		}
		if start >= 0 && col > start {
			cur.X0, cur.X1 = start, col
			zones[i] = append(zones[i], cur)
		}
	}
	return zones
}
