// Pannello label su selezione (Piano 10, Task 7, D10-L1/L2): `l` con Gmail apre
// l'elenco di tutte le label utente. Ogni label ha la casella ☑ se è su tutte le
// conversazioni bersaglio, ◪ (stato misto) se è solo su alcune, ☐ se su nessuna.
// Spazio cambia la casella, digitare filtra, invio applica aggiunte e rimozioni
// insieme. Questo file tiene la logica (pura) del pannello e il suo cablaggio: apertura,
// tasti, comandi verso lo store. Il rendering è in internal/ui/labelpick.
package app

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/labelpick"
	"github.com/andsigno82/gmail-tui/internal/ui/toast"
)

// labelFilterPrompt è il prompt del filtro del pannello.
const labelFilterPrompt = "filtro: "

// presence è in quante conversazioni bersaglio c'è una label.
type presence int

const (
	presNone presence = iota // in nessuna
	presSome                 // in alcune: stato misto (◪) finché non si tocca
	presAll                  // in tutte (☑)
)

// presenceOf conta le conversazioni bersaglio che hanno la label (Conversation.Labels,
// unione delle label dei messaggi della conversazione).
func presenceOf(name string, convs []mail.Conversation) presence {
	n := 0
	for _, c := range convs {
		if slices.Contains(c.Labels, name) {
			n++
		}
	}
	switch {
	case n == 0:
		return presNone
	case n == len(convs):
		return presAll
	}
	return presSome
}

// pickLabel è una riga del pannello. on è la casella corrente; touched dice se
// l'utente l'ha cambiata: solo le righe toccate producono aggiunte o rimozioni.
type pickLabel struct {
	name    string
	start   presence
	on      bool
	touched bool
}

// mixed dice se la riga è nello stato misto iniziale, non ancora toccata.
func (p pickLabel) mixed() bool { return !p.touched && p.start == presSome }

// mark è la casella della riga: ◪ misto, ☑ attiva, ☐ spenta.
func (p pickLabel) mark() string {
	switch {
	case p.mixed():
		return "◪"
	case p.on:
		return "☑"
	}
	return "☐"
}

// toggle cambia la casella (spazio). Dallo stato misto va a ☑ (misto → ☑ → ☐ → ☑).
func (p *pickLabel) toggle() {
	if p.mixed() {
		p.on = true
	} else {
		p.on = !p.on
	}
	p.touched = true
}

// change dice se la riga va aggiunta a tutte le bersaglio (add) o rimossa da tutte
// (remove), rispetto alla presenza iniziale. Una riga non toccata non cambia nulla.
func (p pickLabel) change() (add, remove bool) {
	if !p.touched {
		return false, false
	}
	if p.on {
		return p.start != presAll, false
	}
	return false, p.start != presNone
}

// labelPicker è lo stato del pannello: le conversazioni bersaglio (catturate
// all'apertura), le righe, il cursore sulle righe visibili, il filtro digitato e
// loading finché ListMailboxes non risponde.
type labelPicker struct {
	convs   []mail.Conversation
	items   []pickLabel
	input   textinput.Model
	cursor  int
	loading bool
}

// newLabelPicker costruisce il pannello con le label date, ciascuna con la sua
// presenza sulle bersaglio.
func newLabelPicker(convs []mail.Conversation, names []string) labelPicker {
	items := make([]pickLabel, 0, len(names))
	for _, n := range names {
		pres := presenceOf(n, convs)
		items = append(items, pickLabel{name: n, start: pres, on: pres == presAll})
	}
	in := textinput.New()
	in.CharLimit = 64
	return labelPicker{convs: convs, items: items, input: in}
}

// labelNames sono le label utente dell'elenco dei mailbox (ListMailboxes): ruolo
// vuoto (le label non hanno ruolo SPECIAL-USE) e msg.IsUserLabel.
func labelNames(boxes []mail.Mailbox) []string {
	var out []string
	for _, b := range boxes {
		if b.Role == "" && msg.IsUserLabel(b.Name) {
			out = append(out, b.Name)
		}
	}
	return out
}

// filterText è il filtro digitato, senza spazi ai bordi.
func (p labelPicker) filterText() string { return strings.TrimSpace(p.input.Value()) }

// setFilter imposta il filtro e riporta il cursore dentro le righe visibili.
func (p *labelPicker) setFilter(s string) {
	p.input.SetValue(s)
	p.clamp()
}

