package render

import (
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// ansiRE toglie dalle righe rese gli SGR (CSI) e gli OSC (OSC 8 con ST o BEL):
// i test confrontano il testo visibile e mai i colori (Piano 15, Task 6).
var ansiRE = regexp.MustCompile("\x1b\\[[0-9;:]*[A-Za-z]|\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)")

func plain(s string) string { return ansiRE.ReplaceAllString(s, "") }

// Sequenze di sottolineatura SGR: 4:3 ondulata, 4:5 tratteggiata.
const (
	sgrCurly  = "4:3"
	sgrDashed = "4:5"
)

func TestLinkPhishingTable(t *testing.T) {
	cases := []struct {
		name       string
		text, href string
		warn       bool
	}{
		// Sottodomini legittimi: stesso dominio registrabile.
		{"www sul lato href", "amazon.it", "https://www.amazon.it/x", false},
		{"www sul lato testo", "www.amazon.it", "https://amazon.it/", false},
		{"mail. sul lato href", "https://www.amazon.it", "https://mail.amazon.it/", false},
		{"sottodominio profondo", "amazon.it", "https://a.b.c.amazon.it/p?q=1", false},
		{"maiuscole", "AMAZON.IT", "https://www.Amazon.it/", false},
		{"porta nell'href", "amazon.it", "https://amazon.it:8443/", false},
		{"punto finale nel testo", "amazon.it.", "https://amazon.it/", false},
		{"testo con percorso", "amazon.it/offerte", "https://www.amazon.it/x", false},
		{"testo URL completo", "https://www.amazon.it/offerte", "https://amazon.it/y", false},
		{"punto finale nell'host href", "amazon.it", "https://amazon.it./", false},
		// Phishing: dominio diverso.
		{"amazon.it.truffa.com", "amazon.it", "https://amazon.it.truffa.com/", true},
		{"amazon.it.truffa.com con www", "www.amazon.it", "https://www.amazon.it.truffa.com/", true},
		{"dominio diverso", "amazon.it", "https://amazon.evil.example/", true},
		{"paypal.com.evil.example", "paypal.com", "https://paypal.com.evil.example", true},
		{"testo URL, href altro", "https://paypal.com", "https://evil.example/paypal.com", true},
		{"userinfo che finge il dominio", "https://amazon.it", "https://amazon.it@evil.example/", true},
		{"userinfo nel testo", "https://amazon.it@evil.example/", "https://amazon.it/", true},
		{"tld diverso", "amazon.it", "https://amazon.com/", true},
		{"suffisso privato (github.io)", "alice.github.io", "https://bob.github.io/", true},
		// Omografie IDN.
		{"omografia cirillica nel testo", "\u0430mazon.it", "https://amazon.it/", true},
		{"omografia cirillica nell'href", "amazon.it", "https://\u0430mazon.it/", true},
		{"omografia cirillica vs punycode di altro host", "\u0430mazon.it", "https://xn--mazon-3ve.it.evil.example/", true},
		{"omografia nel testo URL", "https://p\u0430ypal.com", "https://paypal.com/", true},
		// Stesso dominio IDN in unicode e in punycode.
		{"unicode vs punycode", "b\u00fccher.de", "https://xn--bcher-kva.de/", false},
		{"punycode vs unicode", "xn--bcher-kva.de", "https://b\u00fccher.de/", false},
		{"unicode maiuscolo vs punycode", "B\u00dcCHER.DE", "https://www.xn--bcher-kva.de/", false},
		{"unicode vs unicode", "b\u00fccher.de", "https://b\u00fccher.de/x", false},
		// IP e suffissi pubblici: ricaduta sull'host esatto.
		{"IP uguale", "192.168.0.1", "http://192.168.0.1/x", false},
		{"IP diverso", "192.168.0.1", "http://192.168.0.2/x", true},
		{"IP vs dominio", "192.168.0.1", "https://amazon.it/", true},
		{"IPv6 URL uguale", "http://[2001:db8::1]/", "http://[2001:db8::1]:8080/x", false},
		{"IPv6 URL diverso", "http://[2001:db8::1]/", "http://[2001:db8::2]/", true},
		{"suffisso pubblico uguale", "co.uk", "https://co.uk/", false},
		{"suffisso pubblico, sottodominio diverso", "co.uk", "https://evil.co.uk/", true},
		{"suffisso pubblico vs altro suffisso", "co.uk", "https://com.au/", true},
		// Testo che non e un URL ne un dominio nudo.
		{"testo libero", "clicca qui", "https://evil.example/", false},
		{"testo libero con punto", "Leggi di piu. Ora", "https://evil.example/", false},
		{"versione", "v1.2", "https://evil.example/", false},
		{"numero decimale", "1.5", "https://evil.example/", false},
		{"nome file (TLD non ICANN)", "file.txt", "https://evil.example/", false},
		{"una parola", "Scarica", "https://evil.example/", false},
		{"frase con dominio dentro", "vai su amazon.it ora", "https://evil.example/", false},
		// mailto.
		{"mailto uguale", "info@amazon.it", "mailto:info@amazon.it", false},
		{"mailto stesso dominio", "info@amazon.it", "mailto:altro@www.amazon.it", false},
		{"mailto dominio diverso", "info@amazon.it", "mailto:x@evil.example", true},
		{"testo dominio, mailto diverso", "amazon.it", "mailto:x@evil.example", true},
		{"testo libero con mailto", "Scrivici", "mailto:x@evil.example", false},
		{"testo email, href http diverso", "info@amazon.it", "https://evil.example/", true},
		{"testo email, href http uguale", "info@amazon.it", "https://www.amazon.it/contatti", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := Link(tc.text, tc.href, testCtx(t, 200))
			valid(t, o)
			if len(o.Lines) == 0 || len(o.Zones[0]) == 0 {
				t.Fatalf("nessuna zona per %q -> %q: %+v", tc.text, tc.href, o)
			}
			z := o.Zones[0][0]
			if z.Warn != tc.warn {
				t.Errorf("Warn = %v, atteso %v", z.Warn, tc.warn)
			}
			joined := strings.Join(o.Lines, "\n")
			if got := strings.Contains(plain(joined), "\u26a0"); got != tc.warn {
				t.Errorf("marcatore warn presente = %v, atteso %v (%q)", got, tc.warn, plain(joined))
			}
			if got := strings.Contains(joined, sgrCurly); got != tc.warn {
				t.Errorf("sottolineatura ondulata = %v, atteso %v", got, tc.warn)
			}
			if got := strings.Contains(joined, sgrDashed); got == tc.warn {
				t.Errorf("sottolineatura tratteggiata = %v con warn %v", got, tc.warn)
			}
			if z.Kind != ZoneLink {
				t.Errorf("Kind = %v, atteso ZoneLink", z.Kind)
			}
		})
	}
}

