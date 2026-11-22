// Package childenv costruisce l'ambiente dei comandi esterni lanciati dalla TUI
// (Piano 9, review B; stessa regola del Piano 11, D11-4): la password App
// (GMAIL_APP_PASSWORD) e le variabili GMAIL_TUI_* non escono MAI verso un
// processo esterno, né in ereditarietà né come default.
package childenv

import (
	"os"
	"strings"
)

// Filtered restituisce os.Environ senza GMAIL_APP_PASSWORD e senza le GMAIL_TUI_*.
func Filtered() []string {
	env := os.Environ()
	out := make([]string, 0, len(env))
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if k == "GMAIL_APP_PASSWORD" || strings.HasPrefix(k, "GMAIL_TUI_") {
			continue
		}
		out = append(out, kv)
	}
	return out
}
