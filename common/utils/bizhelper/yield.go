package bizhelper

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
)

type YieldModelConfig struct {
	Size       int
	IndexField string
}

func NewYieldModelConfig() *YieldModelConfig {
	return &YieldModelConfig{
		Size:       1024,
		IndexField: "id",
	}
}

type YieldModelOpts func(*YieldModelConfig)

func WithYieldModel_IndexField(selectField string) YieldModelOpts {
	return func(c *YieldModelConfig) {
		c.IndexField = selectField
	}
}

func YieldModel[T any](ctx context.Context, db *gorm.DB, opts ...YieldModelOpts) chan T {
	var t T
	db = db.Table(db.NewScope(t).TableName())

	cfg := NewYieldModelConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	outC := make(chan T)

	go func() {
		defer close(outC)

		paginator := NewFastPaginator(db, cfg.Size, WithFastPaginator_IndexField(cfg.IndexField))
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
