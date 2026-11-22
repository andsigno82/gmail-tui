package cmd

import "testing"

// TestParseLoginArgs (Piano 4, Task 5): il parsing dei flag di `gmail-tui
// login` precede ogni I/O — nessun flag in v1; --help/-h escono 0 senza
// lanciare la TUI; errori di parsing e argomenti posizionali escono 1;
// parsing ok esce -1 (si prosegue con la TUI in modalità wizard forzata).
func TestParseLoginArgs(t *testing.T) {
	if code := ParseLoginArgs(nil); code != -1 {
		t.Errorf("nessun argomento = %d, voglio -1 (si prosegue)", code)
	}
	if code := ParseLoginArgs([]string{"--help"}); code != 0 {
		t.Errorf("--help = %d, voglio 0", code)
	}
	if code := ParseLoginArgs([]string{"-h"}); code != 0 {
		t.Errorf("-h = %d, voglio 0", code)
	}
	if code := ParseLoginArgs([]string{"foo"}); code != 1 {
		t.Errorf("posizionale sconosciuto = %d, voglio 1", code)
	}
	if code := ParseLoginArgs([]string{"--bogus"}); code != 1 {
		t.Errorf("flag sconosciuto = %d, voglio 1", code)
	}
}
