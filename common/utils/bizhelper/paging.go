package bizhelper

import (
	"fmt"
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

	QueryCountOnce     bool `json:"query_count_once"`    // 如果开启了，那么只有在NewPagination的时候才会查询Count，Next方法调用的时候就不查询Count
	DisableTransaction bool `json:"disable_transaction"` // 如果开启了，那么不使用Transaction
	totalRecord        *int `json:"-"`                   // cache
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
	param       *Param      `json:"-"`
}

// NewPaginatorFromTotal builds a paginator from a known total record count.
// Useful when the caller wants to avoid running an extra Find just to compute paging metadata.
func NewPaginatorFromTotal(page, limit, total int) *Paginator {
	if page < 1 {
		page = 1
	}
	if limit == 0 {
		limit = 10
	}

	p := &Paginator{
		TotalRecord: total,
		Page:        page,
		Limit:       limit,
	}

	if limit == -1 {
		p.TotalPage = 1
		p.Offset = 0
		p.PrevPage = page
		p.NextPage = page
		return p
	}

	if limit < 0 {
		limit = 10
		p.Limit = limit
	}

	p.TotalPage = int(math.Ceil(float64(total) / float64(limit)))
	if p.TotalPage < 1 {
		p.TotalPage = 1
	}
	if page == 1 {
		p.Offset = 0
	} else {
		p.Offset = (page - 1) * limit
	}
	if page > 1 {
		p.PrevPage = page - 1
	} else {
		p.PrevPage = page
	}
	if page >= p.TotalPage {
		p.NextPage = page
	} else {
		p.NextPage = page + 1
	}
	return p
}

// NewPaginatorWithoutTotal builds a paginator when total count is skipped/unknown.
// TotalRecord/TotalPage are set to -1 to signal "unknown".
func NewPaginatorWithoutTotal(page, limit int) *Paginator {
	if page < 1 {
		page = 1
	}
	if limit == 0 {
		limit = 10
	}

	p := &Paginator{
		TotalRecord: -1,
		TotalPage:   -1,
		Page:        page,
		Limit:       limit,
	}

	if limit > 0 {
		p.Offset = (page - 1) * limit
	} else {
		p.Offset = 0
	}

	if page > 1 {
		p.PrevPage = page - 1
	} else {
		p.PrevPage = page
	}

	// Unknown total -> keep NextPage at current page.
	p.NextPage = page
	return p
}

// Paging 分页
func NewPagination(p *Param, result interface{}) (*Paginator, *gorm.DB) {
	defer func() {
		if r := recover(); r != nil {
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
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
	shouldQueryCount := true

	if p.QueryCountOnce && p.totalRecord != nil {
		count = *p.totalRecord
		shouldQueryCount = false
	}

	queryFunc := func(tx *gorm.DB) {
		if tx == nil {
			println("tx is nil")
		}
		defer func() {
			if r := recover(); r != nil {
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()
		if shouldQueryCount {
			if tx.Count(&count); tx.Error != nil {
				return
			}
			if p.QueryCountOnce {
				countVal := count
				p.totalRecord = &countVal
			}
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
	}

	if p.DisableTransaction {
		queryFunc(db)
	} else {
		utils.GormTransactionReturnDb(db, queryFunc)
	}

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
	paginator.param = p
	return &paginator, db
}

func (p *Paginator) Next(result interface{}) (error, bool) {
	if p.param == nil {
		return fmt.Errorf("paginator param is nil"), false
	}
	if p.Page >= p.TotalPage {
		return nil, false
	}
	p.param.Page = p.Page + 1
	newP, db := NewPagination(p.param, result)
	if db.Error != nil {
		return db.Error, false
	}
	*p = *newP
	return nil, true
}

func countRecords(db *gorm.DB, anyType interface{}, done chan bool, count *int) {
	db.Model(anyType).Count(count)
	done <- true
}
