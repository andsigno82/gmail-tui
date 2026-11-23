package richtext

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// msgTestoTroncato dichiara il taglio del corpo testo oltre MaxInputBytes.
const msgTestoTroncato = "[messaggio troncato]"

// urlRE trova gli URL nel testo: stessi schemi e stessa esclusione di
// internal/ui/thread/links.go (urlRE); duplicata perche' richtext non importa
// la UI (Piano 15, Task 5).
var urlRE = regexp.MustCompile(`(?i)(?:https?://|mailto:)[^\s\p{Z}<>"“”«»\p{C}]+`)

// orderedRE riconosce "1. testo" e "1) testo".
var orderedRE = regexp.MustCompile(`^(\d{1,4})[.)] +(\S.*)$`)

// lineKind classifica una riga del corpo testo.
type lineKind uint8

const (
	linePlain lineKind = iota
	lineBlank
	lineBullet
	lineOrdered
)

// textLine e' una riga analizzata: livello di citazione, tipo e testo.
type textLine struct {
	quote    int
	kind     lineKind
	text     string
	num      int  // numero dichiarato (solo lineOrdered)
	indented bool // rientro di almeno due spazi (continuazione di elemento)
}

// FromText costruisce i blocchi dal corpo solo testo (Piano 15, Task 5): righe
// che iniziano con ">" (anche "> >") -> KindQuote annidate come le blockquote
// HTML (Depth 0-based, al massimo MaxDepth livelli); "- ", "* ", "• " ->
// elenco non ordinato; "1. " o "1) " -> elenco ordinato (Start = primo numero);
// http(s):// e mailto: -> Run con Href, punteggiatura finale fuori dal link; il
// resto -> paragrafi, con le righe a capo conservate (una riga vuota separa i
// paragrafi). Oltre MaxInputBytes il testo si taglia e un paragrafo finale lo
// dichiara.
func FromText(text string) []Block {
	truncated := false
	if len(text) > MaxInputBytes {
		cut := MaxInputBytes
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		text = text[:cut]
		truncated = true
	}
	raw := strings.Split(sanitizeText(text), "\n")
	lines := make([]textLine, len(raw))
	for i, l := range raw {
		lines[i] = parseLine(l)
	}
	blocks := buildText(lines, 0)
	if truncated {
		blocks = append(blocks, Block{Runs: []Run{{Text: msgTestoTroncato}}})
	}
	return blocks
}

// parseLine separa i marcatori di citazione e classifica il resto.
func parseLine(line string) textLine {
	l := textLine{}
	s := line
	for l.quote < MaxDepth {
		t := strings.TrimLeft(s, " ")
		if !strings.HasPrefix(t, ">") {
			break
		}
		l.quote++
		s = t[1:]
	}
	if l.quote > 0 {
		s = strings.TrimPrefix(s, " ") // lo spazio dopo ">" fa parte del marcatore
	}
	trimmed := strings.TrimSpace(s)
	l.indented = strings.HasPrefix(s, "  ") || strings.HasPrefix(s, "\t")
	switch {
	case trimmed == "":
		l.kind = lineBlank
	case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "), strings.HasPrefix(trimmed, "• "):
		_, rest, _ := strings.Cut(trimmed, " ")
		if rest = strings.TrimSpace(rest); rest != "" {
			l.kind, l.text = lineBullet, rest
			return l
		}
		l.kind, l.text = linePlain, strings.TrimRight(s, " ")
	default:
		if m := orderedRE.FindStringSubmatch(trimmed); m != nil {
			n, _ := strconv.Atoi(m[1])
			l.kind, l.num, l.text = lineOrdered, n, strings.TrimSpace(m[2])
			return l
		}
		l.kind, l.text = linePlain, strings.TrimRight(s, " ")
	}
	return l
}

// buildText costruisce i blocchi di un gruppo di righe al livello di citazione
// level: le righe con livello maggiore diventano una KindQuote ricorsiva.
func buildText(lines []textLine, level int) []Block {
	var (
		out     []Block
		paras   []string // righe del paragrafo in corso
		items   []string // elementi dell'elenco in corso
		ordered bool
		start   int
	)
	flushPara := func() {
		if len(paras) > 0 {
			out = append(out, Block{Runs: linkRuns(strings.Join(paras, "\n"))})
			paras = nil
		}
	}
	flushList := func() {
		if len(items) == 0 {
			return
		}
		l := Block{Kind: KindList, Ordered: ordered}
		if ordered {
			l.Start = start
		}
		for _, it := range items {
			l.Children = append(l.Children, Block{Runs: linkRuns(it)})
		}
		out = append(out, l)
		items = nil
	}
	for i := 0; i < len(lines); {
		l := lines[i]
		if l.quote > level {
			flushPara()
			flushList()
			j := i
			for j < len(lines) && lines[j].quote > level {
				j++
			}
			out = append(out, Block{Kind: KindQuote, Depth: level, Children: buildText(lines[i:j], level+1)})
			i = j
			continue
		}
		i++
		switch l.kind {
		case lineBlank:
			flushPara()
			flushList()
		case lineBullet, lineOrdered:
			flushPara()
			isOrdered := l.kind == lineOrdered
			if len(items) > 0 && ordered != isOrdered {
				flushList()
			}
			if len(items) == 0 {
				ordered, start = isOrdered, l.num
			}
			items = append(items, l.text)
		default:
			if len(items) > 0 && l.indented { // continuazione dell'elemento
				items[len(items)-1] += " " + strings.TrimSpace(l.text)
				continue
			}
			flushList()
			paras = append(paras, l.text)
		}
	}
	flushPara()
	flushList()
	return out
}

// linkRuns divide il testo in run: gli URL ammessi (D15-T13) diventano link con
// la punteggiatura finale lasciata fuori.
func linkRuns(s string) []Run {
	var runs []Run
	last := 0
	for _, loc := range urlRE.FindAllStringIndex(s, -1) {
		raw := trimURL(s[loc[0]:loc[1]])
		href, ok := sanitizeURL(raw)
		if !ok {
			continue
		}
		if loc[0] > last {
			runs = append(runs, Run{Text: s[last:loc[0]]})
		}
		runs = append(runs, Run{Text: raw, Href: href})
		last = loc[0] + len(raw)
	}
	if last < len(s) {
		runs = append(runs, Run{Text: s[last:]})
	}
	return runs
}

// trimURL toglie la punteggiatura finale tipica di una frase (.,;:!?'"]>}*) e la
// parentesi di chiusura che non ha l'apertura dentro l'URL stesso (come trimURL
// in internal/ui/thread/links.go).
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
