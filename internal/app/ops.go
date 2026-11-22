// Mutazioni e feedback (Task 7): i tasti `a`/`s`/`v`/`l` agiscono sul ref
// corrente — nell'inbox la riga selezionata, nel thread il messaggio aperto.
// Ogni mutazione gira in una tea.Cmd (la rete non blocca mai), dà feedback in
// status bar via opDone e al risultato rilista la lista corrente IN SILENZIO
// (refreshRefs: refs e cursore aggiornati, status intatto — il feedback
// dell'op deve restare visibile finché il prossimo evento non lo sostituisce).
package app

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/config"
	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
	"github.com/andsigno82/gmail-tui/internal/ui/thread"
)

// labelPrompt è il prompt del mini-input delle label (render nella status
// bar); la sua lunghezza in colonne serve a dimensionare l'input.
const labelPrompt = "etichetta: "

// pageSize è la dimensione della pagina di ListMessages (load, scroll in fondo,
// refresh di prima pagina). Piano 7, Task 3.
const pageSize = 50

// refreshCap è il tetto del refresh a profondità piena (D5, Piano 7, Task 3):
// oltre questa soglia il refresh rilista solo fino a refreshCap righe e la
// coda caricata resta com'è (appendUnique in msgsLoaded). Limite noto: per
// rilistare di più serve paginare anche il refresh.
const refreshCap = 500

// newLabelInput costruisce il textinput del mini-prompt label (stili condivisi
// con la ricerca via newPromptInput). Nasce NON focalizzato: il focus arriva
// quando `l` apre il prompt.
func newLabelInput(st styles.Styles) textinput.Model {
	ti := newPromptInput(labelPrompt, "nome label", st)
	ti.CharLimit = 64
	return ti
}

// currentRef restituisce il ref su cui operare: nel thread è il ref del
// messaggio aperto (la copia più aggiornata: stato al fetch + MarkRead + op
// ottimistiche), nell'inbox è il ref della riga sotto il cursore.
func (m Model) currentRef() (msg.MessageRef, bool) {
	if m.view == viewThread {
		if !m.thread.Loaded() {
			return msg.MessageRef{}, false // fetch in corso: nessun ref
		}
		return m.thread.Ref(), true
	}
	if m.view == viewInbox {
		return m.inbox.SelectedRef()
	}
	return msg.MessageRef{}, false
}

// runOp esegue il tasto-mutazione richiesto sul ref corrente. Le op NON sono
// bloccate con op store in flight (pending > 0): il gate del backend
// serializza le chiamate (una op alla volta) e il Mock è sincrono; un
// doppio-toggle sullo stesso ref fra pressione e refresh può correre —
// accettato in v1, il refresh che segue l'opDone corregge la copia in lista.
// Senza ref corrente (lista vuota, thread in caricamento) le op sono un
// no-op silenzioso.
func (m Model) runOp(k string) (Model, tea.Cmd) {
	ref, ok := m.currentRef()
	if !ok {
		return m, nil
	}
	switch k {
	case "a": // archivia: niente conferma (undo non in v1, feedback chiaro)
		m.pending++
		return m, tea.Batch(archive(m.store, ref.ID), m.spinner.Tick)
	case "s": // stella: toggle
		on := !ref.Starred
		updated := ref
		updated.Starred = on
		m.applyLocalRef(updated)
		m.pending++
		return m, tea.Batch(star(m.store, ref.ID, on), m.spinner.Tick)
	case "v": // lettura: toggle (unread → read, read → unread)
		read := ref.Unread
		updated := ref
		updated.Unread = !read
		m.applyLocalRef(updated)
		m.pending++
		return m, tea.Batch(markRead(m.store, ref.ID, read, true), m.spinner.Tick)
	case "l": // label: mini-prompt (enter applica, esc annulla)
		m.label.Reset()
		m.labelOpen = true
		return m, m.label.Focus()
	}
	return m, nil
}

// applyLocalRef propaga lo stato ottimistico del ref alla vista corrente: nel
// thread la copia locale si aggiorna SUBITO (l'header non rende i flag, ma i
// toggle consecutivi dell'utente leggono questo stato); la lista inbox si
// corregge col refresh che segue l'opDone, quindi non si tocca qui.
func (m *Model) applyLocalRef(updated msg.MessageRef) {
	if m.view != viewThread || !m.thread.Loaded() {
		return
	}
	if m.thread.Ref().ID != updated.ID {
		return
	}
	m.thread.SetRef(updated)
	m.pendingRef = updated
}

