package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// fetchLimitDefault è il limite di righe usato quando limit <= 0.
const fetchLimitDefault = 20

// Fetch connette lo store e stampa su out i messaggi che matchano q,
// una riga per messaggio, campi TAB-separati:
//
//	uid <TAB> from <TAB> subject <TAB> date(RFC3339) <TAB> flags
//
// Il primo campo è l'ID completo del messaggio ("MAILBOX:UID": la chiave
// consumabile dagli altri sottocomandi); flags concatena "U" se non letto
// e "*" se starred (stringa vuota se nessuno dei due). limit <= 0 ⇒
// fetchLimitDefault (20).
// Gli errori (connect, lista) vanno su stderr: ritorna 1; 0 se ok.
func Fetch(out io.Writer, store mail.MailStore, q msg.Query, limit int) int {
	ctx := context.Background()
	if err := store.Connect(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "fetch: connect: %v\n", err)
		return 1
	}
	if limit <= 0 {
		limit = fetchLimitDefault
	}
	refs, err := store.ListMessages(ctx, q, mail.Page{Limit: limit})
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetch: %v\n", err)
		return 1
	}
	for _, r := range refs {
		fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\n",
			r.ID, r.From, r.Subject, r.Date.Format(time.RFC3339), flagsFor(r))
	}
	return 0
}

// flagsFor compone la colonna flags: "U" se unread, "*" se starred.
func flagsFor(r msg.MessageRef) string {
	f := ""
	if r.Unread {
		f += "U"
	}
	if r.Starred {
		f += "*"
	}
	return f
}
