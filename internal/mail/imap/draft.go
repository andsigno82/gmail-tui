package imap

// Bozze IMAP (Piano 9, Task 3): APPEND nella cartella con ruolo drafts (flag
// \Draft \Seen) e rimozione per UID SOLO in quella cartella. Gli ID si
// validano prima di toccare il server; mai EXPUNGE generico (AGENTS.md).

import (
	"context"
	"fmt"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// SaveDraft implementa mail.MailStore (Piano 9, Task 3). Sotto gate. Rifiuta
// prima di toccare il server: nessuna cartella con ruolo drafts; ID in replace
// fuori da Bozze; server senza UIDPLUS (serve l'UID dell'APPEND e la rimozione
// selettiva). Dopo l'APPEND rimuove le bozze di replace; se la rimozione
// fallisce l'errore riporta il nuovo ID, così la copia in più è visibile.
func (s *Store) SaveDraft(ctx context.Context, raw []byte, replace []msg.MessageID) (msg.MessageID, error) {
	defer s.acquire(ctx)() // gate: APPEND e rimozione non interleaveano con IDLE

	if err := ctx.Err(); err != nil {
		return "", err
	}
	c, err := s.connected()
	if err != nil {
		return "", err
	}
	box, err := draftsBoxOf(c)
	if err != nil {
		return "", err
	}
	if err := checkInMailbox(box, replace); err != nil {
		return "", err
	}
	if !hasUIDPlus(c) {
		return "", fmt.Errorf("imap bozze: il server non supporta UIDPLUS: rifiuto")
	}

	cmd := c.Append(box, int64(len(raw)), &imap.AppendOptions{
		Flags: []imap.Flag{imap.FlagDraft, imap.FlagSeen},
		Time:  time.Now(),
	})
	if _, err := cmd.Write(raw); err != nil {
		_ = cmd.Close()
		return "", fmt.Errorf("imap append %s: %w", box, err)
	}
	if err := cmd.Close(); err != nil {
		return "", fmt.Errorf("imap append %s: %w", box, err)
	}
	data, err := cmd.Wait()
	if err != nil {
		return "", fmt.Errorf("imap append %s: %w", box, err)
	}
	id := msg.NewID(box, uint32(data.UID))

	if len(replace) > 0 {
		if err := s.deleteAndExpunge(ctx, replace, box); err != nil {
			return id, fmt.Errorf("bozza salvata come %s ma la precedente non rimossa: %w", id, err)
		}
	}
	return id, nil
}

// DeleteDraft implementa mail.MailStore (Piano 9, Task 3). Sotto gate. Rifiuta
// senza agire se un ID sta fuori da Bozze o se manca UIDPLUS (come Trash).
func (s *Store) DeleteDraft(ctx context.Context, ids []msg.MessageID) error {
	defer s.acquire(ctx)()

	if err := ctx.Err(); err != nil {
		return err
	}
	c, err := s.connected()
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	box, err := draftsBoxOf(c)
	if err != nil {
		return err
	}
	if err := checkInMailbox(box, ids); err != nil {
		return err
	}
	if !hasUIDPlus(c) {
		return fmt.Errorf("imap bozze: il server non supporta UIDPLUS: rifiuto")
	}
	return s.deleteAndExpunge(ctx, ids, box)
}

// draftsBoxOf restituisce il nome della cartella con ruolo drafts (chiamare sotto gate).
func draftsBoxOf(c *imapclient.Client) (string, error) {
	boxes, err := listMailboxes(c)
	if err != nil {
		return "", err
	}
	for _, b := range boxes {
		if b.Role == "drafts" {
			return b.Name, nil
		}
	}
	return "", fmt.Errorf("imap bozze: cartella Bozze non trovata (nessuna mailbox con ruolo drafts)")
}

// checkInMailbox verifica che ogni ID stia nella mailbox box. Nessun effetto.
func checkInMailbox(box string, ids []msg.MessageID) error {
	for _, id := range ids {
		mailbox, _, err := id.Split()
		if err != nil {
			return fmt.Errorf("imap bozze: %w", err)
		}
		if mailbox != box {
			return fmt.Errorf("imap bozze: %s fuori da %q: rifiuto senza toccare il server", id, box)
		}
	}
	return nil
}

// hasUIDPlus dice se il server offre la rimozione per UID (UIDPLUS o IMAP4rev2).
func hasUIDPlus(c *imapclient.Client) bool {
	caps := c.Caps()
	return caps.Has(imap.CapUIDPlus) || caps.Has(imap.CapIMAP4rev2)
}
