// Package cmd contiene i sottocomandi CLI non interattivi.
package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/andsigno82/gmail-tui/internal/config"
	"github.com/andsigno82/gmail-tui/internal/theme"
)

// CapsProbe legge le capability annunciate dal server IMAP (Piano 7.1, Task 4).
// La costruisce il main (che conosce credenziali e host) e la passa a Doctor:
// questo package non importa internal/mail/imap. nil = controllo saltato.
type CapsProbe func(ctx context.Context) ([]string, error)

// requiredCaps sono le capability che trash (MOVE, UIDPLUS) e cartelle
// (SPECIAL-USE, LIST-EXTENDED) usano; Gmail le annuncia tutte.
var requiredCaps = []string{"MOVE", "UIDPLUS", "SPECIAL-USE", "LIST-EXTENDED"}

// probeTimeout è il budget del probe IMAP (variabile per i test). 90 s: Piano 16,
// Q16-7, come connectTimeout della TUI (il LOGIN reale può superare i 30 s).
var probeTimeout = 90 * time.Second

// Doctor diagnostica la configurazione e stampa un report.
// exit code: 0 tutto ok (anche con config assente), 1 problemi bloccanti.
// probe (può essere nil) controlla le capability IMAP; la password non viene
// mai stampata.
func Doctor(out io.Writer, cfgPath string, getenv func(string) string, probe CapsProbe) int {
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

	// Piano 7.1, Task 6: un probe non nil implica una password risolta dal
	// main (env → keychain): con env vuota viene dal keychain.
	switch {
	case getenv("GMAIL_APP_PASSWORD") != "":
		fmt.Fprintln(out, "password: GMAIL_APP_PASSWORD ok")
	case probe != nil:
		fmt.Fprintln(out, "password: keychain ok")
	default:
		fmt.Fprintln(out, "password: missing — esegui \"gmail-tui login\" o export GMAIL_APP_PASSWORD")
	}

	if probe != nil {
		problems += checkIMAP(out, probe)
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

// checkIMAP esegue il probe con il budget probeTimeout e confronta le
// capability con requiredCaps: una riga per capability mancante (bloccante),
// un errore di connessione è bloccante. Restituisce il numero di problemi.
func checkIMAP(out io.Writer, probe CapsProbe) int {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	caps, err := probe(ctx)
	if err != nil {
		fmt.Fprintf(out, "imap: ERRORE connessione: %v\n", err)
		return 1
	}
	have := make(map[string]bool, len(caps))
	for _, c := range caps {
		have[strings.ToUpper(c)] = true
	}
	missing := 0
	for _, c := range requiredCaps {
		if !have[c] {
			fmt.Fprintf(out, "imap: MANCA %s\n", c)
			missing++
		}
	}
	if missing == 0 {
		fmt.Fprintf(out, "imap: ok (%s)\n", strings.Join(requiredCaps, " "))
	}
	// Piano 8, Task 6: riga informativa, non bloccante. Senza X-GM-EXT-1 le
	// conversazioni e la ricerca X-GM-RAW non sono disponibili.
	if have["X-GM-EXT-1"] {
		fmt.Fprintln(out, "imap: estensioni Gmail X-GM-EXT-1: sì")
	} else {
		fmt.Fprintln(out, "imap: estensioni Gmail X-GM-EXT-1: no (lista a messaggi singoli, ricerca Gmail non disponibile)")
	}
	return missing
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
