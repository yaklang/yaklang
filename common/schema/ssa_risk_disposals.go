package schema

import (
	"errors"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type SSARiskDisposals struct {
	gorm.Model
	TaskId    string `json:"task_id" gorm:"index"`
	SSARiskID int64  `json:"ssa_risk_id" gorm:"index"`
	// RiskFeatureHash 用于标识风险特征的唯一哈希值,可以用来实现处置的继承
	RiskFeatureHash string `json:"risk_feature_hash" gorm:"index"`
	Status          string `json:"status" gorm:"index"`
	Comment         string `json:"comment" gorm:"type:text"`
}

func (s *SSARiskDisposals) BeforeCreate() {
	s.Status = string(ValidSSARiskDisposalStatus(s.Status))
}

func (s *SSARiskDisposals) BeforeUpdate() {
	s.Status = string(ValidSSARiskDisposalStatus(s.Status))
}

func (s *SSARiskDisposals) BeforeSave() {
	s.Status = string(ValidSSARiskDisposalStatus(s.Status))
}

func (s *SSARiskDisposals) AfterCreate(tx *gorm.DB) error {
	return s.updateRiskLatestDisposalStatus(tx)
}

func (s *SSARiskDisposals) AfterUpdate(tx *gorm.DB) error {
	return s.updateRiskLatestDisposalStatus(tx)
}

func (s *SSARiskDisposals) AfterDelete(tx *gorm.DB) error {
	return s.updateRiskLatestDisposalStatus(tx)
}

// updateRiskLatestDisposalStatus用于更新SSARisk的最新处置状态
func (s *SSARiskDisposals) updateRiskLatestDisposalStatus(tx *gorm.DB) error {
	if s.SSARiskID == 0 {
		return nil
	}

	var latestDisposal SSARiskDisposals
	err := tx.Where("ssa_risk_id = ?", s.SSARiskID).
		Order("updated_at DESC").
		First(&latestDisposal).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Model(&SSARisk{}).
				Where("id = ?", s.SSARiskID).
				Update("latest_disposal_status", string(SSARiskDisposalStatus_NotSet)).Error
		}
		return err
	}

	return tx.Model(&SSARisk{}).
		Where("id = ?", s.SSARiskID).
		Update("latest_disposal_status", latestDisposal.Status).Error
}

func (s *SSARiskDisposals) ToGRPCModel() *ypb.SSARiskDisposalData {
	return &ypb.SSARiskDisposalData{
		Id:        int64(s.ID),
		CreatedAt: s.CreatedAt.Unix(),
		UpdatedAt: s.UpdatedAt.Unix(),
		RiskId:    s.SSARiskID,
		Status:    s.Status,
		Comment:   s.Comment,
		TaskName:  s.TaskId,
	}
}

type SSARiskDisposalStatus string

// SSARisk 处置状态
const (
	SSARiskDisposalStatus_NotSet     SSARiskDisposalStatus = "not_set"    // 未处置
	SSARiskDisposalStatus_NotIssue   SSARiskDisposalStatus = "not_issue"  // 不是问题
	SSARiskDisposalStatus_Suspicious SSARiskDisposalStatus = "suspicious" // 疑似问题
	SSARiskDisposalStatus_IsIssue    SSARiskDisposalStatus = "is_issue"   // 存在漏洞
)

func ValidSSARiskDisposalStatus(s string) SSARiskDisposalStatus {
	switch s {
	case "not_issue", "safe":
		return SSARiskDisposalStatus_NotIssue
	case "suspicious", "possible":
		return SSARiskDisposalStatus_Suspicious
	case "issue", "is_issue":
		return SSARiskDisposalStatus_IsIssue
	default:
		return SSARiskDisposalStatus_NotSet
	}
}
