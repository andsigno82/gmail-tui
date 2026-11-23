package richtext

import (
	"strings"
	"testing"
)

// buttons e' un alias leggibile: i bottoni del modello (anche annidati).
func buttons(blocks []Block) []Block { return collect(blocks, KindButton) }

// oneButton verifica che l'HTML produca esattamente un bottone e lo ritorna.
func oneButton(t *testing.T, src string) Block {
	t.Helper()
	bs := buttons(mustBlocks(src))
	if len(bs) != 1 {
		t.Fatalf("bottoni = %d, atteso 1 (blocchi: %+v)", len(bs), mustBlocks(src))
	}
	return bs[0]
}

func mustBlocks(src string) []Block {
	blocks, _ := FromHTML(src)
	return blocks
}

// noButton verifica che l'HTML non produca bottoni ma lasci il link in un
// paragrafo.
func noButton(t *testing.T, src string) {
	t.Helper()
	blocks := mustBlocks(src)
	if n := len(buttons(blocks)); n != 0 {
		t.Fatalf("bottoni = %d, atteso 0", n)
	}
	if len(links(blocks)) == 0 {
		t.Fatalf("il link deve restare nel paragrafo: %+v", blocks)
	}
}

// Regola 1 (D15-T7): a[href] con background e (padding o border-radius).
func TestButtonRegolaStileBackgroundPadding(t *testing.T) {
	b := oneButton(t, `<a href="https://shop.example.com/x" style="Background-Color: #222; PADDING: 10px 20px">Vai</a>`)
	if len(b.Runs) != 1 || b.Runs[0].Text != "Vai" || b.Runs[0].Href != "https://shop.example.com/x" {
		t.Errorf("runs = %+v", b.Runs)
	}
	if b.Host != "shop.example.com" {
		t.Errorf("Host = %q", b.Host)
	}
}

func TestButtonRegolaStileBackgroundShorthandBorderRadius(t *testing.T) {
	oneButton(t, `<a href="https://shop.example.com/x" style="background:#e4572e;border-radius:4px">Vai</a>`)
}

func TestButtonRegolaStilePaddingLato(t *testing.T) {
	oneButton(t, `<a href="https://shop.example.com/x" style="background-color:#222;padding-left:12px">Vai</a>`)
}

func TestButtonRegolaStileNonBastaIlColore(t *testing.T) {
	// solo background: un link evidenziato, non un bottone.
	noButton(t, `<p><a href="https://shop.example.com/x" style="background-color:#ff0">evidenziato</a></p>`)
	// solo padding: idem.
	noButton(t, `<p><a href="https://shop.example.com/x" style="padding:4px">spaziato</a></p>`)
	// padding nullo e background trasparente non contano.
	noButton(t, `<p><a href="https://shop.example.com/x" style="background:none;padding:10px">x</a></p>`)
	noButton(t, `<p><a href="https://shop.example.com/x" style="background-color:#222;padding:0">x</a></p>`)
}

func TestButtonLinkSenzaStileResta(t *testing.T) {
	noButton(t, `<p>vedi <a href="https://shop.example.com/x">il sito</a></p>`)
}

// Regola 2: td con bgcolor che contiene esattamente un link.
func TestButtonRegolaTdBgcolor(t *testing.T) {
	b := oneButton(t, fixture(t, "table_td_bgcolor.html"))
	if b.Runs[0].Text != "Conferma indirizzo" || b.Host != "accounts.example.org" {
		t.Errorf("bottone = %+v", b)
	}
	// il paragrafo accanto resta un paragrafo
	if !strings.Contains(allText(mustBlocks(fixture(t, "table_td_bgcolor.html"))), "Conferma il tuo indirizzo") {
		t.Error("manca il paragrafo di contorno")
	}
}

func TestButtonRegolaTdBgcolorDueLinkNo(t *testing.T) {
	noButton(t, `<table><tr><td bgcolor="#0b5fff"><a href="https://example.com/a">Uno</a> <a href="https://example.com/b">Due</a></td></tr></table>`)
}

func TestButtonRegolaTdBgcolorBiancoNo(t *testing.T) {
	// celle bianche/trasparenti sono semplice impaginazione.
	for _, bg := range []string{"#ffffff", "#FFF", "white", "transparent"} {
		noButton(t, `<table><tr><td bgcolor="`+bg+`"><a href="https://example.com/a">Vai</a></td></tr></table>`)
	}
}

func TestButtonRegolaTdBgcolorConTestoExtraNo(t *testing.T) {
	// fascia colorata con un paragrafo e un link dentro: non e' un bottone.
	noButton(t, `<table><tr><td bgcolor="#0b5fff">Leggi le novita' del mese e <a href="https://example.com/a">scopri di piu'</a> sul sito</td></tr></table>`)
}

func TestButtonRegolaTdSenzaBgcolorNo(t *testing.T) {
	noButton(t, `<table><tr><td><a href="https://example.com/a">Vai</a></td></tr></table>`)
}

// Regola 3: role="button".
func TestButtonRegolaRole(t *testing.T) {
	b := oneButton(t, fixture(t, "role_button.html"))
	if b.Runs[0].Text != "Segui la spedizione" || b.Host != "orders.example.com" {
		t.Errorf("bottone = %+v", b)
	}
}

func TestButtonRegolaRoleMaiuscolo(t *testing.T) {
	oneButton(t, `<a href="https://example.com/a" role=" Button ">Vai</a>`)
	noButton(t, `<p><a href="https://example.com/a" role="link">Vai</a></p>`)
}