func TestLinkTextAndZoneGeometry(t *testing.T) {
	c := testCtx(t, 200)
	o := Link("Offerte", "https://www.amazon.it/x", c)
	valid(t, o)
	if len(o.Lines) != 1 {
		t.Fatalf("righe = %d, atteso 1", len(o.Lines))
	}
	if got := plain(o.Lines[0]); got != "Offerte" {
		t.Errorf("testo visibile = %q", got)
	}
	z := o.Zones[0][0]
	if z.X0 != 0 || z.X1 != lipgloss.Width(o.Lines[0]) {
		t.Errorf("zona [%d,%d), attesa [0,%d)", z.X0, z.X1, lipgloss.Width(o.Lines[0]))
	}
	if z.URL != "https://www.amazon.it/x" || z.ID == "" {
		t.Errorf("zona: %+v", z)
	}
	if !strings.Contains(o.Lines[0], "]8;") || !strings.Contains(o.Lines[0], "https://www.amazon.it/x") {
		t.Errorf("OSC 8 assente: %q", o.Lines[0])
	}
	if !strings.Contains(o.Lines[0], "id="+z.ID) {
		t.Errorf("id OSC 8 diverso da Zone.ID %q: %q", z.ID, o.Lines[0])
	}
	if _, ok := ZoneAt(o.Zones[0], 0); !ok {
		t.Error("ZoneAt(0) non trova il link")
	}
	if _, ok := ZoneAt(o.Zones[0], z.X1); ok {
		t.Error("X1 e escluso: ZoneAt(X1) non deve trovare il link")
	}
}

func TestLinkEmptyTextShowsHref(t *testing.T) {
	o := Link("", "https://amazon.it/x", testCtx(t, 80))
	valid(t, o)
	if got := plain(strings.Join(o.Lines, "")); got != "https://amazon.it/x" {
		t.Errorf("testo = %q", got)
	}
	if len(o.Zones[0]) != 1 || o.Zones[0][0].Warn {
		t.Errorf("zone: %+v", o.Zones)
	}
}

func TestLinkWarnMarkerFirstLineAndAllLinesWarn(t *testing.T) {
	o := Link("paypal.com/accedi/ora/subito", "https://evil.example/", testCtx(t, 12))
	valid(t, o)
	if len(o.Lines) < 2 {
		t.Fatalf("serve il wrap: %q", o.Lines)
	}
	if !strings.Contains(plain(o.Lines[0]), "\u26a0") {
		t.Errorf("marcatore warn non sulla prima riga: %q", plain(o.Lines[0]))
	}
	for i, zs := range o.Zones {
		if len(zs) != 1 || !zs[0].Warn {
			t.Errorf("riga %d: zone %+v", i, zs)
		}
		if !strings.Contains(o.Lines[i], sgrCurly) {
			t.Errorf("riga %d senza sottolineatura ondulata", i)
		}
	}
}

