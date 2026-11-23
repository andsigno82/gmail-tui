package cache

// Piano 16, Task 4: istantanee di lista e thread (D16-A1, D16-T2, D16-T3) con la
// regola d'epoca R3.

import (
	"fmt"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

var snapAt = time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)

// conv è una riga di conversazione con thread id e GmailID dell'ultimo messaggio = tid.
func conv(box string, tid uint64) mail.Conversation {
	ref := msg.MessageRef{
		ID: msg.NewID(box, uint32(tid)), GmailID: tid, ThreadID: tid,
		Subject: fmt.Sprintf("oggetto %d", tid), Unread: true,
	}
	return mail.Conversation{Latest: ref, ThreadID: tid, Count: 1, Participants: []string{"a"}, Unread: true}
}

func convs(box string, from, to uint64) []mail.Conversation {
	var out []mail.Conversation
	for i := from; i <= to; i++ {
		out = append(out, conv(box, i))
	}
	return out
}

func listOK(t *testing.T, c *Cache, box string) bool {
	t.Helper()
	_, _, _, ok := c.GetList(box)
	return ok
}

func threadOK(c *Cache, tid uint64) bool {
	_, _, ok := c.GetThread(tid)
	return ok
}

// rawHas dice se il bucket radice bucket contiene la chiave key (ispezione del disco,
// senza passare dalla regola d'epoca).
func rawHas(c *Cache, bucket string, key []byte) bool {
	var ok bool
	_ = c.db.View(func(tx *bolt.Tx) error {
		if b := tx.Bucket([]byte(bucket)); b != nil {
			ok = b.Get(key) != nil
		}
		return nil
	})
	return ok
}

func TestPutGetListRoundTrip(t *testing.T) {
	c := newCache(t)
	rows := convs("INBOX", 1, 3)
	rows[1].Labels = []string{"Lavoro"}
	if err := c.PutList("INBOX", 7, rows, true, snapAt); err != nil {
		t.Fatalf("PutList: %v", err)
	}
	got, at, limited, ok := c.GetList("INBOX")
	if !ok || !limited || !at.Equal(snapAt) || len(got) != 3 {
		t.Fatalf("GetList = %d righe at=%v limited=%v ok=%v", len(got), at, limited, ok)
	}
	if got[1].Latest.Subject != "oggetto 2" || got[1].Labels[0] != "Lavoro" || got[2].ThreadID != 3 {
		t.Errorf("roundtrip rotto: %+v", got[1])
	}
	// "" è INBOX (contratto del Snapshotter)
	if _, _, _, ok := c.GetList(""); !ok {
		t.Error("mailbox vuota = INBOX")
	}
	if _, _, _, ok := c.GetList("Altra"); ok {
		t.Error("mailbox mai salvata: miss atteso")
	}
}

func TestListTruncatedToSnapshotRows(t *testing.T) {
	c := newCache(t)
	if err := c.PutList("INBOX", 1, convs("INBOX", 1, SnapshotRows+30), false, snapAt); err != nil {
		t.Fatal(err)
	}
	got, _, _, ok := c.GetList("INBOX")
	if !ok || len(got) != SnapshotRows {
		t.Fatalf("righe = %d, voglio %d (SnapshotRows)", len(got), SnapshotRows)
	}
	if got[SnapshotRows-1].ThreadID != SnapshotRows {
		t.Errorf("la troncatura deve tenere le prime righe, ultima = %d", got[SnapshotRows-1].ThreadID)
	}
}

func TestEmptyListDeletesSnapshot(t *testing.T) {
	c := newCache(t)
	if err := c.PutList("A", 1, convs("A", 1, 2), false, snapAt); err != nil {
		t.Fatal(err)
	}
	if err := c.PutList("A", 1, nil, false, snapAt); err != nil {
		t.Fatal(err)
	}
	if listOK(t, c, "A") || rawHas(c, "snap:list", []byte("A")) {
		t.Fatal("righe vuote: l'istantanea va cancellata")
	}
	// nessuna voce fantasma nell'LRU: 50 liste nuove ci stanno tutte
	for i := 1; i <= MaxListSnapshots; i++ {
		box := fmt.Sprintf("B%d", i)
		if err := c.PutList(box, 1, convs(box, 1, 1), false, snapAt); err != nil {
			t.Fatal(err)
		}
	}
	if !listOK(t, c, "B1") {
		t.Error("l'istantanea cancellata contava ancora nel limite LRU")
	}
}

