package imap

// FetchMessage (cache-first) e SaveAttachment (piano 2, Task 4).

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/jhillyerd/enmime"

	"gmail-tui/internal/msg"
)

// ErrMessageNotFound è wrappato nell'errore ritornato quando il fetch di un
// messaggio non produce dati (uid inesistente nella mailbox).
var ErrMessageNotFound = errors.New("messaggio inesistente")

// FetchMessage restituisce il messaggio completo, cache-first:
//  1. id.Split() → mailbox+uid;
//  2. se la cache è configurata ed è un hit ⇒ ritorno immediato (zero rete);
//  3. miss ⇒ Select della mailbox, UIDFetch del corpo grezzo con BODY.PEEK[]
//     (il PEEK non imposta \Seen), msg.Parse;
//  4. m.Ref.Unread/Starred sono riconciliati dai FLAGS della stessa risposta
//     di fetch (il peek non li altera);
//  5. scrittura in cache con la UIDValidity della SELECT (invalidazione
//     automatica se la mailbox cambia epoca) e ritorno. Senza cache il
//     comportamento è identico, senza scrittura.
//
// Le API go-imap v2 (beta.9) non prendono context: come per Connect,
// FetchMessage controlla solo ctx.Err() in ingresso.
func (s *Store) FetchMessage(ctx context.Context, id msg.MessageID) (*msg.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	mailbox, uid, err := id.Split()
	if err != nil {
		return nil, fmt.Errorf("imap fetch: %w", err)
	}

	if s.opts.Cache != nil {
		m, ok, err := s.opts.Cache.GetMessage(mailbox, uid)
		if err != nil {
			return nil, fmt.Errorf("cache get %s: %w", id, err)
		}
		if ok {
			return m, nil // hit: zero rete, NESSUNA attesa sul gate
		}
	}

	// Miss: da qui si tocca la rete — il gate rompe l'eventuale IDLE del
	// Watch (fix review) e trattiene la connessione fino alla fine del metodo.
	defer s.gate.acquire()()

	raw, flags, uidValidity, err := s.fetchRaw(mailbox, imap.UID(uid))
	if err != nil {
		return nil, err
	}

	m, err := msg.Parse(id, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("imap fetch %s: %w", id, err)
	}
	// Riconciliazione dai FLAGS del fetch (msg.Parse non li conosce).
	m.Ref.Unread = true
	m.Ref.Starred = false
	for _, f := range flags {
		switch f {
		case imap.FlagSeen:
			m.Ref.Unread = false
		case imap.FlagFlagged:
			m.Ref.Starred = true
		}
	}

	if s.opts.Cache != nil {
		if err := s.opts.Cache.SetMessage(mailbox, uidValidity, m); err != nil {
			return nil, fmt.Errorf("cache set %s: %w", id, err)
		}
	}
	return m, nil
}

// SaveAttachment salva su path il contenuto dell'allegato numero part del
// messaggio id. Scarica il messaggio grezzo con PEEK bypassando la cache (il
// corpo potrebbe non esservi mai finito o esservi senza allegati): il
// contenuto grezzo è parsato con enmime e la parte è scritta con 0o644.
// part è un indice in msg.Message.Attachments: prima di indicizzare, gli
// attachment grezzi di enmime passano dallo STESSO filtro degeneri di
// msg.Parse (parti vuote senza nome saltate e indici rinumerati), così i
// PartIndex prodotti dal parse selezionano qui le stesse parti.
// part fuori range ⇒ errore.
func (s *Store) SaveAttachment(ctx context.Context, id msg.MessageID, part int, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	mailbox, uid, err := id.Split()
	if err != nil {
		return fmt.Errorf("imap save attachment: %w", err)
	}

	// Gate (fix review): il download tocca la rete — rompe l'eventuale IDLE.
	defer s.gate.acquire()()

	raw, _, _, err := s.fetchRaw(mailbox, imap.UID(uid))
	if err != nil {
		return err
	}
	env, err := enmime.ReadEnvelope(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("imap save attachment, parse %s: %w", id, err)
	}
	atts := usefulAttachments(env)
	if part < 0 || part >= len(atts) {
		return fmt.Errorf("imap save attachment %s: parte %d fuori range (%d allegati)", id, part, len(atts))
	}
	if err := os.WriteFile(path, atts[part].Content, 0o644); err != nil {
		return fmt.Errorf("imap save attachment, scrivi %s: %w", path, err)
	}
	return nil
}

// usefulAttachments replica il filtro di msg.Parse sugli attachment grezzi
// di enmime (msg/parse.go): le parti degeneri — senza filename,
// application/octet-stream e con corpo vuoto — sono saltate e gli indici
// rinumerati. TENERE SINCRONIZZATO con il filtro di msg.Parse: i PartIndex
// di msg.Message.Attachments valgono su questa lista, non su
// env.Attachments.
func usefulAttachments(env *enmime.Envelope) []*enmime.Part {
	atts := make([]*enmime.Part, 0, len(env.Attachments))
	for _, part := range env.Attachments {
		if part.FileName == "" && part.ContentType == "application/octet-stream" && len(part.Content) == 0 {
			continue // parte degenere: saltata anche da msg.Parse
		}
		atts = append(atts, part)
	}
	return atts
}

// fetchRaw seleziona mailbox e scarica il messaggio grezzo (HEADER+BODY
// completi) con BODY.PEEK[] — non imposta \Seen — insieme ai FLAGS correnti
// e alla UIDValidity della SELECT (chiave di invalidazione della cache).
// Nessun dato (uid inesistente) ⇒ errore che wrappa ErrMessageNotFound.
func (s *Store) fetchRaw(mailbox string, uid imap.UID) (raw []byte, flags []imap.Flag, uidValidity uint32, err error) {
	c, err := s.connected()
	if err != nil {
		return nil, nil, 0, err
	}
	sel, err := c.Select(mailbox, nil).Wait()
	if err != nil {
		return nil, nil, 0, fmt.Errorf("imap select %s: %w", mailbox, err)
	}

	section := &imap.FetchItemBodySection{Peek: true} // BODY.PEEK[] = messaggio intero, senza \Seen
	cmd := c.Fetch(imap.UIDSetNum(uid), &imap.FetchOptions{
		UID:         true,
		Flags:       true,
		BodySection: []*imap.FetchItemBodySection{section},
	})
	bufs, err := cmd.Collect()
	if err != nil {
		return nil, nil, 0, fmt.Errorf("imap fetch body in %s: %w", mailbox, err)
	}
	var buf *imapclient.FetchMessageBuffer
	for _, b := range bufs {
		if b.UID == uid {
			buf = b
			break
		}
	}
	if buf == nil {
		return nil, nil, 0, fmt.Errorf("%w: uid %d in %s", ErrMessageNotFound, uid, mailbox)
	}
	raw = buf.FindBodySection(section)
	if raw == nil {
		return nil, nil, 0, fmt.Errorf("%w: uid %d in %s, corpo assente nella risposta", ErrMessageNotFound, uid, mailbox)
	}
	return raw, buf.Flags, sel.UIDValidity, nil
}
