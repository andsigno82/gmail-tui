package imap

import (
	"context"
	"errors"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Piano 14, Wave 4 (punto 4): l'errore "non connesso" è il sentinel
// mail.ErrNotConnected, riconoscibile con errors.Is dal chiamante (MCP).
func TestNotConnectedIsSentinel(t *testing.T) {
	s := NewStore(Options{Addr: "127.0.0.1:1", Username: "u", Password: "p"})
	_, err := s.ListMessages(context.Background(), msg.Query{}, mail.Page{Limit: 1})
	if !errors.Is(err, mail.ErrNotConnected) {
		t.Fatalf("errore = %v, atteso mail.ErrNotConnected", err)
	}
}
