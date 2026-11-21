package msg

import (
	"fmt"
	"io"
	"net/mail"
	"strings"

	"github.com/jhillyerd/enmime"
	"github.com/k3a/html2text"
)

// parser disattiva la down-conversione automatica HTML→testo di enmime
// (comportamento introdotto in enmime v1.3.0): la regola corpo della spec
// richiede che text/plain vinca e che il fallback HTML passi dalla nostra
// conversione html2text, con HasHTML=true.
var parser = enmime.NewParser(enmime.DisableTextConversion(true))

// Parse normalizza una email MIME grezza in Message.
// Regola corpo (spec): text/plain vince; se solo HTML → HasHTML=true
// e BodyText = conversione HTML→testo. BodyHTML invece è popolato SEMPRE
// che il MIME contenga una parte HTML, anche con parte testuale.
// Un allegato non decodificabile non fallisce il messaggio: viene saltato.
func Parse(id MessageID, raw io.Reader) (*Message, error) {
	env, err := parser.ReadEnvelope(raw)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", id, err)
	}
	m := &Message{Ref: MessageRef{
		ID:      id,
		Subject: env.GetHeader("Subject"),
	}}
	m.Ref.From = displayAddr(env, "From")
	m.To = addrList(env, "To")
	m.Cc = addrList(env, "Cc")
	if d, err := mail.ParseDate(env.GetHeader("Date")); err == nil {
		m.Ref.Date = d
	}
	switch {
	case strings.TrimSpace(env.Text) != "":
		m.BodyText = env.Text
	case strings.TrimSpace(env.HTML) != "":
		m.HasHTML = true
		m.BodyHTML = env.HTML // originale, per "apri nel browser" (Task 5)
		m.BodyText = html2text.HTML2Text(env.HTML)
	}
	// BodyHTML riceve l'HTML originale SEMPRE che il MIME contenga una parte
	// HTML, anche quando text/plain vince come corpo (multipart/alternative):
	// senza, `o` non partirebbe per la forma dominante della posta reale.
	// HasHTML resta invece true SOLO per i messaggi solo-HTML (regola sopra).
	if strings.TrimSpace(env.HTML) != "" {
		m.BodyHTML = env.HTML
	}
	idx := 0
	for _, part := range env.Attachments {
		if part.FileName == "" && part.ContentType == "application/octet-stream" && len(part.Content) == 0 {
			continue // parte vuota/non utile
		}
		m.Attachments = append(m.Attachments, Attachment{
			PartIndex:   idx,
			Filename:    part.FileName,
			ContentType: part.ContentType,
			Size:        int64(len(part.Content)),
		})
		idx++
	}
	return m, nil
}

// displayAddr rende "Nome <addr>" (o l'indirizzo nudo) dal header.
func displayAddr(env *enmime.Envelope, header string) string {
	addrs, err := env.AddressList(header)
	if err != nil || len(addrs) == 0 {
		return env.GetHeader(header)
	}
	a := addrs[0]
	if a.Name != "" {
		return a.Name + " <" + a.Address + ">"
	}
	return a.Address
}

// addrList estrae gli indirizzi di un header come stringhe.
func addrList(env *enmime.Envelope, header string) []string {
	addrs, err := env.AddressList(header)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, a.Address)
	}
	return out
}
