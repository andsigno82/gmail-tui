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
	if err := ValidateAddresses(o); err != nil {
		return err
	}
	if countRecipients(o) == 0 {
		return errors.New("nessun destinatario")
	}
	return nil
}

// ValidateAddresses controlla solo il formato degli indirizzi di To/Cc/Bcc
// (Piano 7, Task 6): le liste vuote passano. Il composer lo usa prima della
// conferma d'invio: una bozza senza destinatari resta invio-able (il
// placeholder "(nessun destinatario)" della conferma); la presenza di almeno
// un destinatario è di ValidateRecipients, usata da `send`.
func ValidateAddresses(o Outgoing) error {
	for _, f := range recipientFields(o) {
		for _, a := range f.addrs {
			a = strings.TrimSpace(a)
			if a == "" {
				continue
			}
			if _, err := mail.ParseAddress(a); err != nil {
				return fmt.Errorf("%s: indirizzo non valido %q", f.label, a)
			}
		}
	}
	return nil
}

// countRecipients conta gli indirizzi non vuoti di To/Cc/Bcc.
func countRecipients(o Outgoing) int {
	n := 0
	for _, f := range recipientFields(o) {
		for _, a := range f.addrs {
			if strings.TrimSpace(a) != "" {
				n++
			}
		}
	}
	return n
}

// recipientFields elenca i campi destinatari con le etichette del composer.
func recipientFields(o Outgoing) []struct {
	label string
	addrs []string
} {
	return []struct {
		label string
		addrs []string
	}{{"A", o.To}, {"Cc", o.Cc}, {"Ccn", o.Bcc}}
}
