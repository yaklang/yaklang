package ssadb

import (
	"context"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

func YieldIrCodesProgramName(db *gorm.DB, ctx context.Context, program string) chan *IrCode {
	db = db.Model(&IrCode{}).Where("program_name = ?", program)
	return YieldIrCodes(db, ctx)
}

func YieldIrCodes(db *gorm.DB, ctx context.Context) chan *IrCode {
	db = db.Model(&IrCode{})
	outC := make(chan *IrCode)
	go func() {
		defer close(outC)

		var page = 1
		for {
			var items []*IrCode
			if _, b := bizhelper.Paging(db, page, 100, &items); b.Error != nil {
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

			if len(items) < 1000 {
				return
			}
		}
	}()
	return outC
}
