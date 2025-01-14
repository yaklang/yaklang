package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type SSARisk struct {
	gorm.Model

	Hash string `json:"hash"`

	// file url yakurl
	CodeSourceUrl string `json:"code_source_url"`
	CodeRange     string `json:"code_range"`
	CodeFragment  string `json:"code_fragment"`
	//
	Title           string `json:"title"`
	TitleVerbose    string `json:"title_verbose"`
	Description     string `json:"description"`
	Solution        string `json:"solution"`
	RiskType        string `json:"risk_type"`
	RiskTypeVerbose string `json:"risk_verbose"`
	Details         string `json:"details"`
	Severity        string `json:"severity"`

	// 来源于哪个插件？
	FromRule string `json:"from_rule"`

	// 设置运行时 ID 为了关联具体漏洞
	RuntimeId string `json:"runtime_id"`
	// 潜在威胁：用于输出合规性质的漏洞内容
	IsPotential bool `json:"is_potential"`

	CVE                 string `json:"cve"`
	IsRead              bool   `json:"is_read"`
	Ignore              bool   `json:"ignore"`
	UploadOnline        bool   `json:"upload_online"`
	CveAccessVector     string `json:"cve_access_vector"`
	CveAccessComplexity string `json:"cve_access_complexity"`
	Tags                string `json:"tags"`

	ResultID    uint64 `json:"result_id"`
	ProgramName string `json:"program_name"`
}

func (s *SSARisk) CalcHash() string {
	return utils.CalcSha1(s.CodeSourceUrl, s.CodeRange, s.RuntimeId, s.ProgramName, s.Title, s.RiskType)
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
		RiskTypeVerbose:     s.RiskTypeVerbose,
		Details:             s.Details,
		Severity:            s.Severity,
		FromRule:            s.FromRule,
		RuntimeID:           s.RuntimeId,
		IsPotential:         s.IsPotential,
		CVE:                 s.CVE,
		CveAccessVector:     s.CveAccessVector,
		CveAccessComplexity: s.CveAccessComplexity,
		Tags:                s.Tags,
		ResultID:            s.ResultID,
		IsRead:              s.IsRead,
	}
}

func (s *SSARisk) BeforeCreate(tx *gorm.DB) (err error) {
	s.Hash = s.CalcHash()
	return nil
}

func (s *SSARisk) AfterCreate(tx *gorm.DB) (err error) {
	broadcastData.Call("ssa_risk", "create")
	return nil
}

func (s *SSARisk) AfterUpdate(tx *gorm.DB) (err error) {
	broadcastData.Call("ssa_risk", "update")
	return nil
}
