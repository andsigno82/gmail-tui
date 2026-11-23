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
