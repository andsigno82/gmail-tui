package cache

// Migrazione dello schema (Piano 8, Task 8; Piano 16, Task 3, D16-T7).

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// migrate porta il database allo schema corrente. Versione 4: nulla. Versione 3:
// i corpi restano e si costruisce l'indice LRU (le 200 unità più recenti per
// Ref.Date; il resto viene rimosso). Qualunque altra versione (assente, < 3 o
// sconosciuta): si scartano i bucket dei messaggi e dell'indice. I bucket meta:
// (uidvalidity) e l'account restano.
func migrate(db *bolt.DB) error {
	return db.Update(func(tx *bolt.Tx) error {
		schema, err := tx.CreateBucketIfNotExists([]byte("schema"))
		if err != nil {
			return err
		}
		switch v := schema.Get([]byte("version")); {
		case v != nil && binaryUint32(v) == schemaVersion:
			return nil
		case v != nil && binaryUint32(v) == 3:
			if err := buildBodyLRU(tx); err != nil {
				return err
			}
		default:
			if err := dropBucketsWithPrefix(tx, "msgs:", "lru:"); err != nil {
				return err
			}
		}
		return schema.Put([]byte("version"), uint32Bytes(schemaVersion))
	})
}

// dropBucketsWithPrefix elimina i bucket radice il cui nome inizia per uno dei prefissi.
func dropBucketsWithPrefix(tx *bolt.Tx, prefixes ...string) error {
	var stale [][]byte
	err := tx.ForEach(func(name []byte, _ *bolt.Bucket) error {
		for _, p := range prefixes {
			if bytes.HasPrefix(name, []byte(p)) {
				stale = append(stale, append([]byte(nil), name...))
				break
			}
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
	return nil
}

// buildBodyLRU costruisce l'indice LRU dei corpi dalle voci esistenti: l'unità è il
// messaggio Gmail (D16-T4), più recente = usato più di recente (data più alta tra
// le copie). Oltre MaxBodies unità le più vecchie si rimuovono.
func buildBodyLRU(tx *bolt.Tx) error {
	if err := dropBucketsWithPrefix(tx, "lru:"); err != nil {
		return err
	}
	type unit struct {
		key     string
		date    time.Time
		entries []lruEntry
	}
	byKey := map[string]*unit{}
	err := tx.ForEach(func(name []byte, b *bolt.Bucket) error {
		if !strings.HasPrefix(string(name), "msgs:") {
			return nil
		}
		mailbox := strings.TrimPrefix(string(name), "msgs:")
		return b.ForEach(func(k, v []byte) error {
			var m msg.Message
			if err := json.Unmarshal(v, &m); err != nil {
				return nil // voce illeggibile: fuori dall'indice, resta orfana ma innocua
			}
			key := bodyUnit(mailbox, k, m.Ref.GmailID)
			u := byKey[key]
			if u == nil {
				u = &unit{key: key}
				byKey[key] = u
			}
			if m.Ref.Date.After(u.date) {
				u.date = m.Ref.Date
			}
			u.entries = append(u.entries, lruEntry{Bucket: string(name), Key: append([]byte(nil), k...)})
			return nil
		})
	})
	if err != nil {
		return err
	}
	units := make([]*unit, 0, len(byKey))
	for _, u := range byKey {
		units = append(units, u)
	}
	// dal meno al più recente; a parità di data l'ordine per chiave rende il risultato
	// deterministico
	sort.Slice(units, func(i, j int) bool {
		if !units[i].date.Equal(units[j].date) {
			return units[i].date.Before(units[j].date)
		}
		return units[i].key < units[j].key
	})
	for _, u := range units {
		if err := lruTouch(tx, lruBody, u.key, u.entries); err != nil {
			return err
		}
	}
	return lruEvict(tx, lruBody, MaxBodies, dropEntry(tx))
}
