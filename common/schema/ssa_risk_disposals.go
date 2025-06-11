package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type SSARiskDisposals struct {
	gorm.Model

	User    string `json:"user" gorm:"index"`
	Status  string `json:"status" gorm:"index"`
	Comment string `json:"comments" gorm:"type:text"`
	RiskId  int64  `json:"risk_id" gorm:"index"`

	Hash string `json:"hash" gorm:"unique_index"`
}

func (s *SSARiskDisposals) CalcHash() string {
	return utils.CalcSha1(s.User, s.Status, s.Comment, s.RiskId)
}

func (s *SSARiskDisposals) BeforeCreate() {
	s.Hash = s.CalcHash()
	s.Status = string(ValidSSARiskDisposalStatus(s.Status))
}

func (s *SSARiskDisposals) BeforeUpdate() {
	s.Hash = s.CalcHash()
	s.Status = string(ValidSSARiskDisposalStatus(s.Status))
}

func (s *SSARiskDisposals) BeforeSave() {
	s.Hash = s.CalcHash()
	s.Status = string(ValidSSARiskDisposalStatus(s.Status))
}

func (s *SSARiskDisposals) ToGRPCModel() *ypb.SSARiskDisposalData {
	return &ypb.SSARiskDisposalData{
		Id:        int64(s.ID),
		CreatedAt: s.CreatedAt.Unix(),
		UpdatedAt: s.UpdatedAt.Unix(),
		User:      s.User,
		Status:    s.Status,
		Comment:   s.Comment,
		RiskId:    s.RiskId,
	}
}

type SSARiskDisposalStatus string

// SSARisk 处置状态
const (
	SSARiskDisposalStatus_NotSet     SSARiskDisposalStatus = "not_set"    // 未处置
	SSARiskDisposalStatus_NotIssue   SSARiskDisposalStatus = "not_issue"  // 不是问题
	SSARiskDisposalStatus_Suspicious SSARiskDisposalStatus = "suspicious" // 疑似问题

	SSARiskDisposalStatus_HisrotyIssue SSARiskDisposalStatus = "history_issue" // 历史遗留问题
	SSARiskDisposalStatus_BadPractice  SSARiskDisposalStatus = "bad_practice"  // 不良实践
	SSARiskDisposalStatus_QualityIssue SSARiskDisposalStatus = "quality_issue" // 质量问题
	SSARiskDisposalStatus_Vuln         SSARiskDisposalStatus = "vuln"          // 可利用的漏洞
)

func ValidSSARiskDisposalStatus(s string) SSARiskDisposalStatus {
	switch s {
	case "not_issue", "safe":
		return SSARiskDisposalStatus_NotIssue
	case "suspicious", "possible":
		return SSARiskDisposalStatus_Suspicious
	case "history_issue":
		return SSARiskDisposalStatus_HisrotyIssue
	case "bad_practice":
		return SSARiskDisposalStatus_BadPractice
	case "quality_issue":
		return SSARiskDisposalStatus_QualityIssue
	case "exploit", "vuln":
		return SSARiskDisposalStatus_Vuln
	default:
		return SSARiskDisposalStatus_NotSet
	}
}
