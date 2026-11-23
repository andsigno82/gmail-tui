package richtext

// Controllo di dominio anti-phishing: UNICA implementazione, usata dai bottoni
// (Block.Warn), dai link del corpo (internal/ui/render) e dalla conferma
// dell'app (Piano 15, revisione indipendente 2026-11-23, punti 1-3). Il
// contenuto dell'email e' non fidato: il controllo e' fail-safe, cioe' nel
// dubbio segnala.

import (
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

// dotClass: il punto ASCII e le varianti che idna normalizza a '.' (U+3002,
// U+FF0E, U+FF61): "paypal。com" e' paypal.com per il browser.
const dotClass = `[.\x{3002}\x{FF0E}\x{FF61}]`

// shownRE trova un URL o un dominio dentro un token del testo: schema://
// opzionale (gruppo 1), utente@ opzionale (gruppo 2) e host (gruppo 3: nome con
// almeno due etichette, anche Unicode, oppure IPv6 tra parentesi quadre). La
// porta e il resto del percorso non servono.
var shownRE = regexp.MustCompile(`(?i)(?:([a-z][a-z0-9+.-]*)://)?(?:([^\s/?#@]*)@)?([\p{L}\p{N}][\p{L}\p{N}-]*(?:` +
	dotClass + `[\p{L}\p{N}-]+)+|\[[0-9a-f:.]+\])`)

// hrefHostRE estrae l'host da un href che url.Parse rifiuta (escape rotti nel
// percorso, per esempio): regex prudente, schema://[utente@]host.
var hrefHostRE = regexp.MustCompile(`(?i)^\s*[a-z][a-z0-9+.-]*://(?:[^/?#@]*@)?(\[[^\]]*\]|[^/?#:@]+)`)

// DomainMismatch dice se il testo di un link mostra un URL o un dominio diverso
// da quello reale di href (D15-07, D15-T6). Si cercano TUTTI i token URL o
// dominio del testo (anche dentro una frase, tra punteggiatura o in un indirizzo
// email) e ciascuno si confronta con gli host reali per dominio registrabile
// (publicsuffix.EffectiveTLDPlusOne dopo idna.Lookup); per IP e suffissi
// pubblici si confronta l'host esatto. Un testo senza domini ("clicca qui",
// "report.pdf") non da mai warn. Fail-safe: se il testo mostra un dominio ma
// l'host reale non si riesce a determinare, e' warn.
func DomainMismatch(text, href string) bool {
	shown := shownHosts(text)
	if len(shown) == 0 {
		return false
	}
	real := hrefHosts(href)
	if len(real) == 0 {
		return true
	}
	for _, s := range shown {
		want := registrable(s)
		for _, r := range real {
			if registrable(r) != want {
				return true
			}
		}
	}
	return false
}

// ShownHost e il primo host mostrato dal testo di un link (normalizzato ASCII,
// senza porta ne percorso): serve alla conferma dei link con Warn.
func ShownHost(text string) (string, bool) {
	if h := shownHosts(text); len(h) > 0 {
		return h[0], true
	}
	return "", false
}

// HrefHost e l'host reale di destinazione di un href (il dominio del primo
// indirizzo per mailto), in forma ASCII; vuoto se non si determina.
func HrefHost(href string) string {
	if h := hrefHosts(href); len(h) > 0 {
		return h[0]
	}
	return ""
}

// ASCIIHost porta un host con caratteri non ASCII in punycode (idna.Lookup), cosi
// un'omografia (a cirillica) non si legge come il dominio vero. Un host gia
// ASCII, o che idna rifiuta, resta com'e.
func ASCIIHost(h string) string {
	ascii := true
	for i := 0; i < len(h); i++ {
		if h[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return h
	}
	if a, err := idna.Lookup.ToASCII(h); err == nil {
		return a
	}
	return h
}

// shownHosts elenca gli host mostrati dal testo (normalizzati). Il testo si
// divide in token (spazi e separatori); per ogni token conta il primo
// URL/dominio vero, quello che il lettore vede come destinazione: un dominio
// dopo il percorso ("a.com/?u=b.com") non e' una seconda destinazione. Con
// schema://utente@host anche l'utente, se e' un dominio, conta ("https://
// paypal.com@evil.com").
func shownHosts(text string) []string {
	t := strings.ToLower(sanitizeText(text))
	tokens := strings.FieldsFunc(t, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(`|,;<>"'()`, r)
	})
	var out []string
	for _, tok := range tokens {
		for _, m := range shownRE.FindAllStringSubmatch(tok, -1) {
			h := normHost(m[3])
			// Con schema:// l'host e' dichiarato (anche un TLD non ICANN); senza,
			// serve un dominio vero: "file.txt" o "v1.2" non lo sono.
			if m[1] == "" && !domainLike(h) {
				continue
			}
			out = append(out, h)
			if m[1] != "" && m[2] != "" {
				if u := normHost(m[2]); domainLike(u) {
					out = append(out, u)
				}
			}
			break
		}
	}
	return out
}

// hrefHosts sono gli host reali di destinazione, normalizzati: l'host dell'URL
// (con la regex prudente se url.Parse fallisce) o i domini di tutti gli
// indirizzi di un mailto (%40 compreso; ci si fida solo se concordano).
func hrefHosts(href string) []string {
	h := strings.TrimSpace(href)
	if len(h) >= 7 && strings.EqualFold(h[:7], "mailto:") {
		rest := h[7:]
		if i := strings.IndexByte(rest, '?'); i >= 0 {
			rest = rest[:i]
		}
		if dec, err := url.PathUnescape(rest); err == nil {
			rest = dec
		}
		var hosts []string
		for _, addr := range strings.Split(rest, ",") {
			if i := strings.LastIndexByte(addr, '@'); i >= 0 {
				if d := normHost(strings.TrimSpace(strings.Trim(addr[i+1:], "<> "))); d != "" {
					hosts = append(hosts, d)
				}
			}
		}
		return hosts
	}
	if u, err := url.Parse(h); err == nil && u.Hostname() != "" {
		return []string{normHost(u.Hostname())}
	}
	if m := hrefHostRE.FindStringSubmatch(h); m != nil {
		return []string{normHost(m[1])}
	}
	return nil
}

// normHost porta un host in forma confrontabile: minuscolo, senza punto finale,
// ASCII via idna.Lookup (le omografie diventano xn-- diversi dall'originale).
// Un IP resta tale (forma canonica). Se idna rifiuta l'host si ricade sul
// minuscolo grezzo: mai fidarsi di cio che non si normalizza.
func normHost(h string) string {
	h = strings.TrimSuffix(strings.ToLower(strings.Trim(h, "[]")), ".")
	if a, err := netip.ParseAddr(h); err == nil {
		return a.Unmap().String()
	}
	if a, err := idna.Lookup.ToASCII(h); err == nil {
		return a
	}
	return h
}

// registrable e il dominio registrabile (eTLD+1) di un host normalizzato. Per IP,
// suffissi pubblici e host non calcolabili ricade sull'host esatto (D15-T6).
func registrable(h string) string {
	if _, err := netip.ParseAddr(h); err == nil {
		return h
	}
	if r, err := publicsuffix.EffectiveTLDPlusOne(h); err == nil {
		return r
	}
	return h
}

// domainLike dice se h (gia normalizzato) e un dominio vero e non una parola con
// un punto: un IP, oppure un nome con un suffisso gestito (ICANN o privato).
// Un TLD sconosciuto ("file.txt", "v1.2") non lo e: niente falsi positivi.
func domainLike(h string) bool {
	if h == "" {
		return false
	}
	if _, err := netip.ParseAddr(h); err == nil {
		return true
	}
	if !strings.Contains(h, ".") {
		return false
	}
	for _, l := range strings.Split(h, ".") {
		if l == "" {
			return false
		}
	}
	suffix, icann := publicsuffix.PublicSuffix(h)
	return icann || strings.Contains(suffix, ".")
}