// Regola 4: class con il token btn o button (token intero, non sottostringa).
func TestButtonRegolaClass(t *testing.T) {
	bs := buttons(mustBlocks(fixture(t, "class_btn.html")))
	if len(bs) != 2 {
		t.Fatalf("bottoni = %d, attesi 2 (btn e Button)", len(bs))
	}
	if bs[0].Runs[0].Text != "Leggi l'articolo" || bs[1].Runs[0].Text != "Iscriviti" {
		t.Errorf("bottoni = %+v", bs)
	}
}

func TestButtonRegolaClassTokenParzialeNo(t *testing.T) {
	blocks := mustBlocks(fixture(t, "class_btn.html"))
	for _, b := range buttons(blocks) {
		if b.Runs[0].Text == "Tema scuro" {
			t.Error(`class="btndark-theme" non e' un bottone`)
		}
	}
	if !strings.Contains(allText(blocks), "Tema scuro") {
		t.Error("il link btndark-theme deve restare come testo di paragrafo")
	}
	noButton(t, `<p><a class="btn-primary" href="https://example.com/a">Vai</a></p>`)
}

// Salvaguardie comuni.
func TestButtonSenzaHrefValidoNo(t *testing.T) {
	// senza href non e' un link; con schema non ammesso il testo resta ma non c'e' bottone.
	for _, src := range []string{
		`<a role="button">Vai</a>`,
		`<a role="button" href="javascript:alert(1)">Vai</a>`,
		`<a class="btn" href="data:text/html,x">Vai</a>`,
		`<a class="btn" href="/relativo">Vai</a>`,
	} {
		blocks := mustBlocks(src)
		if n := len(buttons(blocks)); n != 0 {
			t.Errorf("%s: bottoni = %d", src, n)
		}
		if !strings.Contains(allText(blocks), "Vai") {
			t.Errorf("%s: il testo deve restare", src)
		}
		if len(links(blocks)) != 0 {
			t.Errorf("%s: href non sicuro nel modello", src)
		}
	}
}

func TestButtonNonEPiuLinkDiParagrafo(t *testing.T) {
	blocks := mustBlocks(`<p>Testo <a role="button" href="https://example.com/a">Vai</a> dopo</p>`)
	if len(buttons(blocks)) != 1 {
		t.Fatalf("blocchi = %+v", blocks)
	}
	if len(links(blocks)) != 0 {
		t.Errorf("il bottone non deve restare link di paragrafo: %+v", links(blocks))
	}
	got := allText(blocks)
	if !strings.Contains(got, "Testo") || !strings.Contains(got, "dopo") {
		t.Errorf("testo attorno perso: %q", got)
	}
}

func TestButtonEtichettaDaImmagine(t *testing.T) {
	b := oneButton(t, `<a role="button" href="https://example.com/a"><img src="https://x.example.com/i.png" alt="Scarica"></a>`)
	if b.Runs[0].Text != "Scarica" {
		t.Errorf("etichetta = %q", b.Runs[0].Text)
	}
	// senza testo ne' alt non c'e' etichetta: link normale con testo = href
	blocks := mustBlocks(`<a role="button" href="https://example.com/a"><img src="https://x.example.com/i.png"></a>`)
	if len(buttons(blocks)) != 0 || len(links(blocks)) != 1 {
		t.Errorf("blocchi = %+v", blocks)
	}
}

func TestButtonHostMailto(t *testing.T) {
	b := oneButton(t, `<a role="button" href="mailto:Info@Example.ORG?subject=ciao">Scrivici</a>`)
	if b.Host != "example.org" {
		t.Errorf("Host = %q", b.Host)
	}
}

func TestButtonWarnDominioDiverso(t *testing.T) {
	b := oneButton(t, `<a role="button" href="https://amazon.evil.example/x">amazon.it</a>`)
	if !b.Warn {
		t.Error("Warn atteso: il testo mostra un dominio diverso")
	}
	b = oneButton(t, `<a role="button" href="https://www.amazon.it/x">amazon.it</a>`)
	if b.Warn {
		t.Error("Warn inatteso: stesso dominio registrabile")
	}
	b = oneButton(t, `<a role="button" href="https://www.amazon.it/x">Vedi offerta</a>`)
	if b.Warn {
		t.Error("Warn inatteso: il testo non e' un dominio")
	}
}

// TestButtonNewsletter: le fixture realistiche producono i due bottoni attesi
// (td bgcolor e a con stile) e lasciano il link normale nel paragrafo.
func TestButtonNewsletter(t *testing.T) {
	blocks := mustBlocks(fixture(t, "newsletter.html"))
	bs := buttons(blocks)
	if len(bs) != 2 {
		t.Fatalf("bottoni = %d, attesi 2: %+v", len(bs), bs)
	}
	if bs[0].Runs[0].Text != "Vedi le offerte" || bs[1].Runs[0].Text != "Il tuo account" {
		t.Errorf("etichette = %q, %q", bs[0].Runs[0].Text, bs[1].Runs[0].Text)
	}
	var ls []string
	for _, r := range links(blocks) {
		ls = append(ls, r.Text)
	}
	if strings.Join(ls, "|") != "sfoglia il catalogo|Annulla l'iscrizione" {
		t.Errorf("link di paragrafo = %q", ls)
	}
}
