package app

// Piano 15, Task 16b (richiesta di Andrea, 2026-11-23): gli allegati aperti stanno in
// una radice temporanea dedicata (iniettabile nei test); la cartella della sessione
// si rimuove all'uscita e a ogni avvio si tolgono le sessioni più vecchie di 24 h,
// sempre e solo dentro la radice: mai cartelle esterne.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// useAttachRoot punta la radice degli allegati a una cartella di test.
func useAttachRoot(t *testing.T) string {
	t.Helper()
	CleanupAttachments() // chiude la sessione di un test precedente
	root := filepath.Join(t.TempDir(), "radice")
	orig := attachRoot
	attachRoot = func() string { return root }
	t.Cleanup(func() { CleanupAttachments(); attachRoot = orig })
	return root
}

func openOnePDF(t *testing.T) string {
	t.Helper()
	opened := stubOpenFile(t)
	m, _, ref := attModel(t, msg.Attachment{PartIndex: 0, Filename: "a.pdf", ContentType: "application/pdf", Size: 9})
	clickAttachment(t, m, ref)
	if len(*opened) != 1 {
		t.Fatalf("aperture = %q, voglio 1", *opened)
	}
	return (*opened)[0]
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

// Il file sta in <radice>/s-*/ (0700); CleanupAttachments toglie la sessione e la
// radice vuota, lasciando intatto tutto il resto (altre sessioni, cartelle esterne).
func TestAllegatiInRadiceDedicataRimossiAllUscita(t *testing.T) {
	root := useAttachRoot(t)
	p := openOnePDF(t)

	sess := filepath.Dir(p)
	if filepath.Dir(sess) != root || !strings.HasPrefix(filepath.Base(sess), "s-") {
		t.Fatalf("file %q: voglio <radice>/s-*/att-N.pdf con radice %q", p, root)
	}
	if st, err := os.Stat(sess); err != nil || st.Mode().Perm() != 0o700 {
		t.Errorf("sessione %v, %v: voglio 0700", st, err)
	}
	if st, err := os.Stat(root); err != nil || st.Mode().Perm() != 0o700 {
		t.Errorf("radice %v, %v: voglio 0700", st, err)
	}

	altra := filepath.Join(root, "s-altra-istanza")
	esterna := filepath.Join(t.TempDir(), "esterna")
	for _, d := range []string{altra, esterna} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	CleanupAttachments()
	if exists(sess) || exists(p) {
		t.Error("la cartella della sessione deve sparire all'uscita")
	}
	if !exists(altra) || !exists(esterna) {
		t.Error("cleanup ha toccato cartelle che non sono della sessione")
	}
	if !exists(root) {
		t.Error("la radice non è vuota (altra istanza): non si rimuove")
	}

	// Senza altre sessioni anche la radice vuota sparisce; un secondo cleanup è innocuo.
	if err := os.Remove(altra); err != nil {
		t.Fatal(err)
	}
	p2 := openOnePDF(t)
	CleanupAttachments()
	CleanupAttachments()
	if exists(p2) || exists(root) {
		t.Error("radice vuota non rimossa")
	}
}

// Senza aperture non si crea nulla né si rimuove nulla.
func TestCleanupSenzaSessioneNonFaNulla(t *testing.T) {
	root := useAttachRoot(t)
	CleanupAttachments()
	if exists(root) {
		t.Error("la radice non deve esistere senza aperture")
	}
}

// A ogni avvio si tolgono le sessioni più vecchie di 24 h, solo dentro la radice e
// solo cartelle s-* vere (mai symlink, mai file o cartelle con altri nomi).
func TestSweepAllegatiVecchi(t *testing.T) {
	root := useAttachRoot(t)
	esterna := t.TempDir()
	segreto := filepath.Join(esterna, "dato")
	if err := os.WriteFile(segreto, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	mk := func(name string, mtime time.Time) string {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "att-1.pdf"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
		return p
	}
	vecchia := mk("s-vecchia", old)
	recente := mk("s-recente", time.Now().Add(-23*time.Hour))
	altroNome := mk("altro", old)
	link := filepath.Join(root, "s-link")
	if err := os.Symlink(esterna, link); err != nil {
		t.Skipf("symlink non disponibile: %v", err)
	}

	SweepAttachments()

	if exists(vecchia) {
		t.Error("la sessione di 48 h fa deve sparire")
	}
	if !exists(recente) {
		t.Error("la sessione di 23 h fa non va toccata")
	}
	if !exists(altroNome) {
		t.Error("una cartella con un altro nome non è dell'app: non va toccata")
	}
	if !exists(link) || !exists(segreto) {
		t.Error("un symlink non va seguito né rimosso, e il suo bersaglio resta")
	}
}

// Una radice che è un symlink (o non è una cartella) non si usa: niente scritture
// altrove.
func TestRadiceSymlinkRifiutata(t *testing.T) {
	root := useAttachRoot(t)
	esterna := t.TempDir()
	if err := os.Symlink(esterna, root); err != nil {
		t.Skipf("symlink non disponibile: %v", err)
	}
	opened := stubOpenFile(t)
	m, _, ref := attModel(t, msg.Attachment{PartIndex: 0, Filename: "a.pdf", ContentType: "application/pdf", Size: 9})
	clickAttachment(t, m, ref)
	if len(*opened) != 0 {
		t.Errorf("aperture = %q, voglio nessuna", *opened)
	}
	if ents, _ := os.ReadDir(esterna); len(ents) != 0 {
		t.Errorf("scritto fuori dalla radice: %v", ents)
	}
}
