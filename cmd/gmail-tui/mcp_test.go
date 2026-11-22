package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/andsigno82/gmail-tui/internal/mail"
)

// Piano 14, Task 5: --help esce 0 senza leggere la config né toccare la rete.
func TestRunMCPHelpExitsZero(t *testing.T) {
	if code := runMCP([]string{"-h"}); code != 0 {
		t.Fatalf("exit %d, atteso 0", code)
	}
}

// Piano 14, Task 5: argomenti posizionali inattesi sono un errore d'uso.
func TestRunMCPRejectsPositionalArgs(t *testing.T) {
	if code := runMCP([]string{"extra"}); code != 1 {
		t.Fatalf("exit %d, atteso 1", code)
	}
}

// Piano 14, Task 5 + M6: serveMCP parla il protocollo sul trasporto e redige la
// password da ciò che esce verso il client.
func TestServeMCPRedactsPasswordOverTransport(t *testing.T) {
	const secret = "segreto-123"
	srvT, cliT := mcp.NewInMemoryTransports()
	store := &mail.Mock{Err: errors.New("login fallito per " + secret)}
	done := make(chan int, 1)
	go func() { done <- serveMCP(store, []string{secret}, srvT) }()

	cli := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := cli.Connect(t.Context(), cliT, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "list_folders"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("atteso IsError: il server Mock fallisce la connessione")
	}
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok && strings.Contains(tc.Text, secret) {
			t.Fatalf("password nel risultato verso il client: %q", tc.Text)
		}
	}
	if err := cs.Close(); err != nil {
		t.Fatal(err)
	}
	if code := <-done; code != 0 {
		t.Errorf("serveMCP exit %d alla chiusura del client, atteso 0", code)
	}
}
