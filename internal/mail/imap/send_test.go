package imap

import (
	"context"
	"strings"
	"testing"

	"gmail-tui/internal/msg"
)

// Send è uno stub (l'invio è responsabilità di smtp.Sender): deve sempre
// fallire indicando il percorso corretto.
func TestSendNotSupported(t *testing.T) {
	s := NewStore(Options{})
	err := s.Send(context.Background(), msg.Outgoing{From: "a@b", To: []string{"c@d"}})
	if err == nil {
		t.Fatal("Send su Store IMAP: want errore, got nil")
	}
	if !strings.Contains(err.Error(), "smtp.Sender") {
		t.Errorf("errore = %v, want menzione di smtp.Sender", err)
	}
}
