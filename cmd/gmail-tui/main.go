package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gmail-tui/internal/cmd"
	"gmail-tui/internal/config"
	"gmail-tui/internal/mail"
	"gmail-tui/internal/mail/cache"
	"gmail-tui/internal/mail/imap"
	"gmail-tui/internal/mail/smtp"
	"gmail-tui/internal/msg"
)

// version viene iniettata a build time: -ldflags "-X main.version=0.1.0"
var version = "dev"

// Preset Gmail fissi (spec): gli host non sono configurabili in v1.
const (
	imapAddr = "imap.gmail.com:993"
	smtpHost = "smtp.gmail.com"
	smtpPort = 587
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version":
			fmt.Println("gmail-tui " + version)
			return
		case "themes":
			cmd.Themes(os.Stdout)
			return
		case "doctor":
			cfgPath, err := config.Path()
			if err != nil {
				fmt.Fprintf(os.Stderr, "config path: %v\n", err)
				os.Exit(1)
			}
			os.Exit(cmd.Doctor(os.Stdout, cfgPath, os.Getenv))
		case "fetch":
			os.Exit(runFetch(os.Args[2:]))
		case "send":
			os.Exit(runSend(os.Args[2:]))
		}
	}
	fmt.Fprintln(os.Stderr, "comandi: version | themes | doctor | fetch | send")
	os.Exit(1)
}

// runFetch: gmail-tui fetch [--limit N] [--query "from:x is:unread"].
// Il parsing dei flag precede ogni I/O: --help stampa l'uso ed esce 0
// senza toccare la rete.
func runFetch(args []string) int {
	fs := flag.NewFlagSet("fetch", flag.ContinueOnError)
	limit := fs.Int("limit", 20, "numero massimo di messaggi")
	query := fs.String("query", "", `query stile Gmail, es. "from:x is:unread label:Work"`)
	if exitCode := parseFlags(fs, args); exitCode >= 0 {
		return exitCode
	}
	cfg, ok := loadConfig()
	if !ok {
		return 1
	}
	store, ok := newIMAPStore(cfg)
	if !ok {
		return 1
	}
	return cmd.Fetch(os.Stdout, store, msg.ParseQuery(*query), *limit)
}

// runSend: gmail-tui send --to a@b --subject s [--attach f]... < body.
// Il parsing dei flag precede ogni I/O: --help stampa l'uso ed esce 0
// senza toccare la rete.
func runSend(args []string) int {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	to := fs.String("to", "", "destinatari To, separati da virgola")
	cc := fs.String("cc", "", "destinatari Cc, separati da virgola")
	bcc := fs.String("bcc", "", "destinatari Bcc, separati da virgola")
	subject := fs.String("subject", "", "oggetto")
	var attach attachList
	fs.Var(&attach, "attach", "allegato (flag ripetibile)")
	if exitCode := parseFlags(fs, args); exitCode >= 0 {
		return exitCode
	}
	cfg, ok := loadConfig()
	if !ok {
		return 1
	}
	sender, ok := newSMTPSender(cfg)
	if !ok {
		return 1
	}
	sargs := cmd.SendArgs{To: *to, Cc: *cc, Bcc: *bcc, Subject: *subject, Attach: attach}
	return cmd.Send(os.Stdout, sender, cfg, sargs, os.Stdin)
}

// parseFlags centralizza il comportamento del FlagSet: --help/-h ⇒ uso +
// exit 0; errore di parsing ⇒ (l'uso è già stampato dal package flag) exit 1.
// Ritorna -1 se il parsing è andato a buon fine (si prosegue). Argomenti
// posizionali inattesi sono un errore.
func parseFlags(fs *flag.FlagSet, args []string) int {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	if extra := fs.Args(); len(extra) > 0 {
		fmt.Fprintf(fs.Output(), "%s: argomenti inattesi: %s\n", fs.Name(), strings.Join(extra, " "))
		fs.Usage()
		return 1
	}
	return -1
}

// loadConfig legge la config utente (nessuna rete).
func loadConfig() (config.Config, bool) {
	path, err := config.Path()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		return config.Config{}, false
	}
	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		return config.Config{}, false
	}
	return cfg, true
}

// appPassword recupera la password applicativa (nessuna rete).
func appPassword() (string, bool) {
	pw := os.Getenv("GMAIL_APP_PASSWORD")
	if pw == "" {
		fmt.Fprintln(os.Stderr, "GMAIL_APP_PASSWORD mancante — vedi README")
		return "", false
	}
	return pw, true
}

// newIMAPStore costruisce lo store IMAP con preset Gmail; cache bbolt da
// UserCacheDir (nil, cioè no cache, se non disponibile).
func newIMAPStore(cfg config.Config) (mail.MailStore, bool) {
	pw, ok := appPassword()
	if !ok {
		return nil, false
	}
	return imap.NewStore(imap.Options{
		Addr:     imapAddr,
		Username: cfg.Address,
		Password: pw,
		Cache:    openCache(),
	}), true
}

// newSMTPSender costruisce il sender SMTP con preset Gmail (587 STARTTLS).
func newSMTPSender(cfg config.Config) (*smtp.Sender, bool) {
	pw, ok := appPassword()
	if !ok {
		return nil, false
	}
	return smtp.NewSender(smtp.Options{
		Host:     smtpHost,
		Port:     smtpPort,
		Username: cfg.Address,
		Password: pw,
	}), true
}

// openCache apre la cache bbolt in UserCacheDir/gmail-tui/cache.db;
// qualunque problema ⇒ nil (si prosegue senza cache).
func openCache() *cache.Cache {
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil
	}
	path := filepath.Join(dir, "gmail-tui", "cache.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil
	}
	c, err := cache.Open(path)
	if err != nil {
		return nil
	}
	return c
}

// attachList è il flag ripetibile --attach (un file per occorrenza).
type attachList []string

func (a *attachList) String() string { return strings.Join(*a, ",") }

func (a *attachList) Set(v string) error {
	*a = append(*a, v)
	return nil
}
