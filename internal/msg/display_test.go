package msg

import (
	"strings"
	"testing"
)

// Fix review (ripresa bozze): Parse conserva la forma "Nome <indirizzo>" dei
// destinatari in ToDisplay/CcDisplay/BccDisplay; To/Cc/Bcc restano indirizzi nudi.
func TestParseKeepsRecipientNames(t *testing.T) {
	raw := "From: me@x.dev\r\nTo: Anna Rossi <anna@x.dev>, bob@y.dev\r\nCc: \"Rossi, Carla\" <carla@z.dev>\r\nBcc: Dario <dario@w.dev>\r\nSubject: s\r\n\r\nciao\r\n"
	m, err := Parse(NewID("[Gmail]/Drafts", 1), strings.NewReader(raw))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := strings.Join(m.ToDisplay, " | "); got != "Anna Rossi <anna@x.dev> | bob@y.dev" {
		t.Errorf("ToDisplay = %q", got)
	}
	if got := strings.Join(m.To, ","); got != "anna@x.dev,bob@y.dev" {
		t.Errorf("To = %q, voglio gli indirizzi nudi", got)
	}
	if len(m.CcDisplay) != 1 || !strings.Contains(m.CcDisplay[0], "<carla@z.dev>") || !strings.Contains(m.CcDisplay[0], "Rossi, Carla") {
		t.Errorf("CcDisplay = %v, voglio il nome quotato con la virgola", m.CcDisplay)
	}
	if len(m.BccDisplay) != 1 || m.BccDisplay[0] != "Dario <dario@w.dev>" {
		t.Errorf("BccDisplay = %v", m.BccDisplay)
	}
}
