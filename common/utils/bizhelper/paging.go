package bizhelper

import (
	"math"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

type Param struct {
	DB      *gorm.DB
	Page    int
	Limit   int
	OrderBy []string
	ShowSQL bool
}

// Paginator 分页返回
type Paginator struct {
	TotalRecord int         `json:"total_record"`
	TotalPage   int         `json:"total_page"`
	Records     interface{} `json:"records"`
	Offset      int         `json:"offset"`
	Limit       int         `json:"limit"`
	Page        int         `json:"page"`
	PrevPage    int         `json:"prev_page"`
	NextPage    int         `json:"next_page"`
}

// Paging 分页
func NewPagination(p *Param, result interface{}) (*Paginator, *gorm.DB) {
	db := p.DB

	if p.ShowSQL {
		db = db.Debug()
	}
	if p.Page < 1 {
		p.Page = 1
	}
	if p.Limit == 0 {
		p.Limit = 10
	}
	if len(p.OrderBy) > 0 {
		for _, o := range p.OrderBy {
			db = db.Order(o)
		}
	}

	var paginator Paginator
	var count int
	var offset int

	db = utils.GormTransactionReturnDb(db, func(tx *gorm.DB) {
		if tx.Count(&count); tx.Error != nil {
			return
		}
		if p.Limit == -1 {
			tx.Find(result)
		} else {
			if p.Page == 1 {
				offset = 0
			} else {
				offset = (p.Page - 1) * p.Limit
			}
			tx.Limit(p.Limit).Offset(offset).Find(result)
		}
	})

	if p.Limit == -1 {
		paginator.TotalRecord = count
		paginator.Records = result
		paginator.Page = 1
		paginator.NextPage = 1
		paginator.Offset = 0
		paginator.Limit = count
		paginator.TotalPage = int(math.Ceil(float64(count) / float64(p.Limit)))
		return &paginator, db
	}

	paginator.TotalRecord = count
	paginator.Records = result
	paginator.Page = p.Page

	paginator.Offset = offset
	paginator.Limit = p.Limit
	paginator.TotalPage = int(math.Ceil(float64(count) / float64(p.Limit)))

	if p.Page > 1 {
		paginator.PrevPage = p.Page - 1
	} else {
		paginator.PrevPage = p.Page
	}

	if p.Page == paginator.TotalPage {
		paginator.NextPage = p.Page
	} else {
		paginator.NextPage = p.Page + 1
	}
	return &paginator, db
}

func countRecords(db *gorm.DB, anyType interface{}, done chan bool, count *int) {
	db.Model(anyType).Count(count)
	done <- true
}
