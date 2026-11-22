package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/app"
	"github.com/andsigno82/gmail-tui/internal/cmd"
	"github.com/andsigno82/gmail-tui/internal/config"
	"github.com/andsigno82/gmail-tui/internal/keyring"
	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/mail/cache"
	"github.com/andsigno82/gmail-tui/internal/mail/imap"
	"github.com/andsigno82/gmail-tui/internal/mail/smtp"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/theme"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
	"github.com/andsigno82/gmail-tui/internal/ui/wizard"
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
			os.Exit(cmd.Doctor(os.Stdout, cfgPath, os.Getenv, doctorProbe(cfgPath)))
		case "fetch":
			os.Exit(runFetch(os.Args[2:]))
		case "send":
			os.Exit(runSend(os.Args[2:]))
		case "login":
			// Setup guidato (Piano 4, Task 5): la TUI in modalità wizard
			// forzata (il parsing dei flag precede ogni I/O).
			os.Exit(runLogin(os.Args[2:]))
		}
	}
	// Nessun argomento (o soli flag TUI come --theme): si avvia la TUI (Task
	// 9; wizard first-run quando serve — Piano 4, Task 5). Un primo argomento
	// sconosciuto (es. "foo") arriva qui e il FlagSet lo rigetta come
	// posizionale inatteso — l'errore resta, il testo nuovo.
	os.Exit(runTUI(os.Args[1:], false))
}

// doctorProbe costruisce il probe IMAP di `doctor` (Piano 7.1, Task 4): serve
// un indirizzo in config e una password (env → keychain, stesso ordine della
// TUI). Senza uno dei due, nil: il controllo IMAP è saltato. La password non
// viene mai stampata.
func doctorProbe(cfgPath string) cmd.CapsProbe {
	cfg, err := config.Load(cfgPath)
	if err != nil || cfg.Address == "" {
		return nil
	}
	krStore, _ := keyring.New()
	pw, _ := resolvePassword(os.Getenv, krStore, cfg.Address)
	if pw == "" {
		return nil
	}
	return func(ctx context.Context) ([]string, error) {
		return imap.Capabilities(ctx, imap.Options{
			Addr:     imapAddr,
			Username: cfg.Address,
			Password: pw,
		})
	}
}

// runLogin è il sottocomando `gmail-tui login` (Piano 4, Task 5): il parsing
// dei flag precede ogni I/O (--help esce 0 senza avviare la TUI), poi la TUI
// in modalità wizard FORZATA — indipendentemente da config/password presenti.
func runLogin(args []string) int {
	if code := cmd.ParseLoginArgs(args); code >= 0 {
		return code
	}
	return runTUI(nil, true)
}

// runTUI avvia la TUI (Task 9): config → tema (flag --theme, poi config,
// fallback default con warning) → risoluzione password (Piano 4 Task 5: env →
// keychain → assente, via keyring.Resolve) → wizard first-run se la config
// MANCA e nessuna password è risolvibile (o da `login`); altrimenti store +
// sender (IMAP/SMTP reali se c'è una password — preset Gmail; altrimenti Mock
// demo col seed di app.DemoStore() e il banner — store E sender un solo
// oggetto) → programma.
//
// Limitazione documentata (mandato Task 8): q/esc/ctrl+c dentro la TUI escono
// puliti cancellando il feed Watch; una SIGINT ESTERNA invece arriva al
// runtime come tea.InterruptMsg e NON passa dall'Update — in quel percorso il
// ctx del feed resta aperto fino all'uscita del processo (nessun effetto
// visibile: è la fine del programma).
func runTUI(args []string, forceWizard bool) int {
	themeFlag, code := parseTUIArgs(args)
	if code >= 0 {
		return code
	}
	cfg, ok := loadConfig()
	if !ok {
		// Config assente/rotta: la TUI parte comunque dai default (in wizard/
		// demo mode non serve proprio; con password l'errore vero uscirà in
		// status bar alla prima operazione).
		cfg = config.Default()
	}
	th, warn := loadTheme(themeFlag, cfg.Theme)

	// Risoluzione password (Piano 4, Task 5): env GMAIL_APP_PASSWORD →
	// keychain → assente. keyring.New nil-safe: senza keychain si risolve
	// SOLO env (il panico non esiste, il degrado sì).
	krStore, _ := keyring.New()
	pw, src := resolvePassword(os.Getenv, krStore, cfg.Address)

	// Config MANCANTE? (load non basta: Load su file assente torna Default
	// senza errore — il controllo lo distingue come cmd.Doctor).
	cfgPath, pathErr := config.Path()
	cfgExists := pathErr == nil && fileExists(cfgPath)

	if wizardNeeded(forceWizard, cfgExists, src) {
		return runWizardTUI(cfgPath, krStore, th)
	}

	store, sender, demo := selectStore(cfg, pw, openCache())
	m := app.New(store, sender, th)
	m.SetIdentity(cfg.Address, cfg.Signature) // composer r/f/c (Piano 4, Task 3)
	if demo {
		m.SetDemoBanner()
	}
	if warn != "" {
		// Il warning del tema è più specifico del banner demo: vince.
		m.SetStartupWarning(warn)
	}
	return runProgram(m)
}

