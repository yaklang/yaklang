package aihistory

import (
	"context"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

type options struct {
	ctx                                        context.Context
	db                                         *gorm.DB
	session, mode, snapshotSession             string
	limit, offset, contentOffset, contentLimit int
	itemID                                     int64
	snapshot                                   func() ([]schema.AITimelineHistory, error)
}

type Option func(*options)

func WithContext(ctx context.Context) Option { return func(c *options) { c.ctx = ctx } }
func WithDatabase(db *gorm.DB) Option        { return func(c *options) { c.db = db } }
func WithAISession(session string) Option    { return func(c *options) { c.session = session } }
func WithSearchMode(mode string) Option      { return func(c *options) { c.mode = mode } }
func WithLimit(limit int) Option             { return func(c *options) { c.limit = limit } }
func WithOffset(offset int) Option           { return func(c *options) { c.offset = offset } }
func WithItemID(id int64) Option             { return func(c *options) { c.itemID = id } }
func WithContentOffset(offset int) Option    { return func(c *options) { c.contentOffset = offset } }
func WithContentLimit(limit int) Option      { return func(c *options) { c.contentLimit = limit } }
func WithLiveTimeline(session string, snapshot func() ([]schema.AITimelineHistory, error)) Option {
	return func(c *options) { c.snapshotSession, c.snapshot = session, snapshot }
}

var Exports = map[string]any{
	"Query": Query, "CurrentSession": func() string { return "" },
	"aiSession": WithAISession, "searchMode": WithSearchMode, "limit": WithLimit,
	"offset": WithOffset, "itemID": WithItemID, "contentOffset": WithContentOffset,
	"contentLimit": WithContentLimit,
}
