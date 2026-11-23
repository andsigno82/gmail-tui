package thread

// Cache di rendering del corpo (Piano 15, Task 14, D15-T2): il modello a blocchi
// non si salva su disco (si ricava da BodyHTML a render time) e il risultato reso
// si tiene in memoria per (messaggio, larghezza, tema, variante), con dimensione
// massima fissa: alla 17a voce si espelle la piu vecchia (ordine di inserimento).
// E un puntatore creato in newBase: i Model sono copiati per valore da bubbletea,
// la cache e le richieste in volo restano condivise tra le copie (gotcha dei
// value receiver, AGENTS.md).

import (
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/render"
)

// renderCacheMax e il numero massimo di voci (D15-T2).
const renderCacheMax = 16

// cacheKey identifica un corpo reso. theme e l'impronta degli stili (cambia con
// il tema), variant distingue il corpo da HTML da quello da testo semplice.
type cacheKey struct {
	id      msg.MessageID
	width   int
	theme   string
	variant string
}

// renderCache e la cache FIFO a 16 voci piu l'insieme dei rendering in volo
// (D15-T5), per non lanciare due volte lo stesso lavoro.
type renderCache struct {
	m       map[cacheKey]render.Out
	order   []cacheKey
	pending map[cacheKey]bool
}

func newRenderCache() *renderCache {
	return &renderCache{m: map[cacheKey]render.Out{}, pending: map[cacheKey]bool{}}
}

// get ritorna la voce se presente. Su cache nil (Model zero) non trova nulla.
func (c *renderCache) get(k cacheKey) (render.Out, bool) {
	if c == nil {
		return render.Out{}, false
	}
	o, ok := c.m[k]
	return o, ok
}

// put inserisce la voce; oltre renderCacheMax espelle la piu vecchia. Riscrivere
// una chiave presente aggiorna il valore senza cambiarne l'ordine.
func (c *renderCache) put(k cacheKey, o render.Out) {
	if c == nil {
		return
	}
	if _, ok := c.m[k]; !ok {
		if len(c.order) >= renderCacheMax {
			delete(c.m, c.order[0])
			c.order = c.order[1:]
		}
		c.order = append(c.order, k)
	}
	c.m[k] = o
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
