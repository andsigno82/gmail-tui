package composer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/theme"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// --- helper -------------------------------------------------------------------

// newModel costruisce il composer di test col tema default.
func newModel(t *testing.T, cfg ComposerConfig) Model {
	t.Helper()
	th, err := theme.Load("default")
	if err != nil {
		t.Fatalf("tema default: %v", err)
	}
	return New(cfg, styles.New(th))
}

// pressKey costruisce una pressione di tasto (v2 KeyPressMsg).
func pressKey(k string) tea.KeyPressMsg {
	switch k {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "ctrl+s":
		return tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	case "ctrl+e":
		return tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl}
	default:
		return tea.KeyPressMsg{Code: rune(k[0]), Text: k}
	}
}

// step applica un msg al modello (come fa il runtime).
func step(m Model, msg tea.Msg) (Model, tea.Cmd) {
	return m.Update(msg)
}

// runCmds esegue una cmd srotolando i tea.BatchMsg (come fa il runtime).
func runCmds(c tea.Cmd) []tea.Msg {
	if c == nil {
		return nil
	}
	res := c()
	batch, ok := res.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{res}
	}
	var out []tea.Msg
	for _, c := range batch {
		out = append(out, runCmds(c)...)
	}
	return out
}

// cycleTabs preme n volte tab.
func cycleTabs(m Model, n int) Model {
	for i := 0; i < n; i++ {
		m, _ = step(m, pressKey("tab"))
	}
	return m
}

