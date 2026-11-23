package richtext

import (
	"net/url"
	"strings"
	"unicode"
)

// maxURLLen e la lunghezza massima di un href (stesso limite di linkTarget in
// internal/ui/thread/links.go).
const maxURLLen = 2048

// isInvisible dice se r e' un carattere bidi o zero-width che puo' nascondere
// il dominio reale di un link o riordinare il testo (Piano 15, Task 5,
// D15-T13): U+202A-U+202E, U+2066-U+2069, U+200E/U+200F (bidi), U+061C (ALM),
// U+200B-U+200D, U+2060, U+FEFF (zero-width).
func isInvisible(r rune) bool {
	switch {
	case r >= 0x202A && r <= 0x202E,
		r >= 0x2066 && r <= 0x2069,
		r == 0x200E, r == 0x200F, r == 0x061C,
		r >= 0x200B && r <= 0x200D,
		r == 0x2060, r == 0xFEFF:
		return true
	}
	return false
}

// sanitizeText toglie dal testo i caratteri di controllo (tranne \n; \t diventa
// uno spazio per non incollare le parole), i bidi e gli zero-width, e sostituisce
// i byte non UTF-8 (Piano 15, Task 5, D15-T13).
func sanitizeText(s string) string {
	s = strings.ToValidUTF8(s, "�")
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n':
			return r
		case r == '\t':
			return ' '
		case unicode.IsControl(r), isInvisible(r):
			return -1
		}
		return r
	}, s)
}

// sanitizeURL ripulisce un href (controlli, spazi, bidi e zero-width si
// tolgono prima di validare) e lo accetta solo se http, https o mailto con host
// (o indirizzo) e lunghezza entro maxURLLen (Piano 15, Task 5, D15-T13). Stessa
// logica di linkTarget in internal/ui/thread/links.go: il Task 14 fa puntare
// quella funzione a questa, senza duplicarla.
func sanitizeURL(raw string) (string, bool) {
	s := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.IsSpace(r) || isInvisible(r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(raw, ""))
	if s == "" || len(s) > maxURLLen {
		return "", false
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host == "" {
			return "", false
		}
	case "mailto":
		if u.Opaque == "" {
			return "", false
		}
	default:
		return "", false
	}
	return s, true
}
