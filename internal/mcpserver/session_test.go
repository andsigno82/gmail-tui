package mcpserver

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/andsigno82/gmail-tui/internal/mail"
)

// countingReader conta le Connect e può far fallire le operazioni di lista.
type countingReader struct {
	Reader
	connects int
	failOps  bool
}

func (c *countingReader) Connect(ctx context.Context) error {
	c.connects++
	return c.Reader.Connect(ctx)
}

func (c *countingReader) ListMailboxes(ctx context.Context) ([]mail.Mailbox, error) {
	if c.failOps {
		return nil, errors.New("rete caduta")
	}
	return c.Reader.ListMailboxes(ctx)
}

// Piano 14, fix review 1: dopo un errore dello store, la chiamata successiva
// riconnette (Connect chiude il client precedente).
func TestReconnectsAfterStoreError(t *testing.T) {
	cr := &countingReader{Reader: &mail.Mock{}}
	cs := connect(t, cr, Options{Version: "test"})

	if res := call(t, cs, "list_folders", nil); res.IsError {
		t.Fatalf("prima chiamata: %s", resultText(res))
	}
	cr.failOps = true
	if res := call(t, cs, "list_folders", nil); !res.IsError {
		t.Fatal("con operazione in errore atteso IsError")
	}
	cr.failOps = false
	if res := call(t, cs, "list_folders", nil); res.IsError {
		t.Fatalf("dopo il ripristino la chiamata deve riuscire: %s", resultText(res))
	}
	if cr.connects != 2 {
		t.Fatalf("Connect chiamate = %d, attese 2 (una per sessione)", cr.connects)
	}
}

// blockingReader non ritorna da Connect finché block non è chiuso: lo store
// ignora il contesto, come l'imap.Store attuale.
type blockingReader struct {
	Reader
	block chan struct{}
}

func (b *blockingReader) Connect(context.Context) error {
	<-b.block
	return nil
}

// Piano 14, fix review 5: il budget di connessione vale anche se lo store
// ignora il contesto; il client riceve IsError entro il budget.
func TestConnectBudgetHonouredWhenStoreIgnoresContext(t *testing.T) {
	br := &blockingReader{Reader: &mail.Mock{}, block: make(chan struct{})}
	cs := connect(t, br, Options{Version: "test", ConnectTimeout: 50 * time.Millisecond})
	// Registrato dopo connect: LIFO lo chiude prima della chiusura della sessione.
	t.Cleanup(func() { close(br.block) })

	done := make(chan *mcp.CallToolResult, 1)
	go func() {
		res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "list_folders"})
		if err != nil {
			t.Error(err)
		}
		done <- res
	}()
	select {
	case res := <-done:
		if res == nil || !res.IsError {
			t.Fatal("atteso IsError per connessione scaduta")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("la chiamata non ha rispettato il budget di connessione (5 s)")
	}
}
