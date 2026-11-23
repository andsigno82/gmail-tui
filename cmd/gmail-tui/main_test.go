package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/app"
	"github.com/andsigno82/gmail-tui/internal/config"
	"github.com/andsigno82/gmail-tui/internal/keyring"
	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/mail/imap"
	"github.com/andsigno82/gmail-tui/internal/mail/smtp"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Conformance (Piano 4, Task 4): sia il Sender SMTP che il Mock soddisfano
// l'interfaccia Sender richiesta dal radice (app.New).
var (
	_ app.Sender = (*smtp.Sender)(nil)
	_ app.Sender = (*mail.Mock)(nil)
)

// getenvOf costruisce un getenv finto dalla mappa data (chiave assente = "").
func getenvOf(env map[string]string) func(string) string {
	return func(k string) string { return env[k] }
}

// krStub è un keyring.Store finto per il wiring: Get risponde con il segreto
// scriptato (o ErrNotFound), Set/Delete falliscono rumorosamente (il wiring
// li legge solo per la risoluzione iniziale).
type krStub struct {
	secret string
	err    error
}

func (s krStub) Get(string, string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	if s.secret == "" {
		return "", keyring.ErrNotFound
	}
	return s.secret, nil
}

func (s krStub) Set(string, string, string) error { return errors.New("krStub.Set inatteso") }

func (s krStub) Delete(string, string) error { return errors.New("krStub.Delete inatteso") }

// --- wizardNeeded ------------------------------------------------------------

// TestWizardNeeded (Piano 4, Task 5): il wizard parte SOLO se forzato (`gmail-tui
// login`) oppure se la config MANCA e nessuna password è risolvibile (source
// ""). Config presente senza password → demo mode col banner esistente, NON
// wizard; config assente CON password (env o keychain) → sessione reale.
func TestWizardNeeded(t *testing.T) {
	cases := []struct {
		name            string
		force, cfgExist bool
		source          string
		want            bool
	}{
		{"config presente e nessuna password: demo, non wizard", false, true, "", false},
		{"config assente e nessuna password: wizard", false, false, "", true},
		{"config assente ma env password: reale", false, false, "env", false},
		{"config assente ma keychain: reale", false, false, "keychain", false},
		{"config presente con env password: reale", false, true, "env", false},
		{"login forza sempre", true, true, "env", true},
		{"login forza anche senza nulla", true, false, "", true},
	}
	for _, tc := range cases {
		if got := wizardNeeded(tc.force, tc.cfgExist, "ada@example.com", tc.source); got != tc.want {
			t.Errorf("%s: wizardNeeded(%v, %v, %q) = %v, voglio %v", tc.name, tc.force, tc.cfgExist, tc.source, got, tc.want)
		}
	}
}

// Piano 7.2, fix review: una config senza indirizzo (salvataggio dal menu in
// demo) non è un account: senza password il wizard torna.
func TestWizardNeededEmptyAddress(t *testing.T) {
	if !wizardNeeded(false, true, "", "") {
		t.Error("config con indirizzo vuoto e nessuna password: wizard atteso")
	}
	if wizardNeeded(false, true, "ada@example.com", "") {
		t.Error("config con indirizzo e nessuna password: demo, non wizard")
	}
}

// Piano 7.2, fix review: loadConfig con un valore non valido (layout "grid")
// conserva l'indirizzo e riporta al default solo il campo sbagliato.
func TestLoadConfigKeepsAddressOnInvalidValue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	path, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("address = \"ada@example.com\"\nlayout = \"grid\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, ok := loadConfig()
	if !ok {
		t.Fatal("loadConfig: ok = false, voglio true (valore non valido, non config rotta)")
	}
	if cfg.Address != "ada@example.com" || cfg.Layout != config.LayoutSingle {
		t.Errorf("cfg = %+v, voglio indirizzo conservato e layout di default", cfg)
	}
}

// --- resolvePassword -----------------------------------------------------------

