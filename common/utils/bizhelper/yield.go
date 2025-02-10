package bizhelper

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
)

func YieldModel[T any](ctx context.Context, db *gorm.DB, sizes ...int) chan T {
	var t T
	db = db.Table(db.NewScope(t).TableName())

	size := 1024
	if len(sizes) > 0 {
		size = sizes[0]
	}
	db = db.Debug()
	outC := make(chan T)

	go func() {
		defer close(outC)

		paginator := NewFastPaginator(db, size)
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