// R3: la lista salvata con l'epoca 1 non vale se i corpi della mailbox dicono epoca 2.
func TestListEpochMismatchIsMiss(t *testing.T) {
	c := newCache(t)
	if err := c.PutList("INBOX", 1, convs("INBOX", 1, 2), false, snapAt); err != nil {
		t.Fatal(err)
	}
	// senza corpi (meta assente) l'istantanea vale
	if _, _, _, res := c.LookupList("INBOX"); res != LookupHit {
		t.Fatalf("meta assente: esito %v, voglio hit", res)
	}
	// stessa epoca: vale
	if err := c.SetMessage("INBOX", 1, gm("INBOX", 5, 5)); err != nil {
		t.Fatal(err)
	}
	if _, _, _, res := c.LookupList("INBOX"); res != LookupHit {
		t.Fatalf("stessa epoca: esito %v, voglio hit", res)
	}
	// epoca diversa registrata dai corpi: miss con motivo epoch
	if err := c.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("meta:INBOX")).Put([]byte("uidvalidity"), uint32Bytes(2))
	}); err != nil {
		t.Fatal(err)
	}
	rows, _, _, res := c.LookupList("INBOX")
	if res != LookupEpoch || len(rows) != 0 {
		t.Fatalf("epoca diversa: esito %v righe %d, voglio epoch e nessuna riga", res, len(rows))
	}
	if listOK(t, c, "INBOX") {
		t.Error("GetList deve dare miss con epoca diversa")
	}
}

func TestThreadEpochMismatchIsMiss(t *testing.T) {
	c := newCache(t)
	refs := []msg.MessageRef{{ID: msg.NewID(allMail, 9), GmailID: 9, ThreadID: 77}}
	if err := c.PutThread(77, allMail, 3, refs, snapAt); err != nil {
		t.Fatal(err)
	}
	got, at, ok := c.GetThread(77)
	if !ok || len(got) != 1 || !at.Equal(snapAt) || got[0].GmailID != 9 {
		t.Fatalf("GetThread = %+v at=%v ok=%v", got, at, ok)
	}
	if err := c.SetMessage(allMail, 4, gm(allMail, 1, 1)); err != nil { // corpi d'epoca 4
		t.Fatal(err)
	}
	if _, _, res := c.LookupThread(77); res != LookupEpoch {
		t.Fatalf("epoca diversa: esito %v, voglio epoch", res)
	}
	if threadOK(c, 77) {
		t.Error("GetThread deve dare miss con epoca diversa")
	}
	if threadOK(c, 12345) {
		t.Error("thread mai salvato: miss atteso")
	}
}

