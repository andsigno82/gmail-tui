package mcpserver

import (
	"context"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/mail"
)

// Piano 14, fix review (2): la scadenza del contesto del chiamante non è un errore
// di trasporto: la connessione viva resta, nessuna Connect in più.
func TestContextErrorKeepsConnection(t *testing.T) {
	cr := &countingReader{Reader: &mail.Mock{}}
	cs := connect(t, cr, Options{Version: "test"})
	call(t, cs, "list_folders", nil)

	cr.opErr = context.DeadlineExceeded
	if res := call(t, cs, "list_folders", nil); !res.IsError {
		t.Fatal("con scadenza atteso IsError")
	}
	cr.opErr = nil
	call(t, cs, "list_folders", nil)
	if cr.connects != 1 {
		t.Fatalf("Connect chiamate = %d, attese 1: una scadenza di contesto non è trasporto", cr.connects)
	}
}
