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
	want := `UID STORE 5 -X-GM-LABELS.SILENT (\Inbox)`
	if got := wireRest(<-cmds); got != want {
		t.Errorf("wire = %q, want %q", got, want)
	}
}

// Piano 10, Task 1 (critico): le label di sistema (\Inbox, \Important, \Starred)
// vanno come atomi NUDI. Quotate ("\\Inbox") Gmail crea una label utente di nome
// "\Inbox" invece di ripristinare la Posta in arrivo.
func TestStoreGmailLabelsSystemAtomsUnquoted(t *testing.T) {
	client, cmds := newScriptedClient(t, func(string) []string { return nil })
	if _, err := client.StoreGmailLabels(imap.UIDSetNum(5), &imap.StoreGmailLabels{
		Op:     imap.StoreFlagsAdd,
		Labels: []string{`\Inbox`, `\Important`, `\Starred`},
	}).Collect(); err != nil {
		t.Fatalf("StoreGmailLabels: %v", err)
	}
	want := `UID STORE 5 +X-GM-LABELS (\Inbox \Important \Starred)`
	if got := wireRest(<-cmds); got != want {
		t.Errorf("wire = %q, want %q", got, want)
	}
}

// Sistema e utente nella stessa lista: l'atomo non tocca le label utente,
// che restano quotate, con spazi.
func TestStoreGmailLabelsMixedSystemAndUser(t *testing.T) {
	client, cmds := newScriptedClient(t, func(string) []string { return nil })
	if _, err := client.StoreGmailLabels(imap.UIDSetNum(5), &imap.StoreGmailLabels{
		Op:     imap.StoreFlagsDel,
		Labels: []string{`\Inbox`, "Muy Importante"},
	}).Collect(); err != nil {
		t.Fatalf("StoreGmailLabels: %v", err)
	}
	want := `UID STORE 5 -X-GM-LABELS (\Inbox "Muy Importante")`
	if got := wireRest(<-cmds); got != want {
		t.Errorf("wire = %q, want %q", got, want)
	}
}

// Label non ASCII: il backend le passa gia' in modified UTF-7 (ASCII), quindi
// sul filo arrivano quotate e senza byte fuori ASCII.
func TestStoreGmailLabelsNonASCIIEncoded(t *testing.T) {
	client, cmds := newScriptedClient(t, func(string) []string { return nil })
	if _, err := client.StoreGmailLabels(imap.UIDSetNum(5), &imap.StoreGmailLabels{
		Op:     imap.StoreFlagsAdd,
		Labels: []string{"Perch&AOk-"},
	}).Collect(); err != nil {
		t.Fatalf("StoreGmailLabels: %v", err)
	}
	want := `UID STORE 5 +X-GM-LABELS ("Perch&AOk-")`
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
