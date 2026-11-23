package thread

// Link nel corpo (richiesta di Andrea, 2026-11-23): gli URL del testo si aprono
// con il clic sinistro nel browser di sistema, e ogni porzione visibile e avvolta
// in OSC 8 per il cmd+clic nativo dei terminali (iTerm2, Ghostty, Kitty, WezTerm).
// Il contenuto e non fidato: solo http, https e mailto; ESC, BEL e spazi tolti
// prima di OGNI uso (OSC 8 e apertura); nessuna richiesta di rete dall'app.

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// maxLinkLen e la lunghezza massima (in byte) di un URL cliccabile: oltre resta testo.
const maxLinkLen = 2048

// urlRE trova gli URL nel testo: gli schemi ammessi e, dopo, tutto cio che non e
// spazio, controllo, angolare o virgoletta (le virgolette tipografiche incluse).
var urlRE = regexp.MustCompile(`(?i)(?:https?://|mailto:)[^\s\p{Z}<>"“”«»\p{C}]+`)

// LinkOpenedMsg e l'esito dell'apertura di un link nel browser: Target e la
// destinazione mostrata nel toast (host, o indirizzo per mailto).
type LinkOpenedMsg struct {
	Target string
	Err    error
}

// linkRange e un link nel testo logico di una riga: indici in rune [start,end),
// URL ripulito e id OSC 8 comune a tutte le porzioni del link.
type linkRange struct {
	start, end int
	url        string
	id         string
}

// linkSpan e la porzione visibile di un link in una riga: colonne [x0,x1).
type linkSpan struct {
	x0, x1 int
	url    string
}

// openLink apre un URL nel browser di sistema con il comando di sistema (mai una
// shell). Variabile per i test: nei test non si apre mai un browser vero.
var openLink = func(u string) error { return browserCmd(u).Run() }

// findLinks trova i link di una riga logica (testo gia pulito): solo URL validi,
// con la punteggiatura finale tolta. Gli indici sono in rune.
func findLinks(line string) []linkRange {
	var out []linkRange
	for _, loc := range urlRE.FindAllStringIndex(line, -1) {
		raw := trimURL(line[loc[0]:loc[1]])
		u, ok := linkTarget(raw)
		if !ok {
			continue
		}
		start := utf8.RuneCountInString(line[:loc[0]])
		out = append(out, linkRange{start: start, end: start + utf8.RuneCountInString(raw), url: u})
	}
	return out
}

// trimURL toglie la punteggiatura finale tipica di una frase (.,;:!?'"]>}*) e la
// parentesi di chiusura che non ha l'apertura dentro l'URL stesso.
func trimURL(s string) string {
	for s != "" {
		c := s[len(s)-1]
		switch {
		case strings.IndexByte(".,;:!?'\"]>}*", c) >= 0:
			s = s[:len(s)-1]
		case c == ')' && strings.Count(s, ")") > strings.Count(s, "("):
			s = s[:len(s)-1]
		default:
			return s
		}
	}
	return s
}

// cleanURL toglie da un URL i caratteri di controllo e gli spazi: un ESC o BEL nel
// contenuto non deve poter chiudere l'OSC 8 ne iniettare sequenze.
func cleanURL(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

// linkTarget ripulisce raw e dice se e un link apribile: schema http, https o
// mailto, con host (o indirizzo) e lunghezza entro maxLinkLen. Altrimenti false,
// e il testo resta semplice.
func linkTarget(raw string) (string, bool) {
	s := cleanURL(raw)
	if s == "" || len(s) > maxLinkLen {
		return "", false
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host == "" {
			return "", false
		}
	case "mailto":
		if u.Opaque == "" {
			return "", false
		}
	default:
		return "", false
	}
	return s, true
}

// linkLabel e la destinazione mostrata nel toast: l'host, o l'indirizzo per mailto.
func linkLabel(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return s
	}
	if u.Opaque != "" {
		return u.Opaque
	}
	return u.Host
}

