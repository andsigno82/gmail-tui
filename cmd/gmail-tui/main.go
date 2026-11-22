package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"gmail-tui/internal/app"
	"gmail-tui/internal/cmd"
	"gmail-tui/internal/config"
	"gmail-tui/internal/mail"
	"gmail-tui/internal/mail/cache"
	"gmail-tui/internal/mail/imap"
	"gmail-tui/internal/mail/smtp"
	"gmail-tui/internal/msg"
	"gmail-tui/internal/theme"
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
	// Nessun argomento (o soli flag TUI come --theme): si avvia la TUI (Task
	// 9). Un primo argomento sconosciuto (es. "foo") arriva qui e il FlagSet
	// lo rigetta come posizionale inatteso — l'errore resta, il testo nuovo.
	os.Exit(runTUI(os.Args[1:]))
}

// runTUI avvia la TUI (Task 9): config → tema (flag --theme, poi config,
// fallback default con warning) → store + sender (IMAP/SMTP reali se c'è
// GMAIL_APP_PASSWORD — preset Gmail; altrimenti Mock in demo mode col banner,
// che fa da store E sender — un solo oggetto, Piano 4 Task 4) → programma.
//
// Limitazione documentata (mandato Task 8): q/esc/ctrl+c dentro la TUI escono
// puliti cancellando il feed Watch; una SIGINT ESTERNA invece arriva al
// runtime come tea.InterruptMsg e NON passa dall'Update — in quel percorso il
// ctx del feed resta aperto fino all'uscita del processo (nessun effetto
// visibile: è la fine del programma).
func runTUI(args []string) int {
	themeFlag, code := parseTUIArgs(args)
	if code >= 0 {
		return code
	}
	cfg, ok := loadConfig()
	if !ok {
		// Config assente/rotta: la TUI parte comunque dai default (in demo
		// mode non serve proprio; con password l'errore vero uscirà in
		// status bar alla prima operazione).
		cfg = config.Default()
	}
	th, warn := loadTheme(themeFlag, cfg.Theme)
	store, sender, demo := selectStore(cfg, os.Getenv, openCache())
	m := app.New(store, sender, th)
	m.SetIdentity(cfg.Address, cfg.Signature) // composer r/f/c (Piano 4, Task 3)
	if demo {
		m.SetDemoBanner()
	}
	if warn != "" {
		// Il warning del tema è più specifico del banner demo: vince.
		m.SetStartupWarning(warn)
	}
	p := tea.NewProgram(m)
	// In tea v2 mouse e altscreen sono dichiarati per-View dal Model (View()
	// imposta MouseMode cell motion e AltScreen): nessuna opzione di programma
	// serve; WithContext (Task 8) giudicato non necessario.
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tui: %v\n", err)
		return 1
	}
	return 0
}

// parseTUIArgs parsa i flag della TUI (--theme; --help/-h ⇒ uso + exit 0).
// Ritorna (nome tema, codice uscita); codice -1 = parsing ok, si prosegue.
// Argomenti posizionali (comandi sconosciuti) sono un errore: si esce 1
// dall'uso — nessun alias "tui" esplicito (per il piano: no-args = TUI).
func parseTUIArgs(args []string) (string, int) {
	fs := flag.NewFlagSet("gmail-tui", flag.ContinueOnError)
	fs.Usage = func() { tuiUsage(fs) }
	themeFlag := fs.String("theme", "", "tema della TUI (default da config; vedi `gmail-tui themes`)")
	if exitCode := parseFlags(fs, args); exitCode >= 0 {
		return "", exitCode
	}
	return *themeFlag, -1
}

// tuiUsage stampa l'uso della TUI. Comprende le due righe richieste dal
// mandato Task 8/9: la demo mode senza credenziali e la limitazione SIGINT.
func tuiUsage(fs *flag.FlagSet) {
	out := fs.Output()
	fmt.Fprintln(out, "gmail-tui — client Gmail nel terminale (senza argomenti avvia la TUI)")
	fmt.Fprintln(out, "Senza GMAIL_APP_PASSWORD la TUI parte in demo mode (store finto).")
	fmt.Fprintln(out, "Nota: q/ctrl+c escono puliti; una SIGINT esterna esce senza cancellare il feed Watch (nessun effetto visibile).")
	fmt.Fprintln(out, "Flag:")
	fs.PrintDefaults()
}

// loadTheme risolve il tema per la TUI (Task 9): il flag --theme vince sul
// tema della config; nome assente → "default"; nome inesistente → fallback
// default CON warning (il main lo mostra nello status iniziale della TUI).
func loadTheme(flagTheme, cfgTheme string) (theme.Theme, string) {
	name := flagTheme
	if name == "" {
		name = cfgTheme
	}
	if name == "" {
		name = "default"
	}
	th, err := theme.Load(name)
	if err == nil {
		return th, ""
	}
	// Irraggiungibile con i temi embedded validati, ma difensivo: senza tema
	// gli slot rendono testo senza colore.
	def, _ := theme.Load("default")
	return def, fmt.Sprintf("tema %q non disponibile — uso default", name)
}

// selectStore sceglie store e sender per la TUI (Task 9 + Piano 4 Task 4):
// IMAP/SMTP reali se la password applicativa è presente (preset Gmail fisso —
// host imap non configurabile; password via getenv, il keyring arriva in
// Task 5), altrimenti Mock in demo mode che fa da store E sender (UN solo
// oggetto: chi clona il repo vede l'app, incluso l'invio in Mock.Sent).
// imap.NewStore e smtp.NewSender NON si connettono (la connessione avviene
// alla prima operazione): la costruzione non tocca la rete. La cache è passata
// dal chiamante (nil = senza cache): nei test non si tocca il disco utente.
func selectStore(cfg config.Config, getenv func(string) string, c *cache.Cache) (store mail.MailStore, sender app.Sender, demo bool) {
	if getenv("GMAIL_APP_PASSWORD") == "" {
		mk := &mail.Mock{}
		return mk, mk, true
	}
	store = imap.NewStore(imap.Options{
		Addr:     imapAddr,
		Username: cfg.Address,
		Password: getenv("GMAIL_APP_PASSWORD"),
		Cache:    c,
	})
	sender = smtp.NewSender(smtp.Options{
		Host:     smtpHost,
		Port:     smtpPort,
		Username: cfg.Address,
		Password: getenv("GMAIL_APP_PASSWORD"),
	})
	return store, sender, false
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
