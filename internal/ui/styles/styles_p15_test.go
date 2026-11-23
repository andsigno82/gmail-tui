package styles

import (
	"testing"

	"github.com/andsigno82/gmail-tui/internal/theme"
)

// Piano 15, Task 2: i campi nuovi di Styles esistono e le liste non sono vuote
// (i consumatori dei Task 6-16 le indicizzano prima del Task 9).
func TestCampiPiano15(t *testing.T) {
	for _, name := range theme.Names() {
		t.Run(name, func(t *testing.T) {
			s := New(loadTheme(t, name))
			_ = []any{s.LinkWarn, s.LinkDashed, s.Button, s.ButtonFocus, s.ButtonHost,
				s.ChipAttach, s.ChipImage, s.TableBorder, s.TableHeader, s.TableCell,
				s.FolderCount, s.FolderTree, s.OverlayBorder, s.OverlayShadow, s.StatusSep}
			if len(s.QuoteBar) == 0 || len(s.BadgeAvatar) == 0 || len(s.StatusSegment) == 0 {
				t.Errorf("liste vuote: QuoteBar=%d BadgeAvatar=%d StatusSegment=%d",
					len(s.QuoteBar), len(s.BadgeAvatar), len(s.StatusSegment))
			}
		})
	}
}
