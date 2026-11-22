// Notifiche desktop (Piano 11): alla posta nuova in INBOX la TUI esegue il
// comando `notify_command` della config. Il comando NON passa mai da una shell:
// argv[0] è l'eseguibile, il resto sono argomenti fissi. Mittente, oggetto e
// conteggio arrivano SOLO come variabili d'ambiente GMAIL_TUI_* (Piano 11, D11-4):
// il contenuto delle email non è fidato e non entra mai in argv.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

const (
	// notifyTimeout è il budget di un comando di notifica: allo scadere il
	// processo viene ucciso (Piano 11, D11-5).
	notifyTimeout = 5 * time.Second
	// notifyFromMax e notifySubjectMax limitano le variabili d'ambiente (rune,
	// ellissi inclusa).
	notifyFromMax    = 100
	notifySubjectMax = 200
	// notifyStderrMax limita lo stderr raccolto per il messaggio d'errore.
	notifyStderrMax = 512
)

// sanitizeNotifyText rende sicuro per una notifica un testo non fidato (Piano 11,
// D11-4): rimuove sequenze ANSI/OSC, converte i blank in spazio, scarta i
// caratteri di controllo e di formato (bidi), compatta gli spazi e tronca a max
// rune contando l'ellissi.
func sanitizeNotifyText(s string, max int) string {
	r := []rune(s)
	var b strings.Builder
	for i := 0; i < len(r); {
		c := r[i]
		if c == 0x1b { // ESC: salta la sequenza, senza emetterne il contenuto
			i = skipEscape(r, i)
			continue
		}
		if c == 0x9b { // CSI C1 (Piano 11, review punto 9): come ESC [
			i = skipCSI(r, i+1)
			continue
		}
		switch {
		case c == '\t' || c == '\r' || c == '\n':
			b.WriteRune(' ')
		case unicode.IsControl(c) || unicode.Is(unicode.Cf, c):
			// scartato
		default:
			b.WriteRune(c)
		}
		i++
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	if max <= 0 {
		return ""
	}
	rs := []rune(out)
	if len(rs) <= max {
		return out
	}
	return string(rs[:max-1]) + "…"
}

// skipEscape restituisce l'indice del primo carattere dopo la sequenza ESC che
// inizia in r[i]: CSI (ESC [ … byte finale), OSC (ESC ] … BEL o ESC \\), o ESC
// seguito da un solo carattere.
func skipEscape(r []rune, i int) int {
	if i+1 >= len(r) {
		return len(r)
	}
	switch r[i+1] {
	case '[':
		return skipCSI(r, i+2)
	case ']':
		for j := i + 2; j < len(r); j++ {
			if r[j] == 0x07 {
				return j + 1
			}
			if r[j] == 0x1b && j+1 < len(r) && r[j+1] == '\\' {
				return j + 2
			}
		}
		return len(r)
	default:
		return i + 2
	}
}

// skipCSI restituisce l'indice dopo il byte finale di una CSI il cui corpo inizia
// in r[j] (parametri 0x30-0x3f e intermedi 0x20-0x2f, finale 0x40-0x7e).
func skipCSI(r []rune, j int) int {
	for j < len(r) && (r[j] < 0x40 || r[j] > 0x7e) {
		j++
	}
	return j + 1
}

// notifyEnvBase restituisce os.Environ senza GMAIL_APP_PASSWORD (la password non
// esce mai dalla TUI verso un comando esterno) e senza le GMAIL_TUI_* preesistenti,
// che non devono mescolarsi con quelle del messaggio (Piano 11, review punto 1).
func notifyEnvBase() []string {
	env := os.Environ()
	out := make([]string, 0, len(env))
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if k == "GMAIL_APP_PASSWORD" || strings.HasPrefix(k, "GMAIL_TUI_") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// notifyEnv costruisce l'ambiente del comando (Piano 11, D11-4): FROM e SUBJECT
// del messaggio più recente (refs[0], come da Change.Refs), COUNT del numero di
// messaggi dell'evento, FOLDER fisso. Le voci sono in coda a os.Environ: exec
// tiene l'ultima occorrenza, quindi valgono anche se l'ambiente ne ha altre.
func notifyEnv(refs []msg.MessageRef, count int) []string {
	from, subject := "", ""
	if len(refs) > 0 {
		from = sanitizeNotifyText(refs[0].From, notifyFromMax)
		subject = sanitizeNotifyText(refs[0].Subject, notifySubjectMax)
	}
	return append(notifyEnvBase(),
		"GMAIL_TUI_FROM="+from,
		"GMAIL_TUI_SUBJECT="+subject,
		"GMAIL_TUI_COUNT="+strconv.Itoa(count),
		"GMAIL_TUI_FOLDER=INBOX",
	)
}

// runNotify esegue argv (senza shell) con il budget notifyTimeout.
func runNotify(ctx context.Context, argv, env []string) error {
	return runNotifyTimeout(ctx, argv, env, notifyTimeout)
}

// runNotifyTimeout esegue argv con env e budget d. Errore di avvio, exit diverso
// da 0 o timeout: errore con l'eseguibile e lo stderr sanitizzato. Stdin chiuso,
// stdout scartato; WaitDelay evita di restare appesi a figli che tengono la pipe.
func runNotifyTimeout(ctx context.Context, argv, env []string, d time.Duration) error {
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return errors.New("comando di notifica vuoto")
	}
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = env
	setNotifyProcessGroup(cmd) // Piano 11, review punto 4: il timeout uccide il gruppo
	var stderr capBuffer
	cmd.Stderr = &stderr
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("%s: timeout dopo %v", argv[0], d)
		}
		if msg := sanitizeNotifyText(stderr.String(), notifyStderrMax); msg != "" {
			return fmt.Errorf("%s: %w: %s", argv[0], err, msg)
		}
		return fmt.Errorf("%s: %w", argv[0], err)
	}
	return nil
}

