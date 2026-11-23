package app

// Lista da istantanea (Piano 16, Task 9, D16-T11, D16-T12): all'avvio, prima di
// Connect, la lista della cartella corrente compare dall'istantanea locale in
// stato PROVVISORIO (sola lettura, R1); la lista fresca del server la sostituisce
// per intero (R2). L'app non scrive mai sul disco e non importa la cache: parla
// solo con l'interfaccia opzionale mail.Snapshotter.

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/trace"
)

// listSource dice da dove vengono le righe della lista.
type listSource int

const (
	srcNone   listSource = iota // nessuna lista ancora
	srcCached                   // istantanea locale: provvisoria (R1)
	srcFresh                    // risposta del server: azionabile
)

// snapshotLoaded e l'esito della lettura dell'istantanea della cartella mailbox,
// richiesta alla generazione folderGen: si applica solo se sono ancora quelle correnti.
type snapshotLoaded struct {
	mailbox   string
	folderGen int
	snap      mail.ConversationsSnapshot
}

// loadSnapshot legge l'istantanea della cartella box fuori dal loop UI (lettura
// bbolt). Senza mail.Snapshotter nessuna Cmd; un'istantanea assente o vuota non
// produce alcun messaggio (vale come assente: resta lo splash). D16-T11.
func loadSnapshot(st mail.MailStore, box string, gen int) tea.Cmd {
	sn, ok := st.(mail.Snapshotter)
	if !ok {
		return nil
	}
	return func() tea.Msg {
		snap, ok := sn.CachedConversations(box)
		if !ok || len(snap.Rows) == 0 {
			return nil
		}
		return snapshotLoaded{mailbox: box, folderGen: gen, snap: snap}
	}
}

// applySnapshotLoaded mostra l'istantanea come lista provvisoria. Ricevitore
// puntatore: muta il modello del chiamante. Si applica solo se cartella e
// folderGen coincidono, se non c'e una ricerca e se la lista fresca non e gia
// arrivata (un risultato fresco vince sempre, D16-T11).
func (m *Model) applySnapshotLoaded(ev snapshotLoaded) tea.Cmd {
	rows := ev.snap.Rows
	if m.wizardActive || ev.mailbox != m.folder.Name || ev.folderGen != m.folderGen ||
		m.queryRaw != "" || m.listSource == srcFresh || len(rows) == 0 {
		return nil
	}
	m.listSource = srcCached
	m.listLoaded = true // lo splash sparisce: la lista c'e, anche se provvisoria
	m.provErr = false
	m.snapAge = ""
	if !ev.snap.SavedAt.IsZero() {
		age := m.clock().Sub(ev.snap.SavedAt)
		m.snapAge = ageText(age)
		trace.SnapshotAge(trace.OpSnapList, age)
	}
	m.refs = latestRefs(rows)
	m.sums = nil
	m.rememberConvs(rows)
	m.exhausted = len(rows) < pageSize && !ev.snap.Limited
	m.loadingMore = false
	m.setSel(nil)
	return m.inbox.SetConversations(m.convsOf(m.refs))
}

// ageText rende l'eta dell'istantanea per la barra ("2 ore fa").
func ageText(d time.Duration) string {
	plural := func(n int, one, many string) string {
		if n == 1 {
			return "1 " + one + " fa"
		}
		return fmt.Sprintf("%d %s fa", n, many)
	}
	switch {
	case d < time.Minute:
		return "meno di un minuto fa"
	case d < time.Hour:
		return plural(int(d/time.Minute), "minuto", "minuti")
	case d < 24*time.Hour:
		return plural(int(d/time.Hour), "ora", "ore")
	}
	return plural(int(d/(24*time.Hour)), "giorno", "giorni")
}

// provStatus e il testo della barra con la lista provvisoria (D16-T12, Q16-4):
// "aggiornamento… (dati di 2 ore fa)"; se la lista fresca e fallita, "lista non
// aggiornata". Vuoto con la lista fresca o col Connect fallito (in quel caso c'e
// lo status persistente di connResult).
func (m *Model) provStatus() string {
	if m.listSource != srcCached || m.connectFailed {
		return ""
	}
	text := "aggiornamento…"
	if m.provErr {
		text = "lista non aggiornata"
	}
	if m.snapAge != "" {
		text += " (dati di " + m.snapAge + ")"
	}
	return text
}
