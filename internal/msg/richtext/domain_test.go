package richtext

import "testing"

// TestDomainMismatch (Piano 15, Task 4, D15-T6): il confronto e' per dominio
// registrabile dopo normalizzazione IDN; fallback sull'host esatto per IP e
// suffissi pubblici; un testo che non e' un URL o un dominio non da' mai warn.
func TestDomainMismatch(t *testing.T) {
	tests := []struct {
		name, text, href string
		want             bool
	}{
		{"stesso dominio con www", "amazon.it", "https://www.amazon.it/x", false},
		{"sottodominio ingannevole", "amazon.it", "https://amazon.evil.example/", true},
		{"dominio come prefisso", "paypal.com", "https://paypal.com.evil.example", true},
		{"url completo uguale", "https://www.example.com/a", "https://example.com/b", false},
		{"url completo diverso", "https://example.com/a", "https://example.net/a", true},
		{"userinfo ingannevole", "https://example.com@evil.example/", "https://example.com/", true},
		{"unicode e punycode", "bücher.example", "https://xn--bcher-kva.example/", false},
		{"punycode e unicode", "xn--bcher-kva.example", "https://bücher.example/", false},
		{"omografo cirillico", "аpple.com", "https://apple.com/", true},
		{"maiuscole", "Amazon.IT", "https://AMAZON.it/", false},
		{"porta ignorata", "example.com:8443", "https://example.com/", false},
		{"IP uguale", "http://192.0.2.7/a", "http://192.0.2.7/b", false},
		{"IP diverso", "http://192.0.2.7/a", "http://192.0.2.8/a", true},
		{"suffisso pubblico, host esatto uguale", "foo.co.uk", "https://foo.co.uk/", false},
		{"suffisso pubblico, host diverso", "foo.co.uk", "https://bar.co.uk/", true},
		{"testo non dominio", "clicca qui", "https://evil.example/", false},
		{"testo vuoto", "", "https://evil.example/", false},
		{"nome file non e' dominio", "report.pdf", "https://cdn.example.com/r", false},
		{"indirizzo email come testo, href altro", "info@example.org", "https://evil.example/", true},
		{"indirizzo email come testo, stesso dominio", "info@example.org", "https://www.example.org/c", false},
		{"mailto stesso dominio", "example.org", "mailto:info@example.org", false},
		{"mailto dominio diverso", "example.org", "mailto:info@evil.example", true},
		// Fail-safe (Piano 15, revisione 2026-11-23): href non analizzabile con un
		// dominio nel testo: Warn.
		{"href non analizzabile", "example.org", "%zz", true},
		{"href non analizzabile, testo libero", "clicca qui", "%zz", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DomainMismatch(tt.text, tt.href); got != tt.want {
				t.Errorf("DomainMismatch(%q, %q) = %v, atteso %v", tt.text, tt.href, got, tt.want)
			}
		})
	}
}

// TestDomainMismatchPunteggiaturaEMail (revisione 2026-11-23, punto 1): un
// dominio circondato da punteggiatura, o dentro un indirizzo, vale come
// dominio mostrato; %40 nel mailto e' una @.
func TestDomainMismatchPunteggiaturaEMail(t *testing.T) {
	tests := []struct {
		name, text, href string
		want             bool
	}{
		{"punto finale", "paypal.com.", "https://evil.com/", true},
		{"parentesi", "(paypal.com)", "https://evil.com/", true},
		{"indirizzo", "info@paypal.com", "https://evil.com/", true},
		{"virgolette e due punti", "\"paypal.com\":", "https://evil.com/", true},
		{"punto fullwidth", "paypal。com", "https://evil.com/", true},
		{"IP nudo nel testo", "192.0.2.7", "https://evil.com/", true},
		{"mailto con %40, dominio diverso", "paypal.com", "mailto:x%40evil.com", true},
		{"mailto con %40, stesso dominio", "paypal.com", "mailto:x%40www.paypal.com", false},
		{"mailto con query", "paypal.com", "mailto:x@paypal.com?subject=a", false},
		{"legittimo con punteggiatura", "(www.paypal.com).", "https://mail.paypal.com/", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DomainMismatch(tt.text, tt.href); got != tt.want {
				t.Errorf("DomainMismatch(%q, %q) = %v, atteso %v", tt.text, tt.href, got, tt.want)
			}
		})
	}
}

