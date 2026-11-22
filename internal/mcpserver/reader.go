package mcpserver

import (
	"context"
	"fmt"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Limiti delle liste (Piano 14, M5): default 20 righe, massimo 100.
const (
	defaultLimit = 20
	maxLimit     = 100
)

// Reader è il sottoinsieme in sola lettura di mail.MailStore usato dal server
// (Piano 14, Task 2, D14). Il server non riceve mai lo store completo: non
// può chiamare Send, Archive, Trash, MarkRead, Star, ApplyLabel, SaveAttachment.
// È il punto in cui il Piano 8 aggiungerà la ricerca X-GM-RAW e la conversazione.
type Reader interface {
	Connect(ctx context.Context) error
	ListMailboxes(ctx context.Context) ([]mail.Mailbox, error)
	ListMessages(ctx context.Context, q msg.Query, pg mail.Page) ([]msg.MessageRef, error)
	ListConversations(ctx context.Context, q msg.Query, pg mail.Page) ([]mail.Conversation, error)
	ThreadMessages(ctx context.Context, threadID uint64) ([]msg.MessageRef, error)
	FetchMessage(ctx context.Context, id msg.MessageID) (*msg.Message, error)
}

// normalizePage applica i default e i tetti di M5 a offset e limit.
func normalizePage(offset, limit int) (mail.Page, error) {
	if offset < 0 {
		return mail.Page{}, fmt.Errorf("offset negativo: %d", offset)
	}
	if limit < 0 {
		return mail.Page{}, fmt.Errorf("limit negativo: %d", limit)
	}
	if limit == 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	return mail.Page{Offset: offset, Limit: limit}, nil
}
