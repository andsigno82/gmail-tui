// Login sottocomando (Piano 4, Task 5): `gmail-tui login` lancia la TUI in
// modalità wizard FORZATA (first-run guidato: istruzioni 2FA, credenziali,
// verifica di connessione reale, salvataggio config + keychain — cfr.
// internal/ui/wizard). Non c'è interattività fuori dalla TUI: il parsing dei
// flag precede ogni I/O — --help/-h escono 0 senza avviare la TUI, errori e
// argomenti posizionali escono 1, parsing ok esce -1 (si prosegue).
package cmd

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// LoginUsage stampa l'uso del sottocomando login.
func LoginUsage(out io.Writer) {
	fmt.Fprintln(out, "gmail-tui login — setup guidato nel terminale: wizard first-run")
	fmt.Fprintln(out, "istruzioni 2FA + password applicativa, verifica di connessione REALE,")
	fmt.Fprintln(out, "salvataggio della config e della password nel keychain di sistema.")
	fmt.Fprintln(out, "Senza argomenti avvia la TUI in modalità wizard; ctrl+c esce.")
	fmt.Fprintln(out, "Flag:")
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.PrintDefaults()
}

// ParseLoginArgs parsa gli argomenti di `gmail-tui login`: nessun flag in v1.
// Ritorna il codice d'uscita; -1 = parsing ok, si prosegue con la TUI wizard
// (il chiamante — il main — la lancia). Argomenti posizionali inattesi sono
// un errore, come nelle altre sottocomandi.
func ParseLoginArgs(args []string) int {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.Usage = func() { LoginUsage(fs.Output()) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	if extra := fs.Args(); len(extra) > 0 {
		fmt.Fprintf(fs.Output(), "login: argomenti inattesi: %s\n", strings.Join(extra, " "))
		fs.Usage()
		return 1
	}
	return -1
}
