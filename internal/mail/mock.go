package mail

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Mock è un MailStore in-memory per i test della UI e della logica.
// Campo Err non-nil: ogni chiamata fallisce (injection per test errori).
type Mock struct {
	mu        sync.Mutex
	Msgs      []*msg.Message
	Sent      []msg.Outgoing
	Err       error
	Mailboxes []Mailbox // nil = default (INBOX + cestino Gmail), cfr. defaultMailboxes
	Raw       map[msg.MessageID]map[int][]byte
	changes   chan Change
}

// defaultMailboxes sono le cartelle del Mock se Mailboxes è nil (Piano 7, Task 1).
var defaultMailboxes = []Mailbox{
	{Name: "INBOX", Role: "inbox"},
	{Name: "[Gmail]/Trash", Role: "trash"},
}

// Mock implementa MailStore a compile time (conformance assert).
var _ MailStore = (*Mock)(nil)

// Emit invia un change a tutti i watcher (per i test).
func (m *Mock) Emit(c Change) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.init()
	select {
	case m.changes <- c:
	default: // watcher lento: l'evento si perde, la UI farà re-list
	}
}

func (m *Mock) init() {
	if m.changes == nil {
		m.changes = make(chan Change, 16)
	}
}

func (m *Mock) fail() error { return m.Err }

// Connect implementa MailStore.
func (m *Mock) Connect(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Err
}

// ListMessages implementa MailStore: data desc + offset/limit.
func (m *Mock) ListMessages(_ context.Context, q msg.Query, pg Page) ([]msg.MessageRef, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	// Piano 7, Task 1: la mailbox è q.Label (Label vuota = INBOX), come nel backend IMAP.
	eff, err := effectiveQuery(q) // Piano 8, Task 5: Raw simulato
	if err != nil {
		return nil, err
	}
	mailbox := eff.Label
	if mailbox == "" {
		mailbox = "INBOX"
	}
	refs := make([]msg.MessageRef, 0, len(m.Msgs))
	for _, mm := range m.Msgs {
		if inMailbox(mm.Ref.ID, mailbox) && matches(eff, mm) {
			refs = append(refs, mm.Ref)
		}
	}
	sort.SliceStable(refs, func(i, j int) bool { return refs[i].Date.After(refs[j].Date) })
	if pg.Limit == 0 {
		pg.Limit = 50
	}
	if pg.Offset >= len(refs) {
		return nil, nil
	}
	end := pg.Offset + pg.Limit
	if end > len(refs) {
		end = len(refs)
	}
	return refs[pg.Offset:end], nil
}

