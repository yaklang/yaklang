package entityrepos

import (
	"context"

	"github.com/yaklang/yaklang/common/schema"
)

func (r *EntityRepository) YieldEntities(ctx context.Context) (chan *schema.ERModelEntity, error) {
	//var offset int
	//var page = 1
	var result = make(chan *schema.ERModelEntity)
	go func() {
		db := r.db.Model(&schema.ERModelEntity{})
		var total int64
		db.Where("repository_uuid").Count(&total)
		for {

		}
	}()

	return result, nil
}
