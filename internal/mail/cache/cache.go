// Package cache persiste messaggi normalizzati in bbolt (spec §cache).
// Schema 4 (Piano 16, Task 3): bucket "meta:<mailbox>" chiave "uidvalidity";
// bucket "msgs:<mailbox>" chiave = uid, valore = Message in JSON; bucket "lru:<ns>"
// con l'indice LRU (lru.go); bucket "schema" con versione e account (hash).
package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
	bbolterrors "go.etcd.io/bbolt/errors"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Cache è la cache locale dei messaggi.
type Cache struct {
	db   *bolt.DB
	path string

	// mu protegge pending: i tocchi in lettura non ancora scritti su disco
	// (Piano 16, Task 3, D16-T6).
	mu      sync.Mutex
	pending []pendingTouch
}

// schemaVersion è la versione dei messaggi in cache. Piano 8, Task 8: alla 2
// msg.MessageRef ha GmailID, ThreadID e Labels. Alla 3 anche il fetch del corpo li
// riempie (review punto 1): le entry della versione 2 hanno quei campi a zero e
// vengono scartate all'apertura. Piano 16, Task 3: alla 4 c'è l'indice LRU dei
// corpi (migrate.go: la 3 si migra senza perdere i corpi).
const schemaVersion = 4

// Open apre/crea il database bbolt in path. Piano 16, Task 3, D16-A4: la cartella è
// 0700 e il file 0600 (se esistevano più larghi vengono riportati a questi permessi).
func Open(path string) (*Cache, error) {
	if err := ensurePrivateDir(filepath.Dir(path)); err != nil {
		return nil, fmt.Errorf("cache open %s: %w", path, err)
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, fmt.Errorf("cache open %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("cache open %s: %w", path, err)
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("cache schema %s: %w", path, err)
	}
	return &Cache{db: db, path: path}, nil
}

// ensurePrivateDir crea dir con permessi 0700 e, se esisteva con permessi più
// larghi (le versioni precedenti la creavano 0755), la riporta a 0700.
func ensurePrivateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return os.Chmod(dir, 0o700)
	}
	return nil
}

// epochTx registra l'epoca (UIDVALIDITY) vista per mailbox. Se è diversa da quella
// salvata scarta i corpi della mailbox, le loro voci LRU e le istantanee (Task 4); riporta se ha scartato. La prima epoca vista non è un cambio.
func epochTx(tx *bolt.Tx, mailbox string, uidValidity uint32) (bool, error) {
	meta, err := tx.CreateBucketIfNotExists([]byte("meta:" + mailbox))
	if err != nil {
		return false, err
	}
	changed := false
	if prev := meta.Get([]byte("uidvalidity")); prev != nil && binaryUint32(prev) != uidValidity {
		if err := dropMailboxData(tx, mailbox); err != nil {
			return false, err
		}
		changed = true
	}
	return changed, meta.Put([]byte("uidvalidity"), uint32Bytes(uidValidity))
}

// dropMailboxData toglie i corpi della mailbox, le voci LRU che li indicizzano e le
// istantanee legate alla mailbox (Piano 16, Task 4, R3).
func dropMailboxData(tx *bolt.Tx, mailbox string) error {
	if err := tx.DeleteBucket([]byte("msgs:" + mailbox)); err != nil && !errors.Is(err, bbolterrors.ErrBucketNotFound) {
		return err
	}
	if err := lruDropBucket(tx, lruBody, "msgs:"+mailbox); err != nil {
		return err
	}
	return dropSnapshots(tx, mailbox)
}

// NoteUIDValidity registra l'epoca che il server ha appena dichiarato per mailbox
// (Piano 16, R3). Se è diversa da quella in cache scarta i corpi della mailbox e
// riporta true. Con epoca invariata non scrive nulla.
func (c *Cache) NoteUIDValidity(mailbox string, v uint32) (changed bool, err error) {
	same := false
	err = c.db.View(func(tx *bolt.Tx) error {
		if meta := tx.Bucket([]byte("meta:" + mailbox)); meta != nil {
			if prev := meta.Get([]byte("uidvalidity")); prev != nil && binaryUint32(prev) == v {
				same = true
			}
		}
		return nil
	})
	if err != nil || same {
		return false, err
	}
	err = c.update(func(tx *bolt.Tx) error {
		var e error
		changed, e = epochTx(tx, mailbox, v)
		return e
	})
	return changed, err
}

