// Package mail definisce il contratto MailStore (spec §Interfaccia
// MailStore): la UI parla SOLO con questa interfaccia, mai con IMAP.
package mail

import (
	"context"

	"gmail-tui/internal/msg"
)

// Page è la paginazione di ListMessages (Limit 0 = default 50).
type Page struct {
	Offset, Limit int
}

// ChangeKind indica cosa è cambiato nel feed Watch.
type ChangeKind int

const (
	ChangeNewMail ChangeKind = iota
	ChangeFlags
	ChangeDisconnected
)

// Change è un evento push (IMAP IDLE nel backend reale).
type Change struct {
	Kind ChangeKind
	IDs  []msg.MessageID
}

// MailStore è l'astrazione di backend posta.
type MailStore interface {
	Connect(ctx context.Context) error
	ListMessages(ctx context.Context, q msg.Query, pg Page) ([]msg.MessageRef, error)
	FetchMessage(ctx context.Context, id msg.MessageID) (*msg.Message, error)
	Search(ctx context.Context, q msg.Query) ([]msg.MessageRef, error)
	MarkRead(ctx context.Context, ids []msg.MessageID, read bool) error
	Archive(ctx context.Context, ids []msg.MessageID) error
	Star(ctx context.Context, ids []msg.MessageID, on bool) error
	ApplyLabel(ctx context.Context, ids []msg.MessageID, label string, on bool) error
	SaveAttachment(ctx context.Context, id msg.MessageID, part int, path string) error
	Send(ctx context.Context, m msg.Outgoing) error
	Watch(ctx context.Context) (<-chan Change, error)
}
