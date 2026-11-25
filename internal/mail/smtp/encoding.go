package smtp

// Codifica di trasferimento delle parti di testo (Piano 12a, Task 1, D12-S2).
// Prima text/plain e text/html non dichiaravano Content-Transfer-Encoding, quindi
// valeva il default MIME 7bit: valido solo per ASCII puro con righe fino a 998
// byte (RFC 5322, sezione 2.1.1). Il testo con accenti o con righe lunghe (l'HTML
// che goldmark ricava dal markdown ne ha quasi sempre) usciva fuori regola. Da
// qui ogni parte di testo dichiara come viaggia: 7bit se puo, quoted-printable
// altrimenti.

import (
	"fmt"
	"mime/quotedprintable"
	"strings"
)

// maxRiga7bit e la lunghezza massima di una riga di una parte 7bit, CRLF escluso
// (RFC 5322, sezione 2.1.1).
const maxRiga7bit = 998

// Codifiche dichiarate sulle parti di testo.
const (
	cte7bit = "7bit"
	cteQP   = "quoted-printable"
)

// needsEncoding dice se il testo (gia normalizzato a CRLF) non puo viaggiare
// com'e': contiene un byte sopra 127 o un NUL, oppure una riga sopra maxRiga7bit.
func needsEncoding(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 || s[i] == 0 {
			return true
		}
	}
	for riga := range strings.SplitSeq(s, "\n") {
		if len(strings.TrimSuffix(riga, "\r")) > maxRiga7bit {
			return true
		}
	}
	return false
}

// textPart prepara una parte di testo: il valore di Content-Transfer-Encoding e i
// byte da scrivere. Il testo che puo stare in 7bit esce com'e'; gli altri in
// quoted-printable (righe di 76 caratteri, spazi e tab in fondo alla riga
// codificati), col writer della libreria standard.
func textPart(body string) (cte string, data []byte, err error) {
	if !needsEncoding(body) {
		return cte7bit, []byte(body), nil
	}
	var b strings.Builder
	w := quotedprintable.NewWriter(&b)
	if _, err := w.Write([]byte(body)); err != nil {
		return "", nil, fmt.Errorf("mime: codifica quoted-printable: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", nil, fmt.Errorf("mime: codifica quoted-printable: %w", err)
	}
	return cteQP, []byte(b.String()), nil
}
