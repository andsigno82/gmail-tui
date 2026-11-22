package wizard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"gmail-tui/internal/config"
	"gmail-tui/internal/keyring"
	"gmail-tui/internal/theme"
	"gmail-tui/internal/ui/styles"
)

// --- helper -------------------------------------------------------------------

// newModel costruisce il wizard di test col tema default e le dipendenze
// date (nil-safe: Deps{} è valido ma nessun test lo usa per il flusso completo).
func newModel(t *testing.T, deps Deps) Model {
	t.Helper()
	th, err := theme.Load("default")
	if err != nil {
		t.Fatalf("tema default: %v", err)
	}
	return New(deps, styles.New(th))
}

// pressKey costruisce una pressione di tasto (v2 KeyPressMsg).
func pressKey(k string) tea.KeyPressMsg {
	switch k {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
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

// typeKeys digita la stringa nel campo focalizzato (una pressione per rune).
func typeKeys(m Model, s string) Model {
	for _, r := range s {
		m, _ = step(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// connRecorder è un TestConnect finto: registra gli argomenti e il ctx e
// risponde con l'errore scriptato (nil = successo).
type connRecorder struct {
	calls    int
	address  string
	password string
	deadline time.Duration // deadline residua osservata alla chiamata
	err      error
}

func (r *connRecorder) test(ctx context.Context, address, password string) error {
	r.calls++
	r.address, r.password = address, password
	if d, ok := ctx.Deadline(); ok {
		r.deadline = time.Until(d)
	}
	return r.err
}

// krRecorder è un keyring.Store finto che registra le Set (Get/Delete non
// usati dal wizard: falliscono rumorosamente se chiamati).
type krRecorder struct {
	sets    int
	service string
	user    string
	secret  string
	err     error
}

func (r *krRecorder) Get(service, user string) (string, error) {
	return "", errors.New("krRecorder.Get inatteso")
}

func (r *krRecorder) Set(service, user, secret string) error {
	r.sets++
	r.service, r.user, r.secret = service, user, secret
	return r.err
}

func (r *krRecorder) Delete(service, user string) error {
	return errors.New("krRecorder.Delete inatteso")
}

// atPassword porta il wizard al passo password con credenziali valide digitate
// (welcome → invio → indirizzo → invio → password).
func atPassword(t *testing.T, deps Deps) Model {
	t.Helper()
	m := newModel(t, deps)
	m, _ = step(m, pressKey("enter")) // welcome → address
	m = typeKeys(m, "user@gmail.com") // address
	m, _ = step(m, pressKey("enter")) // address → password
	if m.Step() != StepPassword {
		t.Fatalf("step = %v, voglio StepPassword", m.Step())
	}
	return m
}

// connTestedOf estrae l'unico connTested prodotto dai msgs dati.
func connTestedOf(t *testing.T, msgs []tea.Msg) connTested {
	t.Helper()
	for _, msg := range msgs {
		if ct, ok := msg.(connTested); ok {
			return ct
		}
	}
	t.Fatalf("nessun connTested in %d msgs", len(msgs))
	return connTested{}
}

// completedOf estrae l'unico CompletedMsg prodotto dai msgs dati.
func completedOf(t *testing.T, msgs []tea.Msg) CompletedMsg {
	t.Helper()
	for _, msg := range msgs {
		if cm, ok := msg.(CompletedMsg); ok {
			return cm
		}
	}
	t.Fatalf("nessun CompletedMsg in %d msgs", len(msgs))
	return CompletedMsg{}
}

// --- URL e costanti -------------------------------------------------------------

// TestAppPasswordURL: l'URL delle App Password è quello richiesto dal piano.
func TestAppPasswordURL(t *testing.T) {
	if appPasswordURL != "https://myaccount.google.com/apppasswords" {
		t.Errorf("appPasswordURL = %q, voglio l'URL di myaccount.google.com/apppasswords", appPasswordURL)
	}
}

// --- macchina a stati -----------------------------------------------------------

// TestNewStartsAtWelcome: il wizard parte dal welcome, che mostra le istruzioni
// 2FA, l'URL delle App Password e i tasti (invio continua, s salta demo).
func TestNewStartsAtWelcome(t *testing.T) {
	m := newModel(t, Deps{})
	if m.Step() != StepWelcome {
		t.Errorf("step iniziale = %v, voglio StepWelcome", m.Step())
	}
	v := m.View()
	for _, want := range []string{"Benvenuto", appPasswordURL, "due passaggi", "invio", "s"} {
		if !strings.Contains(v, want) {
			t.Errorf("View non contiene %q", want)
		}
	}
}

// TestWelcomeEnterGoesToAddress: invio al welcome avanza all'input indirizzo
// (il campo nasce focalizzato: la cmd di focus/blink è l'esito).
func TestWelcomeEnterGoesToAddress(t *testing.T) {
	m := newModel(t, Deps{})
	m, cmd := step(m, pressKey("enter"))
	if m.Step() != StepAddress {
		t.Errorf("step = %v, voglio StepAddress", m.Step())
	}
	if cmd == nil {
		t.Error("enter al welcome: cmd di focus attesa (nil)")
	}
}

// TestWelcomeOProducesExecCmd: `o` al welcome produce la Cmd di exec del
// browser sull'URL (non eseguita nel test: aprirebbe il browser vero) e resta
// al welcome.
func TestWelcomeOProducesExecCmd(t *testing.T) {
	m := newModel(t, Deps{})
	m, cmd := step(m, pressKey("o"))
	if cmd == nil {
		t.Fatal("`o` deve produrre una Cmd di exec (non-nil)")
	}
	if m.Step() != StepWelcome {
		t.Errorf("step dopo `o` = %v, voglio restare al welcome", m.Step())
	}
}

// TestWelcomeSkipCompletes: `s` al welcome completa SENZA credenziali:
// CompletedMsg{Skipped: true} — il radice entra in demo mode col seed.
func TestWelcomeSkipCompletes(t *testing.T) {
	m := newModel(t, Deps{})
	m, cmd := step(m, pressKey("s"))
	if m.Step() != StepWelcome {
		t.Errorf("step dopo `s` = %v (il completion arriva via cmd)", m.Step())
	}
	msgs := runCmds(cmd)
	cm := completedOf(t, msgs)
	if !cm.Skipped {
		t.Error("CompletedMsg.Skipped = false, voglio true")
	}
	if cm.Address != "" || cm.Password != "" {
		t.Errorf("skip con credenziali = (%q, %q), voglio vuote", cm.Address, cm.Password)
	}
}

// TestWelcomeOtherKeysIgnored: tasti estranei al welcome (q, x, tab) non
// cambiano passo.
func TestWelcomeOtherKeysIgnored(t *testing.T) {
	m := newModel(t, Deps{})
	for _, k := range []string{"q", "x", "tab", "?"} {
		m2, _ := step(m, pressKey(k))
		if m2.Step() != StepWelcome {
			t.Errorf("tasto %q: step = %v, voglio restare al welcome", k, m2.Step())
		}
	}
}

// TestAddressEscBackToWelcome: esc al passo indirizzo torna al welcome.
func TestAddressEscBackToWelcome(t *testing.T) {
	m := newModel(t, Deps{})
	m, _ = step(m, pressKey("enter"))
	m, _ = step(m, pressKey("esc"))
	if m.Step() != StepWelcome {
		t.Errorf("step dopo esc = %v, voglio StepWelcome", m.Step())
	}
}

// TestAddressEnterValidates: invio con indirizzo vuoto o senza @ resta al
// passo con messaggio chiaro; un indirizzo valido avanza alla password.
func TestAddressEnterValidates(t *testing.T) {
	m := newModel(t, Deps{})
	m, _ = step(m, pressKey("enter"))

	m, _ = step(m, pressKey("enter")) // vuoto
	if m.Step() != StepAddress {
		t.Errorf("invio con indirizzo vuoto: step = %v, voglio restare", m.Step())
	}
	if !strings.Contains(m.View(), "inserisci l'indirizzo") {
		t.Error("messaggio per indirizzo vuoto non visibile")
	}

	m = typeKeys(m, "utente-senza-chiocciola")
	m, _ = step(m, pressKey("enter"))
	if m.Step() != StepAddress {
		t.Errorf("invio senza @: step = %v, voglio restare", m.Step())
	}
	if !strings.Contains(m.View(), "manca @") {
		t.Error("errore per indirizzo senza @ non visibile")
	}

	m = typeKeys(m, "user@gmail.com")
	m, _ = step(m, pressKey("enter"))
	if m.Step() != StepPassword {
		t.Errorf("indirizzo valido: step = %v, voglio StepPassword", m.Step())
	}
}

// TestPasswordEscBackToAddress: esc al passo password torna all'indirizzo
// (il valore digitato resta, il messaggio d'errore si azzera).
func TestPasswordEscBackToAddress(t *testing.T) {
	m := atPassword(t, Deps{})
	m = typeKeys(m, "abc123")
	m, _ = step(m, pressKey("esc"))
	if m.Step() != StepAddress {
		t.Errorf("step dopo esc = %v, voglio StepAddress", m.Step())
	}
	if got := m.address.Value(); got != "user@gmail.com" {
		t.Errorf("indirizzo dopo esc = %q, voglio intatto", got)
	}
}

// TestTestingEscIgnored: durante la verifica (StepTesting) esc NON torna
// indietro: il test è già in volo, l'esito arriverà via connTested.
func TestTestingEscIgnored(t *testing.T) {
	rec := &connRecorder{}
	m := atPassword(t, Deps{TestConnect: rec.test})
	m = typeKeys(m, "app-password")
	m, cmd := step(m, pressKey("enter")) // → testing
	if m.Step() != StepTesting {
		t.Fatalf("step = %v, voglio StepTesting", m.Step())
	}
	m2, _ := step(m, pressKey("esc"))
	if m2.Step() != StepTesting {
		t.Errorf("esc durante il test: step = %v, voglio restare", m2.Step())
	}
	_ = cmd
}

// --- input password mascherato ---------------------------------------------------

// TestPasswordMasked: la password si maschera in View (EchoPassword con
// EchoCharacter '*') ma il valore resta leggibile per la verifica.
func TestPasswordMasked(t *testing.T) {
	m := atPassword(t, Deps{})
	m = typeKeys(m, "abc123")
	if got := m.password.Value(); got != "abc123" {
		t.Errorf("password.Value = %q, voglio %q", got, "abc123")
	}
	v := m.View()
	if !strings.Contains(v, "*") {
		t.Error("View non mostra la maschera (*) della password")
	}
	if strings.Contains(v, "abc123") {
		t.Error("View mostra la password IN CHIARO")
	}
}

// --- verifica connessione REALE (funzione iniettata: nessuna rete nei test) -----

// TestPasswordEnterRunsConnTest: invio alla password avvia la verifica con le
// credenziali digitate e il ctx con il budget di 30s; lo stato va a testing.
func TestPasswordEnterRunsConnTest(t *testing.T) {
	rec := &connRecorder{}
	m := atPassword(t, Deps{TestConnect: rec.test})
	m = typeKeys(m, "app-password")
	m, cmd := step(m, pressKey("enter"))
	if m.Step() != StepTesting {
		t.Fatalf("step = %v, voglio StepTesting", m.Step())
	}
	msgs := runCmds(cmd)
	ct := connTestedOf(t, msgs)
	if ct.err != nil {
		t.Errorf("connTested.err = %v, voglio nil (finto successo)", ct.err)
	}
	if rec.calls != 1 {
		t.Fatalf("chiamate TestConnect = %d, voglio 1", rec.calls)
	}
	if rec.address != "user@gmail.com" || rec.password != "app-password" {
		t.Errorf("credenziali passate = (%q, %q), voglio (user@gmail.com, app-password)", rec.address, rec.password)
	}
	if rec.deadline <= 0 || rec.deadline > 31*time.Second {
		t.Errorf("deadline ctx = %v, voglio (0, 31s]", rec.deadline)
	}
}

// TestConnTestErrorShowsAndRetries: verifica fallita → messaggio chiaro in
// View, ritorno al passo password; invio ripete la verifica (retry).
func TestConnTestErrorShowsAndRetries(t *testing.T) {
	rec := &connRecorder{err: errors.New("boom: LOGIN failed")}
	m := atPassword(t, Deps{TestConnect: rec.test})
	m = typeKeys(m, "app-password")
	m, cmd := step(m, pressKey("enter"))
	m, _ = step(m, connTestedOf(t, runCmds(cmd)))
	if m.Step() != StepPassword {
		t.Errorf("step dopo errore = %v, voglio StepPassword (retry)", m.Step())
	}
	if !strings.Contains(m.View(), "boom") {
		t.Error("l'errore di connessione non è visibile in View")
	}
	// Retry: invio riparte con la stessa password (conservata).
	m2, cmd2 := step(m, pressKey("enter"))
	if m2.Step() != StepTesting {
		t.Errorf("retry: step = %v, voglio StepTesting", m2.Step())
	}
	msgs := runCmds(cmd2)
	ct := connTestedOf(t, msgs)
	if ct.err == nil || !strings.Contains(ct.err.Error(), "boom") {
		t.Errorf("retry: connTested.err = %v, voglio l'errore scriptato", ct.err)
	}
	if rec.calls != 2 {
		t.Errorf("chiamate TestConnect = %d, voglio 2 (retry)", rec.calls)
	}
}

// TestConnTestSuccessGoesToSaving: verifica riuscita → passo saving (salvataggio
// config + keyring in cmd), poi CompletedMsg con le credenziali testate.
func TestConnTestSuccessGoesToSaving(t *testing.T) {
	rec := &connRecorder{}
	kr := &krRecorder{}
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	m := atPassword(t, Deps{TestConnect: rec.test, Keyring: kr, CfgPath: cfgPath})
	m = typeKeys(m, "app-password")
	m, cmd := step(m, pressKey("enter"))
	m, fin := step(m, connTestedOf(t, runCmds(cmd)))
	if m.Step() != StepSaving {
		t.Fatalf("step = %v, voglio StepSaving", m.Step())
	}
	msgs := runCmds(fin)
	cm := completedOf(t, msgs)
	if cm.Skipped {
		t.Error("CompletedMsg.Skipped = true, voglio false dopo il setup")
	}
	if cm.Address != "user@gmail.com" || cm.Password != "app-password" {
		t.Errorf("credenziali nel CompletedMsg = (%q, %q), voglio quelle digitate", cm.Address, cm.Password)
	}
	if cm.ConfigErr != nil {
		t.Errorf("ConfigErr = %v, voglio nil", cm.ConfigErr)
	}
	if cm.KeyringErr != nil {
		t.Errorf("KeyringErr = %v, voglio nil", cm.KeyringErr)
	}
}

// --- successo: config scritta + keyring.Set --------------------------------------

// TestFinalizeWritesConfig: al successo la config è scritta sul percorso delle
// dipendenze: address digitato, name/signature vuote, tema default. NESSUNA
// password in chiaro nel file.
func TestFinalizeWritesConfig(t *testing.T) {
	kr := &krRecorder{}
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "gmail-tui", "config.toml")
	m := newModel(t, Deps{TestConnect: func(context.Context, string, string) error { return nil }, Keyring: kr, CfgPath: cfgPath})
	m, _ = step(m, pressKey("enter"))
	m = typeKeys(m, "user@gmail.com")
	m, _ = step(m, pressKey("enter"))
	m = typeKeys(m, "app-password")
	m, cmd := step(m, pressKey("enter"))
	m, fin := step(m, connTestedOf(t, runCmds(cmd)))
	completedOf(t, runCmds(fin))
	_ = m

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("config non scritta: %v", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config non leggibile: %v", err)
	}
	if cfg.Address != "user@gmail.com" {
		t.Errorf("cfg.Address = %q, voglio user@gmail.com", cfg.Address)
	}
	if cfg.Name != "" || cfg.Signature != "" {
		t.Errorf("cfg.Name/Signature = (%q, %q), voglio vuote", cfg.Name, cfg.Signature)
	}
	if cfg.Theme != "default" {
		t.Errorf("cfg.Theme = %q, voglio default", cfg.Theme)
	}
	if strings.Contains(string(data), "app-password") {
		t.Error("la password È FINITA IN CHIARO nella config")
	}
}

// TestFinalizeSetsKeyring: al successo la password va nel keychain col service
// gmail-tui e l'indirizzo come user.
func TestFinalizeSetsKeyring(t *testing.T) {
	kr := &krRecorder{}
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	m := newModel(t, Deps{TestConnect: func(context.Context, string, string) error { return nil }, Keyring: kr, CfgPath: cfgPath})
	m, _ = step(m, pressKey("enter"))
	m = typeKeys(m, "user@gmail.com")
	m, _ = step(m, pressKey("enter"))
	m = typeKeys(m, "app-password")
	m, cmd := step(m, pressKey("enter"))
	m, fin := step(m, connTestedOf(t, runCmds(cmd)))
	_ = m
	runCmds(fin)
	if kr.sets != 1 {
		t.Fatalf("keyring.Set chiamate = %d, voglio 1", kr.sets)
	}
	if kr.service != keyring.ServiceName || kr.user != "user@gmail.com" || kr.secret != "app-password" {
		t.Errorf("Set args = (%q, %q, %q), voglio (gmail-tui, user@gmail.com, app-password)", kr.service, kr.user, kr.secret)
	}
}

// TestFinalizeKeyringFailureCompletesAnyway: keychain che fallisce → il
// CompletedMsg porta KeyringErr (il radice suggerirà la env var) ma il wizard
// COMPLETA lo stesso: config scritta, credenziali consegnate.
func TestFinalizeKeyringFailureCompletesAnyway(t *testing.T) {
	kr := &krRecorder{err: errors.New("security: exit 45")}
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	m := newModel(t, Deps{TestConnect: func(context.Context, string, string) error { return nil }, Keyring: kr, CfgPath: cfgPath})
	m, _ = step(m, pressKey("enter"))
	m = typeKeys(m, "user@gmail.com")
	m, _ = step(m, pressKey("enter"))
	m = typeKeys(m, "app-password")
	m, cmd := step(m, pressKey("enter"))
	m, fin := step(m, connTestedOf(t, runCmds(cmd)))
	cm := completedOf(t, runCmds(fin))
	_ = m
	if cm.KeyringErr == nil {
		t.Error("KeyringErr = nil, voglio l'errore del keychain finto")
	}
	if cm.ConfigErr != nil {
		t.Errorf("ConfigErr = %v, voglio nil (la config è scritta lo stesso)", cm.ConfigErr)
	}
	if _, err := os.Stat(cfgPath); err != nil {
		t.Errorf("config non scritta nonostante il keychain in errore: %v", err)
	}
}

// TestFinalizeNoKeyringSuggestsEnv: senza keychain (Deps.Keyring nil — OS non
// supportato o New in errore) il completamento porta comunque un KeyringErr
// che indica GMAIL_APP_PASSWORD.
func TestFinalizeNoKeyringSuggestsEnv(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	m := newModel(t, Deps{TestConnect: func(context.Context, string, string) error { return nil }, CfgPath: cfgPath})
	m, _ = step(m, pressKey("enter"))
	m = typeKeys(m, "user@gmail.com")
	m, _ = step(m, pressKey("enter"))
	m = typeKeys(m, "app-password")
	m, cmd := step(m, pressKey("enter"))
	m, fin := step(m, connTestedOf(t, runCmds(cmd)))
	cm := completedOf(t, runCmds(fin))
	_ = m
	if cm.KeyringErr == nil || !strings.Contains(cm.KeyringErr.Error(), "GMAIL_APP_PASSWORD") {
		t.Errorf("KeyringErr = %v, voglio il suggerimento GMAIL_APP_PASSWORD", cm.KeyringErr)
	}
}

// TestFinalizeConfigErrorCompletesAnyway: config che fallisce (percorso
// non scrivibile) → CompletedMsg.ConfigErr, wizard completa comunque con le
// credenziali (la sessione corrente funziona).
func TestFinalizeConfigErrorCompletesAnyway(t *testing.T) {
	kr := &krRecorder{}
	m := newModel(t, Deps{TestConnect: func(context.Context, string, string) error { return nil }, Keyring: kr, CfgPath: filepath.Join(t.TempDir(), "config.toml")})
	m, _ = step(m, pressKey("enter"))
	m = typeKeys(m, "user@gmail.com")
	m, _ = step(m, pressKey("enter"))
	m = typeKeys(m, "app-password")
	m, cmd := step(m, pressKey("enter"))
	// Percorso NON scrivibile: il "padre" del file config è un file normale,
	// la MkdirAll di Save fallisce.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.deps.CfgPath = filepath.Join(blocker, "config.toml")
	m, fin := step(m, connTestedOf(t, runCmds(cmd)))
	cm := completedOf(t, runCmds(fin))
	_ = m
	if cm.ConfigErr == nil {
		t.Error("ConfigErr = nil, voglio l'errore di salvataggio")
	}
	if cm.KeyringErr != nil {
		t.Errorf("KeyringErr = %v, voglio nil (keychain finto ok)", cm.KeyringErr)
	}
}
