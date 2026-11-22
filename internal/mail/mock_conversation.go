package mail

// Piano 8, Task 5: conversazioni, thread e Raw simulati nel Mock. Semantica
// come il backend Gmail: una riga per thread; All Mail = tutti i messaggi
// escluso spam (ruolo junk) e cestino (ruolo trash).

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// ListConversations implementa MailStore: una riga per thread, dal più recente.
// La paginazione è sulle conversazioni (il Mock non ha il limite di messaggi
// letti del backend IMAP).
func (m *Mock) ListConversations(_ context.Context, q msg.Query, pg Page) ([]Conversation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	eff, err := effectiveQuery(q)
	if err != nil {
		return nil, err
	}
	mailbox := eff.Label
	if mailbox == "" {
		mailbox = "INBOX"
	}

	var heads []*msg.Message
	for _, mm := range m.Msgs {
		if inMailbox(mm.Ref.ID, mailbox) && matches(eff, mm) {
			heads = append(heads, mm)
		}
	}
	sort.SliceStable(heads, func(i, j int) bool { return heads[i].Ref.Date.After(heads[j].Ref.Date) })

	seen := make(map[string]bool)
	var convs []Conversation
	for _, h := range heads {
		key := convKey(h)
		if seen[key] {
			continue // la riga è già stata emessa con il messaggio più recente
		}
		seen[key] = true
		convs = append(convs, m.buildConversation(h))
	}

	if pg.Limit == 0 {
		pg.Limit = 50
	}
	if pg.Offset >= len(convs) {
		return nil, nil
	}
	end := pg.Offset + pg.Limit
	if end > len(convs) {
		end = len(convs)
	}
	return convs[pg.Offset:end], nil
}

// ConversationMembers implementa MailStore (Piano 8, Task 9b): gli ID dei
// messaggi del thread nella sola mailbox indicata.
func (m *Mock) ConversationMembers(_ context.Context, mailbox string, threadID uint64) ([]msg.MessageID, error) {
	if threadID == 0 {
		return nil, errors.New("mock: thread 0 non valido")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	var out []msg.MessageID
	for _, mm := range m.Msgs {
		if mm.Ref.ThreadID == threadID && inMailbox(mm.Ref.ID, mailbox) {
			out = append(out, mm.Ref.ID)
		}
	}
	return out, nil
}

// ThreadMessages implementa MailStore: i messaggi del thread, cronologici,
// senza spam e cestino.
func (m *Mock) ThreadMessages(_ context.Context, threadID uint64) ([]msg.MessageRef, error) {
	if threadID == 0 {
		return nil, errors.New("mock: thread 0 non valido")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	var out []msg.MessageRef
	for _, mm := range m.threadMessages(threadID) {
		out = append(out, mm.Ref)
	}
	return out, nil
}

// threadMessages restituisce i messaggi del thread fuori da spam e cestino,
// in ordine cronologico crescente. Chiamare con m.mu presa.
func (m *Mock) threadMessages(threadID uint64) []*msg.Message {
	var out []*msg.Message
	for _, mm := range m.Msgs {
		if mm.Ref.ThreadID == threadID && !m.inExcludedBox(mm.Ref.ID) {
			out = append(out, mm)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Ref.Date.Before(out[j].Ref.Date) })
	return out
}

// buildConversation costruisce la riga di h. Chiamare con m.mu presa.
func (m *Mock) buildConversation(h *msg.Message) Conversation {
	members := []*msg.Message{h}
	if h.Ref.ThreadID != 0 {
		members = m.threadMessages(h.Ref.ThreadID)
		if !containsMsg(members, h) {
			members = append(members, h) // h nel cestino: la riga resta visibile
		}
	}
	c := Conversation{
		Latest:   h.Ref,
		ThreadID: h.Ref.ThreadID,
		Count:    len(members),
	}
	c.Latest.Labels = append([]string(nil), h.Ref.Labels...)
	seenFrom := make(map[string]bool)
	seenLabel := make(map[string]bool)
	for _, mm := range members {
		if mm.Ref.Unread {
			c.Unread = true
		}
		if !seenFrom[mm.Ref.From] {
			seenFrom[mm.Ref.From] = true
			c.Participants = append(c.Participants, mm.Ref.From)
		}
		for _, l := range mm.Ref.Labels {
			if !seenLabel[l] {
				seenLabel[l] = true
				c.Labels = append(c.Labels, l)
			}
		}
	}
	return c
}

// convKey è la chiave di raggruppamento: il thread, o il messaggio se non Gmail.
func convKey(mm *msg.Message) string {
	if mm.Ref.ThreadID != 0 {
		return fmt.Sprintf("thread:%d", mm.Ref.ThreadID)
	}
	return "msg:" + string(mm.Ref.ID)
}

// inExcludedBox dice se l'ID sta in spam o cestino (ruoli junk, trash).
func (m *Mock) inExcludedBox(id msg.MessageID) bool {
	box, _, err := id.Split()
	if err != nil {
		return false
	}
	for _, b := range m.mailboxes() {
		if b.Name == box && (b.Role == "trash" || b.Role == "junk") {
			return true
		}
	}
	return false
}

func containsMsg(list []*msg.Message, x *msg.Message) bool {
	for _, mm := range list {
		if mm == x {
			return true
		}
	}
	return false
}

// effectiveQuery applica q.Raw al sottoinsieme simulato nel Mock: from:,
// to:, label: (come cartella), is:unread, is:starred, has:attachment e testo
// libero. Qualunque altro operatore è errore esplicito: il Mock non deve far
// passare test con risultati falsi.
func effectiveQuery(q msg.Query) (msg.Query, error) {
	if q.Raw == "" {
		return q, nil
	}
	for _, tok := range strings.Fields(q.Raw) {
		key, val, ok := strings.Cut(tok, ":")
		if !ok {
			continue // testo libero
		}
		switch {
		case key == "from", key == "to", key == "label":
		case key == "is" && (val == "unread" || val == "starred"):
		case key == "has" && val == "attachment":
		default:
			return msg.Query{}, fmt.Errorf("mock: operatore Gmail %q non simulato", tok)
		}
	}
	r := msg.ParseQuery(q.Raw)
	out := q
	out.Raw = ""
	if r.From != "" {
		out.From = r.From
	}
	if r.To != "" {
		out.To = r.To
	}
	if r.Label != "" {
		out.Label = r.Label
	}
	out.Unread = q.Unread || r.Unread
	out.Starred = q.Starred || r.Starred
	out.HasAttachment = q.HasAttachment || r.HasAttachment
	out.Text = strings.TrimSpace(q.Text + " " + r.Text)
	return out, nil
}
