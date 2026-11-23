package cache

// Chiusura con flush dei tocchi e compattazione condizionata (Piano 16, Task 3,
// D16-T8).

import (
	"errors"
	"os"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	// compactMinSize è la dimensione del file sotto cui non si compatta mai.
	compactMinSize = 4 << 20
	// compactTxMax limita la dimensione di ogni transazione della copia.
	compactTxMax = 1 << 20
)

// Close scrive i tocchi LRU rimasti in memoria, compatta il file se serve e chiude
// il database. Un errore di compattazione si ignora: il file originale resta valido.
func (c *Cache) Close() error {
	flushErr := c.flush()
	tmp := ""
	if c.shouldCompact() {
		tmp = c.compactToTemp()
	}
	closeErr := c.db.Close()
	if tmp != "" {
		if closeErr != nil || os.Rename(tmp, c.path) != nil {
			_ = os.Remove(tmp)
		}
	}
	return errors.Join(flushErr, closeErr)
}

// shouldCompact: file oltre 4 MiB con pagine libere (libere + in sospeso) oltre il
// 50% della dimensione del file.
func (c *Cache) shouldCompact() bool {
	fi, err := os.Stat(c.path)
	if err != nil || fi.Size() <= compactMinSize {
		return false
	}
	free := int64(c.db.Stats().FreeAlloc)
	return free*2 > fi.Size()
}

// compactToTemp copia il database compattato in <path>.tmp (0600, fsync) e ne
// restituisce il percorso; "" se la copia fallisce (il file temporaneo viene rimosso).
func (c *Cache) compactToTemp() string {
	tmp := c.path + ".tmp"
	_ = os.Remove(tmp) // residuo di una compattazione interrotta
	dst, err := bolt.Open(tmp, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		_ = os.Remove(tmp)
		return ""
	}
	if err := bolt.Compact(dst, c.db, compactTxMax); err != nil {
		_ = dst.Close()
		_ = os.Remove(tmp)
		return ""
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(tmp)
		return ""
	}
	f, err := os.OpenFile(tmp, os.O_RDWR, 0o600)
	if err != nil {
		_ = os.Remove(tmp)
		return ""
	}
	syncErr := f.Sync()
	if cerr := f.Close(); syncErr != nil || cerr != nil {
		_ = os.Remove(tmp)
		return ""
	}
	return tmp
}
