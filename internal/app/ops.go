// Mutazioni e feedback (Task 7): i tasti `a`/`s`/`v`/`l` agiscono sul ref
// corrente — nell'inbox la riga selezionata, nel thread il messaggio aperto.
// Ogni mutazione gira in una tea.Cmd (la rete non blocca mai), dà feedback in
// status bar via opDone e al risultato rilista la lista corrente IN SILENZIO
// (refreshRefs: refs e cursore aggiornati, status intatto — il feedback
// dell'op deve restare visibile finché il prossimo evento non lo sostituisce).
package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/config"
	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
	"github.com/andsigno82/gmail-tui/internal/ui/thread"
	"github.com/andsigno82/gmail-tui/internal/ui/toast"
)

// labelPrompt è il prompt del mini-input delle label (render nella status
// bar); la sua lunghezza in colonne serve a dimensionare l'input.
const labelPrompt = "etichetta: "

// pageSize è la dimensione della pagina di ListMessages (load, scroll in fondo,
// refresh di prima pagina). Piano 7, Task 3.
const pageSize = 50

// searchLimit è il tetto di conversazioni di una ricerca (non paginata nella
// UI, Piano 8, Task 9): la scansione del backend resta limitata a monte.
const searchLimit = 1000

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
		if m.splitOn() {
			// Piano 7.2, Task 4: con due colonne le azioni agiscono sul messaggio
			// in anteprima, con entrambi i focus (la riga del cursore se l'anteprima
			// non è ancora pronta).
			return m.previewRef()
		}
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
	if k == "a" && !strings.EqualFold(m.opBox(), "INBOX") {
		// Piano 7, fix review 3: l'archivio è solo della posta in arrivo. Nel
		// cestino l'expunge sarebbe una eliminazione definitiva; in una label
		// rimuoverebbe solo la label; in Inviati non ha senso. Piano 7, fix
		// code-review 1: la mailbox è quella della RIGA (opBox), non della cartella
		// mostrata. Nessuna op, status informativo (anche con lista vuota).
		cmd := m.toast.Push(toast.Info, "archivia è disponibile solo in Posta in arrivo")
		return m, cmd
	}
	ref, ok := m.currentRef()
	if !ok {
		return m, nil
	}
	switch k {
	case "a": // archivia: niente conferma (undo non in v1, feedback chiaro)
		m.pending++
		return m, tea.Batch(archive(m.store, ref), m.spinner.Tick)
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
		return m, tea.Batch(markReadConv(m.store, ref, read), m.spinner.Tick)
	case "l": // label: mini-prompt (enter applica, esc annulla)
		m.label.Reset()
		m.labelOpen = true
		follow := m.label.Focus()
		return m, follow
	}
	return m, nil
}

// applyLocalRef propaga lo stato ottimistico del ref alla vista corrente: nel
// thread la copia locale si aggiorna SUBITO (l'header non rende i flag, ma i
// toggle consecutivi dell'utente leggono questo stato); la lista inbox si
// corregge col refresh che segue l'opDone, quindi non si tocca qui.
func (m *Model) applyLocalRef(updated msg.MessageRef) {
	// Piano 7.2, Task 4: con due colonne il messaggio in anteprima è quello delle
	// azioni: la sua copia locale (prevRef e thread) si aggiorna come nel thread.
	inPreview := m.view == viewInbox && m.previewVisible() && m.prevRef.ID == updated.ID
	if inPreview {
		m.prevRef = updated
	}
	if (m.view != viewThread && !inPreview) || !m.thread.Loaded() {
		return
	}
	if m.thread.Ref().ID != updated.ID {
		return
	}
	m.thread.SetRef(updated)
	if m.view == viewThread {
		m.pendingRef = updated
	}
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
	if strings.EqualFold(name, m.opBox()) {
		// Piano 7, fix review 4: la COPY nella stessa mailbox duplicherebbe il
		// messaggio. Piano 7, fix code-review 1: il confronto è con la mailbox
		// della riga (opBox), non con la cartella mostrata. Rifiuto con status
		// d'errore, lo store non è chiamato.
		cmd := m.toast.Push(toast.Error, fmt.Sprintf("etichetta %s non applicata: è la cartella corrente", name))
		return m, cmd
	}
	ref, ok := m.currentRef()
	if !ok {
		return m, nil
	}
	m.pending++
	return m, tea.Batch(applyLabel(m.store, ref, name, true), m.spinner.Tick)
}

