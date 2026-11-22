package mcpserver

import (
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/andsigno82/gmail-tui/internal/mail"
)

// connect collega un client MCP in-process (trasporto in memoria dell'SDK) al
// server costruito con r. Piano 14, Task 3: nessuna rete, nessun processo.
func connect(t *testing.T, r Reader, opts Options) *mcp.ClientSession {
	t.Helper()
	ctx := t.Context()
	srvT, cliT := mcp.NewInMemoryTransports()
	ss, err := NewServer(r, opts).Connect(ctx, srvT, nil)
	if err != nil {
		t.Fatalf("connessione server: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cli := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := cli.Connect(ctx, cliT, nil)
	if err != nil {
		t.Fatalf("connessione client: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// Piano 14, Task 3: esattamente i quattro tool di sola lettura, tutti con
// readOnlyHint e con la descrizione che dichiara il contenuto non fidato.
func TestToolsListExactlyReadOnlyTools(t *testing.T) {
	cs := connect(t, &mail.Mock{}, Options{Version: "test"})
	res, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("%s: readOnlyHint mancante", tool.Name)
		}
		if !strings.Contains(tool.Description, "non fidato") {
			t.Errorf("%s: la descrizione non dichiara il contenuto non fidato", tool.Name)
		}
	}
	sort.Strings(names)
	want := []string{"list_folders", "list_recent", "read_message", "read_thread", "search_messages"} // Wave 4: + read_thread
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("tool registrati = %v, attesi %v", names, want)
	}
}

// Piano 14, Task 3 (M3): la connessione è lazy. Un fallimento diventa un
// risultato con IsError, non un errore di protocollo, e la chiamata successiva
// riprova.
func TestLazyConnectRetriesAfterFailure(t *testing.T) {
	m := &mail.Mock{Err: errors.New("rete giù")}
	cs := connect(t, m, Options{Version: "test"})
	ctx := t.Context()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "list_folders"})
	if err != nil {
		t.Fatalf("errore di protocollo inatteso: %v", err)
	}
	if !res.IsError || !strings.Contains(resultText(res), "connessione fallita") {
		t.Fatalf("atteso IsError con 'connessione fallita', ottenuto %+v", res)
	}

	m.Err = nil
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "list_folders"})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("la chiamata successiva doveva riuscire: %s", resultText(res))
	}
}

// resultText concatena il testo dei contenuti di un risultato tool.
func resultText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}