// visible sono gli indici di items che contengono il filtro (senza distinguere
// maiuscole); con filtro vuoto sono tutti.
func (p labelPicker) visible() []int {
	q := strings.ToLower(p.filterText())
	var out []int
	for i, it := range p.items {
		if strings.Contains(strings.ToLower(it.name), q) {
			out = append(out, i)
		}
	}
	return out
}

// visibleNames sono i nomi delle righe visibili, nell'ordine dell'elenco.
func (p labelPicker) visibleNames() []string {
	var out []string
	for _, i := range p.visible() {
		out = append(out, p.items[i].name)
	}
	return out
}

// clamp riporta il cursore dentro le righe visibili.
func (p *labelPicker) clamp() {
	n := len(p.visible())
	p.cursor = min(max(p.cursor, 0), max(n-1, 0))
}

// move sposta il cursore di d righe, senza uscire dalle visibili.
func (p *labelPicker) move(d int) {
	p.cursor += d
	p.clamp()
}

// toggleCursor cambia la casella della riga sotto il cursore (spazio).
func (p *labelPicker) toggleCursor() {
	v := p.visible()
	if p.cursor < len(v) {
		p.items[v[p.cursor]].toggle()
	}
}

// commit calcola le aggiunte e le rimozioni da applicare. Se il filtro non
// corrisponde a nessuna label, crea la label col nome digitato e la aggiunge; un
// nome che non è una label utente è rifiutato senza toccare l'elenco.
func (p *labelPicker) commit() (add, remove []string, err error) {
	if name := p.filterText(); name != "" && len(p.visible()) == 0 {
		if !msg.IsUserLabel(name) {
			return nil, nil, fmt.Errorf("%q non è un nome di etichetta valido", name)
		}
		p.items = append(p.items, pickLabel{name: name, start: presNone, on: true, touched: true})
	}
	for _, it := range p.items {
		a, r := it.change()
		if a {
			add = append(add, it.name)
		}
		if r {
			remove = append(remove, it.name)
		}
	}
	return add, remove, nil
}

// labelsListed è il risultato di ListMailboxes per il pannello (Piano 10, Task 7).
type labelsListed struct {
	boxes []mail.Mailbox
	err   error
}

// listLabelsCmd carica le mailbox fuori dal loop UI: il pannello ne ricava le label.
func listLabelsCmd(st mail.MailStore) tea.Cmd {
	return func() tea.Msg {
		boxes, err := st.ListMailboxes(context.Background())
		return labelsListed{boxes: boxes, err: err}
	}
}

// gmailMode dice se lo store è in modalità Gmail (X-GM-EXT-1, capability GmailExt): il
// pannello label con aggiunte e rimozioni su tutte le cartelle richiede Gmail (Piano 10,
// Task 7). Senza Gmail resta il mini-prompt di sempre. Non si decide da ThreadID.
func gmailMode(st mail.MailStore) bool {
	g, ok := st.(gmailAware)
	return ok && g.GmailExt()
}

// labelTargets sono le conversazioni su cui agisce il pannello: la selezione se c'è,
// altrimenti la riga operata (come a/#/s/v singoli).
func (m *Model) labelTargets() []mail.Conversation {
	if m.view == viewInbox && len(m.sel) > 0 {
		return m.selectedConvs()
	}
	ref, ok := m.opRef()
	if !ok {
		return nil
	}
	return m.convsOf([]msg.MessageRef{ref})
}

// openLabelPick apre il pannello sulle conversazioni date: il filtro prende il focus e
// l'elenco arriva da ListMailboxes (loading finché non risponde).
func (m Model) openLabelPick(convs []mail.Conversation) (Model, tea.Cmd) {
	m.labelPickOpen = true
	m.labelPick = newLabelPicker(convs, nil)
	m.labelPick.loading = true
	m.labelPick.input = newPromptInput(labelFilterPrompt, "nome etichetta", m.styles)
	follow := m.labelPick.input.Focus()
	m.pending++
	return m, tea.Batch(listLabelsCmd(m.store), follow, m.spinner.Tick)
}

