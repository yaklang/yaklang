package aimemory

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
)

// Item exposes a memory record and raw readable text without JSON quoting.
type Item struct{ *schema.AIMemoryEntity }

func (i *Item) Dump() string {
	return fmt.Sprintf("memory_id=%s namespace=%s created_at=%s tags=%s\n%s", i.MemoryID, i.SessionID,
		i.CreatedAt.Format("2006-01-02 15:04:05"), strings.Join(i.Tags, ","), i.Content)
}