// TestDomainMismatchFailSafe (punto 2): il controllo cerca TUTTI i domini nel
// testo (anche tra le parole), non si ferma agli spazi e non si arrende se
// url.Parse fallisce.
func TestDomainMismatchFailSafe(t *testing.T) {
	tests := []struct {
		name, text, href string
		want             bool
	}{
		{"dominio dentro una frase", "Vai a paypal.com", "https://evil.com/", true},
		{"URL poi parole", "https://paypal.com/ accedi", "https://evil.com/", true},
		{"due domini, uno falso", "paypal.com o evil.com", "https://evil.com/", true},
		{"href con escape rotto", "https://paypal.com/", "https://evil.com/%zz", true},
		{"testo con escape rotto", "https://paypal.com/%zz", "https://evil.com/", true},
		{"href con escape rotto, stesso dominio", "paypal.com", "https://paypal.com/%zz", false},
		{"testo con escape rotto, stesso dominio", "https://paypal.com/%zz", "https://paypal.com/", false},
		{"href senza host decidibile", "Vai a paypal.com", "https://", true},
		// legittimi: nessun warn
		{"testo senza domini", "Accedi al tuo account", "https://evil.com/", false},
		{"versione e numeri", "versione 1.2 del 3.5", "https://evil.com/", false},
		{"www. dello stesso dominio", "Vai a www.paypal.com", "https://paypal.com/", false},
		{"mail. dello stesso dominio", "Apri mail.paypal.com ora", "https://paypal.com/x", false},
		{"sottodominio dello stesso registrabile", "Vai a paypal.com", "https://a.b.paypal.com/", false},
		{"query con altro dominio nel testo URL", "https://a.com/?u=b.com", "https://a.com/", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DomainMismatch(tt.text, tt.href); got != tt.want {
				t.Errorf("DomainMismatch(%q, %q) = %v, atteso %v", tt.text, tt.href, got, tt.want)
			}
		})
	}
}

// TestBottoneWarnEtichetteInsidiose (punti 1-2): il Warn del bottone nasce dallo
// stesso controllo dei link.
func TestBottoneWarnEtichetteInsidiose(t *testing.T) {
	for _, label := range []string{"paypal.com.", "(paypal.com)", "info@paypal.com", "Vai a paypal.com", "https://paypal.com/ accedi"} {
		b := oneButton(t, `<a href="https://evil.com/x" role="button">`+label+`</a>`)
		if !b.Warn {
			t.Errorf("etichetta %q verso evil.com: Warn = false", label)
		}
	}
	if b := oneButton(t, `<a href="https://www.paypal.com/x" role="button">Vai a paypal.com</a>`); b.Warn {
		t.Errorf("stesso dominio: Warn = true")
	}
}

// TestBottoneHostPunycode (punto 3): l'host del bottone e' sempre ASCII.
func TestBottoneHostPunycode(t *testing.T) {
	b := oneButton(t, "<a href=\"https://аpple.com/x\" role=\"button\">Apple</a>")
	if b.Host != "xn--pple-43d.com" {
		t.Errorf("Host = %q, atteso xn--pple-43d.com", b.Host)
	}
}

func TestASCIIHost(t *testing.T) {
	for in, want := range map[string]string{
		"example.com":      "example.com",
		"аpple.com":        "xn--pple-43d.com",
		"bücher.de":        "xn--bcher-kva.de",
		"xn--bcher-kva.de": "xn--bcher-kva.de",
		"192.0.2.7":        "192.0.2.7",
	} {
		if got := ASCIIHost(in); got != want {
			t.Errorf("ASCIIHost(%q) = %q, atteso %q", in, got, want)
		}
	}
}
