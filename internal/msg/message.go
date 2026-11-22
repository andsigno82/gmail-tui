// Package msg contiene i tipi dati normalizzati della posta:
// unica fonte di verità per UI e backend.
package msg

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// MessageID identifica un messaggio: "MAILBOX:UID" (es. "INBOX:42").
type MessageID string

// NewID compone un MessageID da mailbox e UID IMAP.
func NewID(mailbox string, uid uint32) MessageID {
	return MessageID(mailbox + ":" + strconv.FormatUint(uint64(uid), 10))
}

// Split decostruisce l'ID.
func (id MessageID) Split() (mailbox string, uid uint32, err error) {
	s := string(id)
	i := strings.LastIndexByte(s, ':')
	if i <= 0 || i == len(s)-1 {
		return "", 0, fmt.Errorf("message id %q malformato", s)
	}
	n, err := strconv.ParseUint(s[i+1:], 10, 32)
	if err != nil {
		return "", 0, fmt.Errorf("message id %q: uid non numerico", s)
	}
	return s[:i], uint32(n), nil
}

// MessageRef è la riga di lista: tutto ciò che serve senza il corpo.
type MessageRef struct {
	ID        MessageID
	From      string
	Subject   string
	Date      time.Time
	Unread    bool
	Starred   bool
	ThreadLen int
}

// Attachment descrive un allegato (PartIndex = indice in Message.Attachments).
type Attachment struct {
	PartIndex   int
	Filename    string
	ContentType string
	Size        int64
}

// Message è il messaggio completo normalizzato.
type Message struct {
	Ref      MessageRef
	To, Cc   []string
	BodyText string
	// BodyHTML contiene l'HTML originale se presente (anche per messaggi con
	// parte testuale); HasHTML resta true solo per messaggi SOLO HTML.
	// Serve alla TUI per "apri nel browser" (Piano 3, Task 5).
	BodyHTML    string
	HasHTML     bool
	Attachments []Attachment
}

// Outgoing è un'email da inviare (Attachments = percorsi file).
type Outgoing struct {
	From        string
	To, Cc, Bcc []string
	Subject     string
	BodyText    string
	Attachments []string
	// Markdown (Piano 7.2, Task 5): il corpo è markdown; con true il MIME è
	// multipart/alternative (text/plain sorgente + text/html GFM).
	Markdown bool
}
