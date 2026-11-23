package cache

// Limiti della cache (Piano 16, Task 2, D16-A3): solo costanti, nessun campo di
// config.
const (
	// MaxBodies e il numero di messaggi Gmail (D16-T4) i cui corpi restano in
	// cache: oltre, si rimuove il meno usato di recente (LRU).
	MaxBodies = 200
	// MaxThreadSnapshots e il numero di istantanee di thread conservate.
	MaxThreadSnapshots = 200
	// MaxListSnapshots e il numero di istantanee di lista (una per cartella).
	MaxListSnapshots = 50
	// SnapshotRows e il numero di righe di conversazione salvate per cartella.
	SnapshotRows = 50
)