func TestNoteUIDValidityDropsSnapshots(t *testing.T) {
	c := newCache(t)
	for _, box := range []string{"INBOX", allMail} {
		if _, err := c.NoteUIDValidity(box, 1); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.PutList("INBOX", 1, convs("INBOX", 1, 2), false, snapAt); err != nil {
		t.Fatal(err)
	}
	if err := c.PutList("Altra", 1, convs("Altra", 1, 2), false, snapAt); err != nil {
		t.Fatal(err)
	}
	refs := []msg.MessageRef{{ID: msg.NewID(allMail, 9), GmailID: 9, ThreadID: 77}}
	if err := c.PutThread(77, allMail, 1, refs, snapAt); err != nil {
		t.Fatal(err)
	}
	// la cartella cambia epoca: cade la sua lista, non il thread (mailbox Tutti i messaggi)
	if changed, err := c.NoteUIDValidity("INBOX", 2); err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if rawHas(c, "snap:list", []byte("INBOX")) {
		t.Error("epoca cambiata: l'istantanea della lista va scartata dal disco")
	}
	if !rawHas(c, "snap:list", []byte("Altra")) || !threadOK(c, 77) {
		t.Error("le istantanee di altre mailbox non vanno toccate")
	}
	// Tutti i messaggi cambia epoca: cadono tutte le istantanee di thread
	if changed, err := c.NoteUIDValidity(allMail, 2); err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if rawHas(c, "snap:thread", threadKey(77)) {
		t.Error("epoca di Tutti i messaggi cambiata: le istantanee di thread vanno scartate")
	}
	// e il limite LRU non conserva voci fantasma: 200 thread nuovi ci stanno tutti
	for i := uint64(1000); i < 1000+MaxThreadSnapshots; i++ {
		if err := c.PutThread(i, allMail, 2, refs, snapAt); err != nil {
			t.Fatal(err)
		}
	}
	if !threadOK(c, 1000) {
		t.Error("voce LRU fantasma dell'istantanea scartata")
	}
}

func TestThreadSnapshotsLRU201(t *testing.T) {
	c := newCache(t)
	refs := []msg.MessageRef{{ID: msg.NewID(allMail, 1), GmailID: 1, ThreadID: 1}}
	for i := uint64(1); i <= MaxThreadSnapshots; i++ {
		if err := c.PutThread(i, allMail, 1, refs, snapAt); err != nil {
			t.Fatal(err)
		}
	}
	if !threadOK(c, 1) { // lettura: la 1 diventa la più recente alla scrittura successiva
		t.Fatal("thread 1 assente")
	}
	if err := c.PutThread(MaxThreadSnapshots+1, allMail, 1, refs, snapAt); err != nil {
		t.Fatal(err)
	}
	if threadOK(c, 2) {
		t.Error("il thread 2 (il meno usato) doveva essere espulso")
	}
	if !threadOK(c, 1) || !threadOK(c, MaxThreadSnapshots+1) {
		t.Error("il thread 1 (letto) e il 201 (nuovo) devono restare")
	}
}

func TestListSnapshotsLRU51(t *testing.T) {
	c := newCache(t)
	put := func(i int) {
		box := fmt.Sprintf("M%d", i)
		if err := c.PutList(box, 1, convs(box, 1, 1), false, snapAt); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= MaxListSnapshots; i++ {
		put(i)
	}
	if !listOK(t, c, "M1") {
		t.Fatal("M1 assente")
	}
	put(MaxListSnapshots + 1)
	if listOK(t, c, "M2") {
		t.Error("la lista M2 (la meno usata) doveva essere espulsa")
	}
	if !listOK(t, c, "M1") {
		t.Error("la lista M1, letta, doveva restare")
	}
}

// Un'istantanea riscritta (stesso thread o stessa cartella) non occupa due posti.
func TestSnapshotRewriteKeepsOneSlot(t *testing.T) {
	c := newCache(t)
	refs := []msg.MessageRef{{ID: msg.NewID(allMail, 1), GmailID: 1, ThreadID: 1}}
	for i := 0; i < 5; i++ {
		if err := c.PutThread(1, allMail, 1, refs, snapAt); err != nil {
			t.Fatal(err)
		}
	}
	for i := uint64(2); i <= MaxThreadSnapshots; i++ {
		if err := c.PutThread(i, allMail, 1, refs, snapAt); err != nil {
			t.Fatal(err)
		}
	}
	if !threadOK(c, 2) {
		t.Error("le riscritture dello stesso thread hanno occupato più posti")
	}
}

// File corrotto = miss, mai un panic.
func TestCorruptSnapshotsAreMiss(t *testing.T) {
	c := newCache(t)
	err := c.db.Update(func(tx *bolt.Tx) error {
		l, err := tx.CreateBucketIfNotExists([]byte("snap:list"))
		if err != nil {
			return err
		}
		if err := l.Put([]byte("INBOX"), []byte("{rotto")); err != nil {
			return err
		}
		th, err := tx.CreateBucketIfNotExists([]byte("snap:thread"))
		if err != nil {
			return err
		}
		return th.Put(threadKey(5), []byte{0xff, 0x00, 0x01})
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, res := c.LookupList("INBOX"); res != LookupMiss {
		t.Errorf("lista corrotta: esito %v, voglio miss", res)
	}
	if _, _, res := c.LookupThread(5); res != LookupMiss {
		t.Errorf("thread corrotto: esito %v, voglio miss", res)
	}
	// le patch non vanno in panic né in errore su voci corrotte
	if err := c.PatchSnapshotRefs([]uint64{1}, func(*msg.MessageRef) bool { return true }); err != nil {
		t.Errorf("PatchSnapshotRefs su voci corrotte: %v", err)
	}
	if err := c.RemoveListRows("INBOX", []uint64{1}, nil); err != nil {
		t.Errorf("RemoveListRows su voce corrotta: %v", err)
	}
	if err := c.RemoveThreadRefs([]uint64{1}); err != nil {
		t.Errorf("RemoveThreadRefs su voce corrotta: %v", err)
	}
	// e una nuova scrittura sopra la voce corrotta funziona
	if err := c.PutList("INBOX", 1, convs("INBOX", 1, 1), false, snapAt); err != nil || !listOK(t, c, "INBOX") {
		t.Errorf("riscrittura sopra voce corrotta: err=%v", err)
	}
}

func markRead(r *msg.MessageRef) bool {
	if !r.Unread {
		return false
	}
	r.Unread = false
	return true
}

func TestPatchSnapshotRefsUpdatesUnreadAndLabels(t *testing.T) {
	c := newCache(t)
	// riga 10: thread di due messaggi non letti (gid 10 è Latest, gid 9 più vecchio);
	// riga 20: solo Latest, senza istantanea di thread
	rows := []mail.Conversation{conv("INBOX", 10), conv("INBOX", 20)}
	if err := c.PutList("INBOX", 1, rows, false, snapAt); err != nil {
		t.Fatal(err)
	}
	refs := []msg.MessageRef{
		{ID: msg.NewID(allMail, 9), GmailID: 9, ThreadID: 10, Unread: true, Labels: []string{"A"}},
		{ID: msg.NewID(allMail, 10), GmailID: 10, ThreadID: 10, Unread: true, Labels: []string{"B"}},
	}
	if err := c.PutThread(10, allMail, 1, refs, snapAt); err != nil {
		t.Fatal(err)
	}

	// 1) leggo il Latest della riga 10: l'altro messaggio resta non letto → riga non letta
	if err := c.PatchSnapshotRefs([]uint64{10}, markRead); err != nil {
		t.Fatalf("PatchSnapshotRefs: %v", err)
	}
	got, _, _, _ := c.GetList("INBOX")
	if len(got) != 2 {
		t.Fatalf("righe = %d, voglio 2", len(got))
	}
	if got[0].Latest.Unread {
		t.Error("Latest della riga 10 dovrebbe risultare letto")
	}
	if !got[0].Unread {
		t.Error("riga 10: resta un messaggio non letto nel thread, la riga deve restare non letta")
	}
	th, _, _ := c.GetThread(10)
	if th[1].Unread || !th[0].Unread {
		t.Errorf("ref del thread: %+v", th)
	}
	// 2) leggo anche l'altro: la riga diventa letta
	if err := c.PatchSnapshotRefs([]uint64{9}, markRead); err != nil {
		t.Fatal(err)
	}
	got, _, _, _ = c.GetList("INBOX")
	if got[0].Unread {
		t.Error("riga 10: tutti i messaggi letti, la riga deve risultare letta")
	}
	// 3) riga 20 senza istantanea di thread: Unread dal Latest
	if !got[1].Unread {
		t.Fatal("riga 20 non doveva cambiare")
	}
	if err := c.PatchSnapshotRefs([]uint64{20}, markRead); err != nil {
		t.Fatal(err)
	}
	got, _, _, _ = c.GetList("INBOX")
	if got[1].Unread || got[1].Latest.Unread {
		t.Errorf("riga 20: Unread dal Latest, atteso letta: %+v", got[1])
	}
	// 4) label: aggiungere "X" al Latest del thread 10 ricalcola le label della riga
	addX := func(r *msg.MessageRef) bool { r.Labels = append(append([]string(nil), r.Labels...), "X"); return true }
	if err := c.PatchSnapshotRefs([]uint64{10}, addX); err != nil {
		t.Fatal(err)
	}
	got, _, _, _ = c.GetList("INBOX")
	want := []string{"A", "B", "X"}
	if fmt.Sprint(got[0].Labels) != fmt.Sprint(want) {
		t.Errorf("label della riga 10 = %v, voglio %v (unione dei ref del thread)", got[0].Labels, want)
	}
	// un gmailID sconosciuto non cambia nulla e non scrive
	before := txID(c)
	if err := c.PatchSnapshotRefs([]uint64{999}, markRead); err != nil {
		t.Fatal(err)
	}
	if txID(c) != before {
		t.Error("patch senza corrispondenze: nessuna scrittura attesa")
	}
}

func TestRemoveListRowsByThread(t *testing.T) {
	c := newCache(t)
	if err := c.PutList("INBOX", 1, convs("INBOX", 1, 4), false, snapAt); err != nil {
		t.Fatal(err)
	}
	if err := c.PutList("Altra", 1, convs("Altra", 1, 4), false, snapAt); err != nil {
		t.Fatal(err)
	}
	// per thread id (2) e per MessageID (riga 4)
	if err := c.RemoveListRows("INBOX", []uint64{2}, []msg.MessageID{msg.NewID("INBOX", 4)}); err != nil {
		t.Fatalf("RemoveListRows: %v", err)
	}
	got, _, _, ok := c.GetList("INBOX")
	if !ok || len(got) != 2 || got[0].ThreadID != 1 || got[1].ThreadID != 3 {
		t.Fatalf("righe rimaste = %+v", got)
	}
	if other, _, _, _ := c.GetList("Altra"); len(other) != 4 {
		t.Error("un'altra cartella non va toccata")
	}
	// tolte tutte le righe: l'istantanea sparisce
	if err := c.RemoveListRows("INBOX", []uint64{1, 3}, nil); err != nil {
		t.Fatal(err)
	}
	if listOK(t, c, "INBOX") || rawHas(c, "snap:list", []byte("INBOX")) {
		t.Error("lista svuotata dalle rimozioni: l'istantanea va cancellata")
	}
	// mailbox senza istantanea: nessun errore
	if err := c.RemoveListRows("Nessuna", []uint64{1}, nil); err != nil {
		t.Errorf("RemoveListRows senza istantanea: %v", err)
	}
}

func TestRemoveThreadRefs(t *testing.T) {
	c := newCache(t)
	refs := func(gids ...uint64) []msg.MessageRef {
		var out []msg.MessageRef
		for _, g := range gids {
			out = append(out, msg.MessageRef{ID: msg.NewID(allMail, uint32(g)), GmailID: g})
		}
		return out
	}
	if err := c.PutThread(1, allMail, 1, refs(1, 2, 3), snapAt); err != nil {
		t.Fatal(err)
	}
	if err := c.PutThread(2, allMail, 1, refs(4), snapAt); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveThreadRefs([]uint64{2, 4}); err != nil {
		t.Fatalf("RemoveThreadRefs: %v", err)
	}
	got, _, ok := c.GetThread(1)
	if !ok || len(got) != 2 || got[0].GmailID != 1 || got[1].GmailID != 3 {
		t.Errorf("thread 1 = %+v", got)
	}
	if threadOK(c, 2) {
		t.Error("thread rimasto senza messaggi: l'istantanea va cancellata")
	}
}

// BindAccount con account diverso svuota anche le istantanee.
func TestBindAccountWipesSnapshots(t *testing.T) {
	c := newCache(t)
	if err := c.BindAccount("a@gmail.com"); err != nil {
		t.Fatal(err)
	}
	if err := c.PutList("INBOX", 1, convs("INBOX", 1, 2), false, snapAt); err != nil {
		t.Fatal(err)
	}
	if err := c.BindAccount("b@gmail.com"); err != nil {
		t.Fatal(err)
	}
	if listOK(t, c, "INBOX") {
		t.Error("cambio account: le istantanee dell'altro account devono sparire")
	}
}

// D16-T6: gli hit delle istantanee non scrivono su disco.
func TestSnapshotHitsDoNotWrite(t *testing.T) {
	c := newCache(t)
	refs := []msg.MessageRef{{ID: msg.NewID(allMail, 1), GmailID: 1, ThreadID: 1}}
	if err := c.PutThread(1, allMail, 1, refs, snapAt); err != nil {
		t.Fatal(err)
	}
	if err := c.PutList("INBOX", 1, convs("INBOX", 1, 1), false, snapAt); err != nil {
		t.Fatal(err)
	}
	before := txID(c)
	for i := 0; i < 20; i++ {
		if !threadOK(c, 1) || !listOK(t, c, "INBOX") {
			t.Fatal("miss inatteso")
		}
	}
	if txID(c) != before {
		t.Error("gli hit delle istantanee hanno scritto su disco")
	}
}

// Le istantanee sopravvivono alla riapertura del file.
func TestSnapshotsPersistAcrossReopen(t *testing.T) {
	path := t.TempDir() + "/cache.db"
	c, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.PutList("INBOX", 1, convs("INBOX", 1, 3), false, snapAt); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	c2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c2.Close() }()
	if got, _, _, ok := c2.GetList("INBOX"); !ok || len(got) != 3 {
		t.Errorf("dopo riapertura: %d righe ok=%v", len(got), ok)
	}
}