// opBox è la mailbox IMAP su cui agisce un'op: quella della RIGA operata (l'ID
// del ref è mailbox+uid), non quella della cartella mostrata — da INBOX una
// ricerca `label:Work` mostra righe di Work. Piano 7, fix code-review 1. Senza
// ref (lista vuota, thread in caricamento) ricade sulla cartella mostrata: lo
// zero value di folder è la posta in arrivo ("INBOX").
func (m Model) opBox() string {
	if ref, ok := m.currentRef(); ok {
		if box, _, err := ref.ID.Split(); err == nil {
			return box
		}
	}
	if m.folder.Name == "" {
		return "INBOX"
	}
	return m.folder.Name
}

// cancelLabel chiude il prompt senza operare (esc).
func (m Model) cancelLabel() (Model, tea.Cmd) {
	m.labelOpen = false
	m.label.Blur()
	return m, nil
}

// rowRole è il ruolo della mailbox della RIGA operata (opBox), come per `a` e
// `l` (Piano 7.1, Task 5). È noto all'app solo se è la cartella mostrata o la
// posta in arrivo; altrove restituisce "" (ignoto): una riga cercata con label:
// da un'altra cartella non ha ruolo noto e il backend decide.
func (m Model) rowRole() string {
	box := m.opBox()
	if strings.EqualFold(box, m.folder.Name) {
		return m.folder.Role
	}
	if strings.EqualFold(box, "INBOX") {
		return "inbox"
	}
	return ""
}

// askTrash apre la conferma dello spostamento nel cestino (`#`, Piano 7, Task
// 5, D4). Il messaggio è quello sotto il cursore nella lista o aperto nel
// thread, catturato ORA: la conferma agisce su questo, non sul cursore. Nella
// mailbox del cestino della riga non si apre nulla: il messaggio è già lì
// (Piano 7.1, Task 5: guarda la riga, non la cartella mostrata).
func (m Model) askTrash() (Model, tea.Cmd) {
	if m.rowRole() == "trash" {
		cmd := m.toast.Push(toast.Info, "già nel cestino")
		return m, cmd
	}
	ref, ok := m.currentRef()
	if !ok {
		return m, nil
	}
	m.trashRef = ref
	m.trashOpen = true
	return m, nil
}

// confirmTrash esegue lo spostamento confermato (`s`). Dal thread si torna alla
// lista (D7): il messaggio non è più nella cartella. L'esito arriva come opDone
// con refresh a profondità piena (Trash cambia la composizione della lista).
func (m Model) confirmTrash() (Model, tea.Cmd) {
	m.trashOpen = false
	if m.view == viewThread {
		m.view = viewInbox
	}
	m.pending++
	return m, tea.Batch(trashCmd(m.store, m.trashRef), m.spinner.Tick)
}

// trashCmd esegue Trash fuori dal loop UI: feedback "spostata nel cestino" e
// refresh a profondità piena. Un errore (es. cestino assente) resta in status
// e la lista non cambia.
// Piano 8, Task 9b: sposta nel cestino la conversazione della riga (ids da
// conversationIDs, guardati sulla mailbox della riga).
func trashCmd(st mail.MailStore, ref msg.MessageRef) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		ids, err := conversationIDs(ctx, st, ref)
		if err == nil {
			err = guardBox(ids, boxOf(ref))
		}
		if err == nil {
			err = st.Trash(ctx, ids)
		}
		return opDone{label: "spostata nel cestino", err: err, refresh: true, deep: true, gone: ids}
	}
}

// conversationIDs risolve i messaggi su cui agisce un'op della riga (Piano 8,
// Task 9b): la conversazione intera nella mailbox della riga, come Gmail web.
// Senza thread Gmail (ThreadID 0) è il solo messaggio della riga. La riga è
// sempre inclusa.
func conversationIDs(ctx context.Context, st mail.MailStore, ref msg.MessageRef) ([]msg.MessageID, error) {
	if ref.ThreadID == 0 {
		return []msg.MessageID{ref.ID}, nil
	}
	ids, err := st.ConversationMembers(ctx, boxOf(ref), ref.ThreadID)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if id == ref.ID {
			return ids, nil
		}
	}
	return append(ids, ref.ID), nil
}

