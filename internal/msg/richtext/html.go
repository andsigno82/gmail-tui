package richtext

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Messaggi dichiarati nel corpo quando un limite di D15-T4 taglia contenuto:
// il troncamento non e' mai silenzioso (Piano 15, Task 4).
const (
	msgTroppiNodi = "[contenuto troncato: troppi elementi]"
)

// FromHTML costruisce i blocchi da BodyHTML (Piano 15, Task 4, D15-T1).
// truncated e' true se l'input superava MaxInputBytes: si parsano solo i primi
// MaxInputBytes (taglio su confine di rune). Nessun CSS e' eseguito, nessun
// <style> applicato, il src delle immagini non e' mai letto (nessuna rete).
// Limiti D15-T4: oltre MaxNodes nodi il resto e' ignorato e un paragrafo finale
// lo dichiara; citazioni ed elenchi annidati oltre MaxDepth si appiattiscono in
// testo; le tabelle si troncano a MaxTableRows x MaxTableCols con dichiarazione.
func FromHTML(src string) (blocks []Block, truncated bool) {
	if len(src) > MaxInputBytes {
		cut := MaxInputBytes
		for cut > 0 && !utf8.RuneStart(src[cut]) {
			cut--
		}
		src = src[:cut]
		truncated = true
	}
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return nil, truncated
	}
	w := &walker{}
	blocks = w.blocks(doc, 0, 0)
	if w.overflow {
		blocks = append(blocks, Block{Runs: []Run{{Text: msgTroppiNodi}}})
	}
	return blocks, truncated
}

// walker percorre l'albero HTML contando i nodi visitati (MaxNodes).
type walker struct {
	nodes    int
	overflow bool
}

// tick conta un nodo; false = budget esaurito, il chiamante smette di scendere.
func (w *walker) tick() bool {
	if w.overflow {
		return false
	}
	w.nodes++
	if w.nodes > MaxNodes {
		w.overflow = true
		return false
	}
	return true
}

// builder accumula il testo in linea corrente (runs) e i blocchi finiti (out).
type builder struct {
	out     []Block
	runs    []Run
	href    string // href del link in corso (vuoto fuori da un link)
	pre     bool   // true dentro <pre>: spazi e a capo conservati
	visible int    // byte non spazio aggiunti finora (serve ai link senza testo)
}

// add aggiunge testo gia' pulito, unendolo al run precedente se ha lo stesso href.
func (b *builder) add(s string) {
	if s == "" {
		return
	}
	b.visible += len(strings.TrimSpace(s))
	if n := len(b.runs); n > 0 && b.runs[n-1].Href == b.href {
		b.runs[n-1].Text += s
		return
	}
	b.runs = append(b.runs, Run{Text: s, Href: b.href})
}

// endsWithSpace dice se il testo accumulato finisce con uno spazio.
func (b *builder) endsWithSpace() bool {
	if len(b.runs) == 0 {
		return true // a inizio paragrafo lo spazio iniziale si scarta
	}
	t := b.runs[len(b.runs)-1].Text
	return t == "" || t[len(t)-1] == ' '
}

// text aggiunge un nodo di testo: sanifica (D15-T13) e, fuori da <pre>,
// comprime ogni sequenza di spazi in uno solo come fa HTML.
func (b *builder) text(s string) {
	s = sanitizeText(s)
	if b.pre {
		b.add(s)
		return
	}
	var sb strings.Builder
	space := b.endsWithSpace()
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !space {
				sb.WriteByte(' ')
				space = true
			}
			continue
		}
		sb.WriteRune(r)
		space = false
	}
	b.add(sb.String())
}

// sep e' un separatore in linea (<br> o confine di blocco appiattito).
func (b *builder) sep() {
	if b.pre {
		b.add("\n")
		return
	}
	b.text(" ")
}

