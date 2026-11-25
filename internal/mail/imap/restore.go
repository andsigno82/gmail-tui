package imap

// Piano 10, Task 5: Restore riporta i messaggi dal cestino alla cartella d'origine con
// MOVE e UID (D10-T3). Guard prima di ogni comando: ogni ID deve stare nella mailbox
// con ruolo "trash", la destinazione deve esistere e non essere il cestino. Mai EXPUNGE.

import (
	"context"
	"fmt"

	"github.com/emersion/go-imap/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Restore implementa mail.MailStore (Piano 10, Task 5, D10-T3). Sotto gate. Rifiuta
// PRIMA di toccare il server: id fuori dal cestino, destinazione assente o uguale al
// cestino, server senza MOVE né UIDPLUS. Le label perse col cestino non tornano qui:
// il chiamante le riapplica con ModifyLabels (D10-C2).
func (s *Store) Restore(ctx context.Context, ids []msg.MessageID, dest string) ([]msg.MessageID, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	defer s.acquire(ctx)() // gate: SELECT/MOVE rompono l'IDLE; listMailboxes sotto gate

	c, err := s.connected()
	if err != nil {
		return nil, err
	}
	boxes, err := listMailboxes(ctx, c)
	if err != nil {
		return nil, err
	}
	trash, ok := trashBox(boxes)
	if !ok {
		return nil, fmt.Errorf("imap restore: cestino non trovato (nessuna mailbox con ruolo trash)")
	}
	if dest == "" || dest == trash || !mailboxListed(boxes, dest) {
		return nil, fmt.Errorf("imap restore: cartella di ritorno %q non valida", dest)
	}
	byMbox, err := groupByMailbox(ids)
	if err != nil {
		return nil, err
	}
	for mailbox := range byMbox {
		if mailbox != trash {
			return nil, fmt.Errorf("imap restore: %q non sta nel cestino %q: rifiuto senza toccare il server", mailbox, trash)
		}
	}
	if caps := c.Caps(); !caps.Has(imap.CapMove) && !caps.Has(imap.CapUIDPlus) {
		return nil, fmt.Errorf("imap restore: il server non supporta MOVE né UIDPLUS: rifiuto")
	}

	if _, err := c.Select(trash, nil).Wait(); err != nil {
		return nil, fmt.Errorf("imap select %s: %w", trash, err)
	}
	data, err := c.Move(imap.UIDSetNum(byMbox[trash]...), dest).Wait()
	if err != nil {
		return nil, fmt.Errorf("imap move %s → %s: %w", trash, dest, err)
	}
	return destIDs(dest, data.DestUIDs), nil
}

// mailboxListed dice se una mailbox con quel nome è nell'elenco LIST.
func mailboxListed(boxes []mail.Mailbox, name string) bool {
	for _, b := range boxes {
		if b.Name == name {
			return true
		}
	}
	return false
}

// destIDs converte i UID di destinazione di un MOVE/COPY (COPYUID) in ID nella
// mailbox. Senza un insieme enumerabile (nil o con "*", che il server non dovrebbe
// inviare) restituisce nil: nessun annulla, ma lo spostamento è avvenuto.
func destIDs(mailbox string, nums imap.NumSet) []msg.MessageID {
	set, ok := nums.(imap.UIDSet)
	if !ok {
		return nil
	}
	uids, ok := set.Nums()
	if !ok {
		return nil
	}
	out := make([]msg.MessageID, 0, len(uids))
	for _, uid := range uids {
		out = append(out, msg.NewID(mailbox, uint32(uid)))
	}
	return out
}
