package yakit

import (
	"errors"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type shellOptions struct {
}

func CreateOrUpdateWebShell(db *gorm.DB, hash string, i interface{}) (*schema.WebShell, error) {
	db = db.Model(&schema.WebShell{})
	shell := &schema.WebShell{}
	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(shell); db.Error != nil {
		return nil, utils.Errorf("create/update WebShell failed: %s", db.Error)
	}

	return shell, nil
}

func UpdateWebShellStateById(db *gorm.DB, id int64, state bool) (*schema.WebShell, error) {
	db = db.Model(&schema.WebShell{}).Debug()
	shell := &schema.WebShell{}

	// First, try to find the record
	if err := db.Where("id = ?", id).First(shell).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// If the record is not found, return an error
			return nil, utils.Errorf("WebShell not found: %s", err)
		} else {
			// Some other error occurred
			return nil, utils.Errorf("retrieve WebShell failed: %s", err)
		}
	}
	// If the record is found, update it
	if err := db.Model(shell).Update("status", state).Error; err != nil {
		return nil, utils.Errorf("update WebShell failed: %s", err)
	}

	return shell, nil
}

func UpdateWebShellById(db *gorm.DB, id int64, i interface{}) (*schema.WebShell, error) {
	db = db.Model(&schema.WebShell{}).Debug()
	shell := &schema.WebShell{}

	// First, try to find the record
	if err := db.Where("id = ?", id).First(shell).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// If the record is not found, return an error
			return nil, utils.Errorf("WebShell not found: %s", err)
		} else {
			// Some other error occurred
			return nil, utils.Errorf("retrieve WebShell failed: %s", err)
		}
	}
	// If the record is found, update it
	if err := db.Model(shell).Update(i).Error; err != nil {
		return nil, utils.Errorf("update WebShell failed: %s", err)
	}

	return shell, nil
}

func DeleteWebShellByID(db *gorm.DB, ids ...int64) error {
	if len(ids) == 1 {
		id := ids[0]
		if db := db.Model(&schema.WebShell{}).Where(
			"id = ?", id,
		).Unscoped().Delete(&schema.WebShell{}); db.Error != nil {
			return db.Error
		}
		return nil
	}
	if db = bizhelper.ExactQueryInt64ArrayOr(db, "id", ids).Unscoped().Delete(&schema.WebShell{}); db.Error != nil {
		return utils.Errorf("delete id(s) failed: %v", db.Error)
	}
	return nil
}

func GetWebShell(db *gorm.DB, id int64) (*ypb.WebShell, error) {
	shell := &schema.WebShell{}
	if db := db.Model(&schema.WebShell{}).Where("id = ?", id).First(shell); db.Error != nil {
		return nil, utils.Errorf("get WebShell failed: %s", db.Error)
	}
	return shell.ToGRPCModel(), nil
}

func QueryWebShells(db *gorm.DB, params *ypb.QueryWebShellsRequest) (*bizhelper.Paginator, []*schema.WebShell, error) {
	if params == nil {
		return nil, nil, utils.Errorf("empty params")
	}

	db = db.Model(&schema.WebShell{}) // .Debug()
	if params.Pagination == nil {
		params.Pagination = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}
	p := params.Pagination

	var ret []*schema.WebShell
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}
