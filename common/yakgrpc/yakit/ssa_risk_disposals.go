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
		db = bizhelper.ExactQueryInt64ArrayOr(db, "ssa_risk_id", filter.GetRiskId())
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
		// 获取所有相关的 Risk 记录
		var risks []schema.SSARisk
		err := tx.Where("id IN (?)", riskIds).Find(&risks).Error
		if err != nil {
			return utils.Errorf("CreateSSARiskDisposals failed to query risks: %v", err)
		}

		// 为每个 Risk 创建处置记录（保留原有的针对单个 Risk 的处置）
		for _, risk := range risks {
			disposal := schema.SSARiskDisposals{
				SSARiskID:       int64(uint64(risk.ID)),
				RiskFeatureHash: risk.RiskFeatureHash, // 设置 RiskFeatureHash 用于继承
				TaskName:        risk.TaskName,
				TaskCreatedAt:   risk.TaskCreatedAt,
				Status:          req.GetStatus(),
				Comment:         req.GetComment(),
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
	return GetSSARiskDisposalsWithInheritance(db, riskId)
}

// GetSSARiskDisposalsOnly 只获取特定 Risk 的直接处置信息（不包括继承）
func GetSSARiskDisposalsOnly(db *gorm.DB, riskId int64) ([]schema.SSARiskDisposals, error) {
	db = db.Model(&schema.SSARiskDisposals{})
	var disposals []schema.SSARiskDisposals
	if err := db.Where("ssa_risk_id = ?", riskId).
		Order("updated_at DESC").
		Find(&disposals).Error; err != nil {
		return nil, utils.Errorf("GetSSARiskDisposalsOnly failed: %v", err)
	}
	return disposals, nil
}

// GetSSARiskDisposalsWithInheritance 获取特定 Risk 的处置信息，包括通过 RiskFeatureHash 继承的历史处置信息
// 只返回早于或等于当前Risk任务创建时间的处置记录，避免后续扫描的处置信息影响当前Risk的查询
func GetSSARiskDisposalsWithInheritance(db *gorm.DB, riskId int64) ([]schema.SSARiskDisposals, error) {
	var risk schema.SSARisk
	if err := db.Where("id = ?", riskId).First(&risk).Error; err != nil {
		return nil, utils.Errorf("GetSSARiskDisposalsWithInheritance failed to query risk: %v", err)
	}

	// 如果没有 RiskFeatureHash，则只返回该 Risk 的直接处置信息
	if risk.RiskFeatureHash == "" {
		return GetSSARiskDisposalsOnly(db, riskId)
	}

	// 查询所有相同 RiskFeatureHash 的处置信息，但只包括早于或等于当前Risk任务创建时间的记录
	var disposals []schema.SSARiskDisposals
	if err := db.Model(&schema.SSARiskDisposals{}).
		Where("risk_feature_hash = ? AND task_created_at <= ?", risk.RiskFeatureHash, risk.TaskCreatedAt).
		Order("task_created_at DESC, updated_at DESC").
		Find(&disposals).Error; err != nil {
		return nil, utils.Errorf("GetSSARiskDisposalsWithInheritance failed: %v", err)
	}

	return disposals, nil
}

func DeleteSSARiskDisposals(db *gorm.DB, req *ypb.DeleteSSARiskDisposalsRequest) (int64, error) {
	var toDelete []schema.SSARiskDisposals
	filteredDB := FilterSSARiskDisposals(db, req.GetFilter())
	if err := filteredDB.Find(&toDelete).Error; err != nil {
		return 0, utils.Errorf("DeleteSSARiskDisposals failed to query records: %v", err)
	}

	// 逐个删除记录，确保 AfterDelete 回调被触发
	var deletedCount int64
	for _, disposal := range toDelete {
		if err := db.Unscoped().Delete(&disposal).Error; err != nil {
			return deletedCount, utils.Errorf("DeleteSSARiskDisposals failed to delete record %d: %v", disposal.ID, err)
		}
		deletedCount++
	}

	return deletedCount, nil
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
