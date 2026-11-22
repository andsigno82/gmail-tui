// Wizard nel radice (Piano 4, Task 5): attivazione (SetWizard), routing dei
// tasti, completamento (skip → demo con seed; successo → factory dei backend
// reali) e i feedback positivi in stile StatusOK (polish ereditato dal
// Piano 3: use-or-drop).
package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"gmail-tui/internal/mail"
	"gmail-tui/internal/theme"
	"gmail-tui/internal/ui/thread"
	"gmail-tui/internal/ui/wizard"
)

// --- doppio per le dipendenze del wizard ----------------------------------------

// factoryRecorder è la factory dei backend reali dei test: registra le
// chiamate e restituisce un Mock fresco (store E sender).
type factoryRecorder struct {
	calls             int
	address, password string
	store             mail.MailStore
	sender            Sender
}

func (f *factoryRecorder) build(address, password string) (mail.MailStore, Sender) {
	f.calls++
	f.address, f.password = address, password
	mk := &mail.Mock{}
	f.store, f.sender = mk, mk
	return mk, mk
}

// krFake è un keyring.Store finto per i test del radice: Set registra o
// fallisce secondo script.
type krFake struct {
	sets       int
	err        error
	lastUser   string
	lastSecret string
}

func (k *krFake) Get(string, string) (string, error) { return "", errors.New("krFake.Get inatteso") }

func (k *krFake) Set(_, user, secret string) error {
	k.sets++
	k.lastUser, k.lastSecret = user, secret
	return k.err
}

func (k *krFake) Delete(string, string) error { return errors.New("krFake.Delete inatteso") }

// okConnect è un TestConnect finto sempre riuscito (nessuna rete).
func okConnect(context.Context, string, string) error { return nil }

// newWizardTestModel costruisce il radice in modalità wizard con le deps date.
func newWizardTestModel(t *testing.T, deps wizard.Deps) (Model, *factoryRecorder) {
	t.Helper()
	m := newTestModel(t)
	f := &factoryRecorder{}
	m.SetWizard(wizard.New(deps, m.styles), f.build)
	return m, f
}