// capBuffer raccoglie al massimo notifyStderrMax byte (lo stderr di un comando
// utente non è limitato, il messaggio d'errore sì).
type capBuffer struct{ buf []byte }

func (c *capBuffer) Write(p []byte) (int, error) {
	if room := notifyStderrMax - len(c.buf); room > 0 {
		c.buf = append(c.buf, p[:min(room, len(p))]...)
	}
	return len(p), nil
}

func (c *capBuffer) String() string { return string(c.buf) }

// notifyRunner esegue il comando di notifica (seam: runNotify in produzione, un
// fake nei test del Model). Piano 11, Task 4.
type notifyRunner func(ctx context.Context, argv, env []string) error

// notifyResult è l'esito del comando di notifica (Piano 11, Task 4).
type notifyResult struct{ err error }

// SetNotify imposta il comando delle notifiche dalla config (Piano 11, Task 4):
// lista di argomenti copiata; lista vuota = notifiche disattivate.
func (m *Model) SetNotify(argv []string) {
	m.notifyArgv = slices.Clone(argv)
}

// notifyCmd prepara il comando di notifica per un ChangeNewMail (Piano 11, Task 4):
// nil se il comando non è configurato, il primo load non è ancora avvenuto o
// l'evento non ha ID. Il comando gira in un tea.Cmd (fuori dal loop UI) e torna
// come notifyResult. Un solo processo per evento: COUNT è il numero di ID dell'evento.
func (m Model) notifyCmd(c mail.Change) tea.Cmd {
	if len(m.notifyArgv) == 0 || !m.notifyArmed || len(c.IDs) == 0 {
		return nil
	}
	run := m.notifyRun
	if run == nil {
		run = runNotify
	}
	argv := slices.Clone(m.notifyArgv)
	env := notifyEnv(c.Refs, len(c.IDs))
	ctx := m.ctx
	return func() tea.Msg { return notifyResult{err: run(ctx, argv, env)} }
}
