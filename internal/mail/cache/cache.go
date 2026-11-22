// Package cache persiste messaggi normalizzati in bbolt (spec §cache).
// Schema: bucket "meta:<mailbox>" chiave "uidvalidity";
// bucket "msgs:<mailbox>" chiave = uid, valore = Message in JSON.
package cache

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
	bbolterrors "go.etcd.io/bbolt/errors"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Cache è la cache locale dei messaggi.
type Cache struct {
	db *bolt.DB
}

// schemaVersion è la versione dei messaggi in cache. Piano 8, Task 8: alla 2
// msg.MessageRef ha GmailID, ThreadID e Labels; le entry della versione
// precedente non hanno quei campi e vengono scartate all'apertura.
const schemaVersion = 2

// Open apre/crea il database bbolt in path.
func Open(path string) (*Cache, error) {
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, fmt.Errorf("cache open %s: %w", path, err)
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("cache schema %s: %w", path, err)
	}
	return &Cache{db: db}, nil
}

// migrate scarta i bucket dei messaggi se la versione dello schema non è quella
// attuale. I bucket meta: (uidvalidity) restano.
func migrate(db *bolt.DB) error {
	return db.Update(func(tx *bolt.Tx) error {
		schema, err := tx.CreateBucketIfNotExists([]byte("schema"))
		if err != nil {
			return err
		}
		if v := schema.Get([]byte("version")); v != nil && binaryUint32(v) == schemaVersion {
			return nil
		}
		var stale [][]byte
		err = tx.ForEach(func(name []byte, _ *bolt.Bucket) error {
			if bytes.HasPrefix(name, []byte("msgs:")) {
				stale = append(stale, append([]byte(nil), name...))
			}
			return nil
		})
		if err != nil {
			return err
		}
		for _, name := range stale {
			if err := tx.DeleteBucket(name); err != nil {
				return err
			}
		}
		return schema.Put([]byte("version"), uint32Bytes(schemaVersion))
	})
}

// SetMessage salva un messaggio; se uidValidity è cambiata da quella
// memorizzata, la mailbox viene svuotata prima (invalidazione spec).
func (c *Cache) SetMessage(mailbox string, uidValidity uint32, m *msg.Message) error {
	return c.db.Update(func(tx *bolt.Tx) error {
		meta, err := tx.CreateBucketIfNotExists([]byte("meta:" + mailbox))
		if err != nil {
			return err
		}
		if prev := meta.Get([]byte("uidvalidity")); prev != nil && binaryUint32(prev) != uidValidity {
			if err := tx.DeleteBucket([]byte("msgs:" + mailbox)); err != nil && !errors.Is(err, bbolterrors.ErrBucketNotFound) {
				return err
			}
		}
		if err := meta.Put([]byte("uidvalidity"), uint32Bytes(uidValidity)); err != nil {
			return err
		}
		msgs, err := tx.CreateBucketIfNotExists([]byte("msgs:" + mailbox))
		if err != nil {
			return err
		}
		data, err := json.Marshal(m)
		if err != nil {
			return fmt.Errorf("marshal %s: %w", m.Ref.ID, err)
		}
		return msgs.Put(uidKey(m.Ref.ID), data)
	})
}

// GetMessage recupera per mailbox+uid; secondo valore false = miss.
func (c *Cache) GetMessage(mailbox string, uid uint32) (*msg.Message, bool, error) {
	var out *msg.Message
	err := c.db.View(func(tx *bolt.Tx) error {
		msgs := tx.Bucket([]byte("msgs:" + mailbox))
		if msgs == nil {
			return nil
		}
		data := msgs.Get(uidKey(msg.NewID(mailbox, uid)))
		if data == nil {
			return nil
		}
		var m msg.Message
		if err := json.Unmarshal(data, &m); err != nil {
			return fmt.Errorf("unmarshal %s/%d: %w", mailbox, uid, err)
		}
		out = &m
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return out, out != nil, nil
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
	return c.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket([]byte("msgs:" + mailbox)); err != nil && !errors.Is(err, bbolterrors.ErrBucketNotFound) {
			return err
		}
		if err := tx.DeleteBucket([]byte("meta:" + mailbox)); err != nil && !errors.Is(err, bbolterrors.ErrBucketNotFound) {
			return err
		}
		return nil
	})
}

// Close chiude il database.
func (c *Cache) Close() error { return c.db.Close() }

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
