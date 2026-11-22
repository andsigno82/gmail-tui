package imapclient_test

// Piano 8, Task 3 (patch Gmail X-GM-EXT-1): criteri SEARCH X-GM-RAW,
// X-GM-MSGID, X-GM-THRID, X-GM-LABELS. Verifica il wire del comando.

import (
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
)

// wireRest restituisce il comando senza il tag iniziale.
func wireRest(line string) string {
	_, rest, _ := strings.Cut(line, " ")
	return rest
}

func TestUIDSearchGmailRawVerbatim(t *testing.T) {
	client, cmds := newScriptedClient(t, func(string) []string {
		return []string{"* SEARCH 3 5"}
	})
	data, err := client.UIDSearch(&imap.SearchCriteria{
		Gmail: &imap.SearchCriteriaGmail{Raw: `subject:"a\b" older_than:7d`},
	}, nil).Wait()
	if err != nil {
		t.Fatalf("UIDSearch: %v", err)
	}
	if uids := data.AllUIDs(); len(uids) != 2 || uids[0] != 3 || uids[1] != 5 {
		t.Errorf("AllUIDs = %v, want [3 5]", uids)
	}
	// Raw contiene " e \: sul wire sono escapati come quoted-string
	want := `UID SEARCH X-GM-RAW "subject:\"a\\b\" older_than:7d"`
	if got := wireRest(<-cmds); got != want {
		t.Errorf("wire = %q, want %q", got, want)
	}
}

func TestUIDSearchGmailIDsAndLabels(t *testing.T) {
	client, cmds := newScriptedClient(t, func(string) []string {
		return []string{"* SEARCH"}
	})
	_, err := client.UIDSearch(&imap.SearchCriteria{
		Header: []imap.SearchCriteriaHeaderField{{Key: "From", Value: "x"}},
		Gmail: &imap.SearchCriteriaGmail{
			MsgID:    1278455344230334865,
			ThreadID: 1266894439832287888,
			Labels:   []string{`\Inbox`, "Muy Importante"},
		},
	}, nil).Wait()
	if err != nil {
		t.Fatalf("UIDSearch: %v", err)
	}
	want := `UID SEARCH FROM "x" X-GM-MSGID 1278455344230334865 X-GM-THRID 1266894439832287888 X-GM-LABELS "\\Inbox" X-GM-LABELS "Muy Importante"`
	if got := wireRest(<-cmds); got != want {
		t.Errorf("wire = %q, want %q", got, want)
	}
}

func TestUIDSearchGmailNonASCIIAddsCharset(t *testing.T) {
	client, cmds := newScriptedClient(t, func(string) []string {
		return []string{"* SEARCH"}
	})
	if _, err := client.UIDSearch(&imap.SearchCriteria{
		Gmail: &imap.SearchCriteriaGmail{Raw: "from:José"},
	}, nil).Wait(); err != nil {
		t.Fatalf("UIDSearch: %v", err)
	}
	if got := wireRest(<-cmds); !strings.HasPrefix(got, "UID SEARCH CHARSET UTF-8 X-GM-RAW ") {
		t.Errorf("wire = %q: con Raw non ASCII serve CHARSET UTF-8", got)
	}
}
