package cmd

import (
	"context"
	"fmt"
	"io"
	"net/mail"
	"os"
	"strings"
	"time"

	"github.com/andsigno82/gmail-tui/internal/config"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Sender è l'interfaccia minima di invio richiesta dal sottocomando send;
// *smtp.Sender la soddisfa (assert di conformenza nel package smtp).
type Sender interface {
	Send(ctx context.Context, m msg.Outgoing) error
}

// SendArgs sono gli argomenti del sottocomando send. To/Cc/Bcc sono liste
// di indirizzi separati da virgola (spazi attorno agli indirizzi ignorati).
type SendArgs struct {
	To, Cc, Bcc string
	Subject     string
	Attach      []string
}

// SendTimeout è il budget dell'intera operazione di invio (dial → STARTTLS →
// AUTH → MAIL → RCPT → DATA → QUIT del sender SMTP): oltre, il ctx passato
// al sender viene annullato e Send fallisce. Condivisa col flusso di invio
// della TUI (internal/app, Piano 4 Task 4: stesso budget per il Sender).
const SendTimeout = 60 * time.Second

// Send legge il corpo del messaggio da body, costruisce msg.Outgoing
// (From = cfg.Address, subject e allegati dagli argomenti: la codifica
// Q-encoded dell'oggetto è responsabilità del Sender/buildMIME) e lo
// consegna a sender con un ctx che scade dopo SendTimeout (il sender SMTP
// usa il ctx per il dial; niente hang indefiniti su rete in balia).
// Su ok stampa "sent to N recipients" su out, dove N è
// il numero di destinatari UNICI di To+Cc+Bcc (stessa semantica di dedup
// del Sender SMTP: vuoti scartati, dedup esatto case-sensitive).
// Nessun destinatario, indirizzo malformato (msg.NormalizeRecipients) o
// errore del sender ⇒ messaggio su stderr, ritorna 1; 0 se ok.
func Send(out io.Writer, sender Sender, cfg config.Config, args SendArgs, body io.Reader) int {
	text, err := io.ReadAll(body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "send: leggo il corpo: %v\n", err)
		return 1
	}
	to, err := parseAddrs(args.To)
	if err != nil {
		fmt.Fprintf(os.Stderr, "send: --to: %v\n", err)
		return 1
	}
	cc, err := parseAddrs(args.Cc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "send: --cc: %v\n", err)
		return 1
	}
	bcc, err := parseAddrs(args.Bcc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "send: --bcc: %v\n", err)
		return 1
	}
	if len(to)+len(cc)+len(bcc) == 0 {
		fmt.Fprintln(os.Stderr, "send: nessun destinatario (--to)")
		return 1
	}
	m := msg.Outgoing{
		From:        cfg.Address,
		To:          to,
		Cc:          cc,
		Bcc:         bcc,
		Subject:     args.Subject,
		BodyText:    string(text),
		Attachments: args.Attach,
	}
	// Piano 7, Task 2: indirizzi malformati fermati prima del round-trip
	// SMTP (prima emergevano solo come errore del server a invio avviato).
	// Piano 7.1, Task 3: forma canonica (dominio punycode, nome RFC 2047).
	m, err = msg.NormalizeRecipients(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "send: %v\n", err)
		return 1
	}
	rcpts := bareRecipients(m)
	ctx, cancel := context.WithTimeout(context.Background(), SendTimeout)
	defer cancel()
	if err := sender.Send(ctx, m); err != nil {
		fmt.Fprintf(os.Stderr, "send: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "sent to %d recipients\n", len(rcpts))
	return 0
}

// parseAddrs legge una lista di indirizzi RFC 5322 (Piano 7.1, Task 3): la
// virgola dentro un nome quotato ("Rossi, Mario" <m@x.it>) non spezza il
// destinatario. Lista vuota ⇒ slice nil, così l'Outgoing non porta campi vuoti.
func parseAddrs(s string) ([]string, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	list, err := mail.ParseAddressList(s)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(list))
	for _, a := range list {
		out = append(out, a.String())
	}
	return out, nil
}

// bareRecipients restituisce gli indirizzi nudi unici di To+Cc+Bcc di un
// Outgoing già normalizzato: è il conteggio "sent to N recipients".
func bareRecipients(m msg.Outgoing) []string {
	all := concat(concat(m.To, m.Cc), m.Bcc)
	bare := make([]string, 0, len(all))
	for _, a := range all {
		// Già normalizzato da msg.NormalizeRecipients: il parse non fallisce.
		if p, err := mail.ParseAddress(a); err == nil {
			bare = append(bare, p.Address)
		}
	}
	return dedupAddrs(bare)
}

// dedupAddrs scarta i duplicati esatti (case-sensitive: la
// canonicalizzazione degli indirizzi resta al server) conservando l'ordine.
func dedupAddrs(addrs []string) []string {
	seen := make(map[string]struct{}, len(addrs))
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if _, ok := seen[a]; ok {
			continue
		}
		seen[a] = struct{}{}
		out = append(out, a)
	}
	return out
}

func concat(a, b []string) []string {
	out := make([]string, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	return out
}