// confirmLabel chiude il prompt e applica la label digitata al ref corrente
// (on=true). Nome vuoto: nessuna op (come un annullamento). Il ref è letto al
// momento della conferma: col prompt aperto i tasti vanno all'input e il
// mouse è ignorato (fix merge-gate), il cursore non può muoversi fra apertura
// e conferma.
func (m Model) confirmLabel() (Model, tea.Cmd) {
	name := strings.TrimSpace(m.label.Value())
	m.labelOpen = false
	m.label.Blur()
	if name == "" {
		return m, nil
	}
	ref, ok := m.currentRef()
	if !ok {
		return m, nil
	}
	m.pending++
	return m, tea.Batch(applyLabel(m.store, ref.ID, name, true), m.spinner.Tick)
}

// cancelLabel chiude il prompt senza operare (esc).
func (m Model) cancelLabel() (Model, tea.Cmd) {
	m.labelOpen = false
	m.label.Blur()
	return m, nil
}

// markRead esegue MarkRead fuori dal loop UI. refresh=false per l'auto
// MarkRead al fetch (Task 5: solo feedback, la lista si aggiorna via
// Watch/nuova ricerca); refresh=true per il toggle `v` dell'utente. Il
// feedback dipende dalla direzione: "letta" / "segna non letta".
func markRead(st mail.MailStore, id msg.MessageID, read, refresh bool) tea.Cmd {
	label := "segna non letta"
	if read {
		label = "letta"
	}
	return func() tea.Msg {
		err := st.MarkRead(context.Background(), []msg.MessageID{id}, read)
		return opDone{label: label, err: err, refresh: refresh}
	}
}

// star esegue Star fuori dal loop UI: feedback "stella on"/"stella off".
func star(st mail.MailStore, id msg.MessageID, on bool) tea.Cmd {
	label := "stella off"
	if on {
		label = "stella on"
	}
	return func() tea.Msg {
		err := st.Star(context.Background(), []msg.MessageID{id}, on)
		return opDone{label: label, err: err, refresh: true}
	}
}

// archive esegue Archive fuori dal loop UI: feedback "archiviata". Nessuna
// conferma (undo non in v1, Piano 3 Task 7).
func archive(st mail.MailStore, id msg.MessageID) tea.Cmd {
	return func() tea.Msg {
		err := st.Archive(context.Background(), []msg.MessageID{id})
		return opDone{label: "archiviata", err: err, refresh: true, deep: true}
	}
}

// applyLabel esegue ApplyLabel fuori dal loop UI: feedback "etichetta X
// applicata" (on) / "etichetta X rimossa" (off).
func applyLabel(st mail.MailStore, id msg.MessageID, label string, on bool) tea.Cmd {
	feedback := fmt.Sprintf("etichetta %s rimossa", label)
	if on {
		feedback = fmt.Sprintf("etichetta %s applicata", label)
	}
	return func() tea.Msg {
		err := st.ApplyLabel(context.Background(), []msg.MessageID{id}, label, on)
		return opDone{label: feedback, err: err, refresh: true}
	}
}

// saveAttachment esegue store.SaveAttachment fuori dal loop UI (Piano 4,
// Task 2: `:save`/`:open` del thread): il messaggio di richiesta viaggia con
// i dati già risolti dal thread (PartIndex filtrato, percorso destinazione).
// Il risultato attachmentSaved distingue `:save` (feedback "salvato in …")
// da `:open` (la radice risponde con l'exec del browser, come `o`).
func saveAttachment(st mail.MailStore, req thread.SaveRequested) tea.Cmd {
	return func() tea.Msg {
		err := st.SaveAttachment(context.Background(), req.ID, req.Part, req.Path)
		return attachmentSaved{path: req.Path, open: req.Open, err: err}
	}
}