// runProgram avvia il programma Bubble Tea col modello dato (condiviso dal
// percorso normale e dal wizard). In tea v2 mouse e altscreen sono dichiarati
// per-View dal Model (View() imposta MouseMode cell motion e AltScreen):
// nessuna opzione di programma serve; WithContext (Task 8) giudicato non
// necessario.
func runProgram(m tea.Model) int {
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tui: %v\n", err)
		return 1
	}
	return 0
}

// runWizardTUI avvia la TUI in modalità first-run (Piano 4, Task 5): il radice
// nasce col placeholder demo (DemoStore — mai interrogato finché il wizard è
// attivo) e col wizard attivo; al completamento il radice scambia store/sender
// (skip → demo col seed; successo → factory reale con le credenziali
// verificate) e avvia load/feed da sé (app.SetWizard).
func runWizardTUI(cfgPath string, krStore keyring.Store, th theme.Theme) int {
	mk := app.DemoStore()
	m := app.New(mk, mk, th)
	m.SetWizard(wizard.New(wizard.Deps{
		TestConnect: testConnection,
		Keyring:     krStore,
		CfgPath:     cfgPath,
	}, styles.New(th)), backendFactory)
	return runProgram(m)
}

// wizardNeeded decide la modalità first-run (Piano 4, Task 5): wizard SOLO se
// forzato (`gmail-tui login`) oppure se la config MANCA e nessuna password è
// risolvibile (source ""). Config presente senza password → demo mode col
// banner esistente (non wizard); config assente CON password → sessione reale.
func wizardNeeded(force, cfgExists bool, source string) bool {
	return force || (!cfgExists && source == "")
}

// resolvePassword applica l'ordine di risoluzione (Piano 4, Task 5) via
// keyring.Resolve: env GMAIL_APP_PASSWORD → keychain → assente. Nil-safe: kr
// nil (OS senza keychain, o keyring.New in errore) → SOLO env. Un errore del
// keychain diverso da NotFound degrada ad "assente": la sessione parte
// demo/wizard e l'errore vero uscirà alla prima operazione (nessun blocco
// all'avvio per un keychain indisponibile).
func resolvePassword(getenv func(string) string, kr keyring.Store, user string) (pw, source string) {
	if kr == nil {
		if pw := getenv("GMAIL_APP_PASSWORD"); pw != "" {
			return pw, "env"
		}
		return "", ""
	}
	pw, source, err := keyring.Resolve(getenv, kr, user)
	if err != nil {
		return "", ""
	}
	return pw, source
}

