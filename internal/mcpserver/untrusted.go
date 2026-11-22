// Package mcpserver espone la posta in sola lettura tramite MCP su stdio
// (Piano 14). Ogni testo che arriva da un'email è dato non fidato: qui si
// racchiude, si sanifica e si limita prima di uscire verso l'agente.
package mcpserver

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Marcatori del contenuto non fidato (Piano 14, Task 1). Il nonce cambia a
// ogni chiamata: un'email non può contraffare la chiusura del blocco perché
// non conosce il valore. Pattern del nonce per chiamata: ni-c/imap-mcp.
const (
	markerBegin = "BEGIN_UNTRUSTED_EMAIL"
	markerEnd   = "END_UNTRUSTED_EMAIL"
)

// ansiRe rimuove sequenze CSI e OSC (Piano 14, Task 1): un'email non deve
// poter pilotare il terminale di chi legge.
var ansiRe = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)?`)

// NewNonce restituisce 16 byte casuali in esadecimale (crypto/rand).
func NewNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Wrap racchiude il testo non fidato tra i marcatori con il nonce dato.
func Wrap(nonce, text string) string {
	return markerBegin + " nonce=" + nonce + "\n" + text + "\n" + markerEnd + " nonce=" + nonce
}

// Sanitize toglie sequenze ANSI/OSC, caratteri di controllo, caratteri invisibili
// (zero-width) e controlli bidi; CRLF diventa LF. Tab e a capo restano.
func Sanitize(s string) string {
	s = ansiRe.ReplaceAllString(s, "")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\r':
			continue
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case unicode.IsControl(r) || isInvisible(r):
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isInvisible copre zero-width, BOM e override/isolamento bidi: usati per
// nascondere testo o invertirne la lettura.
func isInvisible(r rune) bool {
	switch {
	case r == 0x200B, r == 0x200C, r == 0x200D, r == 0x2060, r == 0xFEFF:
		return true
	case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069:
		return true
	}
	return false
}

// Truncate limita s a maxRunes rune e a maxBytes byte, senza spezzare UTF-8.
// Restituisce il testo e true se è stato troncato.
func Truncate(s string, maxRunes, maxBytes int) (string, bool) {
	n := 0
	for i := 0; i < len(s); {
		_, size := utf8.DecodeRuneInString(s[i:])
		if n+1 > maxRunes || i+size > maxBytes {
			return s[:i], true
		}
		n++
		i += size
	}
	return s, false
}
