// Package smtp implementa l'invio di msg.Outgoing via SMTP (Gmail:
// smtp.gmail.com:587 + STARTTLS): costruzione MIME (buildMIME) e
// conversazione SMTP con emersion/go-smtp.
package smtp

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Options configura il Sender. Insecure = true (SOLO nei test): niente
// STARTTLS, connessione in chiaro.
type Options struct {
	Host     string
	Port     int
	Username string
	Password string
	Insecure bool
	// TLSConfig sostituisce la tls.Config usata per STARTTLS; nil = default
	// (&tls.Config{ServerName: Host}). Utile nei test (certificato
	// self-signed, ServerName custom).
	TLSConfig *tls.Config
}

// Sender invia msg.Outgoing via SMTP. Invariabile: una connessione per Send.
type Sender struct {
	o Options
}

// Assert di conformenza all'interfaccia minima richiesta dal sottocomando
// `send` (gmail-tui/internal/cmd.Sender), espresso come interfaccia anonima:
// il package smtp non dipende da internal/cmd (inversione delle dipendenze:
// è il comando che conosce il sender, non il viceversa).
var _ interface {
	Send(context.Context, msg.Outgoing) error
} = (*Sender)(nil)

// NewSender crea un Sender con le Options date.
func NewSender(o Options) *Sender {
	return &Sender{o: o}
}

// Send costruisce il MIME e lo consegna via SMTP:
//
//	dial → [STARTTLS] → [AUTH PLAIN] → MAIL → RCPT×N → DATA → QUIT.
//
// I destinatari RCPT sono To+Cc+Bcc DEDUPLICATI (in quest'ordine); Bcc NON
// compare negli header del messaggio (vedi buildMIME).
// Gli errori sono wrapped con la fase: dial, starttls, mail, auth,
// rcpt <addr>, data.
func (s *Sender) Send(ctx context.Context, m msg.Outgoing) error {
	body, err := buildMIME(m, time.Now())
	if err != nil {
		return err // già wrapped con contesto da buildMIME
	}
	rcpts, err := recipientAddrs(m)
	if err != nil {
		return err
	}
	// Piano 7.2, fix review: MAIL FROM accetta solo addr-spec; il nome
	// visualizzato di From resta nel solo header (buildMIME).
	from, err := msg.BareAddress(m.From)
	if err != nil {
		return fmt.Errorf("smtp: mittente non valido %q: %w", m.From, err)
	}

	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(s.o.Host, strconv.Itoa(s.o.Port)))
	if err != nil {
		return fmt.Errorf("smtp: dial: %w", err)
	}

	// Nota API: go-smtp v0.26 non espone Client.StartTLS; NewClientStartTLS
	// fa dial già fatto + STARTTLS subito: stessa semantica.
	var c *gosmtp.Client
	if s.o.Insecure {
		c = gosmtp.NewClient(conn)
	} else {
		tlsCfg := s.o.TLSConfig
		if tlsCfg == nil {
			tlsCfg = &tls.Config{ServerName: s.o.Host}
		}
		c, err = gosmtp.NewClientStartTLS(conn, tlsCfg)
		if err != nil {
			_ = conn.Close()
			return fmt.Errorf("smtp: starttls: %w", err)
		}
		// go-smtp v0.26 rinvia l'handshake TLS al primo I/O successivo
		// (lazy): l'EHLO post-STARTTLS lo completa qui, così un errore di
		// verifica TLS (ServerName errato, cert non fidato) esce wrapped
		// con fase `starttls` e non sfuma in mail/auth.
		if err := c.Hello("localhost"); err != nil {
			return fmt.Errorf("smtp: starttls: %w", err)
		}
	}
	defer func() { _ = c.Close() }()

	if s.o.Username != "" {
		if err := c.Auth(sasl.NewPlainClient("", s.o.Username, s.o.Password)); err != nil {
			return fmt.Errorf("smtp: auth: %w", err)
		}
	}

	if err := c.Mail(from, nil); err != nil {
		return fmt.Errorf("smtp: mail: %w", err)
	}

	for _, addr := range rcpts {
		if err := c.Rcpt(addr, nil); err != nil {
			return fmt.Errorf("smtp: rcpt %s: %w", addr, err)
		}
	}

	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp: data: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		_ = w.Close()
		return fmt.Errorf("smtp: data: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp: data: %w", err)
	}

	return c.Quit()
}

// recipientAddrs estrae gli indirizzi nudi (addr-spec) di To+Cc+Bcc per RCPT
// TO (Piano 7.1, Task 2): il nome visualizzato non è ammesso dopo RCPT TO.
// Il dedup avviene sull'indirizzo nudo, così forme diverse dello stesso
// indirizzo danno un solo RCPT. Un indirizzo non parsabile è un errore prima
// di MAIL.
func recipientAddrs(m msg.Outgoing) ([]string, error) {
	all := append(append(append([]string{}, m.To...), m.Cc...), m.Bcc...)
	bare := make([]string, 0, len(all))
	for _, a := range all {
		if strings.TrimSpace(a) == "" {
			continue
		}
		// Piano 7.1, fix code-review: addr-spec con local part quotata (msg.BareAddress).
		b, err := msg.BareAddress(a)
		if err != nil {
			return nil, fmt.Errorf("smtp: indirizzo non valido %q: %w", a, err)
		}
		bare = append(bare, b)
	}
	return dedup(bare), nil
}

// dedup conserva l'ordine e scarta i duplicati esatti e le stringhe vuote
// (case-sensitive: la canonicalizzazione degli indirizzi resta al server).
func dedup(addrs []string) []string {
	seen := make(map[string]struct{}, len(addrs))
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if a == "" {
			continue
		}
		if _, ok := seen[a]; ok {
			continue
		}
		seen[a] = struct{}{}
		out = append(out, a)
	}
	return out
}