// takeRuns ritorna i run accumulati ripuliti (niente spazio finale, niente run
// vuoti) e azzera l'accumulatore.
func (b *builder) takeRuns() []Run {
	runs := b.runs
	b.runs = nil
	b.visible = 0
	if len(runs) == 0 {
		return nil
	}
	last := &runs[len(runs)-1]
	if b.pre {
		last.Text = strings.TrimRight(last.Text, "\n")
	} else {
		last.Text = strings.TrimRight(last.Text, " ")
	}
	out := runs[:0]
	for _, r := range runs {
		if r.Text != "" {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// flush chiude il paragrafo (o <pre>) in corso, se ha contenuto.
func (b *builder) flush() {
	runs := b.takeRuns()
	if runs == nil {
		return
	}
	kind := KindParagraph
	if b.pre {
		kind = KindPre
	}
	b.out = append(b.out, Block{Kind: kind, Runs: runs})
}

// attr ritorna il valore dell'attributo (vuoto se assente).
func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// hasAttr dice se l'attributo e' presente (anche vuoto).
func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

// skipAtoms: elementi il cui contenuto non si mostra mai (D15-T1).
var skipAtoms = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Head: true, atom.Title: true,
	atom.Noscript: true, atom.Iframe: true, atom.Object: true, atom.Embed: true,
	atom.Svg: true, atom.Form: true, atom.Template: true, atom.Link: true,
	atom.Meta: true, atom.Base: true, atom.Canvas: true, atom.Audio: true,
	atom.Video: true, atom.Textarea: true, atom.Select: true, atom.Math: true,
}

// blockAtoms: elementi che aprono un nuovo blocco quando incontrati in linea.
var blockAtoms = map[atom.Atom]bool{
	atom.Address: true, atom.Article: true, atom.Aside: true, atom.Body: true,
	atom.Caption: true, atom.Center: true, atom.Dd: true, atom.Details: true,
	atom.Dir: true, atom.Div: true, atom.Dl: true, atom.Dt: true,
	atom.Fieldset: true, atom.Figcaption: true, atom.Figure: true,
	atom.Footer: true, atom.H1: true, atom.H2: true, atom.H3: true,
	atom.H4: true, atom.H5: true, atom.H6: true, atom.Header: true,
	atom.Hgroup: true, atom.Html: true, atom.Legend: true, atom.Li: true,
	atom.Main: true, atom.Menu: true, atom.Nav: true, atom.P: true,
	atom.Section: true, atom.Summary: true, atom.Tbody: true, atom.Td: true,
	atom.Tfoot: true, atom.Th: true, atom.Thead: true, atom.Tr: true,
	// questi hanno un trattamento proprio in node; qui servono a inline.
	atom.Blockquote: true, atom.Ul: true, atom.Ol: true, atom.Table: true,
	atom.Pre: true, atom.Hr: true,
}

// skip dice se il nodo non va visitato: elementi non visibili o con attributo
// hidden (nessun CSS valutato: solo l'attributo HTML).
func skip(n *html.Node) bool {
	return n.Type == html.ElementNode && (skipAtoms[n.DataAtom] || hasAttr(n, "hidden"))
}

// blocks converte i figli di parent in blocchi. depth conta l'annidamento di
// blocchi (citazioni, elenchi: oltre MaxDepth si appiattisce); quote e' il
// livello di citazione corrente (Depth di KindQuote).
func (w *walker) blocks(parent *html.Node, depth, quote int) []Block {
	b := &builder{}
	for c := parent.FirstChild; c != nil && w.tick(); c = c.NextSibling {
		w.node(b, c, depth, quote)
	}
	b.flush()
	return b.out
}

// node tratta un nodo in modalita' a blocchi.
func (w *walker) node(b *builder, n *html.Node, depth, quote int) {
	switch n.Type {
	case html.TextNode:
		b.text(n.Data)
		return
	case html.ElementNode:
	default:
		return // commenti (anche condizionali Outlook), doctype
	}
	if skip(n) {
		return
	}
	switch n.DataAtom {
	case atom.Br:
		b.flush()
	case atom.Hr:
		b.flush()
		b.out = append(b.out, Block{Kind: KindRule})
	case atom.Img:
		w.image(b, n)
	case atom.A:
		if isButton(n) {
			if blk, ok := w.button(n); ok {
				b.flush()
				b.out = append(b.out, blk)
				return
			}
		}
		w.link(b, n)
	case atom.Pre:
		b.flush()
		sub := &builder{pre: true}
		w.collect(sub, n)
		sub.flush()
		b.out = append(b.out, sub.out...)
	case atom.Blockquote:
		b.flush()
		if depth >= MaxDepth { // appiattita: il testo resta, senza barra
			w.collect(b, n)
			b.flush()
			return
		}
		if kids := w.blocks(n, depth+1, quote+1); len(kids) > 0 {
			b.out = append(b.out, Block{Kind: KindQuote, Depth: quote, Children: kids})
		}
	case atom.Ul, atom.Ol:
		b.flush()
		w.list(b, n, depth, quote)
	case atom.Table:
		b.flush()
		w.table(b, n, depth, quote)
	default:
		if blockAtoms[n.DataAtom] {
			b.flush()
			b.out = append(b.out, w.blocks(n, depth, quote)...)
			return
		}
		// elemento in linea (span, b, font...): puo' contenere blocchi o
		// tabelle (<font><table>), quindi si resta in modalita' a blocchi.
		for c := n.FirstChild; c != nil && w.tick(); c = c.NextSibling {
			w.node(b, c, depth, quote)
		}
	}
}

// collect appiattisce un sottoalbero in linea nel builder: testo, link e alt
// delle immagini, i confini di blocco diventano spazi. Usato per link, celle di
// tabella dati, <pre> e annidamenti oltre il limite.
func (w *walker) collect(b *builder, n *html.Node) {
	for c := n.FirstChild; c != nil && w.tick(); c = c.NextSibling {
		w.inline(b, c)
	}
}

func (w *walker) inline(b *builder, n *html.Node) {
	switch n.Type {
	case html.TextNode:
		b.text(n.Data)
	case html.ElementNode:
		if skip(n) {
			return
		}
		switch {
		case n.DataAtom == atom.Br:
			b.sep()
		case n.DataAtom == atom.Img:
			if !isPixel(n) {
				b.text(oneLine(attr(n, "alt")))
			}
		case n.DataAtom == atom.A:
			w.link(b, n)
		case blockAtoms[n.DataAtom]:
			b.sep()
			w.collect(b, n)
			b.sep()
		default:
			w.collect(b, n)
		}
	}
}

// link aggiunge il contenuto di un <a> come run con href sanificato; se l'href
// non e' ammesso (D15-T13) resta solo il testo. Un link senza testo visibile
// mostra l'href stesso, cosi' la destinazione reale e' sempre leggibile.
func (w *walker) link(b *builder, a *html.Node) {
	href, ok := sanitizeURL(attr(a, "href"))
	if !ok {
		w.collect(b, a)
		return
	}
	prev, before := b.href, b.visible
	b.href = href
	w.collect(b, a)
	if b.visible == before {
		b.add(href)
	}
	b.href = prev
}

// oneLine riduce un testo a una riga sanificata (alt delle immagini).
func oneLine(s string) string {
	return strings.Join(strings.Fields(sanitizeText(s)), " ")
}

// isPixel riconosce i pixel traccianti (larghezza o altezza <= 2): scartati.
func isPixel(n *html.Node) bool {
	for _, k := range []string{"width", "height"} {
		v := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(attr(n, k))), "px")
		if px, err := strconv.Atoi(v); err == nil && px <= 2 {
			return true
		}
	}
	return false
}

