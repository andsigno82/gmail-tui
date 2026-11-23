package thread

// Benchmark del rendering del corpo (indagine prestazioni dopo il Piano 15):
// newsletter sintetica con tabelle annidate, molti link e immagini, a 8/30/150 KB.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/msg/richtext"
)

func perfNewsletterHTML(size int) string {
	var b strings.Builder
	b.WriteString(`<html><head><style>body{margin:0}</style></head><body><table width="600" cellpadding="0" cellspacing="0"><tr><td>`)
	i := 0
	for b.Len() < size {
		i++
		fmt.Fprintf(&b, `<table width="100%%" cellpadding="0" cellspacing="0"><tr><td style="padding:12px"><table><tr><td><table><tr><td>`+
			`<h2 style="font-size:20px;color:#333">Titolo della sezione numero %d dalla newsletter</h2>`+
			`<p style="font-size:14px;line-height:1.5">Lorem ipsum dolor sit amet, <a href="https://example.com/a/%d?utm_source=news&utm_medium=email&utm_campaign=c%d">consectetur adipiscing</a> elit, sed do `+
			`<b>eiusmod tempor</b> incididunt ut labore et <i>dolore magna</i> aliqua. Ut enim ad minim veniam, quis nostrud <a href="https://example.com/b/%d">exercitation ullamco</a> `+
			`laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur.</p>`+
			`<img src="https://img.example.com/%d.jpg" alt="immagine %d" width="560" height="200">`+
			`<ul><li><a href="https://example.com/l1/%d">primo link</a></li><li><a href="https://example.com/l2/%d">secondo link</a></li><li>terzo punto senza link</li></ul>`+
			`<table><tr><td style="background:#0a66c2;padding:10px"><a href="https://example.com/cta/%d" style="color:#fff">Leggi di piu</a></td></tr></table>`+
			`</td></tr></table></td></tr></table></td></tr></table>`, i, i, i, i, i, i, i, i, i)
	}
	b.WriteString(`</td></tr></table></body></html>`)
	return b.String()
}

func BenchmarkFromHTML(b *testing.B) {
	for _, kb := range []int{8, 30, 150} {
		html := perfNewsletterHTML(kb << 10)
		b.Run(fmt.Sprintf("%dKB", kb), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = richtext.FromHTML(html)
			}
		})
	}
}

func BenchmarkBuildBody(b *testing.B) {
	st := testStyles(b)
	for _, kb := range []int{8, 30, 150} {
		full := &msg.Message{BodyHTML: perfNewsletterHTML(kb << 10), BodyText: "x"}
		for _, w := range []int{120, 200} {
			b.Run(fmt.Sprintf("%dKB/w%d", kb, w), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					_ = buildBody(full, w, st)
				}
			})
		}
	}
}
