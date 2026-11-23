package richtext

import (
	"strings"
	"testing"
)

// TestSanitizeURL (Piano 15, Task 5, D15-T13): bidi, zero-width, controlli e
// spazi si tolgono prima di validare; solo http, https, mailto; lunghezza
// massima 2048.
func TestSanitizeURL(t *testing.T) {
	tests := []struct {
		name, in, want string
		ok             bool
	}{
		{"http", "http://example.com/a", "http://example.com/a", true},
		{"https maiuscolo", "HTTPS://Example.com/A", "HTTPS://Example.com/A", true},
		{"mailto", "mailto:info@example.org", "mailto:info@example.org", true},
		{"RLO nel mezzo", "https://exa\u202Emple.com/x", "https://example.com/x", true},
		{"tutti i bidi", "https://\u202A\u202B\u202C\u202D\u2066\u2067\u2068\u2069\u200E\u200Fexample.com", "https://example.com", true},
		{"zero-width nel dominio", "https://exa\u200Bmp\u200Cle\u200D.com/\uFEFF", "https://example.com/", true},
		{"word joiner e ALM", "https://ex\u2060am\u061Cple.com", "https://example.com", true},
		{"controlli e spazi", " https://exa mple.com/\x1b\x07a\tb\n ", "https://example.com/ab", true},
		{"javascript", "javascript:alert(1)", "", false},
		{"javascript mascherato", "java\u200Bscript:alert(1)", "", false},
		{"javascript con controlli", "java\nscript:alert(1)", "", false},
		{"data", "data:text/html;base64,PHNjcmlwdD4=", "", false},
		{"file", "file:///etc/passwd", "", false},
		{"ftp", "ftp://example.com/", "", false},
		{"relativo", "/percorso", "", false},
		{"schema relativo", "//example.com/", "", false},
		{"senza host", "https://", "", false},
		{"mailto vuoto", "mailto:", "", false},
		{"vuoto", "", "", false},
		{"solo invisibili", "\u200B\u202E ", "", false},
		{"invalido", "http://exa%zzmple.com/", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := sanitizeURL(tt.in)
			if ok != tt.ok || got != tt.want {
				t.Errorf("sanitizeURL(%q) = %q, %v; atteso %q, %v", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestSanitizeURLLunghezza(t *testing.T) {
	prefix := "https://example.com/"
	ok := prefix + strings.Repeat("a", maxURLLen-len(prefix))
	if _, good := sanitizeURL(ok); !good {
		t.Error("URL di esattamente 2048 byte rifiutato")
	}
	if _, good := sanitizeURL(ok + "a"); good {
		t.Error("URL di 2049 byte accettato")
	}
	// l'invisibile non fa guadagnare spazio: conta dopo la pulizia
	if _, good := sanitizeURL(ok + strings.Repeat("\u200B", 100)); !good {
		t.Error("gli invisibili vanno tolti prima di misurare")
	}
}

// TestSanitizeText (D15-T13): controlli tolti (tranne \n), tab in spazio,
// bidi e zero-width tolti, byte non UTF-8 sostituiti.
func TestSanitizeText(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"semplice", "ciao mondo", "ciao mondo"},
		{"a capo conservato", "a\nb", "a\nb"},
		{"tab in spazio", "a\tb", "a b"},
		{"ESC e BEL", "a\x1b[31mb\x07", "a[31mb"},
		{"CR tolto", "a\r\nb", "a\nb"},
		{"C1", "a\u0085\u009bb", "ab"},
		{"RLO", "fattura\u202Efdp.exe", "fatturafdp.exe"},
		{"isolati bidi", "a\u2066b\u2067c\u2068d\u2069e", "abcde"},
		{"LRM e RLM", "a\u200Eb\u200Fc", "abc"},
		{"zero-width", "pay\u200Bpal\u200C.\u200Dcom\uFEFF", "paypal.com"},
		{"UTF-8 invalido", "a\xffb", "a�b"},
		{"accenti e emoji intatti", "perché ❤", "perché ❤"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeText(tt.in); got != tt.want {
				t.Errorf("sanitizeText(%q) = %q, atteso %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestFromHTMLSanificaBidiELink: il controllo di dominio vede il dominio
// reale anche se il testo o l'href nascondono caratteri invisibili.
func TestFromHTMLSanificaBidiELink(t *testing.T) {
	blocks, _ := FromHTML("<p><a href=\"https://exa" + "\u200B" + "mple.com/\">visita \u202Eexample.com</a></p>")
	ls := links(blocks)
	if len(ls) != 1 || ls[0].Href != "https://example.com/" {
		t.Fatalf("link = %+v", ls)
	}
	if strings.ContainsAny(ls[0].Text, "\u202E\u200B") {
		t.Errorf("invisibili nel testo del link: %q", ls[0].Text)
	}
}
