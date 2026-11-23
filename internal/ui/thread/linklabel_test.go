package thread

import "testing"

// TestLinkLabelPunycode (revisione Piano 15, punto 3): il toast mostra l'host in
// ASCII (punycode) cosi un'omografia non sembra il dominio vero.
func TestLinkLabelPunycode(t *testing.T) {
	for in, want := range map[string]string{
		"https://\u0430pple.com/x":      "xn--pple-43d.com",
		"https://\u0430pple.com:8443/x": "xn--pple-43d.com:8443",
		"https://example.com/x":         "example.com",
		"mailto:info@example.com":       "info@example.com",
	} {
		if got := LinkLabel(in); got != want {
			t.Errorf("LinkLabel(%q) = %q, atteso %q", in, got, want)
		}
	}
}
