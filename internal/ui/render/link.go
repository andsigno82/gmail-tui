package render

// Link con controllo di dominio anti-phishing (Piano 15, Task 6, D15-07, D15-T6).
// Il contenuto dell'email e non fidato: l'href e ripulito prima di OGNI uso (zona
// e OSC 8), solo http, https e mailto, mai richieste di rete. Il testo mostrato
// e ripulito da controlli e caratteri di formato (RLO, zero width) prima di
// essere reso: non puo iniettare sequenze nel terminale ne nascondere il dominio.

import (
	"fmt"
	"hash/fnv"
	"net/url"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/msg/richtext"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// maxLinkLen e la lunghezza massima (in byte) di un URL cliccabile: come
// internal/ui/thread/links.go, oltre resta testo.
const maxLinkLen = 2048

// osc8Close chiude una porzione di link (OSC 8 con ST = ESC \), come nel thread
// (internal/ui/thread/links.go): niente BEL come terminatore.
const osc8Close = "\x1b]8;;\x1b\\"

// osc8Open apre una porzione di link: ESC ] 8 ; id=<id> ; <url> ESC \. L'URL e
// gia ripulito da safeHref, quindi non puo chiudere la sequenza. Si compone a
// mano (come il thread) perche lipgloss.Hyperlink chiude con BEL.
func osc8Open(u, id string) string {
	return "\x1b]8;id=" + id + ";" + u + "\x1b\\"
}

// warnMarker precede il testo di un link sospetto: mai solo colore (D15-T6).
const warnMarker = "⚠ "

// cleanText rende il testo mostrato una sola riga sicura (filtro unico di
// richtext: controlli e caratteri di formato tolti, spazi collassati).
func cleanText(s string) string { return richtext.CleanLine(s) }

// cleanHref toglie dall'URL spazi, controlli e caratteri di formato (filtro unico
// di richtext, lo stesso di richtext.sanitizeURL).
func cleanHref(s string) string { return richtext.CleanHref(s) }

// safeHref ripulisce raw e dice se e apribile: schema http, https o mailto, con
// host (o indirizzo) e lunghezza entro maxLinkLen. Ritorna anche l'URL parsato.
func safeHref(raw string) (string, *url.URL, bool) {
	s := cleanHref(raw)
	if s == "" || len(s) > maxLinkLen {
		return "", nil, false
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", nil, false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Hostname() == "" {
			return "", nil, false
		}
	case "mailto":
		if u.Opaque == "" {
			return "", nil, false
		}
	default:
		return "", nil, false
	}
	return s, u, true
}

// underlineStyle applica a s il tipo di sottolineatura u e, se lo stile ha un
// colore di primo piano, lo usa come colore della sottolineatura (i valori veri
// vengono dal tema: qui nessun colore fisso).
func underlineStyle(s lipgloss.Style, u lipgloss.Underline) lipgloss.Style {
	s = s.UnderlineStyle(u)
	if fg := s.GetForeground(); fg != (lipgloss.NoColor{}) {
		s = s.UnderlineColor(fg)
	}
	return s
}

// wrapPlain spezza testo semplice (una riga) su width colonne, a parole e, per
// le parole piu larghe, per carattere. Con width <= 0 nessun a capo. Le righe non
// hanno spazi ai bordi e non sono mai vuote.
func wrapPlain(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	var out []string
	for _, l := range strings.Split(lipgloss.Wrap(s, width, ""), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		return []string{s}
	}
	return out
}

// Prefissi dell'id OSC 8: il prefisso di un link sospetto (idPrefixWarn) e
// l'unico segnale che scanZones legge per impostare Zone.Warn sulle righe rese
// dal motore dei Runs (Piano 15, Task 14, lavoro A).
const (
	idPrefix     = "gmr"
	idPrefixWarn = "gmw"
)

// linkInfo e un link pronto per essere reso: target ripulito, testo da mostrare
// (senza marcatore) e verdetto del controllo di dominio.
type linkInfo struct {
	target, display string
	warn            bool
}

// prepLink ripulisce href (safeHref) e testo (cleanText) e applica il controllo di
// dominio (D15-07): e la logica di Link, condivisa col motore dei Runs
// (paragraph.go). Testo vuoto: si mostra l'URL. ok e false se l'href non e
// apribile: il chiamante rende testo semplice senza zona.
func prepLink(text, href string) (linkInfo, bool) {
	target, _, ok := safeHref(href)
	if !ok {
		return linkInfo{}, false
	}
	display := cleanText(text)
	if display == "" {
		display = cleanText(target)
	}
	// Unica implementazione del controllo: richtext (revisione 2026-11-23, punto 1).
	return linkInfo{target: target, display: display, warn: richtext.DomainMismatch(display, target)}, true
}

// linkID e l'id OSC 8 stabile di un link: dipende da target e testo mostrato
// (marcatore compreso) e porta il prefisso che distingue i link sospetti.
func linkID(li linkInfo) string {
	shown, prefix := li.display, idPrefix
	if li.warn {
		shown, prefix = warnMarker+shown, idPrefixWarn
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(li.target + "\x00" + shown))
	return fmt.Sprintf("%s%08x", prefix, h.Sum32())
}

// linkStyle e lo stile di un link: tratteggiato, oppure ondulato in colore errore
// se sospetto (D15-07).
func linkStyle(st styles.Styles, warn bool) lipgloss.Style {
	if warn {
		return underlineStyle(st.LinkWarn, lipgloss.UnderlineCurly)
	}
	return underlineStyle(st.LinkDashed, lipgloss.UnderlineDashed)
}

// Link rende un link con controllo di dominio (Piano 15, Task 6, D15-07). Testo
// vuoto: si mostra l'URL. Con href non apribile (schema vietato, troppo lungo,
// senza host) il testo resta semplice e senza zone. Altrimenti il testo e
// sottolineato tratteggiato; se si presenta come un dominio diverso da quello
// reale (D15-T6) la zona ha Warn, il testo il marcatore e la sottolineatura e
// ondulata in colore errore. Ogni riga del wrap ha una zona [0,larghezza) con lo
// stesso id OSC 8, come il thread.
func Link(text, href string, c Ctx) Out {
	li, ok := prepLink(text, href)
	if !ok {
		display := cleanText(text)
		if display == "" {
			return Out{}
		}
		lines := wrapPlain(display, c.Width)
		return Out{Lines: lines, Zones: make([][]Zone, len(lines))}
	}
	display, warn, target := li.display, li.warn, li.target
	if warn {
		display = warnMarker + display
	}
	base := linkStyle(c.St, warn)
	id := linkID(li)

	lines := wrapPlain(display, c.Width)
	o := Out{Lines: make([]string, len(lines)), Zones: make([][]Zone, len(lines))}
	for i, l := range lines {
		o.Lines[i] = osc8Open(target, id) + base.Render(l) + osc8Close
		o.Zones[i] = []Zone{{X0: 0, X1: lipgloss.Width(l), Kind: ZoneLink, URL: target, ID: id, Warn: warn}}
		if warn {
			o.Zones[i][0].Shown = li.display // Piano 15, Task 16: per la conferma dell'app
		}
	}
	return o
}