// image emette un chip immagine con il solo alt: il src non si legge (D15-08).
func (w *walker) image(b *builder, n *html.Node) {
	if isPixel(n) {
		return
	}
	b.flush()
	b.out = append(b.out, Block{Kind: KindImage, Alt: oneLine(attr(n, "alt"))})
}

// button costruisce il bottone da un <a> (o dal link dentro un <td>). ok e'
// false se manca un href ammesso o un'etichetta: il nodo resta un link normale.
func (w *walker) button(n *html.Node) (Block, bool) {
	a := n
	if n.DataAtom != atom.A {
		a = findLink(n)
	}
	if a == nil {
		return Block{}, false
	}
	href, ok := sanitizeURL(attr(a, "href"))
	if !ok {
		return Block{}, false
	}
	sub := &builder{}
	w.collect(sub, a)
	label := ""
	for _, r := range sub.takeRuns() {
		label += r.Text
	}
	if label == "" {
		return Block{}, false
	}
	return Block{
		Kind: KindButton,
		Runs: []Run{{Text: label, Href: href}},
		Host: hostOf(href),
		Warn: DomainMismatch(label, href),
	}, true
}

// findLink ritorna il primo <a href> del sottoalbero.
func findLink(n *html.Node) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || skip(c) {
			continue
		}
		if c.DataAtom == atom.A && attr(c, "href") != "" {
			return c
		}
		if l := findLink(c); l != nil {
			return l
		}
	}
	return nil
}