// TestResolvePasswordEnv: la env var vince sul keychain (ordine del keyring).
func TestResolvePasswordEnv(t *testing.T) {
	pw, src := resolvePassword(getenvOf(map[string]string{"GMAIL_APP_PASSWORD": "pw-env"}), krStub{secret: "pw-kc"}, "user@gmail.com")
	if pw != "pw-env" || src != "env" {
		t.Errorf("risoluzione = (%q, %q), voglio (pw-env, env)", pw, src)
	}
}

// TestResolvePasswordKeychain: senza env si guarda nel keychain.
func TestResolvePasswordKeychain(t *testing.T) {
	pw, src := resolvePassword(getenvOf(nil), krStub{secret: "pw-kc"}, "user@gmail.com")
	if pw != "pw-kc" || src != "keychain" {
		t.Errorf("risoluzione = (%q, %q), voglio (pw-kc, keychain)", pw, src)
	}
}

// TestResolvePasswordAbsent: né env né keychain → ("", "") — il chiamante
// decide (wizard/demo).
func TestResolvePasswordAbsent(t *testing.T) {
	pw, src := resolvePassword(getenvOf(nil), krStub{}, "user@gmail.com")
	if pw != "" || src != "" {
		t.Errorf("risoluzione = (%q, %q), voglio (\"\", \"\")", pw, src)
	}
}

// TestResolvePasswordNilKeyring: keychain non disponibile (kr nil — OS non
// supportato o keyring.New in errore) → SOLO env, senza panic.
func TestResolvePasswordNilKeyring(t *testing.T) {
	pw, src := resolvePassword(getenvOf(map[string]string{"GMAIL_APP_PASSWORD": "pw-env"}), nil, "user@gmail.com")
	if pw != "pw-env" || src != "env" {
		t.Errorf("risoluzione con kr nil = (%q, %q), voglio (pw-env, env)", pw, src)
	}
	if pw, src := resolvePassword(getenvOf(nil), nil, "user@gmail.com"); pw != "" || src != "" {
		t.Errorf("risoluzione con kr nil e senza env = (%q, %q), voglio vuote", pw, src)
	}
}

// TestResolvePasswordKeyringError: errore del keychain (diverso da NotFound)
// → degrada a assente (il wizard/demo decide; nessun panic).
func TestResolvePasswordKeyringError(t *testing.T) {
	pw, src := resolvePassword(getenvOf(nil), krStub{err: errors.New("security: exec fail")}, "user@gmail.com")
	if pw != "" || src != "" {
		t.Errorf("risoluzione con keychain in errore = (%q, %q), voglio vuote", pw, src)
	}
}

// --- selectStore -------------------------------------------------------------

// TestSelectStoreDemoMode: senza password la factory restituisce il DemoStore
// SEEDED (Piano 4, Task 5: chi clona vede l'app viva) come store E sender —
// un solo oggetto, costruzione senza rete.
func TestSelectStoreDemoMode(t *testing.T) {
	store, sender, demo := selectStore(config.Default(), "", nil)
	if !demo {
		t.Error("demo = false, voglio true senza password")
	}
	mk, ok := store.(*mail.Mock)
	if !ok {
		t.Errorf("store = %T, voglio *mail.Mock", store)
	}
	s, ok := sender.(*mail.Mock)
	if !ok {
		t.Errorf("sender = %T, voglio *mail.Mock (demo: un solo oggetto)", sender)
	}
	if s != mk {
		t.Error("store e sender devono essere lo STESSO Mock in demo mode")
	}
	if len(mk.Msgs) < 5 {
		t.Errorf("seed demo = %d messaggi, voglio 5-6", len(mk.Msgs))
	}
}