func TestLinkWrapZonesMatchLines(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		href  string
		width int
	}{
		{"parole", "clicca qui per vedere tutte le offerte", "https://amazon.it/x", 10},
		{"URL lungo senza spazi", "https://www.amazon.it/offerte-speciali/natale/2026", "https://amazon.it/x", 16},
		{"ampio CJK", "\u65e5\u672c\u8a9e\u65e5\u672c\u8a9e\u65e5\u672c\u8a9e", "https://amazon.it/x", 7},
		{"larghezza 1", "abcdef", "https://amazon.it/x", 1},
		{"larghezza esatta", "abcde fghij", "https://amazon.it/x", 5},
		{"phishing con wrap", "paypal.com/accedi/sicuro/ora", "https://evil.example/", 9},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := Link(tc.text, tc.href, testCtx(t, tc.width))
			valid(t, o)
			if len(o.Lines) < 2 && tc.width > 1 && lipgloss.Width(tc.text) > tc.width {
				t.Fatalf("atteso wrap su %d colonne: %q", tc.width, o.Lines)
			}
			var vis strings.Builder
			var id string
			for i, l := range o.Lines {
				w := lipgloss.Width(l)
				if w > tc.width {
					t.Errorf("riga %d larga %d > %d: %q", i, w, tc.width, plain(l))
				}
				if len(o.Zones[i]) != 1 {
					t.Fatalf("riga %d: zone %+v", i, o.Zones[i])
				}
				z := o.Zones[i][0]
				if z.X0 != 0 || z.X1 != w || z.X1 <= z.X0 {
					t.Errorf("riga %d: zona [%d,%d), larghezza riga %d", i, z.X0, z.X1, w)
				}
				if z.URL != o.Zones[0][0].URL || z.Kind != ZoneLink {
					t.Errorf("riga %d: zona %+v", i, z)
				}
				if i == 0 {
					id = z.ID
				} else if z.ID != id {
					t.Errorf("riga %d: id %q diverso da %q", i, z.ID, id)
				}
				if !strings.Contains(l, "id="+id) {
					t.Errorf("riga %d: id OSC 8 mancante", i)
				}
				vis.WriteString(plain(l))
			}
			got := strings.NewReplacer(" ", "", "\u26a0", "").Replace(vis.String())
			want := strings.ReplaceAll(tc.text, " ", "")
			if got != want {
				t.Errorf("testo perso nel wrap: %q, atteso %q", got, want)
			}
		})
	}
}

func TestLinkWidthZeroNoWrap(t *testing.T) {
	text := strings.Repeat("parola ", 30)
	o := Link(strings.TrimSpace(text), "https://amazon.it/x", testCtx(t, 0))
	valid(t, o)
	if len(o.Lines) != 1 {
		t.Errorf("righe = %d, atteso 1 senza larghezza", len(o.Lines))
	}
}

func TestLinkLongTextBounded(t *testing.T) {
	long := "https://www.amazon.it/" + strings.Repeat("a", 3000)
	o := Link(long, "https://amazon.it/x", testCtx(t, 80))
	valid(t, o)
	if len(o.Lines) < 2 {
		t.Fatalf("righe = %d, atteso wrap", len(o.Lines))
	}
	for i, l := range o.Lines {
		if lipgloss.Width(l) > 80 {
			t.Fatalf("riga %d larga %d", i, lipgloss.Width(l))
		}
		if len(o.Zones[i]) != 1 {
			t.Fatalf("riga %d: zone %+v", i, o.Zones[i])
		}
	}
}

func TestLinkIDStableAndDistinct(t *testing.T) {
	c := testCtx(t, 80)
	a1 := Link("Uno", "https://amazon.it/a", c).Zones[0][0].ID
	a2 := Link("Uno", "https://amazon.it/a", c).Zones[0][0].ID
	b := Link("Uno", "https://amazon.it/b", c).Zones[0][0].ID
	if a1 == "" || a1 != a2 {
		t.Errorf("id non deterministico: %q %q", a1, a2)
	}
	if a1 == b {
		t.Errorf("link diversi con lo stesso id %q", a1)
	}
}

