package aihistory

import (
	"context"
	"fmt"
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// Item is one durable original, with a bounded native Timeline dump. The DB ID
// is stable across session restores; ItemID records the original local ID.
type Item struct {
	*schema.AITimelineHistory
	dump string
}

func (i *Item) Dump() string { return i.dump }

// Query searches all original item rows in one AI session. Current live items
// are excluded before matching/pagination, using stable history identities.
func Query(query string, opts ...Option) ([]*Item, error) {
	c := &options{ctx: context.Background(), mode: "bm25", limit: 10, contentLimit: 2048}
	for _, opt := range opts {
		opt(c)
	}
	if c.ctx == nil {
		return nil, fmt.Errorf("history context is nil")
	}
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(c.session) == "" {
		return nil, fmt.Errorf("ai session ID is required")
	}
	if c.contentOffset < 0 || c.contentLimit < 1 || c.contentLimit > 8192 || c.limit*c.contentLimit > 65536 || c.itemID < 0 {
		return nil, fmt.Errorf("invalid history bounds: content_limit 1..8192; limit × content_limit <=65536; IDs/offsets must be nonnegative")
	}
	if c.db == nil {
		c.db = consts.GetGormProjectDatabase()
	}
	if err := yakit.EnsureAITimelineHistory(c.db, false); err != nil {
		return nil, err
	}
	var excluded []string
	if c.snapshot != nil && c.snapshotSession == c.session {
		items, err := c.snapshot()
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			excluded = append(excluded, item.HistoryID)
		}
		if err := c.db.Transaction(func(tx *gorm.DB) error {
			return yakit.ArchiveAITimelineItems(tx, c.session, items)
		}); err != nil {
			return nil, err
		}
	}
	// Compatibility backfill only for surviving pre-upgrade checkpoint items.
	// Previously discarded originals cannot be recovered from a summary.
	var count int
	if err := c.db.Model(&schema.AITimelineHistory{}).Where("session_id = ?", c.session).Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 && c.db.HasTable(&schema.AIAgentRuntime{}) {
		runtime, err := yakit.GetLatestAIAgentRuntimeByPersistentSession(c.db, c.session)
		if err != nil && !gorm.IsRecordNotFoundError(err) {
			return nil, err
		}
		if runtime != nil && runtime.QuotedTimeline != "" {
			if err := c.db.Transaction(func(tx *gorm.DB) error {
				return yakit.ArchiveAITimelineSnapshot(tx, c.session, runtime.GetTimeline())
			}); err != nil {
				return nil, err
			}
		}
	}
	rows, err := yakit.GrepAITimelineHistory(c.ctx, c.db, query, yakit.AITimelineHistoryQuery{
		SessionID: c.session, Mode: c.mode, Limit: c.limit, Offset: c.offset, ItemID: c.itemID, ExcludeHistoryIDs: excluded})
	if err != nil {
		return nil, err
	}
	items := make([]*Item, 0, len(rows))
	for _, row := range rows {
		dump, err := aicommon.DumpAITimelineHistory([]*schema.AITimelineHistory{row}, c.contentOffset, c.contentLimit)
		if err != nil {
			return nil, err
		}
		items = append(items, &Item{AITimelineHistory: row, dump: dump})
	}
	return items, nil
}
