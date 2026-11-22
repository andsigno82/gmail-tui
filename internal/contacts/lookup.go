package contacts

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/mail"
	"os/exec"
	"strings"
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
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("comando contatti: %w: %s", err, msg)
		}
		return nil, fmt.Errorf("comando contatti: %w", err)
	}
	return parseOutput(stdout.String()), nil
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
		a, err := mail.ParseAddress(strings.TrimSpace(addr))
		if err != nil || a.Address != strings.TrimSpace(addr) {
			continue
		}
		res = append(res, Suggestion{Addr: a.Address, Name: strings.TrimSpace(name)})
	}
	return res
}
