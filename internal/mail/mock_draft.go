package mail

// Bozze del Mock (Piano 9, Task 2): SaveDraft e DeleteDraft con la stessa
// regola del backend IMAP: solo la mailbox con ruolo "drafts" e nessuna
// modifica se un ID non vi sta.

import (
	"bytes"
	"context"
	"fmt"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// draftsBox restituisce il nome della mailbox con ruolo drafts.
func draftsBox(boxes []Mailbox) (string, bool) {
	for _, b := range boxes {
		if b.Role == "drafts" {
			return b.Name, true
		}
	}
	return "", false
}

// checkInDrafts verifica che ogni ID stia nella cartella Bozze e che esista.
// Non modifica nulla: va chiamata prima di ogni scrittura.
func (m *Mock) checkInDrafts(box string, ids []msg.MessageID) error {
	for _, id := range ids {
		if !inMailbox(id, box) {
			return fmt.Errorf("bozza %s fuori da %q: rifiuto senza toccare nulla", id, box)
		}
		if m.find(id) == nil {
			return fmt.Errorf("bozza %s non trovata: %w", id, ErrMessageNotFound)
		}
	}
	return nil
}

// SaveDraft implementa MailStore (Piano 9, Task 2): appende raw nella cartella
// Bozze con un nuovo UID, poi rimuove le bozze di replace (solo se stanno in
// Bozze, validate prima).
func (m *Mock) SaveDraft(_ context.Context, raw []byte, replace []msg.MessageID) (msg.MessageID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return "", m.Err
	}
	box, ok := draftsBox(m.mailboxes())
	if !ok {
		return "", fmt.Errorf("bozze: nessuna mailbox con ruolo drafts")
	}
	if err := m.checkInDrafts(box, replace); err != nil {
		return "", err
	}
	id := msg.NewID(box, m.nextUID(box))
	parsed, err := msg.Parse(id, bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("bozza: %w", err)
	}
	m.Msgs = append(m.Msgs, parsed)
	m.removeIDs(replace)
	return id, nil
}

// DeleteDraft implementa MailStore (Piano 9, Task 2): rimuove le bozze indicate,
// dopo aver validato che stiano tutte in Bozze.
func (m *Mock) DeleteDraft(_ context.Context, ids []msg.MessageID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	box, ok := draftsBox(m.mailboxes())
	if !ok {
		return fmt.Errorf("bozze: nessuna mailbox con ruolo drafts")
	}
	if err := m.checkInDrafts(box, ids); err != nil {
		return err
	}
	m.removeIDs(ids)
	return nil
}

// removeIDs toglie dai messaggi del Mock quelli con gli ID dati (chiamare sotto mu).
func (m *Mock) removeIDs(ids []msg.MessageID) {
	drop := make(map[msg.MessageID]bool, len(ids))
	for _, id := range ids {
		drop[id] = true
	}
	kept := m.Msgs[:0]
	for _, mm := range m.Msgs {
		if !drop[mm.Ref.ID] {
			kept = append(kept, mm)
		}
	}
	m.Msgs = kept
}
