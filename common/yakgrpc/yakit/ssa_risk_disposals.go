package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func FilterSSARiskDisposals(db *gorm.DB, filter *ypb.SSARiskDisposalsFilter) *gorm.DB {
	if filter == nil {
		return db
	}
	db = db.Model(&schema.SSARiskDisposals{})
	if len(filter.GetID()) > 0 {
		db = bizhelper.ExactQueryInt64ArrayOr(db, "id", filter.GetID())
	}
	if len(filter.GetRiskId()) > 0 {
		db = bizhelper.ExactQueryInt64ArrayOr(db, "risk_id", filter.GetRiskId())
	}
	if len(filter.GetStatus()) > 0 {
		db = bizhelper.ExactQueryStringArrayOr(db, "status", filter.GetStatus())
	}
	if filter.GetSearch() != "" {
		db = bizhelper.FuzzSearchEx(db, []string{"comment", "status"}, filter.GetSearch(), false)
	}
	return db
}

func CreateSSARiskDisposals(db *gorm.DB, req *ypb.CreateSSARiskDisposalsRequest) ([]schema.SSARiskDisposals, error) {
	if req == nil {
		return nil, utils.Error("CreateSSARiskDisposals failed: CreateSSARiskDisposalsRequest is nil")
	}
	riskIds := req.GetRiskIds()
	if len(riskIds) == 0 {
		return nil, utils.Error("CreateSSARiskDisposals failed: riskIds is empty")
	}

	var result []schema.SSARiskDisposals
	err := utils.GormTransaction(db, func(tx *gorm.DB) error {
		for _, riskId := range riskIds {
			disposal := schema.SSARiskDisposals{
				Status:    req.GetStatus(),
				Comment:   req.GetComment(),
				SSARiskID: riskId,
			}
			if err := tx.Create(&disposal).Error; err != nil {
				return utils.Errorf("CreateSSARiskDisposals failed during create: %v", err)
			}
			result = append(result, disposal)
		}
		return nil
	})
	if err != nil {
		return nil, utils.Errorf("CreateSSARiskDisposals failed: %v", err)
	}
	return result, nil
}

func QuerySSARiskDisposals(db *gorm.DB, req *ypb.QuerySSARiskDisposalsRequest) (*bizhelper.Paginator, []schema.SSARiskDisposals, error) {
	if req == nil {
		return nil, nil, utils.Error("QuerySSARiskDisposals failed: QuerySSARiskDisposalsRequest is nil")
	}
	paging := req.GetPagination()
	if paging == nil {
		paging = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "created_at",
			Order:   "desc",
		}
	}
	db = bizhelper.OrderByPaging(db, paging)
	db = FilterSSARiskDisposals(db, req.GetFilter())
	var ret []schema.SSARiskDisposals
	pag, db := bizhelper.Paging(db, int(paging.Page), int(paging.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("QuerySSARiskDisposals failed: %v", db.Error)
	}
	return pag, ret, nil
}

func GetSSARiskDisposals(db *gorm.DB, riskId int64) ([]schema.SSARiskDisposals, error) {
	db = db.Model(&schema.SSARiskDisposals{})
	var disposals []schema.SSARiskDisposals
	if err := db.Where("risk_id = ?", riskId).Find(&disposals).Error; err != nil {
		return nil, utils.Errorf("GetSSARiskDisposals failed: %v", err)
	}
	return disposals, nil
}

func DeleteSSARiskDisposals(db *gorm.DB, req *ypb.DeleteSSARiskDisposalsRequest) (int64, error) {
	db = FilterSSARiskDisposals(db, req.GetFilter())
	db = db.Unscoped().Delete(&schema.SSARiskDisposals{})
	if db.Error != nil {
		return 0, utils.Errorf("DeleteSSARiskDisposals failed: %v", db.Error)
	}
	return db.RowsAffected, nil
}

func UpdateSSARiskDisposals(db *gorm.DB, req *ypb.UpdateSSARiskDisposalsRequest) ([]schema.SSARiskDisposals, error) {
	var toUpdate []schema.SSARiskDisposals
	err := utils.GormTransaction(db, func(tx *gorm.DB) error {
		filteredDB := FilterSSARiskDisposals(tx, req.GetFilter())
		var existResult []schema.SSARiskDisposals
		if err := filteredDB.Find(&existResult).Error; err != nil {
			return utils.Errorf("UpdateSSARiskDisposals failed: %v", err)
		}
		toUpdate = lo.Map(existResult, func(item schema.SSARiskDisposals, index int) schema.SSARiskDisposals {
			item.Status = req.GetStatus()
			item.Comment = req.GetComment()
			return item
		})
		tx = tx.Model(&schema.SSARiskDisposals{})
		for _, disposal := range toUpdate {
			if err := tx.Save(&disposal).Error; err != nil {
				return utils.Errorf("UpdateSSARiskDisposals failed during save: %v", err)
			}
		}
		return nil
	})
	return toUpdate, err
}
