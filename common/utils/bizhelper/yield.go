package bizhelper

import (
	"context"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
)

type YieldModelConfig struct {
	Size          int
	IndexField    string
	CountCallback func(int)
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

func WithYieldModel_CountCallback(countCallback func(int)) YieldModelOpts {
	return func(c *YieldModelConfig) {
		c.CountCallback = countCallback
	}
}

func YieldModel[T any](ctx context.Context, db *gorm.DB, opts ...YieldModelOpts) chan T {
	first := true
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
			if first && cfg.CountCallback != nil {
				first = false
				cfg.CountCallback(len(paginator.ids))
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

func YieldModelToMap(ctx context.Context, db *gorm.DB) (chan map[string]any, error) {
	return YieldModelToMapEx(ctx, db, nil)
}

func YieldModelToMapEx(ctx context.Context, db *gorm.DB, countCallback func(int)) (chan map[string]any, error) {
	var count int
	if countCallback != nil {
		if db := db.Count(&count); db.Error == nil {
			countCallback(count)
		}
	}

	rows, err := db.Rows()
	if err != nil {
		return nil, err
	}
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	outC := make(chan map[string]any)
	go func() {
		defer func() {
			rows.Close()
			close(outC)
		}()
		for rows.Next() {
			select {
			case <-ctx.Done():
				return
			default:
			}
			columns := make([]interface{}, len(cols))
			columnPointers := make([]interface{}, len(cols))
			for i := range columns {

				columnPointers[i] = &columns[i]
			}

			if err := rows.Scan(columnPointers...); err != nil {
				return
			}

			m := make(map[string]interface{})
			for i, colName := range cols {
				val := columnPointers[i].(*interface{})
				m[colName] = *val
				colDBType := strings.ToLower(colTypes[i].DatabaseTypeName())
				if colDBType == "bool" || colDBType == "boolean" {
					v := (*val).(int64)
					if v == 1 {
						m[colName] = true
					} else {
						m[colName] = false
					}
				}
			}
			select {
			case <-ctx.Done():
				return
			case outC <- m:
			}
		}
	}()
	return outC, nil
}
