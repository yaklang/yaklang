package yakit

import (
	"context"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

type AliveHost struct {
	gorm.Model

	Hash string `json:"hash"`

	IP        string `json:"ip"`
	IPInteger int64  `json:"ip_integer"`

	// 设置运行时 ID 为了关联具体漏洞
	RuntimeId string `json:"runtime_id"`
}

func CreateOrUpdateAliveHost(db *gorm.DB, hash string, i interface{}) error {
	db = db.Model(&AliveHost{})
	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&AliveHost{}); db.Error != nil {
		return utils.Errorf("create/update AliveHost failed: %s", db.Error)
	}

	return nil
}

func YieldAliveHostRuntimeId(db *gorm.DB, ctx context.Context, runtimeId string) chan *AliveHost {
	outC := make(chan *AliveHost)
	db = db.Model(&AliveHost{})
	db = db.Where("runtime_id = ?", runtimeId)
	go func() {
		defer close(outC)

		var page = 1
		for {
			var items []*AliveHost
			if _, b := bizhelper.NewPagination(&bizhelper.Param{
				DB:    db,
				Page:  page,
				Limit: 1000,
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

			if len(items) < 1000 {
				return
			}
		}
	}()
	return outC
}
