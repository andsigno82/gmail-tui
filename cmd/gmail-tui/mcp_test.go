package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/andsigno82/gmail-tui/internal/mail"
)

// Piano 16, Task 11, D16-T16: il server MCP non apre la cache (nessun dato
// stantio): con una password e stdin chiuso il server termina senza creare
// cache.db.
func TestMCPDoesNotOpenCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GMAIL_APP_PASSWORD", "pw")
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = devNull.Close() }()
	old := os.Stdin
	os.Stdin = devNull
	t.Cleanup(func() { os.Stdin = old })
	_ = runMCP(nil)
	dir, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "gmail-tui", "cache.db")); !os.IsNotExist(err) {
		t.Errorf("runMCP ha aperto la cache: err = %v, voglio file assente", err)
	}
}

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
