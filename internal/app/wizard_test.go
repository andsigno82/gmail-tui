// Wizard nel radice (Piano 4, Task 5): attivazione (SetWizard), routing dei
// tasti, completamento (skip → demo con seed; successo → factory dei backend
// reali) e gli esiti come toast (Piano 7.3, fix review).
package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/config"
	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/theme"
	"github.com/andsigno82/gmail-tui/internal/ui/thread"
	"github.com/andsigno82/gmail-tui/internal/ui/toast"
	"github.com/andsigno82/gmail-tui/internal/ui/wizard"
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
	// Il load parte col seed dopo il connect (bugfix 2026-11-22):
	// consegno DUE generazioni di cmd (beginSession → connResult →
	// load/watch). NON si segue oltre: il follow-up di watchStarted è
	// waitForChange, che blocca sul canale del feed.
	for round := 0; round < 2 && len(cmds) > 0; round++ {
		var next []tea.Cmd
		for _, c := range cmds {
			var mm Model
			mm, fc := deliver(m, c)
			m = mm
			next = append(next, fc...)
		}
		cmds = next
	}
	if want := len(inboxMsgs(mk)); len(m.refs) != want {
		t.Errorf("refs dopo il load = %d, voglio %d (posta in arrivo del seed)", len(m.refs), want)
	}
}

