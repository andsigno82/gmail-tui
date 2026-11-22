package inbox

import (
	"strings"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Piano 10, Task 2: la casella ☑/☐ a inizio riga segue la selezione per ID della
// conversazione (D10-T1): ☑ sulla riga selezionata, ☐ sulle altre.
func TestRowRendersCheckbox(t *testing.T) {
	m := newTestModel(t, testRefs())
	m.SetSelection(map[msg.MessageID]bool{msg.NewID("INBOX", 2): true})
	r := rows(m)
	if len(r) < 3 {
		t.Fatalf("righe = %d, voglio almeno 3", len(r))
	}
	if !strings.Contains(r[1], "☑") || strings.Contains(r[1], "☐") {
		t.Errorf("riga selezionata = %q, voglio ☑", r[1])
	}
	if !strings.Contains(r[0], "☐") || strings.Contains(r[0], "☑") {
		t.Errorf("riga non selezionata = %q, voglio ☐", r[0])
	}
}