func TestLinkRejectedSchemesAreNotClickable(t *testing.T) {
	hrefs := []string{
		"javascript:alert(1)",
		"JaVaScRiPt:alert(1)",
		"data:text/html;base64,PHNjcmlwdD4=",
		"file:///etc/passwd",
		"ftp://amazon.it/x",
		"vbscript:msgbox",
		"//amazon.it/x",
		"/relativo",
		"amazon.it",
		"https://",
		"http:///x",
		"mailto:",
		"",
		"   ",
	}
	for _, h := range hrefs {
		t.Run(h, func(t *testing.T) {
			o := Link("Scarica", h, testCtx(t, 80))
			valid(t, o)
			if len(o.Lines) != 1 || plain(o.Lines[0]) != "Scarica" {
				t.Fatalf("righe = %q", o.Lines)
			}
			for _, zs := range o.Zones {
				if len(zs) != 0 {
					t.Errorf("zona su href rifiutato: %+v", zs)
				}
			}
			if strings.Contains(o.Lines[0], "]8;") || strings.Contains(o.Lines[0], sgrCurly) || strings.Contains(o.Lines[0], "\u26a0") {
				t.Errorf("riga con OSC 8 o warn per href rifiutato: %q", o.Lines[0])
			}
		})
	}
}

func TestLinkAllowedSchemes(t *testing.T) {
	for _, h := range []string{"http://amazon.it/x", "HTTPS://AMAZON.IT/x", "mailto:info@amazon.it"} {
		o := Link("amazon.it", h, testCtx(t, 80))
		if len(o.Zones) != 1 || len(o.Zones[0]) != 1 {
			t.Errorf("%s: zone %+v", h, o.Zones)
		}
	}
}

func TestLinkHrefSanitized(t *testing.T) {
	cases := []struct{ name, href, want string }{
		{"ESC e BEL non chiudono l'OSC", "https://a.com/\x1b]8;;evil\x07x", "https://a.com/]8;;evilx"},
		{"spazi tolti", "https://a .com/ p", "https://a.com/p"},
		{"newline e tab tolti", "https://a.com/\nb\tc", "https://a.com/bc"},
		{"RLO tolto", "https://a.com/\u202etxt.exe", "https://a.com/txt.exe"},
		{"zero width tolto", "https://a\u200b.com/", "https://a.com/"},
		{"NUL tolto", "https://a.com/\x00x", "https://a.com/x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := Link("Apri", tc.href, testCtx(t, 80))
			valid(t, o)
			if len(o.Zones[0]) != 1 {
				t.Fatalf("zone: %+v", o.Zones)
			}
			if got := o.Zones[0][0].URL; got != tc.want {
				t.Errorf("URL = %q, atteso %q", got, tc.want)
			}
			// Esattamente una apertura e una chiusura OSC 8 per riga: nessuna iniezione.
			l := o.Lines[0]
			if n := strings.Count(l, "\x1b]8;"); n != 2 {
				t.Errorf("sequenze OSC 8 = %d, attese 2: %q", n, l)
			}
			if strings.Contains(l, "\x07") {
				t.Errorf("BEL nella riga: %q", l)
			}
		})
	}
}

func TestLinkHrefLengthLimit(t *testing.T) {
	base := "https://amazon.it/"
	ok := base + strings.Repeat("a", 2048-len(base))
	if o := Link("Apri", ok, testCtx(t, 80)); len(o.Zones[0]) != 1 {
		t.Error("URL di 2048 byte deve restare cliccabile")
	}
	for _, h := range []string{ok + "a", base + strings.Repeat("a", 5000)} {
		o := Link("Apri", h, testCtx(t, 80))
		if len(o.Zones[0]) != 0 {
			t.Errorf("URL di %d byte non deve essere cliccabile", len(h))
		}
		if plain(o.Lines[0]) != "Apri" {
			t.Errorf("testo = %q", plain(o.Lines[0]))
		}
	}
}

func TestLinkTextSanitized(t *testing.T) {
	o := Link("ev\x1b[31mil\x07 \u202egpj\nriga", "https://amazon.it/", testCtx(t, 80))
	valid(t, o)
	got := plain(strings.Join(o.Lines, ""))
	if got != "ev[31mil gpj riga" {
		t.Errorf("testo = %q", got)
	}
	for _, l := range o.Lines {
		if strings.Contains(plain(l), "\x1b") || strings.Contains(l, "\x07") {
			t.Errorf("controllo del testo sopravvissuto: %q", l)
		}
	}
}

func TestLinkWhitespaceTextUsesHref(t *testing.T) {
	o := Link(" \n\t ", "https://amazon.it/x", testCtx(t, 80))
	if got := plain(strings.Join(o.Lines, "")); got != "https://amazon.it/x" {
		t.Errorf("testo = %q", got)
	}
}

func TestLinkIndentKeepsZonesCoherent(t *testing.T) {
	o := Indent(Link("Offerte", "https://amazon.it/x", testCtx(t, 80)), 4)
	z := o.Zones[0][0]
	if z.X0 != 4 || z.X1 != 4+len("Offerte") {
		t.Errorf("zona dopo Indent [%d,%d)", z.X0, z.X1)
	}
}
