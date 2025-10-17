package bizhelper

import (
	"fmt"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
)

type FastPaginator struct {
	db          *gorm.DB
	ids         []int
	totalRecord int
	size        int
	offset      int
	page        int
	cfg         *FastPaginatorConfig
	findField   string
	// p   Paginator
}

type FastPaginatorConfig struct {
	OrderBy    string
	IndexField string
}

func NewFastPaginatorConfig() *FastPaginatorConfig {
	return &FastPaginatorConfig{
		IndexField: "id",
	}
}

type FastPaginatorOpts func(*FastPaginatorConfig)

func WithFastPaginator_OrderBy(orderBy string) FastPaginatorOpts {
	return func(c *FastPaginatorConfig) {
		c.OrderBy = orderBy
	}
}

func WithFastPaginator_FindField(field string) FastPaginatorOpts {
	return func(c *FastPaginatorConfig) {
		c.IndexField = field
	}
}

func WithFastPaginator_IndexField(selectField string) FastPaginatorOpts {
	return func(c *FastPaginatorConfig) {
		c.IndexField = selectField
	}
}

func NewFastPaginator(db *gorm.DB, size int, opts ...FastPaginatorOpts) *FastPaginator {
	if size == 0 {
		size = 1024
	}
	cfg := NewFastPaginatorConfig()
	if len(opts) > 0 {
		for _, opt := range opts {
			opt(cfg)
		}
	}
	if cfg.OrderBy != "" {
		db = db.Order(cfg.OrderBy)
	}

	var paginator FastPaginator
	ids := make([]int, 0)

	if err := db.Pluck(cfg.IndexField, &ids).Error; err != nil {
		log.Errorf("failed to get ids: %v", err)
		return nil
	}
	// count := len(ids)

	paginator.cfg = cfg
	paginator.db = db
	paginator.ids = ids
	paginator.totalRecord = len(ids)
	paginator.offset = 0
	paginator.page = 0
	paginator.size = size
	return &paginator
}

func (p *FastPaginator) Next(result any) (error, bool) {
	if p == nil {
		return fmt.Errorf("init FastPaginator fail, maybe model doesn't have id field"), false
	}
	if p.offset >= len(p.ids) {
		return nil, false
	}

	var db *gorm.DB
	if p.size == -1 {
		db = p.db.Find(result)
		p.offset = len(p.ids)
	} else {
		// p.db
		end := p.offset + p.size
		if end > len(p.ids) {
			end = len(p.ids)
		}
		db = ExactQueryIntArrayOr(p.db, p.cfg.IndexField, p.ids[p.offset:end])
		if p.findField != "" {
			db = db.Pluck(p.findField, result)
		} else {
			db = db.Find(result)
		}
		p.page++
		p.offset = end
	}

	return db.Error, true
}