// cleanBody toglie dal corpo cio che non e testo visibile: i ritorni a capo DOS,
// i tab (4 spazi) e i caratteri di controllo (ESC, BEL...), che un messaggio non
// fidato userebbe per iniettare sequenze nel terminale. Il newline resta.
func cleanBody(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\t", "    ")
	return strings.Map(func(r rune) rune {
		if r != '\n' && unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// wrapRow e una riga visibile del corpo: le sue rune e, per ogni rune, l'indice
// nel testo logico (serve a ritrovare i link dopo il a capo).
type wrapRow struct {
	runes []rune
	idx   []int
}

// runeW e la larghezza in colonne di una rune.
func runeW(r rune) int { return lipgloss.Width(string(r)) }

// wrapLine spezza una riga logica a capo sulla larghezza w: per parole (lo spazio
// al punto di a capo si scarta); una parola piu larga della riga va spezzata per
// carattere. Con w <= 0 nessun a capo. Una riga vuota dà una riga vuota.
func wrapLine(rs []rune, w int) []wrapRow {
	if w <= 0 {
		r := wrapRow{runes: rs, idx: make([]int, len(rs))}
		for i := range rs {
			r.idx[i] = i
		}
		return []wrapRow{r}
	}
	var rows []wrapRow
	var cur wrapRow
	curW := 0
	flush := func() {
		rows = append(rows, cur)
		cur = wrapRow{}
		curW = 0
	}
	add := func(i int) {
		cur.runes = append(cur.runes, rs[i])
		cur.idx = append(cur.idx, i)
		curW += runeW(rs[i])
	}
	for i := 0; i < len(rs); {
		sp := unicode.IsSpace(rs[i])
		j := i
		for j < len(rs) && unicode.IsSpace(rs[j]) == sp {
			j++
		}
		tw := 0
		for k := i; k < j; k++ {
			tw += runeW(rs[k])
		}
		switch {
		case curW+tw <= w:
			for k := i; k < j; k++ {
				add(k)
			}
		case tw <= w:
			flush()
			if !sp {
				for k := i; k < j; k++ {
					add(k)
				}
			}
		default:
			for k := i; k < j; k++ {
				if curW > 0 && curW+runeW(rs[k]) > w {
					flush()
				}
				add(k)
			}
		}
		i = j
	}
	if len(cur.runes) > 0 || len(rows) == 0 {
		flush()
	}
	return rows
}

// linkIndexAt e l'indice del link che copre l'indice logico i, o -1.
func linkIndexAt(links []linkRange, i int) int {
	for k, l := range links {
		if i >= l.start && i < l.end {
			return k
		}
	}
	return -1
}

// osc8Close chiude una porzione di link (sequenza OSC 8 con ST = ESC \).
const osc8Close = "\x1b]8;;\x1b\\"

// osc8Open apre una porzione di link: ESC ] 8 ; id=<id> ; <url> ESC \. Il link e
// gia ripulito da controlli e spazi (linkTarget), quindi non puo chiudere la sequenza.
func osc8Open(u, id string) string {
	return "\x1b]8;id=" + id + ";" + u + "\x1b\\"
}

// renderRow compone una riga: il testo nello stile base e ogni porzione di link in
// stile Link, avvolta in OSC 8 (lipgloss Style.Hyperlink). Ritorna il testo
// pre-stilizzato e le porzioni dei link in colonne della riga.
func (m Model) renderRow(r wrapRow, base lipgloss.Style, links []linkRange) (string, []linkSpan) {
	var b strings.Builder
	var spans []linkSpan
	col := 0
	for i := 0; i < len(r.runes); {
		k := linkIndexAt(links, r.idx[i])
		j := i + 1
		for j < len(r.runes) && linkIndexAt(links, r.idx[j]) == k {
			j++
		}
		text := string(r.runes[i:j])
		w := lipgloss.Width(text)
		if k < 0 {
			b.WriteString(base.Render(text))
		} else {
			l := links[k]
			b.WriteString(osc8Open(l.url, l.id) + m.st.Link.Render(text) + osc8Close)
			spans = append(spans, linkSpan{x0: col, x1: col + w, url: l.url})
		}
		col += w
		i = j
	}
	return b.String(), spans
}

// bodyRows compone le righe del corpo a capo sulla larghezza della vista. Le righe
// citate (che iniziano con ">") prendono lo stile Quoted. seq numera i link della
// vista: l'id OSC 8 e comune a tutte le porzioni dello stesso link, anche se il
// link va a capo (Richiesta di Andrea, 2026-11-23).
func (m Model) bodyRows(text string, seq *int) ([]string, [][]linkSpan) {
	var lines []string
	var spans [][]linkSpan
	for _, logical := range strings.Split(cleanBody(text), "\n") {
		links := findLinks(logical)
		for k := range links {
			*seq++
			links[k].id = "gmt" + strconv.Itoa(*seq)
		}
		base := lipgloss.NewStyle()
		if strings.HasPrefix(logical, ">") {
			base = m.st.Quoted
		}
		for _, row := range wrapLine([]rune(logical), m.width) {
			s, sp := m.renderRow(row, base, links)
			lines = append(lines, s)
			spans = append(spans, sp)
		}
	}
	return lines, spans
}

// LinkAt restituisce l'URL del link sotto la cella (x, y) della vista thread, con
// le coordinate relative all'origine della vista (y include l'header). Tiene conto
// dello scorrimento verticale e orizzontale del viewport.
func (m Model) LinkAt(x, y int) (string, bool) {
	if y < headerLines || y-headerLines >= m.bodyHeight() {
		return "", false
	}
	row := y - headerLines + m.vp.YOffset()
	if row < 0 || row >= len(m.linkRows) {
		return "", false
	}
	x += m.vp.XOffset()
	for _, s := range m.linkRows[row] {
		if x >= s.x0 && x < s.x1 {
			return s.url, true
		}
	}
	return "", false
}

// LinkClick: se il clic (x, y) cade su un link, la Cmd che lo apre nel browser di
// sistema e riporta LinkOpenedMsg. Altrimenti false: nessun effetto.
func (m Model) LinkClick(x, y int) (tea.Cmd, bool) {
	u, ok := m.LinkAt(x, y)
	if !ok {
		return nil, false
	}
	return func() tea.Msg {
		return LinkOpenedMsg{Target: linkLabel(u), Err: openLink(u)}
	}, true
}
