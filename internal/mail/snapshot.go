package mail

import (
	"errors"
	"time"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// ErrNotConfirmed segnala un MessageID che non e ancora uscito da una risposta
// fresca del server in questa sessione (Piano 16, Task 2, regola R4): la
// guardia lo restituisce senza rete.
var ErrNotConfirmed = errors.New("messaggio non ancora confermato dal server: attendi l'aggiornamento")

// ConversationsSnapshot e l'istantanea locale della prima pagina di una
// cartella (Piano 16, Task 2). Limited vale true se al momento del salvataggio
// la lista era limitata dal tetto di scansione (mail.ErrScanLimit).
type ConversationsSnapshot struct {
	Rows    []Conversation
	SavedAt time.Time
	Limited bool
}

// ThreadSnapshot e l'istantanea locale dei ref di un thread (Piano 16, Task 2).
type ThreadSnapshot struct {
	Refs    []msg.MessageRef
	SavedAt time.Time
}

// Snapshotter e l'interfaccia OPZIONALE (non fa parte di MailStore) con cui lo
// store espone la cache locale (Piano 16, Task 2, D16-T1). Contratto:
//   - sola lettura dal disco: niente ctx, niente gate, niente rete;
//   - valida prima di Connect;
//   - il risultato e PROVVISORIO (R1): i MessageID che contiene non vanno mai
//     passati a un metodo di MailStore finche non sono confermati dal server;
//   - mailbox "" = INBOX;
//   - un errore del disco vale come miss (ok == false).
//
// Il Mock non la implementa: in demo nessuna istantanea. tracestore (e
// mail.Guard, Task 5) la inoltrano come GmailExt.
type Snapshotter interface {
	CachedConversations(mailbox string) (ConversationsSnapshot, bool)
	CachedThread(threadID uint64) (ThreadSnapshot, bool)
	CachedMessage(id msg.MessageID) (*msg.Message, bool)
}
