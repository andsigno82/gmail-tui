package app

import "context"

// UnreadCounts conta le chiamate (solo dopo il Piano 15: al commit 82efdd8 il
// metodo non esiste nel contratto MailStore).
func (c *perfCountStore) UnreadCounts(ctx context.Context) (map[string]int, error) {
	c.hit("UnreadCounts")
	return c.MailStore.UnreadCounts(ctx)
}
