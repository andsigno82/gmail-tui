package foldertree

import (
	"strconv"

	"github.com/andsigno82/gmail-tui/internal/mail"
)

// countLabel formatta il contatore dei non letti (Piano 15, Task 10): "(N)".
// Il chiamante lo mostra solo se N > 0.
func countLabel(n int) string { return "(" + strconv.Itoa(n) + ")" }

// isCounted dice se la cartella mostra il contatore (Q15-4: INBOX e label, non
// le speciali: Bozze, Inviati, Spam, Cestino, Tutta la posta).
func isCounted(b mail.Mailbox) bool { return isInbox(b) || b.Role == "" }
