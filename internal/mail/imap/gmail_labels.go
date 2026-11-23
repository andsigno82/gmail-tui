package imap

// Piano 10, Task 5: label e annulla di archivia (D10-T4, D10-T5). Con X-GM-EXT-1,
// AllMailIDs legge X-GM-MSGID in INBOX e cerca lo stesso messaggio in Tutti i messaggi;
// ModifyLabels usa STORE ±X-GM-LABELS (StoreGmailLabels della patch, PATCHES.md).
// Senza X-GM-EXT-1 le label sono cartelle: add copia nella label, remove espunge dalla
// label (ogni id deve stare nella label). Mai EXPUNGE generico.

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf16"

	"github.com/emersion/go-imap/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// AllMailIDs implementa mail.MailStore (Piano 10, Task 5, D10-T4). Per ogni ID in
// INBOX legge X-GM-MSGID e cerca lo stesso messaggio in Tutti i messaggi con SEARCH
// X-GM-MSGID. Solo Gmail; senza X-GM-EXT-1 errore prima di ogni comando. Gli ID fuori
// da INBOX sono rifiutati: l'annulla riporta in INBOX solo ciò che è stato archiviato.
func (s *Store) AllMailIDs(ctx context.Context, ids []msg.MessageID) ([]msg.MessageID, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !s.gmailOn() {
		return nil, errGmailNonDisponibile
	}
	uids := make([]imap.UID, 0, len(ids))
	for _, id := range ids {
		box, uid, err := id.Split()
		if err != nil || box != "INBOX" {
			return nil, fmt.Errorf("imap all mail: %s non è in INBOX: rifiuto", id)
		}
		uids = append(uids, imap.UID(uid))
	}
	if len(uids) == 0 {
		return nil, nil
	}

	allMail, err := s.allMailName(ctx) // prende il gate: va chiamata prima di acquire
	if err != nil {
		return nil, err
	}
	defer s.gate.acquire()()
	c, err := s.connected()
	if err != nil {
		return nil, err
	}
	if _, err := c.Select("INBOX", nil).Wait(); err != nil {
		return nil, fmt.Errorf("imap select INBOX: %w", err)
	}
	bufs, err := c.Fetch(imap.UIDSetNum(uids...), &imap.FetchOptions{UID: true, GmailMsgID: true}).Collect()
	if err != nil {
		return nil, fmt.Errorf("imap fetch X-GM-MSGID in INBOX: %w", err)
	}
	gmailIDs := make(map[imap.UID]uint64, len(bufs))
	for _, b := range bufs {
		gmailIDs[b.UID] = b.GmailMsgID
	}

	if _, err := c.Select(allMail, nil).Wait(); err != nil {
		return nil, fmt.Errorf("imap select %s: %w", allMail, err)
	}
	out := make([]msg.MessageID, 0, len(uids))
	for _, uid := range uids {
		gid := gmailIDs[uid]
		if gid == 0 {
			return nil, fmt.Errorf("imap all mail: INBOX:%d senza X-GM-MSGID", uid)
		}
		data, err := c.UIDSearch(&imap.SearchCriteria{Gmail: &imap.SearchCriteriaGmail{MsgID: gid}}, nil).Wait()
		if err != nil {
			return nil, fmt.Errorf("imap search X-GM-MSGID %d in %s: %w", gid, allMail, err)
		}
		found := data.AllUIDs()
		switch len(found) {
		case 0:
			return nil, fmt.Errorf("messaggio %d non in %q: %w", gid, allMail, mail.ErrMessageNotFound)
		case 1:
			out = append(out, msg.NewID(allMail, uint32(found[0])))
		default:
			return nil, fmt.Errorf("imap all mail: %d copie del messaggio %d in %q (attesa 1)", len(found), gid, allMail)
		}
	}
	return out, nil
}

