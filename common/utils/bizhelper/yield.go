package bizhelper

import (
	"context"
	"database/sql"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type YieldModelConfig struct {
	Size          int
	IndexField    string
	CountCallback func(int)
	Limit         int
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

func WithYieldModel_PageSize(size int) YieldModelOpts {
	return func(c *YieldModelConfig) {
		c.Size = size
	}
}

func WithYieldModel_Limit(l int) YieldModelOpts {
	return func(c *YieldModelConfig) {
		c.Limit = l
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
	total := 0
	go func() {
		defer close(outC)

		index := 1

		next := func(res *[]T) (bool, error) {
			defer func() {
				index++
			}()
			_, newDb := PagingByPagination(db, &ypb.Paging{
				Page:  int64(index),
				Limit: int64(cfg.Size),
			}, res)
			if newDb.Error != nil {
				return false, newDb.Error
			}
			if len(*res) == 0 {
				return false, nil
			}
			return true, nil
		}

		tmp := []T{}
		paginator, _ := PagingByPagination(db, &ypb.Paging{
			Page:  1,
			Limit: 1,
		}, &tmp)
		if cfg.CountCallback != nil {
			cfg.CountCallback(paginator.TotalRecord)
		}
		for {
			var items []T
			if ok, err := next(&items); !ok {
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
					total++
					if cfg.Limit > 0 && total >= cfg.Limit {
						return
					}
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
		rows.Close()
		return nil, err
	}
	colTypes, err := rows.ColumnTypes()
	if err != nil {
		rows.Close()
		return nil, err
	}
	outC := make(chan map[string]any)
	go func() {
		defer func() {
			close(outC)
			rows.Close()
		}()
		for rows.Next() {
			select {
			case <-ctx.Done():
				return
			default:
			}
			m, err := RawToMap(rows, cols, colTypes)
			if err != nil {
				log.Errorf("failed to convert row to map: %s", err)
				continue
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

func RawToMap(rows *sql.Rows, cols []string, colTypes []*sql.ColumnType) (map[string]any, error) {
	columns := make([]interface{}, len(cols))
	columnPointers := make([]interface{}, len(cols))
	for i := range columns {

		columnPointers[i] = &columns[i]
	}

	if err := rows.Scan(columnPointers...); err != nil {
		return nil, err
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
	return m, nil
}