// boxOf è la mailbox del messaggio della riga ("" se l'ID è malformato).
func boxOf(ref msg.MessageRef) string {
	box, _, err := ref.ID.Split()
	if err != nil {
		return ""
	}
	return box
}

// guardBox rifiuta un insieme di ID se uno di essi non sta nella mailbox box
// (Piano 8, Task 9b, sicurezza dati): archivia e cestina non devono mai toccare
// un messaggio di un'altra mailbox, in particolare Tutti i messaggi.
func guardBox(ids []msg.MessageID, box string) error {
	for _, id := range ids {
		if b, _, err := id.Split(); err != nil || b != box {
			return fmt.Errorf("rifiutato: %s non sta in %q", id, box)
		}
	}
	return nil
}

// anyUnread dice se un messaggio del set è non letto.
func anyUnread(refs []msg.MessageRef) bool {
	for _, r := range refs {
		if r.Unread {
			return true
		}
	}
	return false
}

// markReadConv è il toggle `v` sulla riga (Piano 8, Task 9b, D8-6): leggi/non letto
// vale per l'INTERA conversazione, come Gmail web (flag soltanto, nessuno spostamento):
// i messaggi sono quelli di All Mail senza spam e cestino (ThreadMessages), più la riga
// stessa. Poi legge lo stato di lettura della conversazione, così il grassetto della
// riga è giusto subito, senza attendere il refresh.
func markReadConv(st mail.MailStore, ref msg.MessageRef, read bool) tea.Cmd {
	label := "segna non letta"
	if read {
		label = "letta"
	}
	return func() tea.Msg {
		ctx := context.Background()
		ids := []msg.MessageID{ref.ID}
		var thread []msg.MessageRef
		var err error
		if ref.ThreadID != 0 {
			thread, err = st.ThreadMessages(ctx, ref.ThreadID)
			ids = withID(refIDs(thread), ref.ID)
		}
		if err == nil {
			err = st.MarkRead(ctx, ids, read)
		}
		ev := opDone{label: label, err: err, refresh: true, localID: ref.ID, localRead: read}
		if err == nil && ref.ThreadID != 0 {
			ev.convSet, ev.convUnread = true, anyUnread(thread)
		}
		return ev
	}
}

// refIDs estrae gli ID delle righe.
func refIDs(refs []msg.MessageRef) []msg.MessageID {
	out := make([]msg.MessageID, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.ID)
	}
	return out
}

// withID aggiunge id a ids se non c'è già (la riga aperta, anche nel cestino).
func withID(ids []msg.MessageID, id msg.MessageID) []msg.MessageID {
	for _, x := range ids {
		if x == id {
			return ids
		}
	}
	return append(ids, id)
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
// Piano 8, Task 9b: archivia la conversazione della riga, guardata sulla
// mailbox della riga (la INBOX, per il controllo di runOp).
func archive(st mail.MailStore, ref msg.MessageRef) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		ids, err := conversationIDs(ctx, st, ref)
		if err == nil {
			err = guardBox(ids, boxOf(ref))
		}
		if err == nil {
			err = st.Archive(ctx, ids)
		}
		return opDone{label: "archiviata", err: err, refresh: true, deep: true, gone: ids}
	}
}

