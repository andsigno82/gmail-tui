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
		{"indirizzo email come testo", "info@example.org", "https://evil.example/", false},
		{"mailto stesso dominio", "example.org", "mailto:info@example.org", false},
		{"mailto dominio diverso", "example.org", "mailto:info@evil.example", true},
		{"href non analizzabile", "example.org", "%zz", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DomainMismatch(tt.text, tt.href); got != tt.want {
				t.Errorf("DomainMismatch(%q, %q) = %v, atteso %v", tt.text, tt.href, got, tt.want)
			}
		})
	}
}
