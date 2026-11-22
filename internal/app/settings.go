// Menu impostazioni (Piano 7.2, Task 3): modale `,` da inbox e thread. Voci
// (decisione U3): Tema, Layout lettura, Segna come letto in anteprima, Nome,
// Firma. ↑↓/j/k scelgono la voce, ←→/h/l cambiano il valore delle voci a
// scelta; tema e layout hanno anteprima immediata (stesso re-styling di
// `:theme`). Invio su una voce a scelta salva tutto in config e chiude; esc
// annulla e ripristina tema e layout. Nome e firma sono campi di testo: invio
// sulla voce ne apre la modifica, esc la chiude tenendo il testo. Mouse e
// tasti operativi restano sospesi finché il menu è aperto (gate in model.go).
package app

import (
	"errors"
	"fmt"
	"net/mail"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/config"
	"github.com/andsigno82/gmail-tui/internal/theme"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// settingsRow è una voce del menu, nell'ordine di decisione U3.
type settingsRow int

const (
	rowTheme settingsRow = iota
	rowLayout
	rowMarkRead
	rowName
	rowSignature
	rowCount
)

// settingsLabels sono le etichette delle voci, nell'ordine di settingsRow.
var settingsLabels = [rowCount]string{"Tema", "Layout lettura", "Segna come letto in anteprima", "Nome", "Firma"}

// settingsLabelWidth è la colonna del valore nel menu (le etichette sono ASCII).
const settingsLabelWidth = 30

// settingsMenu è lo stato del menu `,`. Tema e layout sono già applicati al
// Model mentre il menu è aperto (anteprima); markRead, nome e firma sono bozze
// che il Model riceve solo all'invio. origTheme e origLayout sono i valori da
// ripristinare con esc.
type settingsMenu struct {
	open       bool
	cur        settingsRow
	editing    bool // Nome o Firma in modifica: i tasti vanno al campo
	origTheme  theme.Theme
	origLayout string
	markRead   string
	name       textinput.Model
	sig        textarea.Model
}

// settingsValues sono i valori scritti in config all'invio del menu.
type settingsValues struct {
	Theme, Layout, PreviewMarkRead, Name, Signature string
}

// SetPrefs imposta le preferenze di lettura dalla config (Piano 7.2, Task 2-3):
// layout e segna-come-letto in anteprima. Valori vuoti valgono come default.
func (m *Model) SetPrefs(layout, markRead string) {
	m.layout = layout
	if m.layout == "" {
		m.layout = config.LayoutSingle
	}
	m.markRead = markRead
	if m.markRead == "" {
		m.markRead = config.PreviewMarkReadDelay
	}
}

// SetIdentity imposta l'identità del composer (Piano 4, Task 3; nome e firma
// dal menu impostazioni, Piano 7.2, Task 3): il mittente è "Nome <indirizzo>"
// quando il nome è impostato. Da chiamare PRIMA del primo r/f/c.
func (m *Model) SetIdentity(address, name, signature string) {
	m.address, m.name, m.signature = address, name, signature
	m.from = identityFrom(address, name)
}

// identityFrom compone l'header From: indirizzo nudo senza nome, altrimenti
// "Nome <indirizzo>" con il nome in RFC 2047 se non ASCII (net/mail).
func identityFrom(address, name string) string {
	// Piano 7.2, fix review: senza indirizzo non c'è identità da comporre.
	if address == "" {
		return ""
	}
	if name == "" {
		return address
	}
	return (&mail.Address{Name: name, Address: address}).String()
}

// openSettings apre il menu `,` (Piano 7.2, Task 3): le bozze partono dai
// valori correnti e la voce selezionata è la prima.
func (m Model) openSettings() (Model, tea.Cmd) {
	m.menu = settingsMenu{
		open:       true,
		origTheme:  m.th,
		origLayout: m.layout,
		markRead:   m.markRead,
		name:       newSettingsName(m.styles, m.name),
		sig:        newSettingsSig(m.styles, m.signature, m.width),
	}
	return m, nil
}

// settingsKey gestisce un tasto con il menu aperto: in modifica va al campo
// (esc o invio su Nome tornano alla lista), altrimenti naviga, cambia i valori,
// apre la modifica (invio su Nome/Firma) o salva (invio su una voce a scelta).
// Tutti gli altri tasti sono ignorati: nessuna azione operativa sotto il menu.
func (m Model) settingsKey(ev tea.KeyPressMsg) (Model, tea.Cmd) {
	// Piano 7.2, fix review: ctrl+s salva da qualunque voce, anche durante la
	// modifica di Nome/Firma (invio sul campo apre/chiude soltanto).
	if ev.String() == "ctrl+s" {
		return m.settingsCommit()
	}
	if m.menu.editing {
		if ev.String() == "esc" || (ev.String() == "enter" && m.menu.cur == rowName) {
			return m.stopEditing(), nil
		}
		var cmd tea.Cmd
		if m.menu.cur == rowName {
			m.menu.name, cmd = m.menu.name.Update(ev)
		} else {
			m.menu.sig, cmd = m.menu.sig.Update(ev)
		}
		return m, cmd
	}
	switch ev.String() {
	case "esc":
		return m.settingsCancel(), nil
	case "up", "k":
		m.menu.cur = max(0, m.menu.cur-1)
	case "down", "j":
		m.menu.cur = min(rowCount-1, m.menu.cur+1)
	case "left", "h":
		return m.settingsStep(-1), nil
	case "right", "l":
		return m.settingsStep(+1), nil
	case "enter":
		if m.menu.cur == rowName || m.menu.cur == rowSignature {
			return m.startEditing()
		}
		return m.settingsCommit()
	}
	return m, nil
}

// settingsStep cambia il valore della voce selezionata di un passo (dir ±1,
// con giro). Tema: re-styling immediato; layout: applicato subito; segna come
// letto: bozza. Nome e firma non hanno valori a scelta.
func (m Model) settingsStep(dir int) Model {
	switch m.menu.cur {
	case rowTheme:
		th, err := theme.Load(nextValue(theme.Names(), m.th.Name, dir))
		if err != nil {
			m.status = statusMsg{text: fmt.Sprintf("tema: %v", err), isErr: true}
			return m
		}
		m.th = th
		m.applyStyles(styles.New(th))
	case rowLayout:
		m.layout = nextValue(config.LayoutValues(), m.layout, dir)
		m.relayout() // applicato subito: lista e anteprima si ridispongono (Piano 7.2, Task 4)
	case rowMarkRead:
		m.menu.markRead = nextValue(config.PreviewMarkReadValues(), m.menu.markRead, dir)
	}
	return m
}

// settingsCancel chiude il menu annullando: tema e layout tornano ai valori
// di apertura, le bozze si scartano.
func (m Model) settingsCancel() Model {
	if m.th.Name != m.menu.origTheme.Name {
		m.th = m.menu.origTheme
		m.applyStyles(styles.New(m.th))
	}
	if m.layout != m.menu.origLayout {
		m.layout = m.menu.origLayout
		m.relayout()
	}
	m.menu = settingsMenu{}
	return m
}

// settingsCommit applica le bozze (segna come letto, nome, firma), chiude il
// menu e salva tutto in config (cfgPath, come :theme). Nome e firma valgono dal
// prossimo messaggio composto (from e signature). Errore di salvataggio:
// esito in settingsSaved, i valori restano applicati a runtime.
func (m Model) settingsCommit() (Model, tea.Cmd) {
	name := strings.TrimSpace(m.menu.name.Value())
	sig := strings.TrimSpace(m.menu.sig.Value())
	m.markRead = m.menu.markRead
	m.name, m.signature = name, sig
	m.from = identityFrom(m.address, name)
	m.menu = settingsMenu{}
	cmd := saveSettingsCmd(m.cfgPath, settingsValues{
		Theme:           m.th.Name,
		Layout:          m.layout,
		PreviewMarkRead: m.markRead,
		Name:            name,
		Signature:       sig,
	})
	if cmd == nil {
		m.status = statusMsg{text: "impostazioni applicate (config non disponibile: non salvate)"}
	}
	return m, cmd
}

// saveSettingsCmd scrive i valori del menu nella config (load → set → Save, come
// saveThemeCmd: l'indirizzo e gli altri campi su disco restano). Percorso vuoto
// → nessuna persistenza.
func saveSettingsCmd(path string, v settingsValues) tea.Cmd {
	if path == "" {
		return nil
	}
	return func() tea.Msg {
		cfg, err := config.Load(path)
		// Piano 7.2, fix review: valore non valido → default solo per quel campo.
		if errors.Is(err, config.ErrInvalid) {
			cfg, err = cfg.Sanitized(), nil
		}
		if err == nil {
			cfg.Theme, cfg.Layout, cfg.PreviewMarkRead = v.Theme, v.Layout, v.PreviewMarkRead
			cfg.Name, cfg.Signature = v.Name, v.Signature
			err = cfg.Save(path)
		}
		return settingsSaved{err: err}
	}
}

// startEditing apre la modifica del campo Nome o Firma e ne dà il focus.
func (m Model) startEditing() (Model, tea.Cmd) {
	m.menu.editing = true
	if m.menu.cur == rowName {
		return m, m.menu.name.Focus()
	}
	return m, m.menu.sig.Focus()
}

// stopEditing chiude la modifica tenendo il testo: torna alla lista delle voci.
func (m Model) stopEditing() Model {
	m.menu.editing = false
	m.menu.name.Blur()
	m.menu.sig.Blur()
	return m
}

// settingsView rende il menu impostazioni: titolo e una riga per voce, con la
// voce selezionata evidenziata (ListSelected, colore calcolato di Piano 7.2,
// Task 1). Il campo in modifica rende il proprio widget.
func (m Model) settingsView() string {
	st := m.styles
	var b strings.Builder
	b.WriteString(st.ThreadHeader.Render("impostazioni"))
	b.WriteString("\n\n")
	for r := settingsRow(0); r < rowCount; r++ {
		label := fmt.Sprintf("%-*s", settingsLabelWidth, settingsLabels[r])
		if r == m.menu.cur {
			label = st.ListSelected.Render(label)
		} else {
			label = st.ListNormal.Render(label)
		}
		b.WriteString(label + m.settingsValueText(r) + "\n")
	}
	// Piano 7.2, fix review: riga di aiuto in fondo, modale centrata nel corpo
	// con un pad di 1 colonna.
	b.WriteString("\n" + st.ListDate.Render(settingsHelp))
	box := st.Border.Padding(0, 1).Render(b.String())
	return lipgloss.Place(m.width, max(0, m.height-1), lipgloss.Center, lipgloss.Center, box)
}

// settingsHelp è la riga di aiuto del menu `,` (Piano 7.2, fix review).
const settingsHelp = "↑↓ voce · ←→ valore · invio modifica/salva · ctrl+s salva · esc annulla"

// settingsValueText rende il valore di una voce: ‹ valore › per le voci a
// scelta, il testo (o il widget in modifica) per Nome e Firma.
func (m Model) settingsValueText(r settingsRow) string {
	st := m.styles
	switch r {
	case rowTheme:
		return "‹ " + m.th.Name + " ›"
	case rowLayout:
		return "‹ " + layoutLabel(m.layout) + " ›"
	case rowMarkRead:
		return "‹ " + markReadLabel(m.menu.markRead) + " ›"
	case rowName:
		if m.menu.editing && m.menu.cur == rowName {
			return m.menu.name.View()
		}
		if v := m.menu.name.Value(); v != "" {
			return v
		}
		return st.ListDate.Render("(vuoto)")
	default: // rowSignature
		if m.menu.editing && m.menu.cur == rowSignature {
			return m.menu.sig.View()
		}
		if v := m.menu.sig.Value(); v != "" {
			return strings.ReplaceAll(v, "\n", " ⏎ ")
		}
		return st.ListDate.Render("(vuota)")
	}
}

// layoutLabel e markReadLabel sono i testi del menu per i valori di config
// (Piano 7.2, U4 e U5).
func layoutLabel(v string) string {
	if v == config.LayoutSplit {
		return "due colonne"
	}
	return "una colonna"
}

func markReadLabel(v string) string {
	switch v {
	case config.PreviewMarkReadImmediate:
		return "subito"
	case config.PreviewMarkReadNever:
		return "mai"
	default:
		return "dopo 2 secondi"
	}
}

// nextValue restituisce il valore successivo (dir=+1) o precedente (dir=-1) di
// cur in vals, con giro. Un valore sconosciuto parte dal primo.
func nextValue(vals []string, cur string, dir int) string {
	i := max(0, slices.Index(vals, cur))
	n := len(vals)
	return vals[(i+dir+n)%n]
}

// settingsInputStyles mappa gli slot del tema sul textinput del menu (i default
// di bubbles hardcodano hex: vale il vincolo "solo slot del tema").
func settingsInputStyles(st styles.Styles) textinput.Styles {
	var s textinput.Styles
	s.Focused.Text = st.ListNormal
	s.Focused.Placeholder = st.ListDate
	s.Blurred.Text = st.ListNormal
	s.Blurred.Placeholder = st.ListDate
	s.Cursor = textinput.CursorStyle{Shape: tea.CursorBlock, Blink: true}
	return s
}

// settingsTextareaStyles mappa gli slot del tema sul textarea della firma.
func settingsTextareaStyles(st styles.Styles) textarea.Styles {
	var s textarea.Styles
	s.Focused.Text = st.ListNormal
	s.Focused.Placeholder = st.ListDate
	s.Focused.EndOfBuffer = st.ListDate
	s.Blurred.Text = st.ListNormal
	s.Blurred.Placeholder = st.ListDate
	s.Blurred.EndOfBuffer = st.ListDate
	s.Cursor.Shape = tea.CursorBlock
	s.Cursor.Blink = true
	return s
}

// newSettingsName costruisce il campo Nome (una riga) con il valore dato.
func newSettingsName(st styles.Styles, name string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "nome"
	ti.CharLimit = 128
	ti.SetStyles(settingsInputStyles(st))
	ti.SetValue(name)
	return ti
}

// newSettingsSig costruisce il campo Firma (più righe, senza numeri di riga).
func newSettingsSig(st styles.Styles, sig string, width int) textarea.Model {
	ta := textarea.New()
	ta.Placeholder = "firma (più righe)"
	ta.ShowLineNumbers = false
	ta.SetStyles(settingsTextareaStyles(st))
	ta.SetValue(sig)
	ta.SetHeight(4)
	ta.SetWidth(max(20, width-settingsLabelWidth-4))
	return ta
}
