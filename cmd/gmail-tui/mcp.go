package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/mail/imap"
	"github.com/andsigno82/gmail-tui/internal/mcpserver"
)

// runMCP è il sottocomando `gmail-tui mcp` (Piano 14, Task 5): server MCP in
// sola lettura su stdio. Stdout è il canale del protocollo: ogni diagnostica va
// su stderr. Lo store IMAP non usa cache (M4): non condivide il file bbolt con la TUI.
func runMCP(args []string) int {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	if exitCode := parseFlags(fs, args); exitCode >= 0 {
		return exitCode
	}
	cfg, ok := loadConfig()
	if !ok {
		return 1
	}
	pw, ok := appPassword(cfg)
	if !ok {
		return 1
	}
	store := imap.NewStore(imap.Options{
		Addr:     imapAddr,
		Username: cfg.Address,
		Password: pw,
	})
	defer closeStoreLogged(store, os.Stderr, pw) // Piano 14, fix review 7
	return serveMCP(store, []string{pw}, &mcp.StdioTransport{})
}

// serveMCP serve il protocollo MCP sul trasporto fino alla chiusura (Piano 14, Task 5).
// Gli errori passano dalla redazione dei segreti prima di uscire su stderr.
func serveMCP(store mail.MailStore, secrets []string, tr mcp.Transport) int {
	srv := mcpserver.NewServer(store, mcpserver.Options{Version: version, Secrets: secrets})
	if err := srv.Run(context.Background(), tr); err != nil {
		fmt.Fprintf(os.Stderr, "mcp: %v\n", mcpserver.Redact(err, secrets...))
		return 1
	}
	return 0
}

// closeStoreLogged chiude lo store e registra su stderr l'eventuale errore, con
// i segreti redatti (Piano 14, fix review 7).
func closeStoreLogged(c io.Closer, stderr io.Writer, secrets ...string) {
	if err := c.Close(); err != nil {
		fmt.Fprintf(stderr, "mcp: chiusura store: %v\n", mcpserver.Redact(err, secrets...))
	}
}
