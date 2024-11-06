package bizhelper

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
)

type FastPaginator struct {
	db     *gorm.DB
	ids    []int
	limit  int
	offset int
	page   int
	// p   Paginator
}

func NewFastPaginator(db *gorm.DB, limit int, orderBy ...string) *FastPaginator {
	if limit == 0 {
		limit = 10
	}
	if len(orderBy) > 0 {
		for _, o := range orderBy {
			db = db.Order(o)
		}
	}

	var paginator FastPaginator
	ids := make([]int, 0)

	if err := db.Pluck("id", &ids).Error; err != nil {
		log.Errorf("failed to get ids: %v", err)
		return nil
	}
	// count := len(ids)

	paginator.db = db
	paginator.ids = ids
	paginator.offset = 0
	paginator.page = 0
	paginator.limit = limit
	return &paginator
}

func (p *FastPaginator) Next(result any) (error, bool) {
	if p.offset > len(p.ids) {
		return nil, false
	}

	var db *gorm.DB
	if p.limit == -1 {
		db = p.db.Find(result)
	} else {
		// p.db
		end := p.offset + p.limit
		if end > len(p.ids) {
			end = len(p.ids)
		}
		db = ExactQueryIntArrayOr(p.db, "id", p.ids[p.offset:end])
		db = db.Find(result)
	}

	p.page++
	p.offset = (p.page - 1) * p.limit

	return db.Error, true
}