// hostOf e' il dominio di destinazione di un href gia' sanificato: l'host per
// http(s), il dominio dell'indirizzo per mailto; minuscolo.
func hostOf(href string) string {
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	if strings.EqualFold(u.Scheme, "mailto") {
		addr := u.Opaque
		if i := strings.IndexByte(addr, '?'); i >= 0 {
			addr = addr[:i]
		}
		if i := strings.LastIndexByte(addr, '@'); i >= 0 {
			return strings.ToLower(addr[i+1:])
		}
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// list converte <ul>/<ol>. Ogni elemento e' un Block paragrafo: Runs = testo
// dell'elemento, Children = blocchi annidati (elenchi, citazioni...). Oltre
// MaxDepth l'elenco si appiattisce in testo.
func (w *walker) list(b *builder, n *html.Node, depth, quote int) {
	if depth >= MaxDepth {
		w.collect(b, n)
		b.flush()
		return
	}
	blk := Block{Kind: KindList, Ordered: n.DataAtom == atom.Ol}
	if blk.Ordered {
		blk.Start = 1
		if v, err := strconv.Atoi(strings.TrimSpace(attr(n, "start"))); err == nil && v >= 0 {
			blk.Start = v
		}
	}
	for c := n.FirstChild; c != nil && w.tick(); c = c.NextSibling {
		if c.Type != html.ElementNode || skip(c) {
			continue
		}
		switch c.DataAtom {
		case atom.Li:
			if item, ok := w.item(c, depth, quote); ok {
				blk.Children = append(blk.Children, item)
			}
		case atom.Ul, atom.Ol:
			// elenco annidato direttamente sotto <ul> (HTML non valido ma
			// diffuso): sta sotto l'elemento precedente.
			sub := &builder{}
			w.list(sub, c, depth+1, quote)
			if len(blk.Children) == 0 {
				blk.Children = append(blk.Children, Block{})
			}
			last := &blk.Children[len(blk.Children)-1]
			last.Children = append(last.Children, sub.out...)
		}
	}
	if len(blk.Children) > 0 {
		b.out = append(b.out, blk)
	}
}

// item converte un <li>.
func (w *walker) item(li *html.Node, depth, quote int) (Block, bool) {
	kids := w.blocks(li, depth+1, quote)
	if len(kids) == 0 {
		return Block{}, false
	}
	var item Block
	if kids[0].Kind == KindParagraph {
		item = kids[0]
		kids = kids[1:]
	}
	if len(kids) > 0 {
		item.Children = kids
	}
	return item, true
}

// rowsOf ritorna le righe della tabella (senza scendere nelle tabelle annidate).
func rowsOf(table *html.Node) []*html.Node {
	var rows []*html.Node
	for c := table.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}
		switch c.DataAtom {
		case atom.Tr:
			rows = append(rows, c)
		case atom.Thead, atom.Tbody, atom.Tfoot:
			for r := c.FirstChild; r != nil; r = r.NextSibling {
				if r.Type == html.ElementNode && r.DataAtom == atom.Tr {
					rows = append(rows, r)
				}
			}
		}
	}
	return rows
}

