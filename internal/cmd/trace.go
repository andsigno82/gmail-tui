package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/andsigno82/gmail-tui/internal/trace"
)

// Trace e il sottocomando `gmail-tui trace <file>` (Piano 16): legge una traccia
// dei tempi (GMAIL_TUI_TRACE=1) e ne stampa il riepilogo su out, pronto da
// incollare in chat. Errori su errw, exit 1; 0 se ok.
func Trace(out, errw io.Writer, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(errw, "uso: gmail-tui trace <file.jsonl>")
		return 1
	}
	f, err := os.Open(args[0])
	if err != nil {
		fmt.Fprintf(errw, "trace: %v\n", err)
		return 1
	}
	defer func() { _ = f.Close() }()
	if err := trace.Summarize(f, out); err != nil {
		fmt.Fprintf(errw, "trace: %v\n", err)
		return 1
	}
	return 0
}
