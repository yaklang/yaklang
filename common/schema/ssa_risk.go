package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	ServerPushType_SSARisk = "ssa_risk"
)

type SSARisk struct {
	gorm.Model

	Hash string `json:"hash" gorm:"unique_index"`

	// risk info
	Title        string             `json:"title"`
	TitleVerbose string             `json:"title_verbose"`
	Description  string             `json:"description"`
	Solution     string             `json:"solution"`
	RiskType     string             `json:"risk_type"`
	Details      string             `json:"details"`
	Severity     SyntaxFlowSeverity `json:"severity"`
	// 潜在威胁：用于输出合规性质的漏洞内容
	IsPotential bool `json:"is_potential"`

	// risk info
	CVE                 string `json:"cve"`
	IsRead              bool   `json:"is_read"`
	Ignore              bool   `json:"ignore"`
	UploadOnline        bool   `json:"upload_online"`
	CveAccessVector     string `json:"cve_access_vector"`
	CveAccessComplexity string `json:"cve_access_complexity"`
	Tags                string `json:"tags"`

	// 来源于哪个规则
	FromRule string `json:"from_rule"`
	// 来源于哪个项目
	ProgramName string `json:"program_name" gorm:"index"`
	// file url yakurl
	CodeSourceUrl string `json:"code_source_url" gorm:"index"`
	CodeRange     string `json:"code_range"`
	CodeFragment  string `json:"code_fragment"`
	// for query risk
	FunctionName string `json:"function_name" gorm:"index"`
	Line         int64  `json:"line" gorm:"index"`
	// 设置运行时 ID 关联 SyntaxflowTask
	RuntimeId string `json:"runtime_id"`
	// for query result
	ResultID uint64 `json:"result_id"` // result
	Variable string `json:"variable"`  // result/variable
	Index    int64  `json:"index"`     // result/variable/index
}

func (s *SSARisk) CalcHash() string {
	return utils.CalcSha1(
		s.CodeSourceUrl, s.CodeRange, // source code range
		s.RuntimeId,                                    // syntaxflow scan task id
		s.ProgramName, s.ResultID, s.Variable, s.Index, // syntaxflow result index
		s.Title, s.RiskType, // risk info
	)
}

func SSARiskTypeVerbose(s string) string {
	switch s {
	case "cwe":
		return "CWE"
	case "owasp":
		return "OWASP"
	case "custom":
		return "自定义"
	default:
		return s
	}
}
func (s *SSARisk) ToGRPCModel() *ypb.SSARisk {
	return &ypb.SSARisk{
		Id:                  int64(s.ID),
		CreatedAt:           s.CreatedAt.Unix(),
		UpdatedAt:           s.UpdatedAt.Unix(),
		Hash:                s.Hash,
		ProgramName:         s.ProgramName,
		CodeSourceUrl:       s.CodeSourceUrl,
		CodeRange:           s.CodeRange,
		CodeFragment:        s.CodeFragment,
		Title:               s.Title,
		TitleVerbose:        s.TitleVerbose,
		RiskType:            s.RiskType,
		RiskTypeVerbose:     SSARiskTypeVerbose(s.RiskType),
		Details:             s.Details,
		Severity:            string(s.Severity),
		FromRule:            s.FromRule,
		RuntimeID:           s.RuntimeId,
		IsPotential:         s.IsPotential,
		CVE:                 s.CVE,
		CveAccessVector:     s.CveAccessVector,
		CveAccessComplexity: s.CveAccessComplexity,
		Tags:                s.Tags,
		ResultID:            s.ResultID,
		IsRead:              s.IsRead,
		Variable:            s.Variable,
		Index:               s.Index,
		FunctionName:        s.FunctionName,
		Line:                s.Line,
		Description:         s.Description,
		Solution:            s.Solution,
	}
}

func (s *SSARisk) BeforeCreate(tx *gorm.DB) (err error) {
	if s.RiskType == "" {
		s.RiskType = "其他"
	}
	s.Severity = ValidSeverityType(s.Severity)
	s.Hash = s.CalcHash()
	return nil
}

func (s *SSARisk) AfterCreate(tx *gorm.DB) (err error) {
	broadcastData.Call(ServerPushType_SSARisk, map[string]string{
		"task_id": s.RuntimeId,
		"action":  "create",
	})
	return nil
}

func (s *SSARisk) AfterUpdate(tx *gorm.DB) (err error) {
	broadcastData.Call(ServerPushType_SSARisk, map[string]string{
		"task_id": s.RuntimeId,
		"action":  "update",
	})
	return nil
}
