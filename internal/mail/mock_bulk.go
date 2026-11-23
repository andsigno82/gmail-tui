package mail

// Piano 10, Task 4: Mock di Restore, AllMailIDs e ModifyLabels (D10-T3..T5),
// con la semantica del backend IMAP: Gmail (NoGmail = false) cambia le label su
// tutte le copie dello stesso GmailID; senza Gmail le label sono cartelle.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// errMockNoGmail è il rifiuto delle op Gmail-only sul Mock con NoGmail.
var errMockNoGmail = errors.New("mock: estensioni Gmail (X-GM-EXT-1) non disponibili")

// boxByRole restituisce il nome della mailbox con il ruolo indicato.
func boxByRole(boxes []Mailbox, role string) (string, bool) {
	for _, b := range boxes {
		if b.Role == role {
			return b.Name, true
		}
	}
	return "", false
}

// hasMailbox dice se la cartella (o label) esiste nell'elenco.
func hasMailbox(boxes []Mailbox, name string) bool {
	for _, b := range boxes {
		if b.Name == name {
			return true
		}
	}
	return false
}

// hasString dice se list contiene s.
func hasString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// findAll restituisce i messaggi degli id, senza duplicati e nell'ordine dato;
// errore al primo id inesistente.
func (m *Mock) findAll(ids []msg.MessageID) ([]*msg.Message, error) {
	seen := make(map[msg.MessageID]bool, len(ids))
	out := make([]*msg.Message, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		mm := m.find(id)
		if mm == nil {
			return nil, fmt.Errorf("messaggio %s non trovato: %w", id, ErrMessageNotFound)
		}
		out = append(out, mm)
	}
	return out, nil
}

// appendCopy aggiunge a box una copia di mm con un nuovo UID (come una COPY IMAP).
func (m *Mock) appendCopy(mm *msg.Message, box string) *msg.Message {
	cp := *mm
	cp.Ref.ID = msg.NewID(box, m.nextUID(box))
	cp.Ref.Labels = append([]string(nil), mm.Ref.Labels...)
	cp.To = append([]string(nil), mm.To...)
	cp.Cc = append([]string(nil), mm.Cc...)
	cp.Attachments = append([]msg.Attachment(nil), mm.Attachments...)
	m.Msgs = append(m.Msgs, &cp)
	return &cp
}

// gmailCopies restituisce il messaggio e tutte le sue copie (stesso GmailID).
func (m *Mock) gmailCopies(mm *msg.Message) []*msg.Message {
	if mm.Ref.GmailID == 0 {
		return []*msg.Message{mm}
	}
	var out []*msg.Message
	for _, c := range m.Msgs {
		if c.Ref.GmailID == mm.Ref.GmailID {
			out = append(out, c)
		}
	}
	return out
}

// gmailCopyIn restituisce la copia con quel GmailID nella cartella, o nil.
func (m *Mock) gmailCopyIn(gid uint64, box string) *msg.Message {
	for _, c := range m.Msgs {
		if c.Ref.GmailID == gid && inMailbox(c.Ref.ID, box) {
			return c
		}
	}
	return nil
}

// notify segnala un cambio ai watcher (best-effort, come le altre op del Mock).
func (m *Mock) notify(ids []msg.MessageID) {
	m.init()
	select {
	case m.changes <- Change{Kind: ChangeFlags, IDs: ids}:
	default:
	}
}

// Restore implementa MailStore (Piano 10, Task 4, D10-T3): riporta i messaggi dal
// cestino a dest con un nuovo UID. Rifiuta senza agire se un ID non sta nel cestino
// o se dest non esiste (o è il cestino stesso).
func (m *Mock) Restore(_ context.Context, ids []msg.MessageID, dest string) ([]msg.MessageID, error) {
	ids = append([]msg.MessageID(nil), ids...) // copia: il chiamante può riusare la slice
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	boxes := m.mailboxes()
	trash, ok := trashBox(boxes)
	if !ok {
		return nil, fmt.Errorf("cestino non trovato")
	}
	if dest == trash || !hasMailbox(boxes, dest) {
		return nil, fmt.Errorf("cartella di ritorno %q non valida", dest)
	}
	for _, id := range ids {
		if !inMailbox(id, trash) {
			return nil, fmt.Errorf("messaggio %s non è nel cestino: rifiuto senza azioni", id)
		}
	}
	targets, err := m.findAll(ids)
	if err != nil {
		return nil, err
	}
	next := m.nextUID(dest)
	out := make([]msg.MessageID, 0, len(targets))
	for _, mm := range targets {
		mm.Ref.ID = msg.NewID(dest, next)
		out = append(out, mm.Ref.ID)
		next++
	}
	m.notify(ids)
	return out, nil
}

// AllMailIDs implementa MailStore (Piano 10, Task 4, D10-T4): per ogni ID in INBOX,
// l'ID della copia con lo stesso GmailID in Tutti i messaggi. Senza Gmail errore.
func (m *Mock) AllMailIDs(_ context.Context, ids []msg.MessageID) ([]msg.MessageID, error) {
	ids = append([]msg.MessageID(nil), ids...) // copia: il chiamante può riusare la slice
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	if m.NoGmail {
		return nil, errMockNoGmail
	}
	allMail, ok := boxByRole(m.mailboxes(), "all")
	if !ok {
		return nil, fmt.Errorf("cartella Tutti i messaggi non trovata")
	}
	targets, err := m.findAll(ids)
	if err != nil {
		return nil, err
	}
	out := make([]msg.MessageID, 0, len(targets))
	for _, mm := range targets {
		if !inMailbox(mm.Ref.ID, "INBOX") {
			return nil, fmt.Errorf("messaggio %s non è in INBOX: rifiuto", mm.Ref.ID)
		}
		if mm.Ref.GmailID == 0 {
			return nil, fmt.Errorf("messaggio %s senza ID Gmail", mm.Ref.ID)
		}
		cp := m.gmailCopyIn(mm.Ref.GmailID, allMail)
		if cp == nil {
			return nil, fmt.Errorf("copia di %s in %q non trovata: %w", mm.Ref.ID, allMail, ErrMessageNotFound)
		}
		out = append(out, cp.Ref.ID)
	}
	return out, nil
}

