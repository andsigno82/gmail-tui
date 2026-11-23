package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// cacheReale è la UserCacheDir della macchina, letta prima dell'isolamento.
var cacheReale string

// TestMain isola i test dalla config e dalla cache reali dell'utente (Piano 16,
// Task 13): un test che arriva a openCache (o a config.Path) senza un proprio
// t.Setenv("HOME") apriva ~/Library/Caches/gmail-tui/cache.db della macchina.
// È successo con TestBackendFactory, che ha migrato la cache reale allo schema 4.
func TestMain(m *testing.M) {
	cacheReale, _ = os.UserCacheDir()
	home, err := os.MkdirTemp("", "gmail-tui-cmd-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for k, v := range map[string]string{
		"HOME":            home,
		"XDG_CONFIG_HOME": home + "/.config",
		"XDG_CACHE_HOME":  home + "/.cache",
		"AppData":         home, // os.UserConfigDir su Windows
		"LocalAppData":    home, // os.UserCacheDir su Windows
	} {
		if err := os.Setenv(k, v); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}

// TestCacheIsolataDallaMacchina: nessun test del pacchetto vede la cache reale.
func TestCacheIsolataDallaMacchina(t *testing.T) {
	dir, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if cacheReale != "" && strings.HasPrefix(dir, cacheReale) {
		t.Fatalf("UserCacheDir = %q: i test vedono la cache reale %q", dir, cacheReale)
	}
}
