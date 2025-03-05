package yakit

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

func CreateOrUpdateAliveHost(db *gorm.DB, hash string, i interface{}) error {
	db = db.Model(&schema.AliveHost{})
	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&schema.AliveHost{}); db.Error != nil {
		return utils.Errorf("create/update AliveHost failed: %s", db.Error)
	}

	return nil
}

func YieldAliveHostRuntimeId(db *gorm.DB, ctx context.Context, runtimeId string) chan *schema.AliveHost {
	db = db.Model(&schema.AliveHost{})
	db = db.Where("runtime_id = ?", runtimeId)
	return bizhelper.YieldModel[*schema.AliveHost](ctx, db)
}