// ModifyLabels implementa MailStore (Piano 10, Task 4, D10-T5). Vedi il contratto
// in store.go. Con Gmail le label cambiano su tutte le copie del messaggio; \Inbox
// aggiunto riporta una copia in INBOX, rimosso ne toglie le copie in INBOX.
func (m *Mock) ModifyLabels(_ context.Context, ids []msg.MessageID, add, remove []string) error {
	ids = append([]msg.MessageID(nil), ids...) // copia: il chiamante può riusare la slice
	add = append([]string(nil), add...)
	remove = append([]string(nil), remove...)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	if err := checkMockLabels(add, remove, m.NoGmail); err != nil {
		return err
	}
	targets, err := m.findAll(ids)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}
	if m.NoGmail {
		err = m.modifyFolderLabels(targets, add, remove)
	} else {
		err = m.modifyGmailLabels(targets, add, remove)
	}
	if err != nil {
		return err
	}
	m.notify(ids)
	return nil
}

// checkMockLabels rifiuta label vuote, attributi di sistema (salvo \Inbox con Gmail)
// e la stessa label in add e remove. Nessuna modifica: prima di ogni azione.
func checkMockLabels(add, remove []string, noGmail bool) error {
	for _, l := range append(append([]string(nil), add...), remove...) {
		if l == "" {
			return fmt.Errorf("label vuota")
		}
		if strings.HasPrefix(l, `\`) && (noGmail || l != `\Inbox`) {
			return fmt.Errorf("attributo di sistema %q non modificabile come label", l)
		}
	}
	for _, a := range add {
		if hasString(remove, a) {
			return fmt.Errorf("label %q sia in add sia in remove", a)
		}
	}
	return nil
}

// modifyGmailLabels applica add/remove sulle label Gmail di tutte le copie.
func (m *Mock) modifyGmailLabels(targets []*msg.Message, add, remove []string) error {
	inboxAdd := hasString(add, `\Inbox`)
	inboxDel := hasString(remove, `\Inbox`)
	if inboxAdd || inboxDel {
		for _, mm := range targets {
			if mm.Ref.GmailID == 0 {
				return fmt.Errorf("messaggio %s senza ID Gmail: \\Inbox non applicabile", mm.Ref.ID)
			}
		}
	}
	for _, mm := range targets {
		for _, c := range m.gmailCopies(mm) {
			c.Ref.Labels = applyUserLabels(c.Ref.Labels, add, remove)
		}
		gid := mm.Ref.GmailID
		if inboxAdd && m.gmailCopyIn(gid, "INBOX") == nil {
			m.appendCopy(mm, "INBOX")
		}
		if inboxDel {
			kept := make([]*msg.Message, 0, len(m.Msgs))
			for _, c := range m.Msgs {
				if c.Ref.GmailID == gid && inMailbox(c.Ref.ID, "INBOX") {
					continue
				}
				kept = append(kept, c)
			}
			m.Msgs = kept
		}
	}
	return nil
}

// applyUserLabels toglie remove e aggiunge le label utente di add (gli attributi di
// sistema, come \Inbox, non sono label utente e non entrano nella lista).
func applyUserLabels(labels, add, remove []string) []string {
	out := make([]string, 0, len(labels)+len(add))
	for _, l := range labels {
		if !hasString(remove, l) && !hasString(out, l) {
			out = append(out, l)
		}
	}
	for _, a := range add {
		if !msg.IsUserLabel(a) || hasString(out, a) {
			continue
		}
		out = append(out, a)
	}
	return out
}

// modifyFolderLabels applica add/remove senza Gmail: add copia nella label (creata
// se manca), remove toglie il messaggio dalla label. Ogni target deve stare in
// ogni label rimossa: controllato prima di toccare il Mock.
func (m *Mock) modifyFolderLabels(targets []*msg.Message, add, remove []string) error {
	for _, r := range remove {
		for _, mm := range targets {
			if !inMailbox(mm.Ref.ID, r) {
				return fmt.Errorf("messaggio %s non è in %q: rimozione rifiutata senza toccare il server", mm.Ref.ID, r)
			}
		}
	}
	for _, a := range add {
		if !hasMailbox(m.mailboxes(), a) {
			boxes := append([]Mailbox(nil), m.mailboxes()...)
			m.Mailboxes = append(boxes, Mailbox{Name: a})
		}
		for _, mm := range targets {
			if !inMailbox(mm.Ref.ID, a) {
				m.appendCopy(mm, a)
			}
		}
	}
	if len(remove) > 0 {
		drop := make(map[msg.MessageID]bool, len(targets))
		for _, mm := range targets {
			for _, r := range remove {
				if inMailbox(mm.Ref.ID, r) {
					drop[mm.Ref.ID] = true
				}
			}
		}
		kept := make([]*msg.Message, 0, len(m.Msgs))
		for _, c := range m.Msgs {
			if !drop[c.Ref.ID] {
				kept = append(kept, c)
			}
		}
		m.Msgs = kept
	}
	return nil
}
