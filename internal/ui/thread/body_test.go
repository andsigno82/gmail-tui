package thread

import (
	"strings"
	"testing"
)

// Piano 15, Task 14: il corpo del thread e reso dal modello a blocchi (richtext +
// render). Questi test guardano il risultato visibile (righe, zone cliccabili),
// non i tipi interni.

// htmlBody ha un link, una citazione, un elenco, una tabella e un bottone.
const htmlBody = `<p>Ciao, vai su <a href="https://www.example.com/x">example.com</a> ora.</p>
<blockquote><p>citato da Anna</p></blockquote>
<ul><li>uno</li><li>due</li></ul>
<table><tr><th>Voce</th><th>Prezzo</th></tr><tr><td>Pane</td><td>2</td></tr></table>
<p><a class="btn" href="https://shop.example.com/ordine">Vedi ordine</a></p>`

// htmlModel: vista singola con BodyHTML (BodyText e il fallback html2text).
func htmlModel(t *testing.T, w, h int) Model {
	t.Helper()
	full := testMsg()
	full.BodyHTML = htmlBody
	full.BodyText = "Ciao, vai su example.com ora."
	m := newModel(t, full)
	m.SetSize(w, h)
	return m
}

// bodyPlain sono le righe visibili del corpo senza stili ne OSC 8.
func bodyPlain(m Model) []string {
	var out []string
	for _, l := range strings.Split(m.vp.View(), "\n") {
		out = append(out, strings.TrimRight(stripANSI(l), " "))
	}
	return out
}

// rowOf trova la riga del corpo che contiene s e la colonna di s.
func rowOf(t *testing.T, m Model, s string) (row, col int) {
	t.Helper()
	for i, l := range bodyPlain(m) {
		if x := strings.Index(l, s); x >= 0 {
			return i, len([]rune(l[:x]))
		}
	}
	t.Fatalf("%q non trovato nel corpo:\n%s", s, strings.Join(bodyPlain(m), "\n"))
	return 0, 0
}

func TestCorpoHTMLRenderizzatoAdBlocchi(t *testing.T) {
	m := htmlModel(t, 80, 40)
	body := strings.Join(bodyPlain(m), "\n")
	for _, want := range []string{
		"Ciao, vai su example.com ora.",
		"▎ citato da Anna", // citazione con barra
		"- uno",            // elenco
		"Pane",             // cella di tabella
		"╭",                // bordo del bottone
		"Vedi ordine",
		"→ shop.example.com", // host accanto al bottone
	} {
		if !strings.Contains(body, want) {
			t.Errorf("corpo senza %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "<a") || strings.Contains(body, "href") {
		t.Errorf("HTML grezzo nel corpo:\n%s", body)
	}
}

func TestLinkHTMLCliccabile(t *testing.T) {
	m := htmlModel(t, 80, 40)
	row, col := rowOf(t, m, "example.com ora")
	if u, ok := m.LinkAt(col, headerLines+row); !ok || u != "https://www.example.com/x" {
		t.Errorf("LinkAt = (%q,%v), voglio il link HTML", u, ok)
	}
	if _, ok := m.LinkAt(0, headerLines+row); ok {
		t.Errorf("colonna 0 (testo semplice) classificata come link")
	}
}

func TestTestoSempliceConCitazioneConBarra(t *testing.T) {
	m := newModel(t, testMsg()) // solo BodyText, con righe "> "
	m.SetSize(80, 30)
	body := strings.Join(bodyPlain(m), "\n")
	if !strings.Contains(body, "▎ il 20 novembre andiamo") {
		t.Errorf("citazione senza barra laterale:\n%s", body)
	}
	if !strings.Contains(body, "A lunedì,\nMario") && !strings.Contains(body, "A lunedì,") {
		t.Errorf("testo semplice perso:\n%s", body)
	}
}

func TestIntestazioneASchedaConBadge(t *testing.T) {
	m := newModel(t, testMsg())
	m.SetSize(80, 30)
	head := stripANSI(m.header)
	if lines := strings.Split(head, "\n"); len(lines) != headerLines-1 {
		t.Errorf("righe della scheda = %d, voglio %d (headerLines-1)", len(lines), headerLines-1)
	}
	for _, want := range []string{"MR", "╭", "╰", "Mario Rossi <mario@example.com>", "piano settimana"} {
		if !strings.Contains(head, want) {
			t.Errorf("scheda senza %q:\n%s", want, head)
		}
	}
}

func TestAllegatiComeChip(t *testing.T) {
	m := newModel(t, testMsg())
	m.SetSize(80, 30)
	v := stripANSI(m.View())
	for _, want := range []string{"▣ report.pdf · application/pdf", "▣ dati.csv · text/csv"} {
		if !strings.Contains(v, want) {
			t.Errorf("chip %q assente:\n%s", want, v)
		}
	}
	if strings.Contains(v, "\U0001F4CE") {
		t.Errorf("resta la vecchia lista con la graffetta")
	}
}

// zoneURLs elenca gli URL dei link del corpo, uno per link (le porzioni a capo
// dello stesso link hanno lo stesso id OSC 8).
func zoneURLs(m Model) []string {
	var out []string
	seen := map[string]bool{}
	for _, zs := range m.linkRows {
		for _, z := range zs {
			if !seen[z.ID] {
				seen[z.ID] = true
				out = append(out, z.URL)
			}
		}
	}
	return out
}

// TestRiconoscimentoURLEPunteggiatura (testo semplice): URL in testo libero, senza
// la punteggiatura finale tipica; solo http, https e mailto. Prima del Task 14 era
// un test di findLinks/linkTarget; ora passa dal modello a blocchi.
func TestRiconoscimentoURLEPunteggiatura(t *testing.T) {
	cases := []struct {
		name, in string
		want     []string
	}{
		{"url in mezzo al testo", "vedi https://example.com/a?b=1 qui", []string{"https://example.com/a?b=1"}},
		{"punto finale", "fine: https://example.com.", []string{"https://example.com"}},
		{"virgola e due punti", "https://example.com/x, oppure http://example.org/y:", []string{"https://example.com/x", "http://example.org/y"}},
		{"parentesi tonde esterne", "(https://example.com/x)", []string{"https://example.com/x"}},
		{"mailto", "scrivi a mailto:anna@example.com, grazie", []string{"mailto:anna@example.com"}},
		{"schemi non ammessi", "javascript:alert(1) file:///etc/passwd ftp://example.com", nil},
		{"schema senza host", "https:// e http://", nil},
		{"nessun schema", "nessun link qui, www.example.com", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := zoneURLs(testBodyModel(t, c.in, 100, 20))
			if strings.Join(got, "|") != strings.Join(c.want, "|") {
				t.Errorf("zone di %q = %q, voglio %q", c.in, got, c.want)
			}
		})
	}
}