// ModifyLabels implementa mail.MailStore (Piano 10, Task 5, D10-T5). Rifiuta prima di
// toccare il server: label vuote o attributi di sistema (salvo \Inbox con Gmail), la
// stessa label in add e remove, e (senza Gmail) id che non stanno nelle label rimosse.
func (s *Store) ModifyLabels(ctx context.Context, ids []msg.MessageID, add, remove []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	gmail := s.gmailOn()
	if err := checkLabelSet(add, remove, gmail); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	if gmail {
		return s.modifyGmailLabels(ids, add, remove)
	}
	return s.modifyFolderLabels(ctx, ids, add, remove)
}

// checkLabelSet valida add e remove (nessuna azione in caso di errore).
func checkLabelSet(add, remove []string, gmail bool) error {
	for _, l := range append(append([]string(nil), add...), remove...) {
		if l == "" {
			return fmt.Errorf("imap ModifyLabels: label vuota")
		}
		if strings.HasPrefix(l, `\`) && (!gmail || l != `\Inbox`) {
			return fmt.Errorf("imap ModifyLabels: attributo di sistema %q non modificabile come label", l)
		}
	}
	for _, a := range add {
		for _, r := range remove {
			if a == r {
				return fmt.Errorf("imap ModifyLabels: label %q sia in add sia in remove", a)
			}
		}
	}
	return nil
}

// modifyGmailLabels invia STORE +/-X-GM-LABELS (SILENT) per mailbox di appartenenza
// degli id. È sicuro anche su Tutti i messaggi: cambia solo le label.
func (s *Store) modifyGmailLabels(ids []msg.MessageID, add, remove []string) error {
	byMbox, err := groupByMailbox(ids)
	if err != nil {
		return err
	}
	encAdd := encodeLabels(add)
	encRemove := encodeLabels(remove)

	defer s.gate.acquire()()
	c, err := s.connected()
	if err != nil {
		return err
	}
	// Piano 10, review punto 4: con errore a metà (es. il STORE di rimozione rifiutato
	// dopo quello di aggiunta riuscito) la cache segue la parte che il server ha
	// applicato: aggiunte e rimozioni si registrano a parte, per mailbox.
	var addOK, remOK []msg.MessageID
	err = func() error {
		for mailbox, uids := range byMbox {
			if _, err := c.Select(mailbox, nil).Wait(); err != nil {
				return fmt.Errorf("imap select %s: %w", mailbox, err)
			}
			if len(encAdd) > 0 {
				cmd := c.StoreGmailLabels(imap.UIDSetNum(uids...), &imap.StoreGmailLabels{Op: imap.StoreFlagsAdd, Silent: true, Labels: encAdd})
				if _, err := cmd.Collect(); err != nil {
					return fmt.Errorf("imap store +X-GM-LABELS in %s: %w", mailbox, err)
				}
				addOK = append(addOK, idsIn(ids, mailbox)...)
			}
			if len(encRemove) > 0 {
				cmd := c.StoreGmailLabels(imap.UIDSetNum(uids...), &imap.StoreGmailLabels{Op: imap.StoreFlagsDel, Silent: true, Labels: encRemove})
				if _, err := cmd.Collect(); err != nil {
					return fmt.Errorf("imap store -X-GM-LABELS in %s: %w", mailbox, err)
				}
				remOK = append(remOK, idsIn(ids, mailbox)...)
			}
		}
		return nil
	}()
	if cerr := s.updateCachedLabelSet(addOK, add, nil); cerr != nil && err == nil {
		err = cerr
	}
	if cerr := s.updateCachedLabelSet(remOK, nil, remove); cerr != nil && err == nil {
		err = cerr
	}
	return err
}

// idsIn restituisce gli ID che stanno nella mailbox data, nell'ordine dato.
func idsIn(ids []msg.MessageID, mailbox string) []msg.MessageID {
	var out []msg.MessageID
	for _, id := range ids {
		if box, _, err := id.Split(); err == nil && box == mailbox {
			out = append(out, id)
		}
	}
	return out
}

// modifyFolderLabels (senza Gmail): la rimozione richiede che ogni id stia nella label
// (controllo prima di toccare il server, come ApplyLabel off); l'aggiunta copia nella
// label, creata se manca, saltando gli id già nella label.
func (s *Store) modifyFolderLabels(ctx context.Context, ids []msg.MessageID, add, remove []string) error {
	for _, r := range remove {
		for _, id := range ids {
			if box, _, err := id.Split(); err != nil || box != r {
				return fmt.Errorf("imap ModifyLabels: %s non è nella label %q: rifiuto senza toccare il server", id, r)
			}
		}
	}
	byMbox, err := groupByMailbox(ids)
	if err != nil {
		return err
	}

	defer s.gate.acquire()()
	c, err := s.connected()
	if err != nil {
		return err
	}
	for _, a := range add {
		if err := ensureMailbox(c, a); err != nil {
			return err
		}
		for mailbox, uids := range byMbox {
			if mailbox == a {
				continue
			}
			if _, err := c.Select(mailbox, nil).Wait(); err != nil {
				return fmt.Errorf("imap select %s: %w", mailbox, err)
			}
			if _, err := c.Copy(imap.UIDSetNum(uids...), a).Wait(); err != nil {
				return fmt.Errorf("imap copy %s → %s: %w", mailbox, a, err)
			}
		}
	}
	for _, r := range remove {
		if err := s.deleteAndExpunge(ctx, ids, r); err != nil {
			return err
		}
	}
	return s.updateCachedLabelSet(ids, add, remove)
}

// updateCachedLabelSet mantiene la cache coerente con ModifyLabels (D10-C5): le label
// utente entrano o escono dalle copie per GmailID delle entry già in cache (come
// updateCachedLabels). Gli attributi di sistema (\Inbox) non sono label utente.
func (s *Store) updateCachedLabelSet(ids []msg.MessageID, add, remove []string) error {
	if s.opts.Cache == nil {
		return nil
	}
	userAdd := msg.UserLabels(add)
	userRemove := msg.UserLabels(remove)
	mutate := func(c *msg.Message) bool {
		labels := c.Ref.Labels
		for _, r := range userRemove {
			labels = withLabel(labels, r, false)
		}
		for _, a := range userAdd {
			labels = withLabel(labels, a, true)
		}
		c.Ref.Labels = labels
		return true
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
		if !ok || m.Ref.GmailID == 0 {
			continue
		}
		if err := s.opts.Cache.UpdateGmailCopies(m.Ref.GmailID, mutate); err != nil {
			return fmt.Errorf("cache copie %s: %w", id, err)
		}
	}
	return nil
}

// encodeLabels codifica i nomi label in modified UTF-7 per il wire (X-GM-LABELS); gli
// attributi di sistema restano invariati (sono ASCII).
func encodeLabels(labels []string) []string {
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		out = append(out, encodeModifiedUTF7(l))
	}
	return out
}

// encodeModifiedUTF7 codifica in modified UTF-7 (RFC 3501 §5.1.3, il duale di
// decodeModifiedUTF7 nella patch go-imap): i caratteri ASCII stampabili restano tali,
// '&' diventa "&-", le sequenze non ASCII sono UTF-16 in base64 con ',' al posto di '/'
// racchiuse tra '&' e '-'.
func encodeModifiedUTF7(s string) string {
	var out strings.Builder
	var pending []uint16
	flush := func() {
		if len(pending) == 0 {
			return
		}
		raw := make([]byte, 0, 2*len(pending))
		for _, u := range pending {
			raw = append(raw, byte(u>>8), byte(u))
		}
		out.WriteString("&" + strings.ReplaceAll(base64.RawStdEncoding.EncodeToString(raw), "/", ",") + "-")
		pending = pending[:0]
	}
	for _, r := range s {
		if r >= 0x20 && r <= 0x7e {
			flush()
			if r == '&' {
				out.WriteString("&-")
			} else {
				out.WriteRune(r)
			}
			continue
		}
		pending = append(pending, utf16.Encode([]rune{r})...)
	}
	flush()
	return out.String()
}
