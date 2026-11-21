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
//
// Thread-safety (Piano 3, Task 1): le operazioni sono SERIALIZZATE — UNA OP
// ALLA VOLTA. Il backend IMAP ha una connessione e SEARCH/STORE/EXPUNGE
// agiscono sulla mailbox correntemente selezionata: chiamate concorrenti
// (es. multiple tea.Cmd della TUI) sono corrette — il gate del backend le
// mette in coda, ma non interleaveano i comandi sulla connessione.
type MailStore interface {
	Connect(ctx context.Context) error

	// ListMessages restituisce una pagina di righe di lista secondo q e pg.
	//
	// Semantica IMAP/Gmail:
	//   - q.Label seleziona la mailbox (la label) invece di INBOX;
	//   - q.HasAttachment è best-effort: post-filtro client-side, non una
	//     condizione valutata dal server;
	//   - MessageRef.ThreadLen vale sempre 1: la lunghezza del thread non
	//     è disponibile via IMAP ENVELOPE, il raggruppamento in thread è
	//     responsabilità della UI (Piano 3).
	ListMessages(ctx context.Context, q msg.Query, pg Page) ([]msg.MessageRef, error)

	FetchMessage(ctx context.Context, id msg.MessageID) (*msg.Message, error)

	// Search cerca senza paginazione; valgono la semantica e i limiti
	// documentati su ListMessages (Label seleziona la mailbox,
	// HasAttachment best-effort client-side, ThreadLen = 1).
	Search(ctx context.Context, q msg.Query) ([]msg.MessageRef, error)

	MarkRead(ctx context.Context, ids []msg.MessageID, read bool) error

	// Archive archivia i messaggi. Su Gmail/IMAP rimuove il messaggio
	// dalla mailbox corrente: l'expunge equivale alla rimozione della
	// label e il messaggio resta in All Mail.
	Archive(ctx context.Context, ids []msg.MessageID) error

	Star(ctx context.Context, ids []msg.MessageID, on bool) error

	// ApplyLabel gestisce una label Gmail mappata su cartella IMAP:
	// on = copia il messaggio nella cartella-label, off = lo rimuove
	// dalla cartella-label.
	ApplyLabel(ctx context.Context, ids []msg.MessageID, label string, on bool) error

	SaveAttachment(ctx context.Context, id msg.MessageID, part int, path string) error
	Send(ctx context.Context, m msg.Outgoing) error

	// Watch ritorna il canale degli eventi push del feed.
	//
	// Contratto:
	//   - un solo canale per chiamata: ogni chiamata crea il proprio;
	//   - il canale viene chiuso quando ctx è annullato (ctx.Done);
	//   - gli eventi sono best-effort: il canale è bufferizzato, se il
	//     consumer è lento gli eventi si perdono e il consumer deve
	//     re-listare;
	//   - ChangeDisconnected viene emesso prima di una riconnessione;
	//   - la baseline iniziale e la ri-verifica post-riconnessione girano
	//     dentro un round del gate (come le op: una alla volta) — la
	//     chiamata può attendere ops in flight, senza interleave.
	Watch(ctx context.Context) (<-chan Change, error)
}