// FetchMessage implementa MailStore (copia difensiva).
func (m *Mock) FetchMessage(_ context.Context, id msg.MessageID) (*msg.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	for _, mm := range m.Msgs {
		if mm.Ref.ID == id {
			cp := *mm
			cp.To = append([]string(nil), mm.To...)
			cp.Cc = append([]string(nil), mm.Cc...)
			cp.Attachments = append([]msg.Attachment(nil), mm.Attachments...)
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("messaggio %s non trovato", id)
}

// Search implementa MailStore: stesso filtro di ListMessages, senza pagine.
func (m *Mock) Search(ctx context.Context, q msg.Query) ([]msg.MessageRef, error) {
	return m.ListMessages(ctx, q, Page{Limit: 1000})
}

// MarkRead implementa MailStore.
func (m *Mock) MarkRead(_ context.Context, ids []msg.MessageID, read bool) error {
	ids = append([]msg.MessageID(nil), ids...) // copia: il chiamante può riusare la slice
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	for _, mm := range m.Msgs {
		for _, id := range ids {
			if mm.Ref.ID == id {
				mm.Ref.Unread = !read
			}
		}
	}
	m.init()
	select {
	case m.changes <- Change{Kind: ChangeFlags, IDs: ids}:
	default:
	}
	return nil
}

// Archive implementa MailStore: rimuove dalla lista (via da INBOX).
func (m *Mock) Archive(_ context.Context, ids []msg.MessageID) error {
	ids = append([]msg.MessageID(nil), ids...) // copia: il chiamante può riusare la slice
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	kept := m.Msgs[:0]
	for _, mm := range m.Msgs {
		drop := false
		for _, id := range ids {
			if mm.Ref.ID == id {
				drop = true
			}
		}
		if !drop {
			kept = append(kept, mm)
		}
	}
	m.Msgs = kept
	m.init()
	select {
	case m.changes <- Change{Kind: ChangeFlags, IDs: ids}:
	default:
	}
	return nil
}

// Star implementa MailStore.
func (m *Mock) Star(_ context.Context, ids []msg.MessageID, on bool) error {
	ids = append([]msg.MessageID(nil), ids...) // copia: il chiamante può riusare la slice
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	for _, mm := range m.Msgs {
		for _, id := range ids {
			if mm.Ref.ID == id {
				mm.Ref.Starred = on
			}
		}
	}
	m.init()
	select {
	case m.changes <- Change{Kind: ChangeFlags, IDs: ids}:
	default:
	}
	return nil
}

// ApplyLabel implementa MailStore: no-op nel mock (le label sono IMAP-only).
func (m *Mock) ApplyLabel(_ context.Context, ids []msg.MessageID, _ string, _ bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_ = ids
	return m.fail()
}

// SaveAttachment implementa MailStore usando i contenuti in m.Raw.
func (m *Mock) SaveAttachment(_ context.Context, id msg.MessageID, part int, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	data, ok := m.Raw[id][part]
	if !ok {
		return fmt.Errorf("allegato %s/%d non trovato", id, part)
	}
	return os.WriteFile(path, data, 0o644)
}

// Send implementa MailStore: registra in Sent.
func (m *Mock) Send(_ context.Context, out msg.Outgoing) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	m.Sent = append(m.Sent, out)
	return nil
}

// ListMailboxes implementa MailStore (Piano 7, Task 1).
func (m *Mock) ListMailboxes(context.Context) ([]Mailbox, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	return append([]Mailbox(nil), m.mailboxes()...), nil
}

// Trash implementa MailStore: sposta gli id nel cestino con un nuovo ID nel
// prefisso del cestino. Valida TUTTI gli id prima di agire (niente azioni
// parziali, come il Trash IMAP).
func (m *Mock) Trash(_ context.Context, ids []msg.MessageID) error {
	ids = append([]msg.MessageID(nil), ids...) // copia: il chiamante può riusare la slice
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	if len(ids) == 0 {
		return nil
	}
	trash, ok := trashBox(m.mailboxes())
	if !ok {
		return fmt.Errorf("cestino non trovato")
	}

	seen := make(map[msg.MessageID]bool, len(ids))
	var targets []*msg.Message
	for _, id := range ids {
		if seen[id] {
			continue // id duplicato: un solo spostamento
		}
		seen[id] = true
		if inMailbox(id, trash) {
			return fmt.Errorf("messaggio %s già nel cestino", id)
		}
		mm := m.find(id)
		if mm == nil {
			return fmt.Errorf("messaggio %s non trovato", id)
		}
		targets = append(targets, mm)
	}

	next := m.nextUID(trash)
	for _, mm := range targets {
		mm.Ref.ID = msg.NewID(trash, next)
		next++
	}
	m.init()
	select {
	case m.changes <- Change{Kind: ChangeFlags, IDs: ids}:
	default:
	}
	return nil
}

// mailboxes restituisce le cartelle configurate, o il default se Mailboxes è nil.
func (m *Mock) mailboxes() []Mailbox {
	if m.Mailboxes != nil {
		return m.Mailboxes
	}
	return defaultMailboxes
}

// find restituisce il messaggio con quell'ID, o nil.
func (m *Mock) find(id msg.MessageID) *msg.Message {
	for _, mm := range m.Msgs {
		if mm.Ref.ID == id {
			return mm
		}
	}
	return nil
}

// nextUID restituisce il primo UID libero nella mailbox (max + 1), come
// farebbe IMAP assegnando un UID nuovo al messaggio spostato.
func (m *Mock) nextUID(mailbox string) uint32 {
	var maxUID uint32
	for _, mm := range m.Msgs {
		if box, uid, err := mm.Ref.ID.Split(); err == nil && box == mailbox && uid > maxUID {
			maxUID = uid
		}
	}
	return maxUID + 1
}

// trashBox restituisce il nome della mailbox con ruolo trash.
func trashBox(boxes []Mailbox) (string, bool) {
	for _, b := range boxes {
		if b.Role == "trash" {
			return b.Name, true
		}
	}
	return "", false
}

// inMailbox dice se l'ID appartiene alla mailbox (prefisso "MAILBOX:").
func inMailbox(id msg.MessageID, mailbox string) bool {
	box, _, err := id.Split()
	return err == nil && box == mailbox
}

// Watch implementa MailStore.
func (m *Mock) Watch(context.Context) (<-chan Change, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.init()
	return m.changes, nil
}

// matches applica msg.Query a un messaggio (case-insensitive).
func matches(q msg.Query, m *msg.Message) bool {
	if q.Unread && !m.Ref.Unread {
		return false
	}
	if q.Starred && !m.Ref.Starred {
		return false
	}
	if q.HasAttachment && len(m.Attachments) == 0 {
		return false
	}
	if q.From != "" && !strings.Contains(strings.ToLower(m.Ref.From), strings.ToLower(q.From)) {
		return false
	}
	if q.To != "" && !containsAny(m.To, q.To) {
		return false
	}
	if q.Text != "" {
		hay := strings.ToLower(m.Ref.Subject + " " + m.Ref.From + " " + m.BodyText)
		if !strings.Contains(hay, strings.ToLower(q.Text)) {
			return false
		}
	}
	return true
}

func containsAny(addrs []string, want string) bool {
	for _, a := range addrs {
		if strings.Contains(strings.ToLower(a), strings.ToLower(want)) {
			return true
		}
	}
	return false
}
