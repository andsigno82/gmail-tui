package app

import (
	"runtime"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Indagine prestazioni (dopo il Piano 15): Model pesa ~340 KB e i metodi con
// ricevitore per valore lo copiavano sullo heap a ogni chiamata (anche una decina
// di volte per View e per tasto). I metodi di sola lettura del percorso di View e
// syncPreview/loadMoreAtEnd hanno ricevitore puntatore: questo test fissa il
// tetto di byte allocati cosi una regressione (un helper tornato per valore) si vede.
func totalAllocPer(n int, f func()) uint64 {
	var a, b runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&a)
	for range n {
		f()
	}
	runtime.ReadMemStats(&b)
	return (b.TotalAlloc - a.TotalAlloc) / uint64(n)
}

func TestViewNonCopiaIlModelSulloHeap(t *testing.T) {
	m := pagedModel(t, perfStore())
	m, _ = step(m, tea.WindowSizeMsg{Width: 120, Height: 35})
	const limit = 1 << 20 // prima: ~2,1 MB per View; ora ~0,2 MB
	if got := totalAllocPer(20, func() { _ = m.View() }); got > limit {
		t.Errorf("View alloca %d byte, tetto %d: un metodo del percorso di View e tornato per valore?", got, limit)
	}
}

func TestTastoInListaNonCopiaIlModelDiTroppo(t *testing.T) {
	m := pagedModel(t, perfStore())
	m, _ = step(m, tea.WindowSizeMsg{Width: 120, Height: 35})
	const limit = 3 << 20 // prima: ~4,4 MB per tasto; ora ~2,3 MB (di cui parte e del passo di test)
	j := pressKey("j")
	k := pressKey("k")
	i := 0
	got := totalAllocPer(20, func() {
		key := j
		if i%2 == 1 {
			key = k
		}
		i++
		m, _ = step(m, key)
	})
	if got > limit {
		t.Errorf("un tasto in lista alloca %d byte, tetto %d", got, limit)
	}
}
