package richtext

import (
	"net"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

// domainRE: dominio nudo (almeno due etichette, lettere e cifre anche Unicode).
var domainRE = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N}-]*(\.[\p{L}\p{N}][\p{L}\p{N}-]*)+$`)

// portRE: porta finale di un host nudo.
var portRE = regexp.MustCompile(`:\d{1,5}$`)

// DomainMismatch dice se il testo di un link mostra un URL o un dominio
// diverso da quello reale di href (Piano 15, Task 4, D15-07, D15-T6). Entrambi
// i lati si normalizzano con idna.Lookup (ASCII) e si confrontano per dominio
// registrabile (publicsuffix.EffectiveTLDPlusOne); se non si calcola (IP,
// suffisso pubblico) si confronta l'host esatto. Un testo che non e' un URL ne'
// un dominio con TLD noto ("clicca qui", "report.pdf") non da mai warn.
func DomainMismatch(text, href string) bool {
	th, ok := textHost(text)
	if !ok {
		return false
	}
	hh := hostOf(href)
	if hh == "" {
		return false
	}
	return registrable(th) != registrable(hh)
}

// textHost estrae l'host mostrato dal testo di un link, se il testo e' un URL
// o un dominio nudo.
func textHost(text string) (string, bool) {
	t := strings.ToLower(strings.TrimSpace(sanitizeText(text)))
	if t == "" || strings.ContainsAny(t, " \n") {
		return "", false
	}
	var host string
	if strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://") {
		u, err := url.Parse(t)
		if err != nil || u.Hostname() == "" {
			return "", false
		}
		host = u.Hostname()
	} else {
		if i := strings.IndexAny(t, "/?#"); i >= 0 {
			t = t[:i]
		}
		host = portRE.ReplaceAllString(t, "")
		if !domainRE.MatchString(host) {
			return "", false
		}
		// solo TLD noti (ICANN): "report.pdf" non e' un dominio.
		if _, icann := publicsuffix.PublicSuffix(toASCII(host)); !icann {
			return "", false
		}
	}
	return strings.TrimSuffix(host, "."), true
}

// toASCII normalizza un host IDN in ASCII (punycode); se non e' valido ritorna
// l'host com'e'.
func toASCII(host string) string {
	if a, err := idna.Lookup.ToASCII(host); err == nil {
		return a
	}
	return host
}

// registrable ritorna il dominio registrabile dell'host normalizzato, o l'host
// esatto per IP e suffissi pubblici.
func registrable(host string) string {
	h := toASCII(strings.TrimSuffix(host, "."))
	if net.ParseIP(h) != nil {
		return h
	}
	if d, err := publicsuffix.EffectiveTLDPlusOne(h); err == nil {
		return d
	}
	return h
}
