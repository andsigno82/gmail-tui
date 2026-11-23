package thread

// Zone cliccabili del thread (Piano 15, Task 14; link nel corpo: richiesta di Andrea,
// 2026-11-23). La mappa clic e unica: ogni elemento cliccabile (link, bottone,
// chip allegato) e una render.Zone dentro l'Out del corpo o dei chip; LinkAt,
// ZoneAt e LinkClick la interrogano. I link si aprono con il clic sinistro nel
// browser di sistema e ogni porzione visibile e avvolta in OSC 8 (dal renderer)
// per il cmd+clic nativo. Il contenuto e non fidato: le zone portano solo URL
// http, https e mailto gia ripuliti da render (safeHref); nessuna richiesta di
// rete dall'app.

import (
	"net/url"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/render"
)

// LinkOpenedMsg e l'esito dell'apertura di un link nel browser: Target e la
// destinazione mostrata nel toast (host, o indirizzo per mailto).
type LinkOpenedMsg struct {
	Target string
	Err    error
}

// ButtonClickMsg: clic su un bottone del corpo (Piano 15, Task 14, D15-01). URL e
// la destinazione gia sanificata (solo http, https, mailto). Il thread NON apre
// nulla: l'app decide (Piano 15, Task 16), per esempio con lo stesso browser di
// sistema dei link e un toast con l'host.
type ButtonClickMsg struct {
	URL   string
	Warn  bool   // testo con dominio diverso dal reale: l'app chiede conferma (Piano 15, Task 16)
	Shown string // testo mostrato del bottone, per la conferma
}

// LinkWarnMsg: clic su un link con Warn (Piano 15, Task 16). Il thread NON lo apre:
// l'app chiede conferma nella barra ("il link porta a <host>, non a <mostrato>").
type LinkWarnMsg struct {
	URL   string
	Shown string
}

// AttachmentClickMsg: clic su un chip allegato (Piano 15, Task 14, D15-06). ID e
// il messaggio selezionato che porta l'allegato (scarta i messaggi stantii);
// Index e la posizione in msg.Message.Attachments di quel messaggio (non il
// PartIndex: la radice lo ricava da li). L'apertura vera (allowlist dei tipi, file
// temporaneo, D15-T14) e dell'app (Piano 15, Task 16); `:save` resta com'e.
type AttachmentClickMsg struct {
	ID    msg.MessageID
	Index int
}

// openLink apre un URL nel browser di sistema con il comando di sistema (mai una
// shell). Variabile per i test: nei test non si apre mai un browser vero.
var openLink = func(u string) error { return browserCmd(u).Run() }

// linkLabel e la destinazione mostrata nel toast: l'host, o l'indirizzo per mailto.
func linkLabel(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return s
	}
	if u.Opaque != "" {
		return u.Opaque
	}
	return u.Host
}

// OpenURL apre un URL gia sanificato nel browser di sistema (stesso comando senza
// shell e ambiente filtrato dei link): lo usa l'app per i bottoni e per i link
// confermati (Piano 15, Task 16). LinkLabel e la destinazione per il toast.
func OpenURL(u string) error { return openLink(u) }

// LinkLabel e linkLabel esportata (host, o indirizzo per mailto).
func LinkLabel(s string) string { return linkLabel(s) }

// ZoneAt restituisce la zona cliccabile sotto la cella (x, y) della vista thread,
// con le coordinate relative all'origine della vista (y include la scheda
// dell'header). Copre il corpo (scorrimento verticale e orizzontale del viewport
// compresi) e i chip degli allegati sotto il corpo (Piano 15, Task 14).
func (m Model) ZoneAt(x, y int) (render.Zone, bool) {
	if n := len(m.attZones); n > 0 {
		if top := headerLines + m.bodyHeight() + 1; y >= top && y < top+n {
			return render.ZoneAt(m.attZones[y-top], x)
		}
	}
	if y < headerLines || y-headerLines >= m.bodyHeight() {
		return render.Zone{}, false
	}
	row := y - headerLines + m.vp.YOffset()
	if row < 0 || row >= len(m.linkRows) {
		return render.Zone{}, false
	}
	return render.ZoneAt(m.linkRows[row], x+m.vp.XOffset())
}

// LinkAt restituisce l'URL del link o del bottone sotto la cella (x, y); i chip
// degli allegati non hanno URL (usare ZoneAt). Stessa firma di sempre.
func (m Model) LinkAt(x, y int) (string, bool) {
	z, ok := m.ZoneAt(x, y)
	if !ok || z.URL == "" {
		return "", false
	}
	return z.URL, true
}

// LinkClick: se il clic (x, y) cade su una zona cliccabile, la Cmd corrispondente.
// Link: apre il browser di sistema e riporta LinkOpenedMsg. Bottone: emette
// ButtonClickMsg. Allegato: emette AttachmentClickMsg (le aperture vere sono
// dell'app, Piano 15, Task 16). Altrove false: nessun effetto.
func (m Model) LinkClick(x, y int) (tea.Cmd, bool) {
	z, ok := m.ZoneAt(x, y)
	if !ok {
		return nil, false
	}
	switch z.Kind {
	case render.ZoneLink:
		u := z.URL
		if z.Warn {
			// Piano 15, Task 16: dominio mostrato diverso dal reale, mai aperto al volo.
			warn := LinkWarnMsg{URL: u, Shown: z.Shown}
			return func() tea.Msg { return warn }, true
		}
		return func() tea.Msg {
			return LinkOpenedMsg{Target: linkLabel(u), Err: openLink(u)}
		}, true
	case render.ZoneButton:
		click := ButtonClickMsg{URL: z.URL, Warn: z.Warn, Shown: z.Shown}
		return func() tea.Msg { return click }, true
	case render.ZoneAttachment:
		click := AttachmentClickMsg{ID: m.selRef().ID, Index: z.Index}
		return func() tea.Msg { return click }, true
	}
	return nil, false
}