// fileExists è il controllo "config sul disco?" (come cmd.Doctor distingue il
// first-run): Load su file assente torna Default senza errore, non basta.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
// mandato Task 8/9 (aggiornate al Piano 4 Task 5: wizard al primo avvio e
// sottocomando login) e la limitazione SIGINT.
func tuiUsage(fs *flag.FlagSet) {
	out := fs.Output()
	fmt.Fprintln(out, "gmail-tui — client Gmail nel terminale (senza argomenti avvia la TUI)")
	fmt.Fprintln(out, "Senza credenziali: se la config manca parte il wizard first-run (o esegui \"gmail-tui login\"); se la config c'è parte in demo mode (store finto).")
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

// selectStore sceglie store e sender per la TUI (Task 9 + Piano 4 Task 4-5):
// IMAP/SMTP reali se la password applicativa è risolta (preset Gmail fisso —
// host imap non configurabile; la password arriva dalla risoluzione env →
// keychain, sopra), altrimenti il Mock demo CON SEED (app.DemoStore(), Piano 4
// Task 5: chi clona il repo vede l'app viva) che fa da store E sender (UN solo
// oggetto: include l'invio registrato in Mock.Sent).
// imap.NewStore e smtp.NewSender NON si connettono (la connessione avviene
// alla prima operazione): la costruzione non tocca la rete. La cache è passata
// dal chiamante (nil = senza cache): nei test non si tocca il disco utente.
func selectStore(cfg config.Config, password string, c *cache.Cache) (store mail.MailStore, sender app.Sender, demo bool) {
	if password == "" {
		mk := app.DemoStore()
		return mk, mk, true
	}
	store = imap.NewStore(imap.Options{
		Addr:     imapAddr,
		Username: cfg.Address,
		Password: password,
		Cache:    c,
	})
	sender = smtp.NewSender(smtp.Options{
		Host:     smtpHost,
		Port:     smtpPort,
		Username: cfg.Address,
		Password: password,
	})
	return store, sender, false
}

// testConnection è la verifica di connessione REALE del wizard (Piano 4,
// Task 5): preset Gmail (host fisso), TLS, Connect + ListMessages limit 1.
// Il wizard applica il budget di 30s al ctx prima di chiamarla (cfr.
// wizard.connectTimeout): nessun hang possibile oltre il budget.
func testConnection(ctx context.Context, address, password string) error {
	st := imap.NewStore(imap.Options{
		Addr:     imapAddr,
		Username: address,
		Password: password,
	})
	if err := st.Connect(ctx); err != nil {
		return err
	}
	_, err := st.ListMessages(ctx, msg.Query{}, mail.Page{Limit: 1})
	return err
}

// backendFactory costruisce store e sender REALI con le credenziali verificate
// dal wizard (preset Gmail); NESSUNA connessione qui (il dial avviene alla
// prima operazione — come selectStore). Soddisfa app.BackendFactory, passata
// al radice via SetWizard (Piano 4, Task 5).
func backendFactory(address, password string) (mail.MailStore, app.Sender) {
	store := imap.NewStore(imap.Options{
		Addr:     imapAddr,
		Username: address,
		Password: password,
		Cache:    openCache(),
	})
	sender := smtp.NewSender(smtp.Options{
		Host:     smtpHost,
		Port:     smtpPort,
		Username: address,
		Password: password,
	})
	return store, sender
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

// appPassword recupera la password applicativa per i sottocomandi CLI
// (nessuna rete) con lo STESSO ordine della TUI (Piano 4, Task 5): env
// GMAIL_APP_PASSWORD → keychain (keyring.Resolve) → assente. keyring.New
// nil-safe: senza keychain si risolve solo env.
func appPassword(cfg config.Config) (string, bool) {
	krStore, _ := keyring.New()
	pw, _ := resolvePassword(os.Getenv, krStore, cfg.Address)
	if pw == "" {
		fmt.Fprintln(os.Stderr, "password non trovata (env GMAIL_APP_PASSWORD e keychain) — esegui \"gmail-tui login\" o vedi README")
		return "", false
	}
	return pw, true
}

// newIMAPStore costruisce lo store IMAP con preset Gmail; cache bbolt da
// UserCacheDir (nil, cioè no cache, se non disponibile).
func newIMAPStore(cfg config.Config) (mail.MailStore, bool) {
	pw, ok := appPassword(cfg)
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
	pw, ok := appPassword(cfg)
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
