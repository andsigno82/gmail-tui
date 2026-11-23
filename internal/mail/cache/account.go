package cache

// Legame della cache con l'account (Piano 16, Task 3, D16-T9).

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	bolt "go.etcd.io/bbolt"
)

// accountKey è la chiave, nel bucket schema, dell'hash dell'account.
const accountKey = "account"

// BindAccount lega la cache all'indirizzo address: il file è per macchina, non per
// account, e un cambio di account non deve mostrare la posta dell'altro. Si salva
// solo lo SHA-256 esadecimale dell'indirizzo in minuscolo (mai in chiaro). Se l'hash
// è diverso da quello salvato la cache si svuota per intero (corpi, epoche, indice,
// istantanee); se manca (cache dello schema 3) viene adottata senza svuotarla.
func (c *Cache) BindAccount(address string) error {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(address))))
	want := []byte(hex.EncodeToString(sum[:]))
	same := false
	err := c.db.View(func(tx *bolt.Tx) error {
		if sc := tx.Bucket([]byte("schema")); sc != nil {
			same = string(sc.Get([]byte(accountKey))) == string(want)
		}
		return nil
	})
	if err != nil || same {
		return err
	}
	// i tocchi in memoria si riferiscono a unità che stanno per sparire (o restano
	// validi se si adotta): quelli di unità inesistenti vengono comunque ignorati
	return c.update(func(tx *bolt.Tx) error {
		sc, err := tx.CreateBucketIfNotExists([]byte("schema"))
		if err != nil {
			return err
		}
		if sc.Get([]byte(accountKey)) != nil {
			if err := wipeAll(tx); err != nil {
				return err
			}
		}
		return sc.Put([]byte(accountKey), want)
	})
}

// wipeAll elimina ogni bucket radice tranne schema (versione e account).
func wipeAll(tx *bolt.Tx) error {
	var names [][]byte
	err := tx.ForEach(func(name []byte, _ *bolt.Bucket) error {
		if string(name) != "schema" {
			names = append(names, append([]byte(nil), name...))
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, n := range names {
		if err := tx.DeleteBucket(n); err != nil {
			return err
		}
	}
	return nil
}