// TestSelectStoreIMAP: con la password risolta la factory costruisce lo store
// IMAP reale e il sender SMTP (preset Gmail) SENZA connettersi: né NewStore né
// NewSender toccano la rete (la connessione avviene alla prima operazione),
// demo=false. Cache nil: il test non tocca il disco.
func TestSelectStoreIMAP(t *testing.T) {
	cfg := config.Config{Address: "user@gmail.com"}
	store, sender, demo := selectStore(cfg, "pw", nil)
	if demo {
		t.Error("demo = true, voglio false con password presente")
	}
	s, ok := store.(*imap.Store)
	if !ok {
		t.Fatalf("store = %T, voglio *imap.Store", store)
	}
	if s == nil {
		t.Error("store IMAP nil")
	}
	snd, ok := sender.(*smtp.Sender)
	if !ok {
		t.Fatalf("sender = %T, voglio *smtp.Sender", sender)
	}
	if snd == nil {
		t.Error("sender SMTP nil")
	}
}

// --- backendFactory ------------------------------------------------------------

// TestBackendFactory (Piano 4, Task 5; Piano 16, Task 11): la factory passata al
// wizard costruisce i backend REALI con le credenziali verificate SENZA toccare
// la rete (il dial avviene alla prima operazione). Lo store è avvolto dalla
// guardia R4: un ID non confermato è rifiutato senza rete.
func TestBackendFactory(t *testing.T) {
	store, sender := backendsWith("user@gmail.com", "pw", nil)
	if _, ok := sender.(*smtp.Sender); !ok {
		t.Errorf("sender = %T, voglio *smtp.Sender", sender)
	}
	err := store.MarkRead(context.Background(), []msg.MessageID{msg.NewID("INBOX", 7)}, true)
	if !errors.Is(err, mail.ErrNotConfirmed) {
		t.Errorf("MarkRead su ID non confermato = %v, voglio mail.ErrNotConfirmed", err)
	}
}

// --- cache legata all'account (Piano 16, Task 11) ------------------------------

// isolaCache porta UserCacheDir e UserConfigDir in una cartella temporanea.
func isolaCache(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
}

// cacheDirDi è la cartella della cache (UserCacheDir/gmail-tui) sotto l'isolamento.
func cacheDirDi(t *testing.T) string {
	t.Helper()
	dir, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "gmail-tui")
}

// Piano 16, Task 11, D16-A4/D16-T15: senza password (demo) nessun file di cache.
func TestDemoCreatesNoCacheFile(t *testing.T) {
	isolaCache(t)
	if c := cacheFor("ada@example.com", ""); c != nil {
		closeCache(c)
		t.Fatal("cacheFor senza password = cache aperta, voglio nil")
	}
	if _, err := os.Stat(filepath.Join(cacheDirDi(t), "cache.db")); !os.IsNotExist(err) {
		t.Errorf("file cache in demo: err = %v, voglio file assente", err)
	}
}

// Piano 16, Task 11, D16-A4: cartella 0700 e cache.db 0600.
func TestOpenCacheDirIs0700(t *testing.T) {
	isolaCache(t)
	c := openCache("ada@example.com")
	if c == nil {
		t.Fatal("openCache = nil, voglio cache aperta")
	}
	defer closeCache(c)
	assertPerm(t, cacheDirDi(t), 0o700)
	assertPerm(t, filepath.Join(cacheDirDi(t), "cache.db"), 0o600)
}

// Piano 16, Task 11: una cartella 0755 creata da versioni precedenti viene
// riportata a 0700 all'apertura della cache.
func TestOpenCacheTightensExistingDir(t *testing.T) {
	isolaCache(t)
	dir := cacheDirDi(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	c := openCache("ada@example.com")
	if c == nil {
		t.Fatal("openCache = nil, voglio cache aperta")
	}
	defer closeCache(c)
	assertPerm(t, dir, 0o700)
}

// Piano 16, Task 11: lo store reale è avvolto dalla guardia R4 e inoltra
// l'istantanea (Snapshotter) anche con la traccia spenta.
func TestRealStoreIsGuardedAndSnapshotter(t *testing.T) {
	st := realStore(imap.NewStore(imap.Options{Addr: imapAddr, Username: "ada@example.com", Password: "pw"}))
	if _, ok := st.(mail.Snapshotter); !ok {
		t.Fatalf("store reale = %T, voglio mail.Snapshotter", st)
	}
	err := st.MarkRead(context.Background(), []msg.MessageID{msg.NewID("INBOX", 7)}, true)
	if !errors.Is(err, mail.ErrNotConfirmed) {
		t.Errorf("MarkRead su ID non confermato = %v, voglio mail.ErrNotConfirmed", err)
	}
}

func assertPerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != want {
		t.Errorf("permessi di %s = %o, voglio %o", filepath.Base(path), got, want)
	}
}

