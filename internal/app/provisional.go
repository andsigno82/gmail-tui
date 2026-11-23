package app

// Stato provvisorio = sola lettura locale (Piano 16, Task 9, regola R1). Righe e
// thread mostrati da un'istantanea portano MessageID dell'epoca in cui sono stati
// salvati: su di essi si puo solo navigare, leggere e aprire cio che sta in cache.
// Le azioni sono rifiutate con un toast informativo, senza toccare lo store. UN
// SOLO punto di controllo per i tasti (refuseWhileProvisional); gli altri
// percorsi (pagina successiva, ripresa bozza, :save/:open, click sugli allegati,
// segna-come-letto automatico) usano provisionalReason. In difesa, R4 (mail.Guard)
// ferma comunque ogni chiamata con un ID non confermato.

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/thread"
	"github.com/andsigno82/gmail-tui/internal/ui/toast"
)

const (
	// provisionalToast e il rifiuto di R1.
	provisionalToast = "aggiornamento in corso: azione disponibile tra poco"
	// goneToast: la riga aperta non e piu nella lista fresca (R2).
	goneToast = "conversazione non più in questa cartella"
)

// provisionalReason dice perche le azioni sono rifiutate nella vista corrente (""
// = azionabile): lista da istantanea; thread (a schermo intero o in anteprima) da
// istantanea o aperto da una riga provvisoria; riga sparita dalla lista fresca.
func (m *Model) provisionalReason() string {
	switch m.view {
	case viewInbox:
		if m.listSource == srcCached {
			return provisionalToast
		}
		if m.previewReady() && m.threadCached {
			return provisionalToast
		}
	case viewThread:
		if m.openGone {
			return goneToast
		}
		if m.threadCached || m.openProv {
			return provisionalToast
		}
	}
	return ""
}

// freshRow ritrova nella lista fresca la riga della conversazione old (Piano 16,
// Task 10, R2): per ThreadID + GmailID, MAI per MessageID (stesso ID, un altro
// messaggio, e il caso del cambio d'epoca). Se il messaggio piu recente della
// conversazione e cambiato (posta nuova) il ThreadID, che in Gmail e globale e
// indipendente dall'epoca, identifica comunque la stessa conversazione. Senza
// ThreadID (server senza X-GM-EXT-1) nessuna corrispondenza.
func (m *Model) freshRow(old msg.MessageRef) (msg.MessageRef, bool) {
	if old.ThreadID == 0 {
		return msg.MessageRef{}, false
	}
	var loose *msg.MessageRef
	for i := range m.refs {
		r := &m.refs[i]
		if r.ThreadID != old.ThreadID {
			continue
		}
		if r.GmailID == old.GmailID {
			return *r, true
		}
		if loose == nil {
			loose = r
		}
	}
	if loose != nil {
		return *loose, true
	}
	return msg.MessageRef{}, false
}

// realignOpen riallinea la conversazione aperta e l'anteprima alla lista fresca
// appena arrivata (Piano 16, Task 10, R2): la riga provvisoria diventa la riga fresca
// (azionabile) o, se non c'e piu, le azioni sulla conversazione restano rifiutate
// (openGone). Il segna-come-letto automatico sospeso fino a qui parte ora, sugli ID
// del thread fresco. Chiamato con listSource gia srcFresh e m.refs gia sostituiti.
func (m *Model) realignOpen() tea.Cmd {
	var cmds []tea.Cmd
	if m.openProv {
		m.openProv = false
		if row, ok := m.freshRow(m.pendingRef); ok {
			m.pendingRef, m.rowRef, m.openGone = row, row, false
			if m.view == viewThread && m.thread.Loaded() && !m.threadCached {
				if unread := m.thread.UnreadIDs(); len(unread) > 0 {
					m.thread.MarkRead()
					cmds = append(cmds, markReadIDs(m.store, m.pendingRef, unread))
				}
			}
		} else {
			m.openGone = true
		}
	}
	if m.prevHas {
		if row, ok := m.freshRow(m.prevRef); ok {
			m.prevRef = row
			if !(m.thread.Loaded() && m.thread.AnchorID() == m.prevReq) {
				// Piano 16, Task 13: nessun thread costruito per la richiesta vecchia (il timer di
				// caricamento e ancora in volo): l'ancora del thread futuro e l'ID fresco, e la
				// richiesta riparte con un token nuovo (un esito vecchio si scarta).
				m.prevReq = row.ID
				m.prevTok++
				cmds = append(cmds, previewTimer(m.prevTok))
			}
			if m.previewVisible() && m.previewReady() && !m.threadCached {
				nm, c := m.previewMarkRead(m.prevTok)
				*m = nm
				cmds = append(cmds, c)
			}
		} else {
			// La riga in anteprima non c'e piu: syncPreview riparte dal cursore.
			m.prevHas, m.prevReq = false, ""
			m.prevTok++
			if m.view == viewInbox {
				m.thread, m.threadCached = thread.Model{}, false
			}
		}
	}
	return tea.Batch(cmds...)
}

// missingBodiesCmd chiede i corpi che dopo la conciliazione mancano: del messaggio
// selezionato e degli espansi (un BodyRequested arrivato sul thread provvisorio e stato
// ignorato). Piano 16, Task 13: prima si chiedeva solo il selezionato e un messaggio
// espanso a mano mentre il thread era provvisorio restava su "corpo in caricamento".
func (m *Model) missingBodiesCmd() tea.Cmd {
	if !m.thread.Loaded() {
		return nil
	}
	var cmds []tea.Cmd
	for _, id := range m.thread.MissingBodies() {
		m.pending++
		cmds = append(cmds, fetchBody(m.store, m.threadGen, m.thread.AnchorID(), id))
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(append(cmds, m.spinner.Tick)...)
}

// isActionKey dice se il tasto e un'azione che agisce su righe o messaggi: a, #, s,
// v, l, r, f e (solo in lista) x, X. c, /, m, R, u, esc, navigazione restano attivi.
func (m *Model) isActionKey(k tea.KeyPressMsg) bool {
	if key.Matches(k, keys.Archive, keys.Trash, keys.Star, keys.Read, keys.Label, keys.Reply, keys.Forward) {
		return true
	}
	return m.view == viewInbox && key.Matches(k, keys.Select, keys.SelectAll)
}

// refuseWhileProvisional e il punto unico di R1 per i tasti: se k e un'azione e i
// dati sono provvisori, ritorna (true, toast) e il chiamante non deve fare altro.
// Ricevitore puntatore: il toast muta il modello del chiamante.
func (m *Model) refuseWhileProvisional(k tea.KeyPressMsg) (bool, tea.Cmd) {
	if !m.isActionKey(k) {
		return false, nil
	}
	reason := m.provisionalReason()
	if reason == "" {
		return false, nil
	}
	return true, m.toast.Push(toast.Info, reason)
}