// applyLabel esegue ApplyLabel fuori dal loop UI: feedback "etichetta X
// applicata" (on) / "etichetta X rimossa" (off).
// Piano 8, Task 9b: la label va a tutta la conversazione della riga nella
// cartella (la COPY non rimuove nulla, la guardia resta sulla mailbox della riga).
func applyLabel(st mail.MailStore, ref msg.MessageRef, label string, on bool) tea.Cmd {
	feedback := fmt.Sprintf("etichetta %s rimossa", label)
	if on {
		feedback = fmt.Sprintf("etichetta %s applicata", label)
	}
	return func() tea.Msg {
		ctx := context.Background()
		ids, err := conversationIDs(ctx, st, ref)
		if err == nil {
			err = guardBox(ids, boxOf(ref))
		}
		if err == nil {
			err = st.ApplyLabel(ctx, ids, label, on)
		}
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
		// Piano 7.2, fix review: valore non valido → default solo per quel campo.
		if errors.Is(err, config.ErrInvalid) {
			cfg, err = cfg.Sanitized(), nil
		}
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
	m.refreshGen = m.listGen // il token si libera solo al risultato di questa generazione (fix review 5)
	m.pending++
	limit := refreshLimit(len(m.refs), deep)
	return tea.Batch(refreshRefs(m.store, m.query, m.queryRaw, limit, m.folder.Name, m.listGen), m.spinner.Tick)
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
// status. Riusa il pattern del load (Piano 8, Task 9: una riga per conversazione):
// query vuota → ListConversations (limit righe dalla prima pagina, come Init),
// query di ricerca → ListConversations con la query corrente nella cartella
// (non paginata: limit 0).
func refreshRefs(st mail.MailStore, q msg.Query, raw string, limit int, box string, gen int) tea.Cmd {
	return func() tea.Msg {
		var convs []mail.Conversation
		var err error
		if raw == "" {
			convs, err = st.ListConversations(context.Background(), msg.Query{Label: box}, mail.Page{Limit: limit})
		} else {
			convs, err = st.ListConversations(context.Background(), scopeQuery(q, box), mail.Page{Limit: searchLimit})
			limit = 0
		}
		lim, err := scanLimited(err)
		return msgsLoaded{refs: latestRefs(convs), convs: convs, err: err, query: raw, silent: true, limited: lim, limit: limit, mailbox: box, gen: gen}
	}
}

// loadMore esegue ListConversations per la pagina successiva fuori dal loop UI
// (Piano 7, Task 3; Piano 8, Task 9): l'offset è il numero di conversazioni già
// caricate. Il risultato porta la query di origine per lo scarto dei risultati
// stanti.
func loadMore(st mail.MailStore, box string, offset int, raw string, gen int) tea.Cmd {
	return func() tea.Msg {
		convs, err := st.ListConversations(context.Background(), msg.Query{Label: box}, mail.Page{Offset: offset, Limit: pageSize})
		lim, err := scanLimited(err)
		return moreLoaded{refs: latestRefs(convs), convs: convs, err: err, query: raw, limited: lim, mailbox: box, gen: gen}
	}
}

// rememberConvs registra il riepilogo delle conversazioni ricevute, per ID del
// messaggio più recente (Piano 8, Task 9). Una lista senza convs non cambia nulla.
func (m *Model) rememberConvs(convs []mail.Conversation) {
	if len(convs) == 0 {
		return
	}
	if m.sums == nil {
		m.sums = make(map[msg.MessageID]mail.Conversation, len(convs))
	}
	for _, c := range convs {
		m.sums[c.Latest.ID] = c
	}
}

// convsOf costruisce le righe-conversazione dei ref della lista (Piano 8, Task 9):
// il riepilogo noto per ID, con il messaggio più recente dato dal ref stesso;
// senza riepilogo, una conversazione di un solo messaggio.
func (m Model) convsOf(refs []msg.MessageRef) []mail.Conversation {
	out := make([]mail.Conversation, len(refs))
	for i, r := range refs {
		c, ok := m.sums[r.ID]
		if !ok {
			c = mail.SingleConversation(r)
		}
		c.Latest = r
		out[i] = c
	}
	return out
}

// withoutIDs toglie da refs gli ID dati, ordine preservato. Piano 7, fix review 1:
// le op che fanno uscire un messaggio dalla cartella (Archive/Trash) lo tolgono
// dalla coda prima del merge del refresh profondo.
func withoutIDs(refs []msg.MessageRef, ids []msg.MessageID) []msg.MessageRef {
	drop := make(map[msg.MessageID]bool, len(ids))
	for _, id := range ids {
		drop[id] = true
	}
	out := make([]msg.MessageRef, 0, len(refs))
	for _, r := range refs {
		if !drop[r.ID] {
			out = append(out, r)
		}
	}
	return out
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