// cellsOf ritorna le celle <td>/<th> di una riga.
func cellsOf(tr *html.Node) []*html.Node {
	var cells []*html.Node
	for c := tr.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && (c.DataAtom == atom.Td || c.DataAtom == atom.Th) && !skip(c) {
			cells = append(cells, c)
		}
	}
	return cells
}

// hasStructure dice se il sottoalbero contiene blocchi, tabelle o bottoni:
// segno che la tabella e' di impaginazione e non una tabella di dati.
func hasStructure(n *html.Node) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || skip(c) {
			continue
		}
		switch c.DataAtom {
		case atom.Table, atom.P, atom.Div, atom.Ul, atom.Ol, atom.Blockquote,
			atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Hr, atom.Pre:
			return true
		}
		if isButton(c) || hasStructure(c) {
			return true
		}
	}
	return false
}

// table converte una <table>. Le newsletter impaginano con tabelle: se una
// cella contiene blocchi, tabelle o bottoni, o la tabella ha una cella sola, e'
// di layout e le celle diventano blocchi in sequenza. Altrimenti e' una
// KindTable con celle in linea (tabelle annidate appiattite in testo), troncata
// a MaxTableRows x MaxTableCols con dichiarazione (D15-T4).
func (w *walker) table(b *builder, n *html.Node, depth, quote int) {
	rows := rowsOf(n)
	nCells, layout := 0, false
	for _, tr := range rows {
		for _, c := range cellsOf(tr) {
			nCells++
			if !layout && (isButton(c) || hasStructure(c)) {
				layout = true
			}
		}
	}
	if nCells == 0 {
		return
	}
	if nCells == 1 {
		layout = true
	}
	if layout {
		for _, tr := range rows {
			if !w.tick() {
				return
			}
			for _, c := range cellsOf(tr) {
				if !w.tick() {
					return
				}
				w.cell(b, c, depth, quote)
			}
		}
		return
	}

	tbl := Block{Kind: KindTable}
	trunc := len(rows) > MaxTableRows
	for i, tr := range rows {
		if i >= MaxTableRows || !w.tick() {
			break
		}
		cells := cellsOf(tr)
		if len(cells) > MaxTableCols {
			trunc = true
			cells = cells[:MaxTableCols]
		}
		row := make([]Cell, 0, len(cells))
		empty := true
		for _, c := range cells {
			if !w.tick() {
				break
			}
			sub := &builder{}
			w.collect(sub, c)
			runs := sub.takeRuns()
			if runs != nil {
				empty = false
			}
			row = append(row, Cell{Runs: runs, Header: c.DataAtom == atom.Th})
		}
		if !empty { // le righe distanziatrici vuote non servono
			tbl.Rows = append(tbl.Rows, row)
		}
	}
	if len(tbl.Rows) > 0 {
		b.out = append(b.out, tbl)
	}
	if trunc {
		b.out = append(b.out, Block{Runs: []Run{{Text: fmt.Sprintf(
			"[tabella troncata: massimo %d righe e %d colonne]", MaxTableRows, MaxTableCols)}}})
	}
}

// cell tratta una cella di una tabella di layout: un bottone (<td bgcolor>) o
// i blocchi del suo contenuto.
func (w *walker) cell(b *builder, c *html.Node, depth, quote int) {
	b.flush()
	if isButton(c) {
		if blk, ok := w.button(c); ok {
			b.out = append(b.out, blk)
			return
		}
	}
	b.out = append(b.out, w.blocks(c, depth, quote)...)
}
