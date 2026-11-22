package contacts

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/andsigno82/gmail-tui/internal/childenv"
)

// Comando esterno dei contatti (Piano 9, Task 10, D9-G): config
// address_book_cmd = ["khard", "email", "--parsable", "%s"]. È un array eseguito
// direttamente, MAI via shell: la query è un argomento intatto. Ogni %s negli
// argomenti è sostituito dalla query; senza %s la query è l'ultimo argomento.
// Output: una riga per contatto "indirizzo<TAB>nome" (nome opzionale).

// maxLookupLines è il tetto di righe lette dall'output del comando.
const maxLookupLines = 50

// Lookup esegue il comando per la query e ne parsa i contatti. Il timeout è del
// chiamante (ctx): allo scadere il processo viene ucciso. Un exit diverso da zero
// è un errore che porta lo stderr del comando.
func Lookup(ctx context.Context, argv []string, query string) ([]Suggestion, error) {
	if len(argv) == 0 {
		return nil, errors.New("comando contatti: nessun comando configurato")
	}
	args := make([]string, 0, len(argv))
	placeholder := false
	for _, a := range argv[1:] {
		if strings.Contains(a, "%s") {
			placeholder = true
			a = strings.ReplaceAll(a, "%s", query)
		}
		args = append(args, a)
	}
	if !placeholder {
		args = append(args, query)
	}
	cmd := exec.CommandContext(ctx, argv[0], args...)
	cmd.Env = childenv.Filtered() // Piano 9, review B: niente password App nell'ambiente del figlio
	setProcessGroup(cmd)          // Piano 9, review C: il timeout uccide anche i discendenti
	cmd.Cancel = func() error { return killProcessGroup(cmd) }
	cmd.WaitDelay = waitDelay
	var stderr bytes.Buffer
	stdout := &capWriter{max: maxLookupBytes}
	cmd.Stdout = stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("comando contatti: %w: %s", err, msg)
		}
		return nil, fmt.Errorf("comando contatti: %w", err)
	}
	return parseOutput(stdout.String()), nil
}

// maxLookupBytes è il tetto dell'output letto (Piano 9, review C): oltre, il
// resto viene scartato senza crescere in memoria.
const maxLookupBytes = 64 << 10

// waitDelay limita l'attesa dei pipe dopo la morte del processo (un nipote può
// tenere lo stdout aperto).
const waitDelay = 2 * time.Second

// capWriter conserva al più max byte e scarta il resto (il figlio non riceve errori).
type capWriter struct {
	buf bytes.Buffer
	max int
}

func (c *capWriter) Write(p []byte) (int, error) {
	if room := c.max - c.buf.Len(); room > 0 {
		if len(p) <= room {
			c.buf.Write(p)
		} else {
			c.buf.Write(p[:room])
		}
	}
	return len(p), nil
}

func (c *capWriter) String() string { return c.buf.String() }

var _ io.Writer = (*capWriter)(nil)

// ansiSeq è una sequenza di escape (CSI o ESC singolo) da rimuovere dai nomi.
var ansiSeq = regexp.MustCompile("\x1b\\[[0-?]*[ -/]*[@-~]|\x1b.?")

// sanitizeText toglie controlli C0/C1, DEL, sequenze ESC e override bidi dal
// testo esterno (Piano 9, review C): un nome non può falsare la lista.
func sanitizeText(s string) string {
	s = ansiSeq.ReplaceAllString(s, "")
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || isBidi(r) {
			return -1
		}
		return r
	}, s)
}

// isBidi riconosce i caratteri di controllo bidirezionali (override e isolate).
func isBidi(r rune) bool {
	return (r >= 0x200e && r <= 0x200f) || (r >= 0x202a && r <= 0x202e) ||
		(r >= 0x2066 && r <= 0x2069) || r == 0x061c
}

// parseOutput legge "indirizzo[TAB nome]" per riga: le righe senza un indirizzo
// valido sono scartate; al più maxLookupLines righe vengono considerate.
func parseOutput(out string) []Suggestion {
	var res []Suggestion
	for i, line := range strings.Split(out, "\n") {
		if i >= maxLookupLines {
			break
		}
		line = strings.TrimRight(line, "\r")
		addr, name, _ := strings.Cut(line, "\t")
		if sanitizeText(addr) != addr {
			continue // indirizzo con controlli: scartato (Piano 9, review C)
		}
		a, err := mail.ParseAddress(strings.TrimSpace(addr))
		if err != nil || a.Address != strings.TrimSpace(addr) {
			continue
		}
		res = append(res, Suggestion{Addr: a.Address, Name: strings.TrimSpace(sanitizeText(name))})
	}
	return res
}
