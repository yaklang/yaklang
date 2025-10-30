package scannode

import (
	"github.com/jinzhu/gorm"
	"github.com/jinzhu/gorm/dialects/postgres"
)

type VulnTargetType string

const (
	VulnTargetType_Url     VulnTargetType = "web"
	VulnTargetType_Service VulnTargetType = "service"
	VulnTargetType_Risk    VulnTargetType = "risk"
)

type Vuln struct {
	gorm.Model

	Title        string
	IPAddr       string
	IPv4Int      uint32
	Host         string // domain/ip
	Port         int
	IsPrivateNet bool

	// url
	Target     string
	TargetRaw  postgres.Jsonb
	TargetType VulnTargetType

	// xray: plugin
	Plugin string

	Detail postgres.Jsonb

	Hash string `gorm:"index"`

	FromThreatAnalysisTaskId    string
	FromThreatAnalysisRuntimeId string
	SubTaskId                   string

	Payload         string `json:"payload"`
	RiskTypeVerbose string `json:"risk_type_verbose"`
	RiskType        string `json:"risk_type"`
	Severity        string `json:"severity"`
	FromYakScript   string `json:"from_yak_script"`
	TitleVerbose    string `json:"title_verbose"`
	ReverseToken    string `json:"reverse_token"`
	Url             string `json:"url"`

	Description string `json:"description"`
	Solution    string `json:"solution"`

	Request  string `json:"request"`
	Response string `json:"response"`

	Parameter string `json:"parameter"`

	IsPotential         bool   `json:"is_potential"`
	CVE                 string `json:"cve"`
	CveAccessVector     string `json:"cve_access_vector"`
	CveAccessComplexity string `json:"cve_access_complexity"`
}
