package thread

// Cache di rendering del corpo (Piano 15, Task 14, D15-T2): il modello a blocchi
// non si salva su disco (si ricava da BodyHTML a render time) e il risultato reso
// si tiene in memoria per (messaggio, larghezza, tema, variante), con dimensione
// massima fissa: oltre renderCacheMax voci si espelle la piu vecchia (ordine di
// inserimento) che NON sia in uso dal contenuto corrente.
//
// Revisione indipendente 2026-11-23, punto 6: (a) le voci stanno in un
// RenderCache che l'app crea una volta sola e passa a ogni thread (NewLazyCached):
// la cache vale tra messaggi, riaprire un thread non rilancia i rendering;
// (b) i corpi espansi in uso (marcati da bodyOut a ogni refresh) non si espellono,
// altrimenti con piu di renderCacheMax corpi grandi ogni risultato espelleva un
// corpo ancora visibile, che si rilanciava all'infinito. Il numero di voci puo
// quindi superare renderCacheMax finche tanti corpi sono espansi insieme.
// I rendering in volo restano per Model (la generazione e del Model).
// Sono puntatori: i Model sono copiati per valore da bubbletea, cache e richieste
// in volo restano condivise tra le copie (gotcha dei value receiver, AGENTS.md).

import (
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/render"
)

// renderCacheMax e il numero massimo di voci non in uso (D15-T2).
const renderCacheMax = 16

// cacheKey identifica un corpo reso. theme e l'impronta degli stili (cambia con
// il tema), variant distingue il corpo da HTML da quello da testo semplice.
type cacheKey struct {
	id      msg.MessageID
	width   int
	theme   string
	variant string
}

// RenderCache sono i corpi resi, condivisibili tra i thread dell'app. Si usa solo
// dal goroutine di Update (i rendering in background restituiscono il risultato
// con un messaggio): nessun lock.
type RenderCache struct {
	m     map[cacheKey]render.Out
	order []cacheKey
	inUse map[cacheKey]struct{} // chiavi richieste dall'ultimo refresh: non si espellono
}

// NewRenderCache crea una cache vuota da condividere (NewLazyCached).
func NewRenderCache() *RenderCache {
	return &RenderCache{m: map[cacheKey]render.Out{}, inUse: map[cacheKey]struct{}{}}
}

// renderCache e la vista di un Model: le voci condivise piu l'insieme dei
// rendering in volo di questo Model (D15-T5), per non lanciare due volte lo
// stesso lavoro.
type renderCache struct {
	*RenderCache
	pending map[cacheKey]bool
}

// newRenderCache crea una cache privata (un solo Model).
func newRenderCache() *renderCache { return newRenderCacheOn(nil) }

// newRenderCacheOn usa le voci di rc (nil = private).
func newRenderCacheOn(rc *RenderCache) *renderCache {
	if rc == nil {
		rc = NewRenderCache()
	}
	return &renderCache{RenderCache: rc, pending: map[cacheKey]bool{}}
}

// beginRound azzera le chiavi in uso: l'inizio di un refresh, che le ri-marca
// tutte via use.
func (c *renderCache) beginRound() {
	if c != nil {
		c.inUse = map[cacheKey]struct{}{}
	}
}

// use marca k come richiesta dal contenuto corrente (non espellibile).
func (c *renderCache) use(k cacheKey) {
	if c != nil {
		c.inUse[k] = struct{}{}
	}
}

// get ritorna la voce se presente. Su cache nil (Model zero) non trova nulla.
func (c *renderCache) get(k cacheKey) (render.Out, bool) {
	if c == nil {
		return render.Out{}, false
	}
	o, ok := c.m[k]
	return o, ok
}

// put inserisce la voce; oltre renderCacheMax espelle la piu vecchia non in uso.
// Riscrivere una chiave presente aggiorna il valore senza cambiarne l'ordine.
func (c *renderCache) put(k cacheKey, o render.Out) {
	if c == nil {
		return
	}
	if _, ok := c.m[k]; !ok {
		c.order = append(c.order, k)
	}
	c.m[k] = o
	for len(c.order) > renderCacheMax {
		i := 0
		for i < len(c.order) {
			if _, busy := c.inUse[c.order[i]]; !busy {
				break
			}
			i++
		}
		if i == len(c.order) {
			return // tutte in uso: la cache cresce, non si rompe il contenuto visibile
		}
		delete(c.m, c.order[i])
		c.order = append(c.order[:i], c.order[i+1:]...)
	}
}

// lookup ritorna la voce o la calcola con compute e la memorizza: stessa chiave,
// nessun ricalcolo.
func (c *renderCache) lookup(k cacheKey, compute func() render.Out) render.Out {
	if o, ok := c.get(k); ok {
		return o
	}
	o := compute()
	c.put(k, o)
	return o
}

// markPending segna k come in volo; false se lo era gia (o cache nil).
func (c *renderCache) markPending(k cacheKey) bool {
	if c == nil || c.pending[k] {
		return false
	}
	c.pending[k] = true
	return true
}

// donePending toglie k dai rendering in volo.
func (c *renderCache) donePending(k cacheKey) {
	if c != nil {
		delete(c.pending, k)
	}
}

// resetPending dimentica i rendering in volo: al cambio di generazione i loro
// risultati saranno scartati, quindi vanno rilanciati se servono ancora.
func (c *renderCache) resetPending() {
	if c != nil {
		c.pending = map[cacheKey]bool{}
	}
}
