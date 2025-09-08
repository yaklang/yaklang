package entityrepos

import (
	"context"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

func (r *EntityRepository) YieldEntities(ctx context.Context) chan *schema.ERModelEntity {
	db := r.db.Model(&schema.ERModelEntity{})
	db = bizhelper.ExactQueryString(db, "repository_uuid", r.info.Uuid)
	return bizhelper.YieldModel[*schema.ERModelEntity](ctx, db)
}

func (r *EntityRepository) YieldRelationships(ctx context.Context) chan *schema.ERModelRelationship {
	db := r.db.Model(&schema.ERModelRelationship{})
	db = bizhelper.ExactQueryString(db, "repository_uuid", r.info.Uuid)
	return bizhelper.YieldModel[*schema.ERModelRelationship](ctx, db)
}

type HopBlock struct {
	Src          *schema.ERModelEntity
	Relationship *schema.ERModelRelationship
	Next         *HopBlock
	IsEnd        bool
	Dst          *schema.ERModelEntity
}

type KHopQueryResult struct {
	K    int
	Hops *HopBlock
}

type KHopConfig struct {
	K int
}

type KHopQueryOption func(*KHopConfig)

func (r *EntityRepository) QueryKHop(ctx context.Context, opts ...KHopQueryOption) chan *KHopQueryResult {
	var ch = make(chan *KHopQueryResult)
	go func() {
		var visitedEntity = make(map[string]struct{})
		_ = visitedEntity
		for ele := range r.YieldEntities(ctx) {
			_, ok := visitedEntity[ele.Uuid]
			if ok {
				continue
			}
			visitedEntity[ele.Uuid] = struct{}{}
			// bfs
		}
	}()
	return ch
}