// SetMessage salva un messaggio; se uidValidity è cambiata da quella
// memorizzata, la mailbox viene svuotata prima (invalidazione spec). Tocca l'unità
// LRU del messaggio ed espelle le meno usate oltre MaxBodies.
func (c *Cache) SetMessage(mailbox string, uidValidity uint32, m *msg.Message) error {
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", m.Ref.ID, err)
	}
	return c.update(func(tx *bolt.Tx) error {
		if _, err := epochTx(tx, mailbox, uidValidity); err != nil {
			return err
		}
		msgs, err := tx.CreateBucketIfNotExists([]byte("msgs:" + mailbox))
		if err != nil {
			return err
		}
		key := uidKey(m.Ref.ID)
		if err := msgs.Put(key, data); err != nil {
			return err
		}
		entry := lruEntry{Bucket: "msgs:" + mailbox, Key: key}
		if err := lruTouch(tx, lruBody, bodyUnit(mailbox, key, m.Ref.GmailID), []lruEntry{entry}); err != nil {
			return err
		}
		return lruEvict(tx, lruBody, MaxBodies, dropEntry(tx))
	})
}

// GetMessage recupera per mailbox+uid; secondo valore false = miss. Una voce non
// decodificabile è un miss. Un hit registra il tocco LRU solo in memoria (nessuna
// scrittura su disco, D16-T6).
func (c *Cache) GetMessage(mailbox string, uid uint32) (*msg.Message, bool, error) {
	var out *msg.Message
	key := uidKey(msg.NewID(mailbox, uid))
	err := c.db.View(func(tx *bolt.Tx) error {
		msgs := tx.Bucket([]byte("msgs:" + mailbox))
		if msgs == nil {
			return nil
		}
		data := msgs.Get(key)
		if data == nil {
			return nil
		}
		var m msg.Message
		if err := json.Unmarshal(data, &m); err != nil {
			return nil // voce corrotta = miss; la prossima SetMessage la riscrive
		}
		out = &m
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if out == nil {
		return nil, false, nil
	}
	c.noteTouch(lruBody, bodyUnit(mailbox, key, out.Ref.GmailID))
	return out, true, nil
}

// UIDValidity restituisce l'ultima uidvalidity memorizzata per mailbox.
func (c *Cache) UIDValidity(mailbox string) (uint32, bool, error) {
	var v uint32
	var ok bool
	err := c.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte("meta:" + mailbox))
		if meta == nil {
			return nil
		}
		if b := meta.Get([]byte("uidvalidity")); b != nil {
			v, ok = binaryUint32(b), true
		}
		return nil
	})
	return v, ok, err
}

// DropMailbox svuota i dati della mailbox.
func (c *Cache) DropMailbox(mailbox string) error {
	return c.update(func(tx *bolt.Tx) error {
		if err := dropMailboxData(tx, mailbox); err != nil {
			return err
		}
		if err := tx.DeleteBucket([]byte("meta:" + mailbox)); err != nil && !errors.Is(err, bbolterrors.ErrBucketNotFound) {
			return err
		}
		return nil
	})
}

// uidKey deriva la chiave bucket dal MessageID (8 byte big-endian).
func uidKey(id msg.MessageID) []byte {
	_, uid, err := id.Split()
	if err != nil {
		return []byte(id) // fallback: id grezza come stringa
	}
	return uint32Bytes(uid)
}

func uint32Bytes(v uint32) []byte {
	return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
}

func binaryUint32(b []byte) uint32 {
	if len(b) != 4 {
		return 0
	}
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

// UpdateGmailCopies applica mutate a ogni copia in cache del messaggio con GmailID
// gmailID, in qualunque mailbox (INBOX, Tutti i messaggi, label): stesso messaggio
// Gmail, stessi flag e label (Piano 8, review punto 3). mutate riporta se ha cambiato
// la copia: solo le copie cambiate vengono riscritte. gmailID 0 (non Gmail) non fa nulla.
func (c *Cache) UpdateGmailCopies(gmailID uint64, mutate func(*msg.Message) bool) error {
	if gmailID == 0 {
		return nil
	}
	type pendingCopy struct {
		bucket, key, data []byte
	}
	var todo []pendingCopy
	err := c.db.View(func(tx *bolt.Tx) error {
		return tx.ForEach(func(name []byte, b *bolt.Bucket) error {
			if !strings.HasPrefix(string(name), "msgs:") {
				return nil
			}
			return b.ForEach(func(k, v []byte) error {
				var m msg.Message
				if err := json.Unmarshal(v, &m); err != nil {
					return nil // voce corrotta: non è una copia aggiornabile
				}
				if m.Ref.GmailID != gmailID || !mutate(&m) {
					return nil
				}
				data, err := json.Marshal(&m)
				if err != nil {
					return fmt.Errorf("marshal %s: %w", m.Ref.ID, err)
				}
				todo = append(todo, pendingCopy{bucket: append([]byte(nil), name...), key: append([]byte(nil), k...), data: data})
				return nil
			})
		})
	})
	if err != nil || len(todo) == 0 {
		return err
	}
	return c.update(func(tx *bolt.Tx) error {
		for _, p := range todo {
			if err := tx.Bucket(p.bucket).Put(p.key, p.data); err != nil {
				return err
			}
		}
		return nil
	})
}
