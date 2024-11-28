package bizhelper

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
)

func YieldModel[T any](ctx context.Context, db *gorm.DB, sizes ...int) chan T {
	size := 1024
	if len(sizes) > 0 {
		size = sizes[0]
	}
	outC := make(chan T)

	go func() {
		defer close(outC)

		page := 1
		var items []T
		for {
			if _, b := NewPagination(&Param{
				DB:    db,
				Page:  page,
				Limit: size,
			}, &items); b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
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
