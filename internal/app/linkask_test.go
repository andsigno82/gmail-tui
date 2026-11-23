package app

import (
	"strings"
	"testing"
)

// TestLinkAskTextPunycode (revisione Piano 15, punto 3): la conferma mostra l'host
// reale in punycode, anche quando e' l'omografia a essere quello vero.
func TestLinkAskTextPunycode(t *testing.T) {
	got := linkAsk{url: "https://аpple.com/x", shown: "apple.com"}.text()
	if !strings.Contains(got, "xn--pple-43d.com") || strings.Contains(got, "а") {
		t.Errorf("text = %q", got)
	}
	// il dominio mostrato in forma omografa si legge anch'esso in punycode.
	got = linkAsk{url: "https://apple.com/x", shown: "Vai a аpple.com"}.text()
	if !strings.Contains(got, "non a xn--pple-43d.com") {
		t.Errorf("text = %q", got)
	}
}