// TestWizardSkipSetsIdentity: dopo lo skip l'identity del radice è il
// destinatario dei seed demo ("tu@example.com") — `c` apre il composer col
// campo Da precompilato (non vuoto).
func TestWizardSkipSetsIdentity(t *testing.T) {
	m, _ := newWizardTestModel(t, wizard.Deps{})
	m, cmd := step(m, pressKey("s"))
	m, _ = deliver(m, cmd)
	if m.from != "tu@example.com" {
		t.Errorf("identity dopo lo skip = %q, voglio tu@example.com (destinatario del seed demo)", m.from)
	}
	mm, _ := step(m, pressKey("c"))
	if mm.currentView() != viewComposer {
		t.Fatalf("vista dopo c = %v, voglio viewComposer", mm.currentView())
	}
	if got := mm.composer.Draft().From; got != "tu@example.com" {
		t.Errorf("Draft.From = %q, voglio tu@example.com", got)
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
	if !toastIs(m, toast.Success) || !strings.Contains(toastText(m), "configurazione completata") {
		t.Errorf("toast = %q, voglio il feedback positivo del setup", toastText(m))
	}
	if kr.sets != 1 || kr.lastUser != "user@gmail.com" {
		t.Errorf("keyring.Set = %d chiamate (user %q), voglio 1 (user@gmail.com)", kr.sets, kr.lastUser)
	}
}

// Piano 7.2, fix review: dopo il wizard il radice riprende nome, firma e
// preferenze di lettura dalla config salvata; un `,` successivo non le scrive vuote.
func TestWizardSuccessLoadsSavedSettings(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	saved := config.Config{Address: "user@gmail.com", Name: "Ada", Signature: "-- \nAda", Theme: "default", Layout: config.LayoutSplit, PreviewMarkRead: config.PreviewMarkReadNever}
	if err := saved.Save(cfgPath); err != nil {
		t.Fatal(err)
	}
	m, _ := newWizardTestModel(t, wizard.Deps{TestConnect: okConnect, Keyring: &krFake{}, CfgPath: cfgPath})
	m.SetConfigPath(cfgPath)
	m, cmds := wizardToTesting(t, m)
	for _, c := range cmds {
		m, cmds = deliver(m, c)
	}
	m, _ = step(m, completedIn(t, cmds))
	if m.name != "Ada" || m.signature != saved.Signature {
		t.Errorf("nome/firma dopo il wizard = (%q, %q), voglio quelli della config", m.name, m.signature)
	}
	if m.layout != config.LayoutSplit || m.markRead != config.PreviewMarkReadNever {
		t.Errorf("preferenze dopo il wizard = (%q, %q), voglio (split, never)", m.layout, m.markRead)
	}
	if want := identityFrom("user@gmail.com", "Ada"); m.from != want {
		t.Errorf("From = %q, voglio %q", m.from, want)
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
	if !toastIs(m, toast.Error) || !strings.Contains(toastText(m), "GMAIL_APP_PASSWORD") {
		t.Errorf("toast = %q, voglio il suggerimento GMAIL_APP_PASSWORD", toastText(m))
	}
}

// TestWizardKeyringWarningStaysInBarUntilLoad (Piano 7.3, fix review): l'avviso
// di setup (keychain) sta nella barra neutra finché arriva il primo caricamento
// riuscito; il toast d'errore invece si chiude con un tasto.
func TestWizardKeyringWarningStaysInBarUntilLoad(t *testing.T) {
	kr := &krFake{err: errors.New("security: exit 45")}
	m, _ := newWizardTestModel(t, wizard.Deps{TestConnect: okConnect, Keyring: kr, CfgPath: t.TempDir() + "/config.toml"})
	m, cmds := wizardToTesting(t, m)
	for _, c := range cmds {
		m, cmds = deliver(m, c)
	}
	m, _ = step(m, completedIn(t, cmds))
	if !strings.Contains(m.statusText(), "GMAIL_APP_PASSWORD") {
		t.Fatalf("barra dopo il wizard = %q, voglio l'avviso con GMAIL_APP_PASSWORD", m.statusText())
	}
	if !toastIs(m, toast.Error) {
		t.Fatalf("toast = %v, voglio l'errore del keychain", m.toast.Kinds())
	}
	m, _ = step(m, pressKey("j"))
	if got := m.toast.Texts(); len(got) != 0 {
		t.Errorf("il tasto non ha chiuso il toast d'errore: %q", got)
	}
	if !strings.Contains(m.statusText(), "GMAIL_APP_PASSWORD") {
		t.Fatalf("il tasto ha cancellato l'avviso dalla barra: %q", m.statusText())
	}
	m, _ = step(m, msgsLoaded{gen: m.listGen, mailbox: m.folder.Name, query: m.queryRaw, refs: testRefs(t), limit: 50})
	if got := m.statusText(); got != "" {
		t.Errorf("barra dopo il primo load = %q, voglio vuota", got)
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

// --- esiti come toast (Piano 7.3, Task 2) ------------------------------------------

// assertToast verifica che l'esito text sia l'ultimo toast di tipo kind e che la
// barra di stato non lo conservi più (resta neutra).
func assertToast(t *testing.T, m Model, kind toast.Kind, text string) {
	t.Helper()
	if !toastIs(m, kind) || toastText(m) != text {
		t.Errorf("toast = %q (kind %v), voglio %q di tipo %v", toastText(m), m.toast.Kinds(), text, kind)
	}
	if m.statusText() == text {
		t.Errorf("la barra conserva l'esito %q", text)
	}
}

// TestOpDoneSuccessUsesToast: i feedback positivi delle mutazioni e dell'invio
// (opDone senza errore) diventano toast di successo.
func TestOpDoneSuccessUsesToast(t *testing.T) {
	for _, label := range []string{"inviato", "archiviata", "stella on", "letta", "etichetta Work applicata"} {
		m := newTestModel(t)
		mm, _ := step(m, opDone{label: label})
		assertToast(t, mm, toast.Success, label)
	}
}

// TestOpDoneErrorUsesErrorToast: l'errore di un'op è un toast d'errore.
func TestOpDoneErrorUsesErrorToast(t *testing.T) {
	m := newTestModel(t)
	mm, _ := step(m, opDone{label: "invio", err: errors.New("boom")})
	if !toastIs(mm, toast.Error) || !strings.Contains(toastText(mm), "boom") {
		t.Errorf("toast = %q, voglio errore con boom", toastText(mm))
	}
}

// TestAttachmentSavedUsesToast: ":save" riuscito → "salvato in …" come toast di successo.
func TestAttachmentSavedUsesToast(t *testing.T) {
	m := newTestModel(t)
	mm, _ := step(m, attachmentSaved{path: "/tmp/allegato.pdf"})
	assertToast(t, mm, toast.Success, "salvato in /tmp/allegato.pdf")
}

// TestOpenedMsgUsesToast: il browser è uscito pulito → "aperto nel browser" come toast.
func TestOpenedMsgUsesToast(t *testing.T) {
	m := newTestModel(t)
	m, _ = step(m, msgsLoaded{}) // consuma il load iniziale: niente spinner nel bar
	mm, _ := step(m, thread.OpenedMsg{})
	assertToast(t, mm, toast.Success, "aperto nel browser")
}

// TestThemeChangedUsesToast: `:theme` riuscito → "tema … applicato" come toast.
func TestThemeChangedUsesToast(t *testing.T) {
	m := newTestModel(t)
	m, _ = step(m, msgsLoaded{}) // consuma il load iniziale: niente spinner nel bar
	th, err := theme.Load("dracula")
	if err != nil {
		t.Fatalf("tema dracula: %v", err)
	}
	mm, _ := step(m, thread.ThemeChanged{Th: th, St: m.styles})
	assertToast(t, mm, toast.Success, "tema dracula applicato")
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
