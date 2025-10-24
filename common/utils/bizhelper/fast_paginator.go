package bizhelper

import (
	"context"
	"fmt"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
)

type FastPaginator struct {
	db          *gorm.DB
	ids         []int64
	totalRecord int
	size        int
	offset      int
	page        int
	// p   Paginator
	OrderBy    string
	IndexField string
}

type FastPaginatorOpts func(*FastPaginator)

func WithFastPaginator_IDs(ids []int64) FastPaginatorOpts {
	return func(c *FastPaginator) {
		c.ids = ids
	}
}

func WithFastPaginator_OrderBy(orderBy string) FastPaginatorOpts {
	return func(c *FastPaginator) {
		c.OrderBy = orderBy
	}
}

func WithFastPaginator_IndexField(selectField string) FastPaginatorOpts {
	return func(c *FastPaginator) {
		c.IndexField = selectField
	}
}

func FastPagination[T any](ctx context.Context, db *gorm.DB, cfg *YieldModelConfig, opts ...FastPaginatorOpts) chan T {
	outC := make(chan T)
	go func() {
		defer close(outC)
		size := 0
		if cfg != nil {
			if cfg.Size > 0 {
				size = cfg.Size
			}
			if cfg.IndexField != "" {
				opts = append(opts, WithFastPaginator_IndexField(cfg.IndexField))
			}
		}

		paginator := NewFastPaginator(db, size, opts...)
		if cfg != nil && cfg.CountCallback != nil {
			cfg.CountCallback(paginator.totalRecord)
		}
		for {
			var items []T
			if err, ok := paginator.Next(&items); !ok {
				break
			} else if err != nil {
				log.Errorf("paging failed: %s", err)
				break
			}

			for _, d := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- d:
				}
			}
		}
	}()
	return outC
}

func NewFastPaginator(db *gorm.DB, size int, opts ...FastPaginatorOpts) *FastPaginator {
	if size == 0 {
		size = 10
	}

	var paginator = &FastPaginator{
		IndexField: "id",
	}
	if len(opts) > 0 {
		for _, opt := range opts {
			opt(paginator)
		}
	}
	if paginator.OrderBy != "" {
		db = db.Order(paginator.OrderBy)
	}

	if len(paginator.ids) == 0 {
		paginator.ids = make([]int64, 0)
		if err := db.Pluck(paginator.IndexField, &paginator.ids).Error; err != nil {
			log.Errorf("failed to get ids: %v", err)
			return nil
		}
	}
	paginator.db = db
	paginator.totalRecord = len(paginator.ids)
	paginator.offset = 0
	paginator.page = 0
	paginator.size = size
	return paginator
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
		db = ExactQueryInt64ArrayOr(p.db, p.IndexField, p.ids[p.offset:end])
		db = db.Find(result)
		p.page++
		p.offset = end
	}

	return db.Error, true
}
