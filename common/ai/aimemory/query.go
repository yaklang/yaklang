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

// Query is the item-oriented counterpart of SearchMemory. Existing callers can
// keep using SearchMemory's schema records; scripts can print each Item.Dump().
func Query(query string, opts ...Option) ([]*Item, error) {
	rows, err := SearchMemory(query, opts...)
	if err != nil {
		return nil, err
	}
	items := make([]*Item, 0, len(rows))
	for _, row := range rows {
		items = append(items, &Item{AIMemoryEntity: row})
	}
	return items, nil
}

func Amend(operator, memoryID, content string, tags []string, opts ...Option) (*Item, error) {
	row, err := AmendMemory(operator, memoryID, content, tags, opts...)
	if err != nil {
		return nil, err
	}
	return &Item{AIMemoryEntity: row}, nil
}
