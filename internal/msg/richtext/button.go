package richtext

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// isButton applica le euristiche bottone di D15-T7 (Piano 15, Task 4). Guardano
// solo gli attributi HTML e l'attributo style inline: nessun foglio di stile e'
// letto, nessun CSS e' valutato. Regole, in alternativa:
//
//  1. a[href] con background/background-color e (padding o border-radius) nello
//     style inline;
//  2. td con bgcolor (non bianco ne' trasparente) che contiene esattamente un
//     link e nessun altro testo;
//  3. a[href] con role="button";
//  4. a[href] con class che contiene il token intero btn o button.
//
// Il chiamante verifica poi che href e etichetta siano utilizzabili: se non
// lo sono il nodo resta un link normale.
func isButton(n *html.Node) bool {
	if n == nil || n.Type != html.ElementNode {
		return false
	}
	switch n.DataAtom {
	case atom.A:
		if strings.TrimSpace(attr(n, "href")) == "" {
			return false
		}
		return styleLooksButton(attr(n, "style")) || roleIsButton(n) || classIsButton(n)
	case atom.Td:
		return cellIsButton(n)
	}
	return false
}

// styleLooksButton e' la regola 1: uno sfondo e una forma (padding o
// border-radius) dichiarati nello style inline.
func styleLooksButton(style string) bool {
	var bg, shape bool
	for _, decl := range strings.Split(style, ";") {
		name, val, ok := strings.Cut(decl, ":")
		if !ok {
			continue
		}
		name = strings.ToLower(strings.TrimSpace(name))
		val = strings.ToLower(strings.TrimSpace(strings.Replace(val, "!important", "", 1)))
		switch {
		case name == "background" || name == "background-color":
			switch val {
			case "", "none", "transparent", "inherit", "initial", "unset":
			default:
				bg = true
			}
		case name == "border-radius" || name == "padding" || strings.HasPrefix(name, "padding-"):
			if !isZeroLength(val) {
				shape = true
			}
		}
	}
	return bg && shape
}

// isZeroLength: valore vuoto o fatto solo di zeri (0, 0px, 0 0 0 0...).
func isZeroLength(v string) bool {
	fields := strings.Fields(v)
	if len(fields) == 0 {
		return true
	}
	for _, f := range fields {
		switch f {
		case "0", "0px", "0em", "0rem", "0%":
		default:
			return false
		}
	}
	return true
}

// roleIsButton e' la regola 3.
func roleIsButton(a *html.Node) bool {
	return strings.EqualFold(strings.TrimSpace(attr(a, "role")), "button")
}

// classIsButton e' la regola 4: token interi, mai sottostringhe (btndark-theme
// e btn-primary da soli non bastano).
func classIsButton(a *html.Node) bool {
	for _, tok := range strings.Fields(strings.ToLower(attr(a, "class"))) {
		if tok == "btn" || tok == "button" {
			return true
		}
	}
	return false
}

// neutralBg: bgcolor che indicano semplice impaginazione, non un bottone.
var neutralBg = map[string]bool{
	"": true, "#fff": true, "#ffffff": true, "white": true,
	"transparent": true, "none": true,
}

// cellIsButton e' la regola 2: cella colorata il cui unico contenuto e' un link
// (il testo della cella coincide con quello del link, non e' vuoto).
func cellIsButton(td *html.Node) bool {
	if neutralBg[strings.ToLower(strings.TrimSpace(attr(td, "bgcolor")))] {
		return false
	}
	links, text, linkText := 0, "", ""
	var walk func(n *html.Node, inLink bool)
	walk = func(n *html.Node, inLink bool) {
		for c := n.FirstChild; c != nil && links <= 1; c = c.NextSibling {
			switch {
			case c.Type == html.TextNode:
				text += " " + c.Data
				if inLink {
					linkText += " " + c.Data
				}
			case c.Type != html.ElementNode || skip(c):
			case c.DataAtom == atom.A && strings.TrimSpace(attr(c, "href")) != "":
				links++
				walk(c, true)
			default:
				walk(c, inLink)
			}
		}
	}
	walk(td, false)
	if links != 1 {
		return false
	}
	norm := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	return norm(text) != "" && norm(text) == norm(linkText)
}