// deliver esegue la cmd data e consegna i messaggi prodotti al modello (come
// fa il runtime), IGNORANDO i tick dello spinner (solo frame, nessuno stato).
// Ritorna il modello aggiornato e le cmd prodotte in cascata NON eseguite: le
// sceglie il test (il re-arm dei tick dormirebbe un frame).
func deliver(m Model, c tea.Cmd) (Model, []tea.Cmd) {
	var cmds []tea.Cmd
	for _, msg := range runCmd(c) {
		if _, ok := msg.(spinner.TickMsg); ok {
			continue
		}
		mm, cmd := m.Update(msg)
		m = mm.(Model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return m, cmds
}

// completedIn estrae il CompletedMsg prodotto eseguendo le cmds date (i
// messaggi NON vengono consegnati al modello: li consegna il test).
func completedIn(t *testing.T, cmds []tea.Cmd) wizard.CompletedMsg {
	t.Helper()
	for _, c := range cmds {
		for _, msg := range runCmd(c) {
			if got, ok := msg.(wizard.CompletedMsg); ok {
				return got
			}
		}
	}
	t.Fatal("nessun CompletedMsg prodotto dalle cmds")
	return wizard.CompletedMsg{}
}

// typeWiz digita la stringa nel campo focalizzato del wizard (attraverso il
// routing del radice).
func typeWiz(m Model, s string) Model {
	for _, r := range s {
		m, _ = step(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// --- attivazione ----------------------------------------------------------------

// TestSetWizardActivates: SetWizard sostituisce la vista con il wizard (il
// load iniziale NON parte: parte al completamento), pending a 0 e Init senza
// loadRefs.
func TestSetWizardActivates(t *testing.T) {
	m, _ := newWizardTestModel(t, wizard.Deps{})
	if got := m.currentView(); got != viewWizard {
		t.Errorf("vista = %v, voglio viewWizard", got)
	}
	if m.pending != 0 {
		t.Errorf("pending = %d, voglio 0 (il load parte al completamento)", m.pending)
	}
	if !strings.Contains(m.View().Content, "Benvenuto") {
		t.Error("View non mostra il welcome del wizard")
	}
	// Init: nessun msgsLoaded (il load parte al completamento del wizard).
	for _, msg := range runCmd(m.Init()) {
		if _, isLoad := msg.(msgsLoaded); isLoad {
			t.Error("Init in modalità wizard emette il load della lista")
		}
	}
}

// TestWizardKeysRoutedToWizard: in modalità wizard TUTTI i tasti vanno al
// wizard — enter avanza (non apre messaggi), q/esc non escono dall'app.
func TestWizardKeysRoutedToWizard(t *testing.T) {
	m, _ := newWizardTestModel(t, wizard.Deps{})
	m, cmd := step(m, pressKey("enter"))
	if m.wiz.Step() != wizard.StepAddress {
		t.Errorf("enter: passo wizard = %v, voglio StepAddress (routing al wizard)", m.wiz.Step())
	}
	if cmd == nil {
		t.Error("enter al wizard: cmd di focus attesa")
	}
	m, cmd = step(m, pressKey("esc"))
	if m.wiz.Step() != wizard.StepWelcome {
		t.Errorf("esc al passo indirizzo: passo = %v, voglio StepWelcome (esc NON esce dall'app)", m.wiz.Step())
	}
	if cmd != nil {
		t.Errorf("esc al welcome (da indirizzo): cmd = %v, voglio nil (nessun quit)", cmd)
	}
	m, cmd = step(m, pressKey("q"))
	if m.wiz.Step() != wizard.StepWelcome {
		t.Errorf("q al welcome: passo = %v (q è ignorato dal wizard)", m.wiz.Step())
	}
	if cmd != nil {
		t.Errorf("q al wizard: cmd = %v, voglio nil (nessun quit)", cmd)
	}
}

// TestWizardMouseIgnored: col wizard attivo il mouse è ignorato (keyboard-first).
func TestWizardMouseIgnored(t *testing.T) {
	m, _ := newWizardTestModel(t, wizard.Deps{})
	m2, _ := step(m, tea.MouseClickMsg{X: 3, Y: 3})
	if m2.wiz.Step() != wizard.StepWelcome {
		t.Errorf("click: passo wizard = %v, voglio nessun cambiamento", m2.wiz.Step())
	}
}

// --- skip → demo con seed --------------------------------------------------------

// TestWizardSkipSeedsDemoStore: `s` al welcome → CompletedMsg{Skipped} →
// il radice passa al DemoStore SEEDED (6 messaggi, store E sender un oggetto),
// banner demo e parte il load.
func TestWizardSkipSeedsDemoStore(t *testing.T) {
	m, f := newWizardTestModel(t, wizard.Deps{})
	m, cmd := step(m, pressKey("s"))
	m, cmds := deliver(m, cmd)
	if f.calls != 0 {
		t.Errorf("factory chiamata allo skip (%d volte), voglio mai", f.calls)
	}
	mk, ok := m.store.(*mail.Mock)
	if !ok {
		t.Fatalf("store = %T, voglio *mail.Mock (demo)", m.store)
	}
	if len(mk.Msgs) < 5 {
		t.Errorf("seed demo = %d messaggi, voglio 5-6", len(mk.Msgs))
	}
	if m.sender != m.store {
		t.Error("in demo store e sender devono essere lo stesso oggetto")
	}
	if got := m.statusText(); got != demoBannerText {
		t.Errorf("status = %q, voglio il banner demo %q", got, demoBannerText)
	}
	if got := m.currentView(); got != viewInbox {
		t.Errorf("vista dopo lo skip = %v, voglio viewInbox", got)
	}
	// Il load parte col seed: la lista si poplia dei messaggi demo.
	for _, c := range cmds {
		m, _ = deliver(m, c)
	}
	if len(m.refs) != len(mk.Msgs) {
		t.Errorf("refs dopo il load = %d, voglio %d", len(m.refs), len(mk.Msgs))
	}
}

// --- successo → backend reali ----------------------------------------------------

// wizardToTesting porta il wizard del radice al passo testing con credenziali
// digitate (welcome → invio → indirizzo → invio → password → invio).
func wizardToTesting(t *testing.T, m Model) (Model, []tea.Cmd) {
	t.Helper()
	m, _ = step(m, pressKey("enter")) // welcome → address
	m = typeWiz(m, "user@gmail.com")
	m, _ = step(m, pressKey("enter")) // address → password
	m = typeWiz(m, "app-password")
	m, cmd := step(m, pressKey("enter")) // password → testing (verifica in flight)
	if m.wiz.Step() != wizard.StepTesting {
		t.Fatalf("passo wizard = %v, voglio StepTesting", m.wiz.Step())
	}
	return m, []tea.Cmd{cmd}
}

// TestWizardSuccessBuildsRealBackends: verifica riuscita → config+keyring
// (dipendenze finte) → CompletedMsg → il radice chiama la factory con le
// credenziali VERIFICATE, scambia store/sender, imposta l'identity e torna
// all'inbox con status positivo.
func TestWizardSuccessBuildsRealBackends(t *testing.T) {
	kr := &krFake{}
	dir := t.TempDir()
	m, f := newWizardTestModel(t, wizard.Deps{TestConnect: okConnect, Keyring: kr, CfgPath: dir + "/config.toml"})
	m, cmds := wizardToTesting(t, m)
	for _, c := range cmds {
		m, cmds = deliver(m, c)
	}
	// L'esito della verifica avvia il salvataggio: una cmd in coda.
	if m.wiz.Step() != wizard.StepSaving {
		t.Fatalf("passo wizard = %v, voglio StepSaving", m.wiz.Step())
	}
	cm := completedIn(t, cmds)
	if cm.Address != "user@gmail.com" || cm.Password != "app-password" {
		t.Fatalf("CompletedMsg = (%q, %q), voglio le credenziali digitate", cm.Address, cm.Password)
	}
	if f.calls != 0 {
		t.Fatalf("factory chiamata PRIMA del CompletedMsg (%d volte)", f.calls)
	}
	m, _ = step(m, cm)
	if f.calls != 1 {
		t.Fatalf("factory chiamate = %d, voglio 1", f.calls)
	}
	if f.address != "user@gmail.com" || f.password != "app-password" {
		t.Errorf("factory args = (%q, %q), voglio le credenziali verificate", f.address, f.password)
	}
	if m.store != f.store || m.sender != f.sender {
		t.Error("store/sender non sostituiti con quelli della factory")
	}
	if m.from != "user@gmail.com" {
		t.Errorf("identity mittente = %q, voglio l'indirizzo del wizard", m.from)
	}
	if got := m.currentView(); got != viewInbox {
		t.Errorf("vista dopo il completamento = %v, voglio viewInbox", got)
	}
	if !m.status.isOK || !strings.Contains(m.statusText(), "configurazione completata") {
		t.Errorf("status = %q (isOK=%v), voglio il feedback positivo del setup", m.statusText(), m.status.isOK)
	}
	if kr.sets != 1 || kr.lastUser != "user@gmail.com" {
		t.Errorf("keyring.Set = %d chiamate (user %q), voglio 1 (user@gmail.com)", kr.sets, kr.lastUser)
	}
}

// TestWizardSuccessKeyringFailureSuggestsEnv: keychain in errore → il wizard
// completa comunque e il radice mostra lo status col suggerimento della env
// var (in stile errore: la password non sopravvive al processo).
func TestWizardSuccessKeyringFailureSuggestsEnv(t *testing.T) {
	kr := &krFake{err: errors.New("security: exit 45")}
	m, f := newWizardTestModel(t, wizard.Deps{TestConnect: okConnect, Keyring: kr, CfgPath: t.TempDir() + "/config.toml"})
	m, cmds := wizardToTesting(t, m)
	for _, c := range cmds {
		m, cmds = deliver(m, c)
	}
	cm := completedIn(t, cmds)
	if cm.KeyringErr == nil {
		t.Fatal("CompletedMsg.KeyringErr = nil, voglio l'errore del keychain")
	}
	m, _ = step(m, cm)
	if f.calls != 1 {
		t.Errorf("factory chiamate = %d, voglio 1 (il wizard completa comunque)", f.calls)
	}
	if !m.status.isErr || !strings.Contains(m.statusText(), "GMAIL_APP_PASSWORD") {
		t.Errorf("status = %q (isErr=%v), voglio il suggerimento GMAIL_APP_PASSWORD", m.statusText(), m.status.isErr)
	}
}

// TestWizardSecondCompletedDropped: un secondo CompletedMsg (doppio `s` in
// volo) è STANTE: nessuna seconda costruzione dei backend.
func TestWizardSecondCompletedDropped(t *testing.T) {
	m, f := newWizardTestModel(t, wizard.Deps{})
	m, cmd := step(m, pressKey("s"))
	m, _ = deliver(m, cmd)
	step(m, wizard.CompletedMsg{Skipped: true}) // secondo msg: stante
	if f.calls != 0 {
		t.Errorf("factory chiamate = %d, voglio 0 (msg stante)", f.calls)
	}
}

// --- StatusOK (polish ereditato dal Piano 3) --------------------------------------

// assertStatusOK verifica che la status bar renda il testo col STYLE StatusOK
// (feedback positivo) e non col normale StatusBar.
func assertStatusOK(t *testing.T, m Model, text string) {
	t.Helper()
	if !m.status.isOK {
		t.Errorf("status.isOK = false per %q", text)
	}
	if m.status.isErr {
		t.Errorf("status.isErr = true per %q (feedback positivo)", text)
	}
	okRender := m.styles.StatusOK.Render(text)
	barRender := m.styles.StatusBar.Render(text)
	v := m.View().Content
	if !strings.Contains(v, okRender) {
		t.Errorf("View non usa lo stile StatusOK per %q", text)
	}
	if strings.Contains(v, barRender) {
		t.Errorf("View rende ancora %q col normale stile StatusBar", text)
	}
}

// TestOpDoneSuccessUsesStatusOK: i feedback positivi delle mutazioni e
// dell'invio (opDone senza errore) passano allo stile StatusOK.
func TestOpDoneSuccessUsesStatusOK(t *testing.T) {
	for _, label := range []string{"inviato", "archiviata", "stella on", "letta", "etichetta Work applicata"} {
		m := newTestModel(t)
		mm, _ := step(m, opDone{label: label})
		assertStatusOK(t, mm, label)
	}
}

// TestOpDoneErrorKeepsStatusErr: l'errore resta in stile StatusErr (non OK).
func TestOpDoneErrorKeepsStatusErr(t *testing.T) {
	m := newTestModel(t)
	mm, _ := step(m, opDone{label: "invio", err: errors.New("boom")})
	if !mm.status.isErr || mm.status.isOK {
		t.Errorf("status (isErr=%v, isOK=%v), voglio errore", mm.status.isErr, mm.status.isOK)
	}
}

// TestAttachmentSavedUsesStatusOK: ":save" riuscito → "salvato in …" in OK.
func TestAttachmentSavedUsesStatusOK(t *testing.T) {
	m := newTestModel(t)
	mm, _ := step(m, attachmentSaved{path: "/tmp/allegato.pdf"})
	assertStatusOK(t, mm, "salvato in /tmp/allegato.pdf")
}

// TestOpenedMsgUsesStatusOK: il browser è uscito pulito → "aperto nel browser" in OK.
func TestOpenedMsgUsesStatusOK(t *testing.T) {
	m := newTestModel(t)
	m, _ = step(m, msgsLoaded{}) // consuma il load iniziale: niente spinner nel bar
	mm, _ := step(m, thread.OpenedMsg{})
	assertStatusOK(t, mm, "aperto nel browser")
}

// TestThemeChangedUsesStatusOK: `:theme` riuscito → "tema … applicato" in OK.
func TestThemeChangedUsesStatusOK(t *testing.T) {
	m := newTestModel(t)
	m, _ = step(m, msgsLoaded{}) // consuma il load iniziale: niente spinner nel bar
	th, err := theme.Load("dracula")
	if err != nil {
		t.Fatalf("tema dracula: %v", err)
	}
	mm, _ := step(m, thread.ThemeChanged{Th: th, St: m.styles})
	assertStatusOK(t, mm, "tema dracula applicato")
}

// --- help del wizard --------------------------------------------------------------

// TestWizardHelpBindings: la status bar in modalità wizard documenta i tasti
// (continua / salta demo / uscita) e l'overlay help non publicizza `?`
// (irraggiungibile: i tasti vanno tutti al wizard).
func TestWizardHelpBindings(t *testing.T) {
	m, _ := newWizardTestModel(t, wizard.Deps{})
	short := m.shortHelpFor(false, viewWizard)
	if len(short) == 0 {
		t.Error("shortHelp viewWizard vuota")
	}
	for _, b := range short {
		if h := b.Help(); h.Key == "?" {
			t.Error("il help del wizard publicizza `?` (tasto irraggiungibile)")
		}
	}
	if full := m.fullHelpFor(false, viewWizard); len(full) == 0 {
		t.Error("fullHelp viewWizard vuota")
	}
}
