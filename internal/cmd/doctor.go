// Package cmd contiene i sottocomandi CLI non interattivi.
package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/andsigno82/gmail-tui/internal/config"
	"github.com/andsigno82/gmail-tui/internal/theme"
)

// Doctor diagnostica la configurazione e stampa un report.
// exit code: 0 tutto ok (anche con config assente), 1 problemi bloccanti.
func Doctor(out io.Writer, cfgPath string, getenv func(string) string) int {
	problems := 0

	cfg, err := config.Load(cfgPath)
	switch {
	case err != nil:
		fmt.Fprintf(out, "config: %s ERRORE: %v\n", cfgPath, err)
		problems++
		cfg = config.Default()
	case fileExists(cfgPath):
		fmt.Fprintf(out, "config: %s (address=%q)\n", cfgPath, cfg.Address)
	default:
		fmt.Fprintln(out, "config: nessuna, uso defaults (parte il wizard first-run, oppure \"gmail-tui login\")")
	}

	if _, err := theme.Load(cfg.Theme); err != nil {
		fmt.Fprintf(out, "theme %s: ERRORE: %v\n", cfg.Theme, err)
		problems++
	} else {
		fmt.Fprintf(out, "theme: %s ok\n", cfg.Theme)
	}

	if getenv("GMAIL_APP_PASSWORD") == "" {
		fmt.Fprintln(out, "password: missing — esegui \"gmail-tui login\" o export GMAIL_APP_PASSWORD")
	} else {
		fmt.Fprintln(out, "password: GMAIL_APP_PASSWORD ok")
	}

	if dir, err := os.UserCacheDir(); err == nil {
		cacheDir := filepath.Join(dir, "gmail-tui")
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			fmt.Fprintf(out, "cache: %s NON SCRIVIBILE: %v\n", cacheDir, err)
			problems++
		} else {
			fmt.Fprintf(out, "cache: %s ok\n", cacheDir)
		}
	}

	if problems > 1 {
		problems = 1 // contratto documentato: exit code 0/1
	}
	return problems
}

// Themes stampa i temi disponibili (uno per riga).
func Themes(out io.Writer) {
	for _, name := range theme.Names() {
		fmt.Fprintln(out, name)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
