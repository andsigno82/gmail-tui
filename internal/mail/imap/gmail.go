package imap

// Piano 8, Task 6: modalità Gmail dello Store (X-GM-EXT-1). Il flag è deciso
// dalla capability annunciata al login; senza, lo Store si comporta come oggi.

import (
	"github.com/emersion/go-imap/v2"
)

// capGmailExt1 è la capability delle estensioni Gmail IMAP.
const capGmailExt1 imap.Cap = "X-GM-EXT-1"

// gmailOn dice se il server annuncia X-GM-EXT-1 (letto sotto mu).
func (s *Store) gmailOn() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gmail
}

// GmailExt dice se lo Store opera in modalità Gmail (X-GM-EXT-1 annunciata al
// login). È un'interfaccia opzionale, fuori da mail.MailStore: la UI la legge
// per scegliere la sintassi di `/` (Piano 8, Task 11).
func (s *Store) GmailExt() bool { return s.gmailOn() }