// saveThemeCmd persiste il tema applicato a runtime (Piano 4, Task 2:
// `:theme <nome>`): load della config dal percorso dato → set Theme → Save,
// così gli altri campi (address/name/signature) restano quelli su disco anche
// se cambiati fuori dall'app. Percorso vuoto (os.UserConfigDir non
// disponibile) → nessuna persistenza (il tema runtime resta applicato).
// L'errore torna come themePersisted: gestito in statusMsg senza perdere il
// tema già in uso.
func saveThemeCmd(path, name string) tea.Cmd {
	if path == "" {
		return nil
	}
	return func() tea.Msg {
		cfg, err := config.Load(path)
		if err == nil {
			cfg.Theme = name
			err = cfg.Save(path)
		}
		return themePersisted{err: err}
	}
}

// requestRefresh prepara il re-list silenzioso della lista corrente (query
// corrente) — il meccanismo condiviso dal refresh dopo le op (Task 7) e dal
// feed Watch (Task 8). Coalescing: con un re-list già in flight NON si
// accatasta — marca dirty e il follow-up parte al risultato di quello in
// corso (msgsLoaded); i cambi arrivati durante il refresh non si perdono.
// deep (Piano 7, Task 3, D5): profondità piena, chiesta solo per ChangeNewMail,
// ChangeDisconnected, Archive e Trash; si ricorda anche se la richiesta arriva
// durante un refresh in volo (dirtyDeep).
func (m *Model) requestRefresh(deep bool) tea.Cmd {
	if m.refreshing {
		m.dirty = true
		m.dirtyDeep = m.dirtyDeep || deep
		return nil
	}
	m.dirty = false // il refresh che parte ora copre anche i cambi pendenti
	deep = deep || m.dirtyDeep
	m.dirtyDeep = false
	m.refreshing = true
	m.pending++
	limit := refreshLimit(len(m.refs), deep)
	return tea.Batch(refreshRefs(m.store, m.query, m.queryRaw, limit, m.folder.Name), m.spinner.Tick)
}

// refreshLimit è il Limit del re-list silenzioso (Piano 7, Task 3, D5): la sola
// prima pagina per le op che non cambiano la composizione della lista; con
// deep, tutta la profondità caricata (minimo una pagina) fino a refreshCap.
func refreshLimit(loaded int, deep bool) int {
	if !deep {
		return pageSize
	}
	return min(max(pageSize, loaded), refreshCap)
}

// refreshRefs rilista la lista corrente (cartella box) fuori dal loop UI, nello
// stato silenzioso (msgsLoaded.silent): aggiorna refs e cursore senza toccare lo
// status. Riusa il pattern del load: query vuota → ListMessages (limit righe
// dalla prima pagina, come Init), query di ricerca → Search con la query
// corrente nella cartella (non paginata: limit 0).
func refreshRefs(st mail.MailStore, q msg.Query, raw string, limit int, box string) tea.Cmd {
	return func() tea.Msg {
		var refs []msg.MessageRef
		var err error
		if raw == "" {
			refs, err = st.ListMessages(context.Background(), msg.Query{Label: box}, mail.Page{Limit: limit})
		} else {
			refs, err = st.Search(context.Background(), inFolder(q, box))
			limit = 0
		}
		return msgsLoaded{refs: refs, err: err, query: raw, silent: true, limit: limit, mailbox: box}
	}
}

// loadMore esegue ListMessages per la pagina successiva fuori dal loop UI
// (Piano 7, Task 3): l'offset è il numero di righe già caricate. Il risultato
// porta la query di origine per lo scarto dei risultati stanti.
func loadMore(st mail.MailStore, box string, offset int, raw string) tea.Cmd {
	return func() tea.Msg {
		refs, err := st.ListMessages(context.Background(), msg.Query{Label: box}, mail.Page{Offset: offset, Limit: pageSize})
		return moreLoaded{refs: refs, err: err, query: raw, mailbox: box}
	}
}

// appendUnique concatena i ref di more a base saltando gli ID già presenti
// (il primo vince, ordine preservato). Piano 7, Task 3: dedup delle pagine.
func appendUnique(base, more []msg.MessageRef) []msg.MessageRef {
	seen := make(map[msg.MessageID]bool, len(base)+len(more))
	out := make([]msg.MessageRef, 0, len(base)+len(more))
	for _, group := range [][]msg.MessageRef{base, more} {
		for _, r := range group {
			if !seen[r.ID] {
				seen[r.ID] = true
				out = append(out, r)
			}
		}
	}
	return out
}
