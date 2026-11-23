package imap

// Search (piano 2, Task 6): ricerca server-side senza paginazione, con
// post-filtro client-side per HasAttachment.

import (
	"context"
	"errors"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// maxAttachmentCandidates limita il post-filtro HasAttachment: al più tanti
// ref candidati (i più recenti) passano per FetchMessage, così una ricerca
// su una mailbox enorme non scarica il mondo.
const maxAttachmentCandidates = 200

// Search cerca i messaggi che matchano q, SENZA paginazione (contratto
// mail.MailStore: valgono le semantica e i limiti documentati su
// ListMessages).
//
// Il flusso è quello di ListMessages: Select della mailbox q.Label (o
// INBOX), UIDSearch con i criteri tradotti da searchCriteria(q), fetch di
// ENVELOPE+FLAGS+INTERNALDATE, mapping in ref dal più nuovo al più vecchio
// — ma sulla lista intera, senza finestra di pagina.
//
// Se q.HasAttachment: IMAP SEARCH non ha un criterio per gli allegati, quindi
// è applicato come post-filtro best-effort client-side (filterWithAttachment).
//
// Le API go-imap v2 (beta.9) non prendono context: come per Connect, Search
// controlla solo ctx.Err() in ingresso.
//
// Gate (fix review + Piano 3 Task 1): come ListMessages, l'intera sequenza —
// incluso il post-filtro HasAttachment — corre sotto lo STESSO holder: il
// fetch di corpo del post-filtro NON ri-acquisisce (con la mutua esclusione
// piena acquire non è rientrante: sarebbe deadlock), gira come
// fetchMessage(heldGate=true) sotto l'holder di Search.
func (s *Store) Search(ctx context.Context, q msg.Query) ([]msg.MessageRef, error) {
	if q.Raw != "" { // Piano 8, Task 8: sintassi Gmail nativa, senza post-filtri
		return s.searchRaw(ctx, q)
	}
	defer s.acquire(ctx)()

	mailbox, uids, err := s.selectAndSearch(ctx, q)
	if err != nil {
		return nil, err
	}
	refs, err := s.fetchRefs(mailbox, uids)
	if err != nil {
		return nil, err
	}
	if !q.HasAttachment {
		return refs, nil
	}
	return s.filterWithAttachment(ctx, refs)
}

// filterWithAttachment è il post-filtro best-effort di Query.HasAttachment:
// IMAP SEARCH non ha un criterio per gli allegati, quindi per ogni ref
// candidato (al più maxAttachmentCandidates, i più recenti) si chiama
// fetchMessage(heldGate=true) — cache-first, quindi a buon mercato sulle
// ripetizioni; gira sotto l'holder di Search, senza ri-acquisire — e
// si tengono solo i messaggi con len(Attachments) > 0. I ref oltre il cap
// non sono esaminati né restituiti: con HasAttachment la ricerca restituisce
// al più maxAttachmentCandidates risultati. Un messaggio sparito tra SEARCH
// e FETCH (ErrMessageNotFound) è saltato; gli altri errori (rete, cache)
// sono propagati.
func (s *Store) filterWithAttachment(ctx context.Context, refs []msg.MessageRef) ([]msg.MessageRef, error) {
	if len(refs) > maxAttachmentCandidates {
		refs = refs[:maxAttachmentCandidates]
	}
	out := make([]msg.MessageRef, 0, len(refs))
	for _, ref := range refs {
		m, err := s.fetchMessage(ctx, ref.ID, true)
		if err != nil {
			if errors.Is(err, ErrMessageNotFound) {
				continue // espunto tra SEARCH e FETCH: best-effort, skip
			}
			return nil, err
		}
		if len(m.Attachments) > 0 {
			out = append(out, ref)
		}
	}
	return out, nil
}
