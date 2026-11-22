package msg

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"
)

// ValidateRecipients controlla i destinatari prima del round-trip SMTP
// (Piano 7, Task 2): almeno un destinatario non vuoto in To/Cc/Bcc e ogni
// indirizzo valido per net/mail.ParseAddress (RFC 5322, nome visualizzato
// ammesso). L'errore cita il campo con le etichette del composer (A, Cc,
// Ccn) e l'indirizzo. Gli elementi vuoti sono ignorati, come in splitAddrs.
func ValidateRecipients(o Outgoing) error {
	fields := []struct {
		label string
		addrs []string
	}{{"A", o.To}, {"Cc", o.Cc}, {"Ccn", o.Bcc}}

	n := 0
	for _, f := range fields {
		for _, a := range f.addrs {
			a = strings.TrimSpace(a)
			if a == "" {
				continue
			}
			if _, err := mail.ParseAddress(a); err != nil {
				return fmt.Errorf("%s: indirizzo non valido %q", f.label, a)
			}
			n++
		}
	}
	if n == 0 {
		return errors.New("nessun destinatario")
	}
	return nil
}