// labelPickKey gestisce i tasti col pannello aperto: esc chiude senza cambiare nulla,
// invio applica, spazio cambia la casella sotto il cursore, frecce scorrono, il resto
// digita il filtro. Con l'elenco in arrivo accetta solo esc.
func (m Model) labelPickKey(ev tea.KeyPressMsg) (Model, tea.Cmd) {
	switch ev.String() {
	case "esc":
		m.labelPickOpen = false
		m.labelPick.input.Blur()
		return m, nil
	case "enter":
		return m.confirmLabelPick()
	}
	if m.labelPick.loading {
		return m, nil
	}
	switch ev.String() {
	case "space":
		// Lo spazio cambia la casella solo con filtro vuoto; altrimenti è testo del filtro
		// (Piano 10, Task 7: label con spazi, "Da leggere").
		if m.labelPick.input.Value() == "" {
			m.labelPick.toggleCursor()
			return m, nil
		}
	case "tab":
		m.labelPick.toggleCursor()
		return m, nil
	case "up":
		m.labelPick.move(-1)
		return m, nil
	case "down":
		m.labelPick.move(1)
		return m, nil
	}
	var cmd tea.Cmd
	m.labelPick.input, cmd = m.labelPick.input.Update(ev)
	m.labelPick.clamp()
	return m, cmd
}

// confirmLabelPick chiude il pannello e applica aggiunte e rimozioni insieme (D10-L1).
// Un nome non valido è rifiutato con toast, senza op; senza modifiche non parte nulla.
// Dopo un'applicazione la selezione si azzera (D10-S3).
func (m Model) confirmLabelPick() (Model, tea.Cmd) {
	if m.labelPick.loading {
		return m, nil
	}
	m.labelPickOpen = false
	m.labelPick.input.Blur()
	add, remove, err := m.labelPick.commit()
	if err != nil {
		cmd := m.toast.Push(toast.Error, "etichetta non applicata: "+err.Error())
		return m, cmd
	}
	if len(add)+len(remove) == 0 {
		return m, nil
	}
	convs := m.labelPick.convs
	m.setSel(nil)
	m.pending++
	return m, tea.Batch(modifyLabelsCmd(m.store, convs, add, remove, m.listGen), m.spinner.Tick)
}

// modifyLabelsCmd applica le label su tutti i messaggi delle conversazioni (D10-L2):
// gli ID sono quelli del thread in Tutti i messaggi, così il cambio vale da qualunque
// cartella (STORE X-GM-LABELS, solo label, nessun messaggio spostato).
func modifyLabelsCmd(st mail.MailStore, convs []mail.Conversation, add, remove []string, gen int) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		ids, err := allMailIDs(ctx, st, convs)
		if err == nil {
			err = st.ModifyLabels(ctx, ids, add, remove)
		}
		return opDone{label: fmt.Sprintf("etichette aggiornate su %d", len(convs)), err: err, refresh: true, gen: gen, checkGen: true}
	}
}

// allMailIDs sono gli ID dei messaggi delle conversazioni (ThreadMessages: Tutti i
// messaggi, senza spam e cestino), senza duplicati. Una conversazione senza thread è il
// suo messaggio più recente.
func allMailIDs(ctx context.Context, st mail.MailStore, convs []mail.Conversation) ([]msg.MessageID, error) {
	var out []msg.MessageID
	seen := make(map[msg.MessageID]bool)
	for _, c := range convs {
		part := []msg.MessageID{c.Latest.ID}
		if c.ThreadID != 0 {
			thread, err := st.ThreadMessages(ctx, c.ThreadID)
			if err != nil {
				return nil, err
			}
			if len(thread) > 0 {
				part = refIDs(thread)
			}
		}
		for _, id := range part {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out, nil
}

// labelPickView è il corpo del pannello: titolo con il numero di conversazioni, filtro,
// righe. Con filtro senza corrispondenze invita a creare l'etichetta col nome digitato.
func (m *Model) labelPickView() string {
	if m.labelPick.loading {
		return m.windowFor(m.styles.ListDate.Render("caricamento etichette…"), "etichette", labelPickMinW)
	}
	var rows []labelpick.Row
	for _, i := range m.labelPick.visible() {
		it := m.labelPick.items[i]
		rows = append(rows, labelpick.Row{Mark: it.mark(), Name: it.name})
	}
	empty := "nessuna etichetta: digita un nome e invio per crearla"
	if f := m.labelPick.filterText(); f != "" {
		empty = fmt.Sprintf("invio crea l'etichetta %q", f)
	}
	title := fmt.Sprintf("etichette su %d conversazioni", len(m.labelPick.convs))
	// Piano 15, Task 16 (D15-11): finestra con ombra; il titolo e quello della finestra.
	body := labelpick.Render(m.styles, "", m.labelPick.input.View(), rows, m.labelPick.cursor, max(m.height-6, 1), empty)
	return m.windowFor(body, title, labelPickMinW)
}

// labelPickMinW e la larghezza interna minima della finestra del pannello label: non
// cambia mentre si filtra le righe (Piano 15, Task 16).
const labelPickMinW = 44
