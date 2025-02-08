package bizhelper

import (
	"context"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
)

func YieldModel[T any](ctx context.Context, db *gorm.DB) chan T {
	return YieldModelEx[T](ctx, db, 1024, nil)
}

func YieldModelEx[T any](ctx context.Context, db *gorm.DB, size int, countCallback func(int)) chan T {
	first := true
	outC := make(chan T)

	go func() {
		defer close(outC)

		page := 1
		var items []T
		for {
			p, b := NewPagination(&Param{
				DB:    db,
				Page:  page,
				Limit: size,
			}, &items)
			if b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
			}
			if first && countCallback != nil {
				first = false
				countCallback(p.TotalRecord)
			}

			page++

			for _, d := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- d:
				}
			}

			if len(items) < size {
				return
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