// seedDir prepara una directory di test col filepicker (t.Chdir rende
// deterministica la directory corrente).
func seedDir(t *testing.T) (dir string, a, b string) {
	t.Helper()
	dir = t.TempDir()
	t.Chdir(dir)
	a = filepath.Join(dir, "a.txt")
	b = filepath.Join(dir, "b.txt")
	if err := os.WriteFile(a, []byte("contenuto a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("contenuto b"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, a, b
}

// --- New / focus iniziale ------------------------------------------------------

// TestNewFocusFirstEmpty: il focus iniziale va al primo campo vuoto (stile
// pop): nuovo messaggio → A (To); reply/forward precompilati → Corpo.
func TestNewFocusFirstEmpty(t *testing.T) {
	if m := newModel(t, ComposerConfig{}); m.focus != FieldTo {
		t.Errorf("focus nuovo messaggio = %v, voglio FieldTo", m.focus)
	}
	full := ComposerConfig{
		From: "me@x.dev", To: "anna@x.dev", Subject: "Re: prima",
		QuotedBody: "Il 20 nov 2026, anna@x.dev ha scritto:\n> ciao",
	}
	if m := newModel(t, full); m.focus != FieldBody {
		t.Errorf("focus reply precompilato = %v, voglio FieldBody", m.focus)
	}
}

// TestNewPrefillsFields: i campi arrivano dalla config (To/Cc/Subject/Body/
// QuotedBody nel textarea), gli allegati sono copiati.
func TestNewPrefillsFields(t *testing.T) {
	cfg := ComposerConfig{
		From: "me@x.dev", Signature: "firma",
		To: "anna@x.dev", Cc: "bob@x.dev", Subject: "Oggetto",
		Body:        "riga mia",
		QuotedBody:  "> citazione",
		Attachments: []string{"/tmp/f.pdf"},
	}
	m := newModel(t, cfg)
	if got := m.to.Value(); got != "anna@x.dev" {
		t.Errorf("To = %q, voglio %q", got, "anna@x.dev")
	}
	if got := m.subject.Value(); got != "Oggetto" {
		t.Errorf("Subject = %q, voglio %q", got, "Oggetto")
	}
	if got := m.body.Value(); got != "riga mia\n> citazione" {
		t.Errorf("body = %q, voglio Body+QuotedBody uniti", got)
	}
	if !m.ccVisible {
		t.Error("Cc con valore in config deve nascere visibile")
	}
	if len(m.attachments) != 1 || m.attachments[0] != "/tmp/f.pdf" {
		t.Errorf("attachments = %v, voglio [/tmp/f.pdf]", m.attachments)
	}
}

// --- navigazione campi ---------------------------------------------------------

// TestTabNavigatesSkippingFrom: tab cicla i campi editabili nell'ordine
// To→Subject→Corpo→Allegati (Cc nascosto) e torna al capo; From (readonly
// display) non è mai focalizzato. shift+tab torna indietro.
func TestTabNavigatesSkippingFrom(t *testing.T) {
	m := newModel(t, ComposerConfig{})
	want := []Field{FieldTo, FieldSubject, FieldBody, FieldAttachments, FieldTo}
	// Ciclo completo: To → Subject → Corpo → Allegati → A (wrap ai capi).
	for _, w := range want {
		if m.focus != w {
			t.Fatalf("focus = %v, voglio %v", m.focus, w)
		}
		m, _ = step(m, pressKey("tab"))
	}
	// Dopo il ciclo il focus è su Subject (5 tab): shift+tab torna a To e un
	// altro shift+tab da To arriva ad Allegati (capo all'indietro, senza From).
	m, _ = step(m, pressKey("shift+tab"))
	if m.focus != FieldTo {
		t.Errorf("shift+tab da Subject: focus = %v, voglio FieldTo", m.focus)
	}
	m, _ = step(m, pressKey("shift+tab"))
	if m.focus != FieldAttachments {
		t.Errorf("shift+tab da To: focus = %v, voglio FieldAttachments", m.focus)
	}
}

// TestCcInNavigationWhenVisible: col toggle attivo Cc entra nel ciclo tab
// (To→Cc→Subject) e shift+tab da Subject torna a Cc.
func TestCcInNavigationWhenVisible(t *testing.T) {
	m := newModel(t, ComposerConfig{})
	mm, _ := step(m, pressKey("ctrl+e"))
	if !mm.ccVisible {
		t.Fatal("ctrl+e non mostra Cc")
	}
	if mm.focus != FieldCc {
		t.Errorf("focus dopo il toggle = %v, voglio FieldCc", mm.focus)
	}
	mm, _ = step(mm, pressKey("tab"))
	if mm.focus != FieldSubject {
		t.Errorf("tab da Cc: focus = %v, voglio FieldSubject", mm.focus)
	}
	mm, _ = step(mm, pressKey("shift+tab"))
	if mm.focus != FieldCc {
		t.Errorf("shift+tab da Subject: focus = %v, voglio FieldCc", mm.focus)
	}
	// Nuovo toggle nasconde Cc e il focus torna su A.
	mm, _ = step(mm, pressKey("ctrl+e"))
	if mm.ccVisible {
		t.Error("il secondo ctrl+e deve nascondere Cc")
	}
	if mm.focus != FieldTo {
		t.Errorf("focus dopo hide = %v, voglio FieldTo", mm.focus)
	}
}

// TestArrowsNavigateSingleLine: in To (input mono-riga) le frecce navigano i
// campi (giù → Subject, su → To di nuovo); nel Corpo (textarea) giù resta nel
// testo (spetta al textarea).
func TestArrowsNavigateSingleLine(t *testing.T) {
	m := newModel(t, ComposerConfig{})
	mm, _ := step(m, pressKey("down"))
	if mm.focus != FieldSubject {
		t.Errorf("giù da A: focus = %v, voglio FieldSubject", mm.focus)
	}
	mm, _ = step(mm, pressKey("up"))
	if mm.focus != FieldTo {
		t.Errorf("su da Oggetto: focus = %v, voglio FieldTo", mm.focus)
	}
	// Nel corpo le frecce muovono il cursore: il campo non cambia.
	mm = cycleTabs(mm, 2) // To → Subject → Corpo
	mm, _ = step(mm, pressKey("down"))
	if mm.focus != FieldBody {
		t.Errorf("giù nel Corpo: focus = %v, voglio FieldBody (cursor movement)", mm.focus)
	}
}

// --- Draft ----------------------------------------------------------------------

// TestDraftMapsFields: Draft raccoglie i campi in msg.Outgoing — To/Cc splittati
// sulle virgole (spazi trimmati, vuoti scartati), Subject trimmato, corpo e
// allegati copiati.
func TestDraftMapsFields(t *testing.T) {
	m := newModel(t, ComposerConfig{From: "me@x.dev"})
	m.to.SetValue(" a@x.dev , b@y.dev , ")
	m.subject.SetValue("  oggetto  ")
	m.body.SetValue("prima riga\nseconda")
	m.attachments = []string{"/tmp/a.pdf", "/tmp/b.pdf"}
	d := m.Draft()
	if d.From != "me@x.dev" {
		t.Errorf("From = %q, voglio me@x.dev", d.From)
	}
	if want := []string{"a@x.dev", "b@y.dev"}; strings.Join(d.To, ",") != strings.Join(want, ",") {
		t.Errorf("To = %v, voglio %v", d.To, want)
	}
	if d.Subject != "oggetto" {
		t.Errorf("Subject = %q, voglio %q", d.Subject, "oggetto")
	}
	if d.BodyText != "prima riga\nseconda" {
		t.Errorf("BodyText = %q, voglio il corpo tal quale", d.BodyText)
	}
	if len(d.Attachments) != 2 || d.Attachments[0] != "/tmp/a.pdf" {
		t.Errorf("Attachments = %v, voglio [/tmp/a.pdf /tmp/b.pdf]", d.Attachments)
	}
}

// TestDraftSignatureRule: la firma da config è appended stile pop
// corpo + "\n\n--\n" + firma, SOLO se non vuota; senza firma il corpo resta
// tal quale.
func TestDraftSignatureRule(t *testing.T) {
	m := newModel(t, ComposerConfig{Signature: "la firma"})
	m.body.SetValue("ciao")
	if got := m.Draft().BodyText; got != "ciao\n\n--\nla firma" {
		t.Errorf("BodyText = %q, voglio %q", got, "ciao\n\n--\nla firma")
	}
	// Corpo vuoto + firma: la firma da sola (comportamento pop, documentato).
	if got := newModel(t, ComposerConfig{Signature: "f"}).Draft().BodyText; got != "\n\n--\nf" {
		t.Errorf("BodyText vuoto+firma = %q, voglio %q", got, "\n\n--\nf")
	}
	if got := newModel(t, ComposerConfig{}).Draft().BodyText; got != "" {
		t.Errorf("BodyText senza firma = %q, voglio vuoto", got)
	}
}

// TestDraftCcOnlyWhenVisible: Cc nascosto NON finisce nella bozza (l'utente
// non lo vede → non va in invio silenziosamente); mostrato, sì.
func TestDraftCcOnlyWhenVisible(t *testing.T) {
	m := newModel(t, ComposerConfig{})
	m.cc.SetValue("cc@x.dev")
	if d := m.Draft(); len(d.Cc) != 0 {
		t.Errorf("Cc nascosto in Draft = %v, voglio vuoto", d.Cc)
	}
	mm, _ := step(m, pressKey("ctrl+e"))
	if d := mm.Draft(); len(d.Cc) != 1 || d.Cc[0] != "cc@x.dev" {
		t.Errorf("Cc visibile in Draft = %v, voglio [cc@x.dev]", d.Cc)
	}
}

// --- cancel / send ---------------------------------------------------------------

// TestEscRequestsCancelAndOtherKeysReset: esc chiede l'annullamento
// (CancelRequested) — la conferma scarta/riprende è del root; QUALSIASI altro
// tasto consuma la richiesta (esc → ripresa → un tasto di testo non ri-apre
// il modale).
func TestEscRequestsCancelAndOtherKeysReset(t *testing.T) {
	m := newModel(t, ComposerConfig{})
	mm, cmd := step(m, pressKey("esc"))
	if cmd != nil {
		t.Errorf("esc: cmd = %v, voglio nil", cmd)
	}
	if !mm.CancelRequested() {
		t.Fatal("esc non ha richiesto il cancel")
	}
	mm, _ = step(mm, pressKey("x"))
	if mm.CancelRequested() {
		t.Error("un tasto normale deve consumare la richiesta di cancel")
	}
	// I tasti che escono in anticipo dal routing (tab/frecce/ctrl+*) devono
	// consumare la richiesta allo stesso modo (regressione: dopo la ripresa
	// dal modale discard, tab ri-apriva la conferma).
	for _, k := range []string{"tab", "shift+tab", "ctrl+e", "ctrl+s", "up", "down"} {
		mm, _ = step(m, pressKey("esc"))
		mm, _ = step(mm, pressKey(k))
		if mm.CancelRequested() {
			t.Errorf("%s non consuma la richiesta di cancel", k)
		}
	}
}

// TestCtrlSProducesSendRequested: ctrl+s produce SendRequestedMsg con la
// bozza corrente (la conferma/invio è del root, Task 4).
func TestCtrlSProducesSendRequested(t *testing.T) {
	m := newModel(t, ComposerConfig{From: "me@x.dev", Signature: "firma"})
	m.to.SetValue("anna@x.dev")
	m.subject.SetValue("ciao")
	m.body.SetValue("corpo")
	mm, cmd := step(m, pressKey("ctrl+s"))
	if cmd == nil {
		t.Fatal("ctrl+s: cmd nil, voglio SendRequestedMsg")
	}
	res, ok := cmd().(SendRequestedMsg)
	if !ok {
		t.Fatalf("ctrl+s: msg = %T, voglio SendRequestedMsg", cmd())
	}
	if res.Out.To[0] != "anna@x.dev" || res.Out.Subject != "ciao" ||
		res.Out.BodyText != "corpo\n\n--\nfirma" {
		t.Errorf("SendRequestedMsg.Out = %+v, voglio la bozza col mittente", res.Out)
	}
	if mm.focus != FieldTo {
		t.Errorf("ctrl+s non deve cambiare il focus (=%v)", mm.focus)
	}
}

// --- allegati (filepicker multi-add) ---------------------------------------------

// TestEnterOpensPickerAndAddsFiles: invio sul campo Allegati apre il
// filepicker sulla directory corrente; invio su un file lo aggiunge alla
// lista (assoluto) e il picker RESTA aperto per il multi-add; esc chiude e
// torna sul campo.
func TestEnterOpensPickerAndAddsFiles(t *testing.T) {
	_, a, b := seedDir(t)
	m := newModel(t, ComposerConfig{})
	m = cycleTabs(m, 3) // → Allegati
	if m.focus != FieldAttachments {
		t.Fatalf("focus = %v, voglio FieldAttachments", m.focus)
	}
	mm, cmd := step(m, pressKey("enter"))
	if !mm.pickerOpen {
		t.Fatal("invio su Allegati non apre il filepicker")
	}
	if cmd == nil {
		t.Fatal("l'apertura del picker deve restituire la Init (lettura dir)")
	}
	// La Init produce la lettura directory: reimmessa come farebbe il runtime.
	for _, ms := range runCmds(cmd) {
		mm, _ = step(mm, ms)
	}
	// Il cursore parte dalla prima voce (a.txt): invio aggiunge.
	mm, _ = step(mm, pressKey("enter"))
	if len(mm.attachments) != 1 || mm.attachments[0] != a {
		t.Fatalf("attachments = %v, voglio [%s]", mm.attachments, a)
	}
	if !mm.pickerOpen {
		t.Error("il picker deve restare aperto per il multi-add")
	}
	// Giù → b.txt, invio aggiunge anche quello.
	mm, _ = step(mm, pressKey("down"))
	mm, _ = step(mm, pressKey("enter"))
	if len(mm.attachments) != 2 || mm.attachments[1] != b {
		t.Fatalf("attachments = %v, voglio [%s %s]", mm.attachments, a, b)
	}
	// esc chiude senza aggiungere, il focus torna sul campo.
	mm, _ = step(mm, pressKey("esc"))
	if mm.pickerOpen {
		t.Error("esc non chiude il picker")
	}
	if mm.focus != FieldAttachments {
		t.Errorf("focus dopo esc = %v, voglio FieldAttachments", mm.focus)
	}
	if len(mm.attachments) != 2 {
		t.Errorf("attachments dopo esc = %v, voglio intatti", mm.attachments)
	}
}

// TestPickerEscWithoutSelectDoesNotAdd: esc nel picker (prima di selezioni)
// chiude senza toccare la lista.
func TestPickerEscWithoutSelectDoesNotAdd(t *testing.T) {
	seedDir(t)
	m := newModel(t, ComposerConfig{})
	m = cycleTabs(m, 3)
	mm, _ := step(m, pressKey("enter"))
	mm, _ = step(mm, pressKey("esc"))
	if mm.pickerOpen || len(mm.attachments) != 0 {
		t.Errorf("esc: pickerOpen=%v attachments=%v, voglio false/[]", mm.pickerOpen, mm.attachments)
	}
}

// TestDRemovesSelected: `d` sul campo Allegati rimuove l'allegato SELEZIONATO
// (j/k e frecce muovono la selezione); lista svuotata → `d` è no-op.
func TestDRemovesSelected(t *testing.T) {
	m := newModel(t, ComposerConfig{Attachments: []string{"/tmp/a.pdf", "/tmp/b.pdf", "/tmp/c.pdf"}})
	m = cycleTabs(m, 3) // → Allegati
	mm, _ := step(m, pressKey("d"))
	if got := strings.Join(mm.attachments, ","); got != "/tmp/b.pdf,/tmp/c.pdf" {
		t.Fatalf("dopo d: %q, voglio /tmp/b.pdf,/tmp/c.pdf", got)
	}
	// La selezione resta sull'indice (0 → b.pdf): giù → c.pdf, d lo toglie.
	mm, _ = step(mm, pressKey("j"))
	mm, _ = step(mm, pressKey("d"))
	if got := strings.Join(mm.attachments, ","); got != "/tmp/b.pdf" {
		t.Fatalf("dopo giù+d: %q, voglio /tmp/b.pdf", got)
	}
	// Frecce: su torna su b.pdf (ultimo dopo le rimozioni), k idem, d svuota.
	mm, _ = step(mm, pressKey("up"))
	mm, _ = step(mm, pressKey("k"))
	mm, _ = step(mm, pressKey("d"))
	if len(mm.attachments) != 0 {
		t.Fatalf("attachments = %v, voglio vuota", mm.attachments)
	}
	// d su lista vuota: no-op senza panic.
	mm, _ = step(mm, pressKey("d"))
	if len(mm.attachments) != 0 {
		t.Errorf("d su lista vuota: attachments = %v", mm.attachments)
	}
}

// --- View -------------------------------------------------------------------------

// TestViewShowsFieldsAndHint: la view rende i campi con etichette italiane,
// il mittente readonly, il corpo, la lista allegati con la selezione e la
// riga d'uso.
func TestViewShowsFieldsAndHint(t *testing.T) {
	m := newModel(t, ComposerConfig{
		From:        "me@x.dev",
		Attachments: []string{"/tmp/a.pdf", "/tmp/b.pdf"},
	})
	v := m.View()
	for _, want := range []string{"Da:", "me@x.dev", "A:", "Oggetto:", "📎 /tmp/a.pdf", "📎 /tmp/b.pdf", "tab campi"} {
		if !strings.Contains(v, want) {
			t.Errorf("view senza %q", want)
		}
	}
	// Nessun allegato: il campo resta visibile con l'hint di aggiunta.
	m0 := newModel(t, ComposerConfig{From: "me@x.dev"})
	if v := m0.View(); !strings.Contains(v, "Allegati:") {
		t.Error("view senza il campo Allegati")
	}
	// Cc compare solo dopo il toggle.
	if strings.Contains(v, "Cc:") {
		t.Error("Cc visibile prima del toggle")
	}
	mm, _ := step(m, pressKey("ctrl+e"))
	if vv := mm.View(); !strings.Contains(vv, "Cc:") {
		t.Error("Cc assente dalla view dopo il toggle")
	}
}

// TestPickerViewReplacesFields: col picker aperto la view mostra il picker e
// l'hint di conferma al posto dei campi.
func TestPickerViewReplacesFields(t *testing.T) {
	seedDir(t)
	m := newModel(t, ComposerConfig{From: "me@x.dev"})
	m = cycleTabs(m, 3)
	m.SetSize(80, 24) // il picker ha bisogno dell'altezza (come dal runtime)
	mm, cmd := step(m, pressKey("enter"))
	for _, ms := range runCmds(cmd) {
		mm, _ = step(mm, ms)
	}
	v := mm.View()
	if strings.Contains(v, "Oggetto:") {
		t.Error("col picker aperto i campi non devono comparire")
	}
	if !strings.Contains(v, "a.txt") || !strings.Contains(v, "b.txt") {
		t.Errorf("view picker = %q, voglio le voci della directory", v)
	}
	if !strings.Contains(v, "esc annulla") {
		t.Error("view picker senza l'hint di conferma")
	}
}

// TestSetSizeResizesInputs: SetSize propaga le dimensioni (larghezza agli
// input mono-riga e al corpo, altezza al corpo) senza panic e senza drop.
func TestSetSizeResizesInputs(t *testing.T) {
	m := newModel(t, ComposerConfig{
		From:        "me@x.dev",
		Attachments: []string{"/tmp/a.pdf"},
	})
	m.SetSize(100, 40)
	if m.width != 100 || m.height != 40 {
		t.Fatalf("size = %dx%d, voglio 100x40", m.width, m.height)
	}
	mm, _ := step(m, pressKey("ctrl+e")) // Cc visibile: usedLines cambia
	mm.SetSize(60, 20)
	v := mm.View()
	if strings.Count(v, "\n") == 0 {
		t.Error("view vuota dopo SetSize")
	}
	// Il body deve avere spazio positivo: il testo rimane nella view.
	if !strings.Contains(mm.View(), "Da:") {
		t.Error("view senza i campi dopo SetSize con Cc")
	}
}
