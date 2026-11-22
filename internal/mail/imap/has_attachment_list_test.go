package imap

import (
	"testing"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Piano 14, Wave 4 review (punto 2): ListMessages applica HasAttachment come
// post-filtro (contratto mail.MailStore). Senza, la ricerca non Gmail (che passa da
// ListMessages) ignorava has:attachment. Server IMAP reale in-process (memserver).
func TestListMessagesHasAttachmentPostFilter(t *testing.T) {
	ts := startTestServer(t)
	withAtt := ts.seedMultipartMail("INBOX", "Mia <mia@example.org>", "Con allegato",
		"testo", "doc.pdf", pdfContent, false, false)
	ts.seedMail("Bob <bob@example.org>", "Senza allegato", "solo testo", false, false)
	store := connectStore(t, ts)

	refs, err := store.ListMessages(t.Context(), msg.Query{HasAttachment: true}, mail.Page{})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(refs) != 1 || refs[0].ID != msg.NewID("INBOX", withAtt) {
		t.Fatalf("refs = %v, atteso solo il messaggio con allegato INBOX:%d", refs, withAtt)
	}
}
