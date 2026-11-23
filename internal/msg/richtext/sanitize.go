package richtext

import (
	"net/url"
	"strings"
	"unicode"
)

// maxURLLen e la lunghezza massima di un href (stesso limite di linkTarget).
const maxURLLen = 2048

// sanitizeText toglie dal testo i caratteri di controllo (tranne \n; \t diventa
// uno spazio) e i byte non UTF-8 (Piano 15, Task 4; completata nel Task 5).
func sanitizeText(s string) string {
	s = strings.ToValidUTF8(s, "�")
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n':
			return r
		case r == '\t':
			return ' '
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, s)
}

// sanitizeURL ripulisce un href e lo accetta solo se http, https o mailto con
// host (o indirizzo) e lunghezza entro maxURLLen (Piano 15, Task 4; completata
// nel Task 5).
func sanitizeURL(raw string) (string, bool) {
	s := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
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
