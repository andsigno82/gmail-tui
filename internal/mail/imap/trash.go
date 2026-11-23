package imap

// Trash (Piano 7, Task 1): spostamento nel cestino via UID MOVE (o, senza MOVE,
// COPY + STORE + UID EXPUNGE). Mai EXPUNGE generico (AGENTS.md).

import (
	"context"
	"fmt"

	"github.com/emersion/go-imap/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Trash implementa mail.MailStore (Piano 7, Task 1). Sotto gate. Rifiuta PRIMA
// di toccare il server (nessuna azione in caso di rifiuto):
//   - nessuna mailbox con ruolo "trash" (il nome localizzato non conta);
//   - almeno un id già nel cestino (nessuna azione sugli altri id);
//   - server senza MOVE né UIDPLUS: il fallback di Client.Move in beta.9 usa
//     EXPUNGE generico, che espungerebbe anche messaggi \Deleted non nostri.
//
// Cache: nessuna invalidazione. La chiave di cache è mailbox+uid: la entry
// d'origine resta orfana e innocua, e il messaggio nel cestino ha un nuovo ID.
//
// Un errore a metà (SELECT/MOVE di un gruppo) lascia già spostati i gruppi
// precedenti: IMAP non offre transazioni, il refresh della UI rilegge lo stato.
// Piano 10, Task 5: restituisce gli ID dei messaggi nel cestino (D10-T2), dai UID di
// destinazione del MOVE (MoveData.DestUIDs, COPYUID). Senza UID dal server la lista
// è vuota: lo spostamento è avvenuto, ma non c'è l'annulla per quei messaggi.
func (s *Store) Trash(ctx context.Context, ids []msg.MessageID) ([]msg.MessageID, error) {
	var out []msg.MessageID
	if err := s.trashMove(ctx, ids, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// trashMove è il corpo di Trash: scrive in out gli ID nel cestino man mano che ogni
// gruppo di mailbox viene spostato.
func (s *Store) trashMove(ctx context.Context, ids []msg.MessageID, out *[]msg.MessageID) error {
	if err := s.refuseAllMail(ctx, ids, "cestina"); err != nil {
		return err
	}
	defer s.acquire(ctx)() // gate: SELECT/MOVE rompono l'IDLE; listMailboxes sotto gate

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

	boxes, err := listMailboxes(c)
	if err != nil {
		return err
	}
	trash, ok := trashBox(boxes)
	if !ok {
		return fmt.Errorf("imap trash: cestino non trovato (nessuna mailbox con ruolo trash, cfr. \\Trash)")
	}

	byMbox, err := groupByMailbox(ids)
	if err != nil {
		return err
	}
	if uids, inTrash := byMbox[trash]; inTrash {
		return fmt.Errorf("imap trash: %d messaggi già nel cestino %q: rifiuto senza toccare il server", len(uids), trash)
	}

	if caps := c.Caps(); !caps.Has(imap.CapMove) && !caps.Has(imap.CapUIDPlus) {
		return fmt.Errorf("imap trash: il server non supporta MOVE né UIDPLUS: rifiuto (il fallback userebbe EXPUNGE generico)")
	}

	for mailbox, uids := range byMbox {
		if _, err := c.Select(mailbox, nil).Wait(); err != nil {
			return fmt.Errorf("imap select %s: %w", mailbox, err)
		}
		data, err := c.Move(imap.UIDSetNum(uids...), trash).Wait()
		if err != nil {
			return fmt.Errorf("imap move %s → %s: %w", mailbox, trash, err)
		}
		*out = append(*out, destIDs(trash, data.DestUIDs)...)
	}
	return nil
}

// trashBox restituisce il nome della mailbox con ruolo "trash".
func trashBox(boxes []mail.Mailbox) (string, bool) {
	for _, b := range boxes {
		if b.Role == "trash" {
			return b.Name, true
		}
	}
	return "", false
}
