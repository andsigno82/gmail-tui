package cache

// Piano 16, Task 3: migrazione dello schema 3 al 4 (D16-T7): i corpi restano, l'indice
// LRU si costruisce dalle voci esistenti per Ref.Date e si tengono le 200 più recenti.

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// writeOldSchema crea a mano un database con la versione ver e n corpi in INBOX
// (gid = uid = 1..n, data crescente con gid) più, per ogni gid in alsoAllMail, la
// copia in Tutti i messaggi (uid 1000+gid).
func writeOldSchema(t *testing.T, path string, ver uint32, n int, alsoAllMail []uint64) {
	t.Helper()
	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	db.NoSync = true
	err = db.Update(func(tx *bolt.Tx) error {
		sc, err := tx.CreateBucketIfNotExists([]byte("schema"))
		if err != nil {
			return err
		}
		if err := sc.Put([]byte("version"), uint32Bytes(ver)); err != nil {
			return err
		}
		meta, err := tx.CreateBucketIfNotExists([]byte("meta:INBOX"))
		if err != nil {
			return err
		}
		if err := meta.Put([]byte("uidvalidity"), uint32Bytes(5)); err != nil {
			return err
		}
		put := func(box string, uid uint32, gid uint64) error {
			b, err := tx.CreateBucketIfNotExists([]byte("msgs:" + box))
			if err != nil {
				return err
			}
			m := &msg.Message{Ref: msg.MessageRef{
				ID: msg.NewID(box, uid), GmailID: gid, Subject: "s",
				Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(gid) * time.Hour),
			}, BodyText: "corpo"}
			data, err := json.Marshal(m)
			if err != nil {
				return err
			}
			return b.Put(uint32Bytes(uid), data)
		}
		for g := 1; g <= n; g++ {
			if err := put("INBOX", uint32(g), uint64(g)); err != nil {
				return err
			}
		}
		for _, g := range alsoAllMail {
			if err := put(allMail, uint32(1000+g), g); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateV3KeepsNewest200(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	// 250 corpi; le copie in Tutti i messaggi di 245..250 non contano come unità nuove
	writeOldSchema(t, path, 3, 250, []uint64{245, 246, 247, 248, 249, 250})
	c, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	c.db.NoSync = true
	defer func() { _ = c.Close() }()
	kept := 0
	for g := uint32(1); g <= 250; g++ {
		ok := has(t, c, "INBOX", g)
		if ok {
			kept++
		}
		if want := g > 50; ok != want {
			t.Errorf("INBOX/%d presente=%v, voglio %v (restano i 200 più recenti per Date)", g, ok, want)
		}
	}
	if kept != MaxBodies {
		t.Errorf("unità tenute = %d, voglio %d", kept, MaxBodies)
	}
	for _, g := range []uint32{245, 250} {
		if !has(t, c, allMail, 1000+g) {
			t.Errorf("copia %d in Tutti i messaggi persa: segue l'unità, che è tra le recenti", g)
		}
	}
	if v, ok, _ := c.UIDValidity("INBOX"); !ok || v != 5 {
		t.Errorf("uidvalidity dopo migrazione = %d ok=%v, voglio 5", v, ok)
	}
}

// L'indice costruito dalla migrazione è coerente: una unità nuova espelle la più
// vecchia rimasta. (Senza letture prima: i tocchi in memoria cambierebbero l'ordine.)
func TestMigrateV3IndexIsConsistent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	writeOldSchema(t, path, 3, 250, nil)
	c, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c.db.NoSync = true
	defer func() { _ = c.Close() }()
	if err := c.SetMessage("INBOX", 5, gm("INBOX", 999, 999)); err != nil {
		t.Fatal(err)
	}
	if has(t, c, "INBOX", 51) {
		t.Error("dopo la migrazione l'unità 51 (la più vecchia) doveva essere espulsa")
	}
	if !has(t, c, "INBOX", 52) {
		t.Error("l'unità 52 non andava espulsa")
	}
}

func TestMigrateV3BelowLimitLosesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	writeOldSchema(t, path, 3, 120, nil)
	c, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	for g := uint32(1); g <= 120; g++ {
		if !has(t, c, "INBOX", g) {
			t.Fatalf("INBOX/%d persa: sotto 200 la migrazione non perde nulla", g)
		}
	}
}

func TestMigrateV2StillDrops(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	writeOldSchema(t, path, 2, 5, nil)
	c, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	for g := uint32(1); g <= 5; g++ {
		if has(t, c, "INBOX", g) {
			t.Errorf("INBOX/%d dello schema 2 ancora presente", g)
		}
	}
}

// File che non è un database bbolt: errore pulito da Open, mai un panic.
func TestOpenCorruptFileIsErrorNotPanic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	junk := make([]byte, 8192)
	for i := range junk {
		junk[i] = byte(i*7 + 3)
	}
	if err := writeFile(path, junk); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("Open su un file corrotto doveva dare errore")
	}
}
