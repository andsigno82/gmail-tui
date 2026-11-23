package thread

// Conciliazione del thread (Piano 16, Task 6, R2, D16-T13): un thread aperto da
// istantanea mostra la versione provvisoria; quando arriva quella fresca il
// modello si riallinea senza saltare. L'identita di un messaggio e il GmailID
// (l'ID se GmailID e 0), MAI l'UID: tra l'epoca dell'istantanea e quella del
// server lo stesso UID puo indicare un altro messaggio.

import (
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// identity e la chiave di conciliazione di un messaggio: GmailID se noto, altrimenti
// l'ID (server senza X-GM-EXT-1). Le due forme non si confondono mai.
type identity struct {
	gmail uint64
	id    msg.MessageID
}

func identityOf(r msg.MessageRef) identity {
	if r.GmailID != 0 {
		return identity{gmail: r.GmailID}
	}
	return identity{id: r.ID}
}

// threadIDOf e il primo ThreadID noto tra i ref (0 = ignoto).
func threadIDOf(refs []msg.MessageRef) uint64 {
	for _, r := range refs {
		if r.ThreadID != 0 {
			return r.ThreadID
		}
	}
	return 0
}

// sameMessage dice se a e b sono lo stesso messaggio con lo stesso contenuto: se
// si, la voce della cache di rendering resta valida.
func sameMessage(a, b *msg.Message) bool {
	return a == b || (a.Ref.ID == b.Ref.ID && a.Ref.GmailID == b.Ref.GmailID &&
		a.BodyText == b.BodyText && a.BodyHTML == b.BodyHTML)
}

// forget toglie dalla cache di rendering ogni voce del messaggio id: la chiave e
// (id, larghezza, tema, variante) e non il contenuto, quindi se sotto lo stesso ID
// c'e un altro messaggio o un altro corpo la voce vecchia sarebbe servita al posto
// giusto (Piano 16, Task 6).
func (c *renderCache) forget(id msg.MessageID) {
	if c == nil {
		return
	}
	kept := c.order[:0]
	for _, k := range c.order {
		if k.id == id {
			delete(c.m, k)
			continue
		}
		kept = append(kept, k)
	}
	c.order = kept
}

// Reconcile sostituisce l'elenco dei messaggi con i ref freschi (Piano 16, Task 6).
// Selezione, espansione e corpi gia presenti si riportano per GmailID; i corpi
// freschi in bodies (per ID fresco) sostituiscono quelli dell'istantanea; i
// messaggi nuovi si aggiungono con l'espansione di default (ultimo e non letti),
// quelli spariti si tolgono. Lo scorrimento resta relativo al messaggio
// selezionato se esiste ancora, altrimenti si seleziona l'ultimo. Un risultato
// vuoto o di un altro thread (ThreadID diverso) si ignora. Gli ID dei ref e dei
// corpi riportati diventano quelli freschi. L'ancora (AnchorID) non cambia: la
// riallinea la radice (R2). Ricevitore puntatore: muta il modello.
func (m *Model) Reconcile(refs []msg.MessageRef, bodies map[msg.MessageID]*msg.Message) {
	if len(refs) == 0 || len(m.conv) == 0 {
		return
	}
	if cur := threadIDOf(m.Refs()); cur != 0 {
		for _, r := range refs {
			if r.ThreadID != 0 && r.ThreadID != cur {
				return // risultato stantio di un altro thread
			}
		}
	}

	old := make(map[identity]entry, len(m.conv))
	oldByID := make(map[msg.MessageID]*msg.Message, len(m.conv))
	for _, e := range m.conv {
		old[identityOf(e.ref)] = e
		if e.full != nil {
			oldByID[e.full.Ref.ID] = e.full
		}
	}
	selKey := identityOf(m.selRef())
	multiBefore := m.multi()
	offset := m.vp.YOffset()
	rel := 0
	if multiBefore && m.sel < len(m.starts) {
		rel = offset - m.starts[m.sel]
	}

	conv := make([]entry, 0, len(refs))
	sel := -1
	for i, r := range refs {
		e := entry{ref: r, expanded: isDefaultExpanded(i, refs)}
		prev, known := old[identityOf(r)]
		if known {
			e.expanded = prev.expanded
			if prev.full != nil {
				e.full = prev.full
				if prev.full.Ref.ID != r.ID { // ID provvisorio: copia con il ref confermato
					c := *prev.full
					c.Ref = r
					e.full = &c
				}
			}
		}
		if b := bodies[r.ID]; b != nil {
			e.full = b
		}
		if e.full != nil {
			if p := oldByID[e.full.Ref.ID]; p != nil && !sameMessage(p, e.full) {
				m.cache.forget(e.full.Ref.ID)
			}
		}
		if known && identityOf(r) == selKey {
			sel = i
		}
		conv = append(conv, e)
	}
	found := sel >= 0
	if !found {
		sel = len(conv) - 1
	}
	m.conv, m.sel = conv, sel

	m.refresh()
	m.resize()
	switch {
	case !m.multi():
		m.vp.SetYOffset(offset)
	case found:
		m.vp.SetYOffset(m.starts[m.sel] + rel)
	default:
		m.reveal()
	}
}

// MissingBodies elenca gli ID dei messaggi senza corpo che lo schermo richiede: il
// selezionato e gli espansi (Piano 16, Task 13). Dopo Reconcile un messaggio espanso a
// mano mentre il thread era provvisorio (la richiesta del corpo era ignorata) non ha
// ancora il corpo fresco: la radice lo chiede.
func (m Model) MissingBodies() []msg.MessageID {
	var out []msg.MessageID
	for i, e := range m.conv {
		if e.full == nil && (i == m.sel || e.expanded) {
			out = append(out, e.ref.ID)
		}
	}
	return out
}
