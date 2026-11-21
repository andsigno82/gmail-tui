package main

import (
	"strings"
	"testing"

	"gmail-tui/internal/config"
	"gmail-tui/internal/mail"
	"gmail-tui/internal/mail/imap"
)

// getenvOf costruisce un getenv finto dalla mappa data (chiave assente = "").
func getenvOf(env map[string]string) func(string) string {
	return func(k string) string { return env[k] }
}

// --- selectStore -------------------------------------------------------------

// TestSelectStoreDemoMode: senza GMAIL_APP_PASSWORD la factory restituisce il
// Mock (demo mode): costruzione senza rete, demo=true.
func TestSelectStoreDemoMode(t *testing.T) {
	store, demo := selectStore(config.Default(), getenvOf(nil), nil)
	if !demo {
		t.Error("demo = false, voglio true senza GMAIL_APP_PASSWORD")
	}
	if _, ok := store.(*mail.Mock); !ok {
		t.Errorf("store = %T, voglio *mail.Mock", store)
	}
}

// TestSelectStoreIMAP: con GMAIL_APP_PASSWORD la factory costruisce lo store
// IMAP reale (preset Gmail) SENZA connettersi: NewStore non tocca la rete (la
// connessione avviene alla prima operazione), demo=false. Cache nil: il test
// non tocca il disco.
func TestSelectStoreIMAP(t *testing.T) {
	cfg := config.Config{Address: "user@gmail.com"}
	store, demo := selectStore(cfg, getenvOf(map[string]string{"GMAIL_APP_PASSWORD": "pw"}), nil)
	if demo {
		t.Error("demo = true, voglio false con GMAIL_APP_PASSWORD presente")
	}
	s, ok := store.(*imap.Store)
	if !ok {
		t.Fatalf("store = %T, voglio *imap.Store", store)
	}
	if s == nil {
		t.Error("store IMAP nil")
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
