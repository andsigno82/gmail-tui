package app

import (
	"fmt"
	"os"
	"testing"
)

// TestMain isola i test dal config reale dell'utente (Piano 7.3, fix test):
// New risolve cfgPath con config.Path() (os.UserConfigDir, che dipende da
// HOME su macOS e da XDG_CONFIG_HOME/HOME su Linux). Senza isolamento un test
// legge — o un salvataggio scrive — ~/Library/Application Support/gmail-tui
// o ~/.config/gmail-tui della macchina.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "gmail-tui-app-test-")
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
