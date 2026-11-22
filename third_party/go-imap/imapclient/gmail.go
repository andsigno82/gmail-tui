// Patch Gmail (Piano 8, Task 2): X-GM-EXT-1 non è supportato da go-imap upstream.
// Elenco delle patch e procedura di riallineamento: PATCHES.md.

package imapclient

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal/imapwire"
)

// FetchItemDataGmailMsgID è l'attributo X-GM-MSGID (ID messaggio Gmail, uint64).
type FetchItemDataGmailMsgID struct {
	ID uint64
}

func (FetchItemDataGmailMsgID) fetchItemData() {}

// FetchItemDataGmailThreadID è l'attributo X-GM-THRID (ID conversazione Gmail, uint64).
type FetchItemDataGmailThreadID struct {
	ID uint64
}

func (FetchItemDataGmailThreadID) fetchItemData() {}

// FetchItemDataGmailLabels è l'attributo X-GM-LABELS: è la lista delle label
// Gmail del messaggio, con i nomi decodificati da modified UTF-7. Gli attributi
// di sistema hanno il backslash (es. \Inbox).
type FetchItemDataGmailLabels struct {
	Labels []string
}

func (FetchItemDataGmailLabels) fetchItemData() {}

// readGmailID legge uno SP e un numero decimale senza segno a 64 bit.
// Un valore fuori da uint64 è errore.
func readGmailID(dec *imapwire.Decoder) (uint64, error) {
	var s string
	if !dec.ExpectSP() || !dec.Expect(dec.Func(&s, isDigitChar), "numero X-GM") {
		return 0, dec.Err()
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("id X-GM %q: %w", s, err)
	}
	return n, nil
}

// readGmailLabels legge uno SP e una lista di label. Le label possono essere
// atomi con backslash (\Inbox, non ammessi da imapwire.IsAtomChar), stringhe
// quotate o atomi qualunque; la lista può essere vuota "()".
func readGmailLabels(dec *imapwire.Decoder) ([]string, error) {
	if !dec.ExpectSP() {
		return nil, dec.Err()
	}
	var labels []string
	err := dec.ExpectList(func() error {
		var s string
		if dec.Quoted(&s) {
			labels = append(labels, decodeGmailLabel(s))
			return nil
		}
		if !dec.Expect(dec.Func(&s, isGmailLabelChar), "etichetta X-GM-LABELS") {
			return dec.Err()
		}
		labels = append(labels, decodeGmailLabel(s))
		return nil
	})
	return labels, err
}

// skipMsgAttValue scarta il valore di un msg-att sconosciuto (SP incluso), con
// liste, stringhe quotate e literal annidati.
func skipMsgAttValue(dec *imapwire.Decoder) error {
	if !dec.ExpectSP() || !dec.DiscardValue() {
		return dec.Err()
	}
	return nil
}

// decodeGmailLabel decodifica il modified UTF-7 dei nomi label ("&ANw-" → Ü).
// Un nome senza '&' non viene toccato; un errore di decodifica lascia il testo
// grezzo, che resta mostrabile.
func decodeGmailLabel(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	d, err := decodeModifiedUTF7(s)
	if err != nil {
		return s
	}
	return d
}

// decodeModifiedUTF7 decodifica il modified UTF-7 di RFC 3501 §5.1.3: "&" apre
// una sequenza base64 con "," al posto di "/" chiusa da "-"; "&-" è un '&'
// letterale. Non è lo UTF-7 di RFC 2152 ("+"), quindi nessuna libreria generica.
func decodeModifiedUTF7(s string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '&' {
			out.WriteByte(s[i])
			i++
			continue
		}
		end := strings.IndexByte(s[i+1:], '-')
		if end < 0 {
			return "", errors.New("modified UTF-7: sequenza non chiusa")
		}
		seg := s[i+1 : i+1+end]
		if seg == "" {
			out.WriteByte('&')
		} else {
			raw, err := base64.RawStdEncoding.DecodeString(strings.ReplaceAll(seg, ",", "/"))
			if err != nil {
				return "", fmt.Errorf("modified UTF-7: %w", err)
			}
			if len(raw)%2 != 0 {
				return "", errors.New("modified UTF-7: lunghezza UTF-16 dispari")
			}
			u16 := make([]uint16, len(raw)/2)
			for j := range u16 {
				u16[j] = uint16(raw[2*j])<<8 | uint16(raw[2*j+1])
			}
			out.WriteString(string(utf16.Decode(u16)))
		}
		i += end + 2
	}
	return out.String(), nil
}

// StoreGmailLabels invia STORE con X-GM-LABELS (Piano 8, Task 4, patch Gmail).
// Come Store: senza Silent il server risponde con i FETCH aggiornati.
func (c *Client) StoreGmailLabels(numSet imap.NumSet, store *imap.StoreGmailLabels) *FetchCommand {
	cmd := &FetchCommand{
		numSet: numSet,
		msgs:   make(chan *FetchMessageData, 128),
	}
	enc := c.beginCommand(uidCmdName("STORE", imapwire.NumSetKind(numSet)), cmd)
	enc.SP().NumSet(numSet).SP()
	switch store.Op {
	case imap.StoreFlagsSet:
		// nessun prefisso: sostituisce le label
	case imap.StoreFlagsAdd:
		enc.Special('+')
	case imap.StoreFlagsDel:
		enc.Special('-')
	default:
		panic(fmt.Errorf("imapclient: unknown store flags op: %v", store.Op))
	}
	enc.Atom("X-GM-LABELS")
	if store.Silent {
		enc.Atom(".SILENT")
	}
	enc.SP().List(len(store.Labels), func(i int) {
		enc.String(store.Labels[i])
	})
	enc.end()
	return cmd
}

func isDigitChar(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isGmailLabelChar(ch byte) bool {
	return ch != ' ' && ch != '(' && ch != ')' && ch != '"' && !unicode.IsControl(rune(ch))
}