// --- loadTheme ---------------------------------------------------------------

// TestLoadThemeDefaults: nome vuoto (né flag né config) → tema default, nessun
// warning.
func TestLoadThemeDefaults(t *testing.T) {
	th, warn := loadTheme("", "")
	if th.Name != "default" {
		t.Errorf("tema = %q, voglio \"default\"", th.Name)
	}
	if warn != "" {
		t.Errorf("warning = %q, voglio vuoto", warn)
	}
}

// TestLoadThemeFlagWins: il flag --theme vince sul tema della config.
func TestLoadThemeFlagWins(t *testing.T) {
	th, warn := loadTheme("dracula", "nord")
	if th.Name != "dracula" {
		t.Errorf("tema = %q, voglio \"dracula\" (il flag vince)", th.Name)
	}
	if warn != "" {
		t.Errorf("warning = %q, voglio vuoto", warn)
	}
}

// TestLoadThemeFromConfig: senza flag vale il tema della config.
func TestLoadThemeFromConfig(t *testing.T) {
	th, warn := loadTheme("", "gruvbox-dark")
	if th.Name != "gruvbox-dark" {
		t.Errorf("tema = %q, voglio \"gruvbox-dark\" (dalla config)", th.Name)
	}
	if warn != "" {
		t.Errorf("warning = %q, voglio vuoto", warn)
	}
}

// TestLoadThemeFallback: tema inesistente → fallback default CON warning (che
// il main mostrerà nello status iniziale della TUI).
func TestLoadThemeFallback(t *testing.T) {
	th, warn := loadTheme("inesistente", "")
	if th.Name != "default" {
		t.Errorf("tema = %q, voglio il fallback \"default\"", th.Name)
	}
	if warn == "" || !strings.Contains(warn, "inesistente") {
		t.Errorf("warning = %q, voglio menzioni \"inesistente\"", warn)
	}
}

// --- parseTUIArgs -------------------------------------------------------------

// TestParseTUIArgs: --theme con valore o forma corta; senza flag esce -1
// (si prosegue); --help esce 0; argomenti posizionali (comandi sconosciuti)
// escono 1.
func TestParseTUIArgs(t *testing.T) {
	if name, code := parseTUIArgs(nil); name != "" || code != -1 {
		t.Errorf("nessun argomento = (%q, %d), voglio (\"\", -1)", name, code)
	}
	if name, code := parseTUIArgs([]string{"--theme", "dracula"}); name != "dracula" || code != -1 {
		t.Errorf("--theme dracula = (%q, %d), voglio (\"dracula\", -1)", name, code)
	}
	if name, code := parseTUIArgs([]string{"-theme", "nord"}); name != "nord" || code != -1 {
		t.Errorf("-theme nord = (%q, %d), voglio (\"nord\", -1)", name, code)
	}
	if name, code := parseTUIArgs([]string{"--theme"}); code != 1 {
		t.Errorf("--theme senza valore = (%q, %d), voglio exit 1", name, code)
	}
	if name, code := parseTUIArgs([]string{"--help"}); code != 0 || name != "" {
		t.Errorf("--help = (%q, %d), voglio (\"\", 0)", name, code)
	}
	if name, code := parseTUIArgs([]string{"help"}); code != 1 {
		t.Errorf("posizionale sconosciuto = (%q, %d), voglio exit 1", name, code)
	}
}
