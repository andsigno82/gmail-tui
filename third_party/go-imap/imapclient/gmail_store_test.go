package imapclient_test

// Piano 8, Task 4 (patch Gmail X-GM-EXT-1): STORE X-GM-LABELS con +, - e set.

import (
	"testing"

	"github.com/emersion/go-imap/v2"
)

func TestStoreGmailLabelsAdd(t *testing.T) {
	client, cmds := newScriptedClient(t, func(string) []string { return nil })
	if _, err := client.StoreGmailLabels(imap.UIDSetNum(5), &imap.StoreGmailLabels{
		Op:     imap.StoreFlagsAdd,
		Labels: []string{"foo", "Muy Importante"},
	}).Collect(); err != nil {
		t.Fatalf("StoreGmailLabels: %v", err)
	}
	want := `UID STORE 5 +X-GM-LABELS ("foo" "Muy Importante")`
	if got := wireRest(<-cmds); got != want {
		t.Errorf("wire = %q, want %q", got, want)
	}
}

func TestStoreGmailLabelsRemoveSilent(t *testing.T) {
	client, cmds := newScriptedClient(t, func(string) []string { return nil })
	if _, err := client.StoreGmailLabels(imap.UIDSetNum(5), &imap.StoreGmailLabels{
		Op:     imap.StoreFlagsDel,
		Silent: true,
		Labels: []string{`\Inbox`},
	}).Collect(); err != nil {
		t.Fatalf("StoreGmailLabels: %v", err)
	}
	want := `UID STORE 5 -X-GM-LABELS.SILENT ("\\Inbox")`
	if got := wireRest(<-cmds); got != want {
		t.Errorf("wire = %q, want %q", got, want)
	}
}

func TestStoreGmailLabelsSetEmpty(t *testing.T) {
	client, cmds := newScriptedClient(t, func(string) []string { return nil })
	if _, err := client.StoreGmailLabels(imap.UIDSetNum(5), &imap.StoreGmailLabels{
		Op: imap.StoreFlagsSet,
	}).Collect(); err != nil {
		t.Fatalf("StoreGmailLabels: %v", err)
	}
	want := `UID STORE 5 X-GM-LABELS ()`
	if got := wireRest(<-cmds); got != want {
		t.Errorf("wire = %q, want %q", got, want)
	}
}
