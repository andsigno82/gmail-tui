package msg

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"golang.org/x/net/idna"
)

// ValidateRecipients controlla i destinatari prima del round-trip SMTP
// (Piano 7, Task 2): almeno un destinatario non vuoto in To/Cc/Bcc e ogni
// indirizzo valido (regole di checkAddress, Piano 7.1 Task 1). L'errore cita il
// campo con le etichette del composer (A, Cc, Ccn) e l'indirizzo. Gli elementi
// vuoti sono ignorati, come in normalizeList (Piano 7.1, fix code-review).
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
// (Piano 7, Task 6): le liste vuote passano. È il primo controllo di
// ValidateRecipients, che il composer (prima della conferma d'invio, fix review
// 7) e `send` chiamano; la presenza di almeno un destinatario sta solo lì.
func ValidateAddresses(o Outgoing) error {
	for _, f := range recipientFields(o) {
		for _, a := range f.addrs {
			a = strings.TrimSpace(a)
			if a == "" {
				continue
			}
			if _, err := checkAddress(a); err != nil {
				return fmt.Errorf("%s: indirizzo non valido %q", f.label, a)
			}
		}
	}
	return nil
}

// NormalizeRecipients valida To/Cc/Bcc come ValidateRecipients e restituisce
// una copia di o con ogni indirizzo in forma canonica: mail.Address.String()
// se c'è un nome visualizzato (RFC 2047 se non ASCII), indirizzo nudo altrimenti;
// dominio convertito in punycode con
// idna.Lookup.ToASCII. Local part non ASCII → errore (niente SMTPUTF8).
// Piano 7.1, Task 1 (decisione D7.1).
func NormalizeRecipients(o Outgoing) (Outgoing, error) {
	if err := ValidateRecipients(o); err != nil {
		return Outgoing{}, err
	}
	return Outgoing{
		From:        o.From,
		To:          normalizeList(o.To),
		Cc:          normalizeList(o.Cc),
		Bcc:         normalizeList(o.Bcc),
		Subject:     o.Subject,
		BodyText:    o.BodyText,
		Attachments: o.Attachments,
		Markdown:    o.Markdown,
	}, nil
}

// normalizeList canonicalizza una lista già validata: scarta i vuoti, conserva
// ordine e duplicati (il dedup resta del Sender). Lista senza elementi ⇒ nil.
func normalizeList(addrs []string) []string {
	var out []string
	for _, a := range addrs {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		// Già validato da ValidateRecipients: l'errore qui non può accadere.
		c, _ := checkAddress(a)
		out = append(out, c)
	}
	return out
}

// ParseRecipientField legge un campo A/Cc/Ccn come lista RFC 5322 (Piano 7.1,
// fix code-review): la virgola nel nome quotato non lo spezza, il campo vuoto
// non dà indirizzi. Un campo non parsabile resta come elemento unico (trimmato):
// NormalizeRecipients lo rifiuta citando il campo, senza perdere indirizzi.
func ParseRecipientField(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	list, err := mail.ParseAddressList(s)
	if err != nil {
		return []string{s}
	}
	out := make([]string, 0, len(list))
	for _, a := range list {
		out = append(out, addrForm(a))
	}
	return out
}

// BareAddress restituisce l'addr-spec di un indirizzo canonico, cioè la forma
// per RCPT TO (Piano 7.1, fix code-review): il nome visualizzato è scartato e la
// local part resta quotata se serve ("a b"@x.it).
func BareAddress(a string) (string, error) {
	p, err := mail.ParseAddress(a)
	if err != nil {
		return "", err
	}
	return addrSpec(p.Address), nil
}

// addrForm restituisce la forma canonica di un indirizzo già parsato: nome
// visualizzato (Address.String, RFC 2047 se non ASCII) oppure addr-spec nuda.
func addrForm(a *mail.Address) string {
	if a.Name == "" {
		return addrSpec(a.Address)
	}
	return a.String()
}

// addrSpec restituisce l'addr-spec con la local part quotata se serve.
// Address.String la racchiude in <>, che qui vengono tolte (Piano 7.1, fix
// code-review: prima la local part quotata perdeva le virgolette).
func addrSpec(addr string) string {
	s := (&mail.Address{Address: addr}).String()
	return strings.TrimSuffix(strings.TrimPrefix(s, "<"), ">")
}

// checkAddress valida un indirizzo e ne restituisce la forma canonica (Piano
// 7.1, Task 1): net/mail.ParseAddress (nome visualizzato ammesso), local part
// ASCII, dominio con almeno un punto (D7.1) e convertibile in punycode.
func checkAddress(a string) (string, error) {
	parsed, err := mail.ParseAddress(a)
	if err != nil {
		return "", err
	}
	at := strings.LastIndex(parsed.Address, "@")
	local, domain := parsed.Address[:at], parsed.Address[at+1:]
	if !isASCII(local) {
		return "", errors.New("local part non ASCII")
	}
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || strings.Contains(domain, "..") {
		return "", errors.New("dominio senza punto")
	}
	ascii, err := idna.Lookup.ToASCII(domain)
	if err != nil {
		return "", err
	}
	parsed.Address = local + "@" + ascii
	return addrForm(parsed), nil
}

// isASCII riporta se s contiene solo caratteri ASCII.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
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
