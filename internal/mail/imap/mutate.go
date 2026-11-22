package imap

// Mutazioni (piano 2, Task 5): MarkRead, Star, Archive, ApplyLabel.
// Le API go-imap v2 (beta.9) non prendono context: come per Connect, i
// metodi controllano solo ctx.Err() in ingresso.

import (
	"context"
	"fmt"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// MarkRead marca i messaggi come letti (read=true: UID STORE ADD \Seen) o
// come non letti (false: REMOVE), sulla mailbox di appartenenza di ogni id.
func (s *Store) MarkRead(ctx context.Context, ids []msg.MessageID, read bool) error {
	defer s.gate.acquire()() // gate (fix review): STORE/EXPUNGE rompono l'IDLE

	op := imap.StoreFlagsAdd
	if !read {
		op = imap.StoreFlagsDel
	}
	return s.storeFlags(ctx, ids, []imap.Flag{imap.FlagSeen}, op,
		func(m *msg.Message) { m.Ref.Unread = !read })
}

// Star aggiunge (on=true: UID STORE ADD \Flagged) o rimuove la stella,
// sulla mailbox di appartenenza di ogni id.
func (s *Store) Star(ctx context.Context, ids []msg.MessageID, on bool) error {
	defer s.gate.acquire()() // gate (fix review): STORE rompe l'IDLE del Watch

	op := imap.StoreFlagsAdd
	if !on {
		op = imap.StoreFlagsDel
	}
	return s.storeFlags(ctx, ids, []imap.Flag{imap.FlagFlagged}, op,
		func(m *msg.Message) { m.Ref.Starred = on })
}

// Archive archivia i messaggi. Semantica Gmail via IMAP: per la mailbox di
// ogni id, UID STORE ADD \Deleted + UID EXPUNGE dei soli uid — l'expunge
// selettivo DALLA cartella equivale alla rimozione della label
// (= archiviazione da INBOX); su Gmail il messaggio resta in All Mail
// (nel memserver dei test l'expunge lo rimuove fisicamente dalla mailbox:
// il comportamento IMAP verificato dai test è proprio questo).
func (s *Store) Archive(ctx context.Context, ids []msg.MessageID) error {
	if err := s.refuseAllMail(ctx, ids, "archivia"); err != nil {
		return err
	}
	defer s.gate.acquire()() // gate (fix review): STORE/EXPUNGE rompono l'IDLE

	return s.deleteAndExpunge(ctx, ids, "")
}

// ApplyLabel gestisce una label Gmail mappata su cartella IMAP.
//
//	on=true:  COPY degli id nella cartella-label; se la cartella non esiste
//	          viene creata con CREATE prima del copy (su Gmail creare la
//	          cartella crea la label);
//	on=false: SELECT della cartella-label, UID STORE ADD \Deleted + UID
//	          EXPUNGE sugli uid degli id (si intende che gli id referenzino
//	          la mail NELLA cartella-label, come restituiti da un list lì:
//	          il uid assegnato dal COPY è quello della copia in label).
//
// Gate (fix review): acquire copre entrambi i rami (COPY + CREATE, e la
// deleteAndExpunge del ramo off) — LIST/CREATE/COPY/STORE romperebbero
// l'IDLE del Watch.
func (s *Store) ApplyLabel(ctx context.Context, ids []msg.MessageID, label string, on bool) error {
	defer s.gate.acquire()()

	if err := ctx.Err(); err != nil {
		return err
	}
	c, err := s.connected()
	if err != nil {
		return err
	}
	if label == "" {
		return fmt.Errorf("imap ApplyLabel: label vuota")
	}

	if on {
		if err := ensureMailbox(c, label); err != nil {
			return err
		}
		byMbox, err := groupByMailbox(ids)
		if err != nil {
			return err
		}
		for mailbox, uids := range byMbox {
			if _, err := c.Select(mailbox, nil).Wait(); err != nil {
				return fmt.Errorf("imap select %s: %w", mailbox, err)
			}
			// COPY nella cartella-label = aggiunge la label (semantica Gmail).
			if _, err := c.Copy(imap.UIDSetNum(uids...), label).Wait(); err != nil {
				return fmt.Errorf("imap copy %s → %s: %w", mailbox, label, err)
			}
		}
		if err := s.updateCachedLabels(ids, label, on); err != nil {
			return err
		}
		return nil
	}

	// off = rimozione dalla cartella-label: espunge gli uid degli id dalla
	// mailbox-label (cfr. Archive per la semantica label/expunge).
	// Guard (fix review, anti data-loss): gli id DEVONO referenziare la
	// mailbox-label (come restituiti da un list lì); un id di un'altra
	// mailbox verrebbe espunto con il suo uid DENTRO la mailbox-label — o
	// peggio, colpirebbe il messaggio sbagliato. Rifiuta PRIMA di toccare
	// il server.
	for _, id := range ids {
		mailbox, _, err := id.Split()
		if err != nil {
			return fmt.Errorf("imap ApplyLabel off: %w", err)
		}
		if mailbox != label {
			return fmt.Errorf(
				"imap ApplyLabel off: id %s è nella mailbox %q, non nella mailbox-label %q: rifiuto senza toccare il server",
				id, mailbox, label)
		}
	}
	return s.deleteAndExpunge(ctx, ids, label)
}

// deleteAndExpunge esegue, per ogni uid: UID STORE ADD \Deleted + UID
// EXPUNGE. Se label è "" la SELECT usa la mailbox di appartenenza di ogni
// id (Archive); altrimenti si seleziona sempre la mailbox label e vi si
// applicano gli uid degli id (ApplyLabel off).
func (s *Store) deleteAndExpunge(ctx context.Context, ids []msg.MessageID, label string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c, err := s.connected()
	if err != nil {
		return err
	}

	batches := make(map[string][]imap.UID)
	if label == "" {
		batches, err = groupByMailbox(ids)
		if err != nil {
			return err
		}
	} else {
		for _, id := range ids {
			_, uid, err := id.Split()
			if err != nil {
				return fmt.Errorf("imap ApplyLabel off: %w", err)
			}
			batches[label] = append(batches[label], imap.UID(uid))
		}
	}

	for mailbox, uids := range batches {
		if _, err := c.Select(mailbox, nil).Wait(); err != nil {
			return fmt.Errorf("imap select %s: %w", mailbox, err)
		}
		if err := s.uidStore(c, uids, imap.StoreFlagsAdd, imap.FlagDeleted); err != nil {
			return err
		}
		if err := s.uidExpunge(c, uids); err != nil {
			return err
		}
	}
	return nil
}

// storeFlags è il cuore di MarkRead/Star: per ogni mailbox di appartenenza
// degli id (batching: gli id possono riferire mailbox diverse) SELECT +
// UID STORE dell'operazione sul flag; dopo un STORE riuscito la copia in
// cache (se presente) viene aggiornata (Get → mutate → Set con la
// uidvalidity corrente) per non servire flag stantii al prossimo fetch
// cache-first: un miss è uno skip silenzioso.
func (s *Store) storeFlags(ctx context.Context, ids []msg.MessageID, flags []imap.Flag, op imap.StoreFlagsOp, cacheMutate func(*msg.Message)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c, err := s.connected()
	if err != nil {
		return err
	}
	byMbox, err := groupByMailbox(ids)
	if err != nil {
		return err
	}
	for mailbox, uids := range byMbox {
		sel, err := c.Select(mailbox, nil).Wait()
		if err != nil {
			return fmt.Errorf("imap select %s: %w", mailbox, err)
		}
		if err := s.uidStore(c, uids, op, flags...); err != nil {
			return err
		}
		if err := s.updateCachedFlags(mailbox, sel.UIDValidity, uids, cacheMutate); err != nil {
			return err
		}
	}
	return nil
}

// uidStore invia l'UID STORE (il kind UID è dedotto dal numset, come per
// FETCH in beta.9) e consuma la risposta: senza .SILENT il server risponde
// con FETCH non taggati dei flag aggiornati, Collect li drena.
func (s *Store) uidStore(c *imapclient.Client, uids []imap.UID, op imap.StoreFlagsOp, flags ...imap.Flag) error {
	if _, err := c.Store(imap.UIDSetNum(uids...), &imap.StoreFlags{Op: op, Flags: flags}, nil).Collect(); err != nil {
		return fmt.Errorf("imap uid store %v (%d uid): %w", flags, len(uids), err)
	}
	return nil
}

// uidExpunge invia UID EXPUNGE dei soli uid richiesti. Scelta del primitivo
// (decisione Task 5): beta.9 espone Client.UIDExpunge(imap.UIDSet) —
// richiede UIDPLUS o IMAP4rev2 — che Gmail supporta: è esattamente
// l'espunge selettivo serve. NON si usa EXPUNGE pieno dopo il marking
// (espungerebbe TUTTI i \Deleted della mailbox, anche non nostri).
func (s *Store) uidExpunge(c *imapclient.Client, uids []imap.UID) error {
	if _, err := c.UIDExpunge(imap.UIDSetNum(uids...)).Collect(); err != nil {
		return fmt.Errorf("imap uid expunge (%d uid): %w", len(uids), err)
	}
	return nil
}

// updateCachedFlags mantiene la cache coerente con un UID STORE riuscito.
func (s *Store) updateCachedFlags(mailbox string, uidValidity uint32, uids []imap.UID, mutate func(*msg.Message)) error {
	if s.opts.Cache == nil {
		return nil
	}
	for _, uid := range uids {
		id := msg.NewID(mailbox, uint32(uid))
		m, ok, err := s.opts.Cache.GetMessage(mailbox, uint32(uid))
		if err != nil {
			return fmt.Errorf("cache get %s: %w", id, err)
		}
		if !ok {
			continue // non in cache: nulla da aggiornare
		}
		mutate(m)
		if err := s.opts.Cache.SetMessage(mailbox, uidValidity, m); err != nil {
			return fmt.Errorf("cache set %s: %w", id, err)
		}
		// Piano 8, review punto 3: lo stesso messaggio Gmail sta anche in altre
		// mailbox (INBOX e Tutti i messaggi): ogni copia prende lo stesso cambio.
		if err := s.opts.Cache.UpdateGmailCopies(m.Ref.GmailID, func(c *msg.Message) bool {
			mutate(c)
			return true
		}); err != nil {
			return fmt.Errorf("cache copie %s: %w", id, err)
		}
	}
	return nil
}

// updateCachedLabels mantiene la cache coerente con ApplyLabel (Piano 8, review
// punto 3): la label entra o esce dai Ref.Labels di ogni copia dello stesso messaggio.
func (s *Store) updateCachedLabels(ids []msg.MessageID, label string, on bool) error {
	if s.opts.Cache == nil {
		return nil
	}
	mutate := func(m *msg.Message) {
		m.Ref.Labels = withLabel(m.Ref.Labels, label, on)
	}
	for _, id := range ids {
		box, uid, err := id.Split()
		if err != nil {
			continue
		}
		m, ok, err := s.opts.Cache.GetMessage(box, uid)
		if err != nil {
			return fmt.Errorf("cache get %s: %w", id, err)
		}
		if !ok {
			continue
		}
		if m.Ref.GmailID == 0 {
			continue // non Gmail: nessuna label Gmail da propagare
		}
		if err := s.opts.Cache.UpdateGmailCopies(m.Ref.GmailID, func(c *msg.Message) bool {
			mutate(c)
			return true
		}); err != nil {
			return fmt.Errorf("cache copie %s: %w", id, err)
		}
	}
	return nil
}

// withLabel aggiunge (on) o toglie label da labels, senza duplicati (idempotente).
func withLabel(labels []string, label string, on bool) []string {
	out := make([]string, 0, len(labels)+1)
	has := false
	for _, l := range labels {
		if l == label {
			has = true
			if !on {
				continue
			}
		}
		out = append(out, l)
	}
	if on && !has {
		out = append(out, label)
	}
	return out
}

// groupByMailbox raggruppa gli uid degli id per mailbox di appartenenza
// (il batching per mailbox permette una sola SELECT+STORE per mailbox).
func groupByMailbox(ids []msg.MessageID) (map[string][]imap.UID, error) {
	byMbox := make(map[string][]imap.UID)
	for _, id := range ids {
		mailbox, uid, err := id.Split()
		if err != nil {
			return nil, fmt.Errorf("imap mutazione: %w", err)
		}
		byMbox[mailbox] = append(byMbox[mailbox], imap.UID(uid))
	}
	return byMbox, nil
}

// ensureMailbox crea mailbox se non esiste già (controllo con LIST).
func ensureMailbox(c *imapclient.Client, mailbox string) error {
	data, err := c.List("", mailbox, nil).Collect()
	if err != nil {
		return fmt.Errorf("imap list %s: %w", mailbox, err)
	}
	for _, d := range data {
		if d.Mailbox == mailbox {
			return nil // esiste già
		}
	}
	if err := c.Create(mailbox, nil).Wait(); err != nil {
		return fmt.Errorf("imap create %s: %w", mailbox, err)
	}
	return nil
}
