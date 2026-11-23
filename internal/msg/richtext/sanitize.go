package richtext

import (
	"net/url"
	"strings"
	"unicode"
)

// maxURLLen e la lunghezza massima di un href (stesso limite di linkTarget in
// internal/ui/thread/links.go).
const maxURLLen = 2048

// Unsafe dice se r va tolto da ogni testo non fidato: e' il filtro UNICO di
// sanificazione (revisione Piano 15, punto 4). Sono i caratteri di controllo (Cc:
// ESC, BEL, C1...) e quelli di formato (Cf: bidi U+202A-U+202E e U+2066-U+206F,
// U+200E/U+200F, U+061C, zero-width U+200B-U+200D, U+2060, U+FEFF, soft hyphen
// U+00AD, tag E0001-E007F...) che possono iniettare sequenze nel terminale,
// nascondere il dominio reale di un link o riordinare il testo (D15-T13).
func Unsafe(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }

// CleanLine rende s una sola riga sicura: ogni spazio (a capo e tab compresi)
// diventa uno spazio singolo, i caratteri Unsafe spariscono, bordi tagliati.
func CleanLine(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsSpace(r):
			return ' '
		case Unsafe(r):
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// CleanHref toglie da un URL spazi e caratteri Unsafe: un ESC o BEL non deve
// poter chiudere l'OSC 8 ne iniettare sequenze.
func CleanHref(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || Unsafe(r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(s, ""))
}

// sanitizeText toglie dal testo i caratteri Unsafe (tranne \n; \t diventa uno
// spazio per non incollare le parole) e sostituisce i byte non UTF-8 (Piano 15,
// Task 5, D15-T13).
func sanitizeText(s string) string {
	s = strings.ToValidUTF8(s, "�")
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n':
			return r
		case r == '\t':
			return ' '
		case Unsafe(r):
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
	s := CleanHref(raw)
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
