package mcpserver

import "unicode/utf8"

// Budget della risposta (Piano 14, fix review 4). I metadati si contano prima
// del corpo, in byte JSON: il limite di 64 KB non dipende dal contenuto e
// nessuna risposta fallisce per i soli metadati. Gli elementi che non ci stanno
// vengono omessi e dichiarati (attachments_omitted, recipients_omitted).

// reserveBytes copre chiavi JSON, preambolo e marcatori del blocco.
const reserveBytes = 4 * 1024

// attachmentOverhead copre le chiavi, l'indentazione di MarshalIndent e la
// dimensione (fino a 19 cifre) di un allegato: ~72 byte più le cifre.
const attachmentOverhead = 96

// recipientOverhead copre l'indentazione, la virgola e il a capo di ogni
// destinatario nell'array JSON indentato (~6 byte).
const recipientOverhead = 8

type budget struct{ left int }

func newBudget() *budget { return &budget{left: maxResultBytes - reserveBytes} }

// spend scala n byte anche oltre il budget: per i campi scalari, sempre presenti.
func (b *budget) spend(n int) { b.left -= n }

// take prenota n byte solo se ci stanno.
func (b *budget) take(n int) bool {
	if n > b.left {
		return false
	}
	b.left -= n
	return true
}

// remaining restituisce i byte liberi, mai negativi.
func (b *budget) remaining() int { return max(0, b.left) }

// jsonLen stima i byte JSON di una stringa, virgolette ed escape inclusi.
func jsonLen(s string) int {
	n := 2
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		n += jsonRuneLen(r, size)
		i += size
	}
	return n
}

// jsonRuneLen restituisce i byte JSON di una rune: " \ e i controlli \n \r \t
// sono a 2 byte; encoding/json scrive \u00XX per gli altri controlli e per
// < > & U+2028 U+2029 (6 byte); l'UTF-8 invalido diventa U+FFFD (3 byte).
func jsonRuneLen(r rune, size int) int {
	switch {
	case r == '"' || r == '\\' || r == '\n' || r == '\r' || r == '\t':
		return 2
	case r < 0x20 || r == '<' || r == '>' || r == '&' || r == 0x2028 || r == 0x2029:
		return 6
	case r == utf8.RuneError && size == 1:
		return 3
	}
	return size
}

// truncateJSON come Truncate, ma il limite maxJSON è in byte JSON (escape inclusi).
func truncateJSON(s string, maxRunes, maxJSON int) (string, bool) {
	n, used := 0, 2
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		cost := jsonRuneLen(r, size)
		if n+1 > maxRunes || used+cost > maxJSON {
			return s[:i], true
		}
		n++
		used += cost
		i += size
	}
	return s, false
}
