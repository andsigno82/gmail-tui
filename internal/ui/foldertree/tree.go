// Package foldertree costruisce e disegna l'albero delle cartelle IMAP e delle
// label Gmail annidate (Piano 15, Task 10, D15-09). Non conosce IMAP: lavora
// solo su mail.Mailbox, come il resto della UI (D15-T9: Delimiter).
package foldertree

import (
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/tree"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// maxLabelCols è la larghezza massima (in rune) del testo di una cartella,
// contatore escluso: come il limite dei nomi allegato (D15-T14).
const maxLabelCols = 40

// segVuoto sostituisce un segmento vuoto del nome IMAP (es. "A//B"), così ogni
// nodo ha un'etichetta non vuota.
const segVuoto = "(senza nome)"

// Row è una cartella selezionabile, nell'ordine in cui compare nell'albero.
// Line è l'indice della riga renderizzata (0-based).
type Row struct {
	Name string // nome IMAP (Query.Label); "INBOX" per la posta in arrivo
	Line int
}

// Build costruisce l'albero da boxes. name dà l'etichetta leggibile delle
// cartelle speciali e della posta in arrivo (in app: folderName, passata come
// argomento per evitare il ciclo di import).
//
// Ordine (Piano 15, Task 10): posta in arrivo, speciali (in ordine fisso, senza
// splittarne il nome: "[Gmail]/Trash" resta una foglia), poi le label in ordine
// alfabetico case-insensitive, con i rami annidati sul Delimiter (D15-T9). Un
// delimitatore vuoto lascia la label piatta.
func Build(boxes []mail.Mailbox, name func(mail.Mailbox) string) *tree.Tree {
	var inbox, speciali, label []mail.Mailbox
	for _, b := range boxes {
		switch {
		case isInbox(b):
			inbox = append(inbox, b)
		case b.Role != "":
			speciali = append(speciali, b)
		default:
			label = append(label, b)
		}
	}
	sort.SliceStable(speciali, func(i, j int) bool {
		ri, rj := rangoSpeciale(speciali[i].Role), rangoSpeciale(speciali[j].Role)
		if ri != rj {
			return ri < rj
		}
		return speciali[i].Name < speciali[j].Name
	})

	// La radice virtuale raccoglie le label; i suoi figli diventano il primo
	// livello dell'albero dopo le speciali.
	virtuale := &node{}
	for _, b := range label {
		inserisciLabel(virtuale, b, name)
	}
	ordina(virtuale)

	var top []*node
	for _, b := range inbox {
		top = append(top, &node{label: etichetta(b, name), name: b.Name, selectable: true, counted: isCounted(b)})
	}
	for _, b := range speciali {
		top = append(top, &node{label: etichetta(b, name), name: b.Name, selectable: true, counted: isCounted(b)})
	}
	top = append(top, virtuale.kids...)

	tr := tree.New()
	for _, n := range top {
		tr = tr.Child(n)
	}
	return tr
}

// Rows restituisce le cartelle selezionabili in ordine di albero.
func Rows(t *tree.Tree) []Row {
	var rows []Row
	line := 0
	var cammina func(ch tree.Children)
	cammina = func(ch tree.Children) {
		for i := 0; i < ch.Length(); i++ {
			n, ok := ch.At(i).(*node)
			if !ok || n.Hidden() {
				continue
			}
			if n.selectable {
				rows = append(rows, Row{Name: n.name, Line: line})
			}
			line++
			cammina(n.Children())
		}
	}
	if t != nil {
		cammina(t.Children())
	}
	return rows
}

// Render disegna l'albero. cursor è l'indice in Rows(t) della riga evidenziata;
// counts è il mapping Name → messaggi non letti (Q15-4: solo INBOX e label).
// Ogni nodo occupa una riga, nell'ordine di Rows: Row.Line indica quale.
func Render(t *tree.Tree, cursor int, counts map[string]int, st styles.Styles) (string, []Row) {
	if t == nil {
		return "", nil
	}
	rows := Rows(t)
	prossimo := 0
	var radici []*shown
	ch := t.Children()
	for i := 0; i < ch.Length(); i++ {
		if n, ok := ch.At(i).(*node); ok {
			if s := mostra(n, cursor, &prossimo, counts, st); s != nil {
				radici = append(radici, s)
			}
		}
	}

	out := tree.New().
		EnumeratorStyle(st.FolderTree.PaddingRight(1)).
		IndenterStyle(st.FolderTree).
		ItemStyleFunc(func(ch tree.Children, i int) lipgloss.Style {
			if s, ok := ch.At(i).(*shown); ok && s.sel {
				return st.ListSelected
			}
			return lipgloss.NewStyle()
		})
	for _, s := range radici {
		out = out.Child(s)
	}
	return out.String(), rows
}

// node è un nodo dell'albero di Build: implementa tree.Node per poter stare
// dentro un *tree.Tree di lipgloss e portare il nome IMAP fino al rendering.
type node struct {
	label      string
	name       string
	selectable bool
	counted    bool
	kids       []*node
	hidden     bool
}

func (n *node) String() string        { return n.label }
func (n *node) Value() string         { return n.label }
func (n *node) Hidden() bool          { return n.hidden }
func (n *node) SetHidden(hidden bool) { n.hidden = hidden }
func (n *node) SetValue(v any)        { n.label, _ = v.(string) }

func (n *node) Children() tree.Children {
	out := make(tree.NodeChildren, 0, len(n.kids))
	for _, k := range n.kids {
		out = append(out, k)
	}
	return out
}

// shown è un nodo di Render: testo già stilizzato e flag del cursore.
type shown struct {
	text string
	sel  bool
	kids []*shown
}

func (s *shown) String() string        { return s.text }
func (s *shown) Value() string         { return s.text }
func (s *shown) Hidden() bool          { return false }
func (s *shown) SetHidden(bool)        {}
func (s *shown) SetValue(v any)        { s.text, _ = v.(string) }

func (s *shown) Children() tree.Children {
	out := make(tree.NodeChildren, 0, len(s.kids))
	for _, k := range s.kids {
		out = append(out, k)
	}
	return out
}

// mostra costruisce il nodo di rendering di n: il cursore si conta in pre-ordine
// sulle sole cartelle selezionabili, come Rows.
func mostra(n *node, cursor int, prossimo *int, counts map[string]int, st styles.Styles) *shown {
	if n.Hidden() {
		return nil
	}
	s := &shown{}
	testo := truncate(n.label, maxLabelCols)
	if n.selectable {
		s.sel = *prossimo == cursor
		*prossimo++
		if c := counts[n.name]; n.counted && c > 0 {
			etichetta := countLabel(c)
			if !s.sel {
				etichetta = st.FolderCount.Render(etichetta)
			}
			testo += " " + etichetta
		}
		s.text = testo
	} else {
		s.text = st.FolderTree.Render(testo)
	}
	for _, k := range n.kids {
		if c := mostra(k, cursor, prossimo, counts, st); c != nil {
			s.kids = append(s.kids, c)
		}
	}
	return s
}

// inserisciLabel aggiunge la label b sotto radice, creando i nodi intermedi non
// selezionabili. Un nome che coincide con un nodo intermedio lo rende
// selezionabile (es. "Lavoro" accanto a "Lavoro/Clienti").
func inserisciLabel(radice *node, b mail.Mailbox, name func(mail.Mailbox) string) {
	segmenti := []string{etichetta(b, name)}
	if b.Delimiter != "" {
		segmenti = strings.Split(b.Name, b.Delimiter)
	}
	padre := radice
	for i, seg := range segmenti {
		if seg == "" {
			seg = segVuoto
		}
		figlio := trovaFiglio(padre, seg)
		if figlio == nil {
			figlio = &node{label: seg}
			padre.kids = append(padre.kids, figlio)
		}
		if i == len(segmenti)-1 {
			figlio.selectable = true
			figlio.name = b.Name
			figlio.counted = isCounted(b)
		}
		padre = figlio
	}
}

func trovaFiglio(padre *node, label string) *node {
	for _, k := range padre.kids {
		if k.label == label {
			return k
		}
	}
	return nil
}

// ordina ordina ricorsivamente i figli per etichetta, case-insensitive.
func ordina(n *node) {
	sort.SliceStable(n.kids, func(i, j int) bool {
		a, b := strings.ToLower(n.kids[i].label), strings.ToLower(n.kids[j].label)
		if a != b {
			return a < b
		}
		return n.kids[i].label < n.kids[j].label
	})
	for _, k := range n.kids {
		ordina(k)
	}
}

// rangoSpeciale fissa l'ordine delle speciali (come Gmail); ruoli sconosciuti in fondo.
func rangoSpeciale(role string) int {
	switch role {
	case "flagged":
		return 0
	case "sent":
		return 1
	case "drafts":
		return 2
	case "all":
		return 3
	case "junk":
		return 4
	case "trash":
		return 5
	}
	return 6
}

// isInbox riconosce la posta in arrivo come in app (canonicalFolder).
func isInbox(b mail.Mailbox) bool {
	return b.Role == "inbox" || b.Name == "INBOX"
}

// etichetta è il testo leggibile della cartella; name nil ripiega sul nome IMAP.
func etichetta(b mail.Mailbox, name func(mail.Mailbox) string) string {
	if name == nil {
		return b.Name
	}
	return name(b)
}

// truncate accorcia s a n rune aggiungendo "…"; n <= 1 rende stringa vuota.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return ""
	}
	return string(r[:n-1]) + "…"
}
