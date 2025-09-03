package cveresources

import (
	"fmt"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
)

// CVE 2.0 格式的根结构
type CVEYearFileV2 struct {
	ResultsPerPage  int                `json:"resultsPerPage"`
	StartIndex      int                `json:"startIndex"`
	TotalResults    int                `json:"totalResults"`
	Format          string             `json:"format"`
	Version         string             `json:"version"`
	Timestamp       string             `json:"timestamp"`
	Vulnerabilities []CVEVulnerability `json:"vulnerabilities"`
}

// CVE 2.0 中的漏洞记录
type CVEVulnerability struct {
	Cve CVE2Data `json:"cve"`
}

// CVE 2.0 格式的CVE数据
type CVE2Data struct {
	ID               string              `json:"id"`
	SourceIdentifier string              `json:"sourceIdentifier"`
	Published        string              `json:"published"`
	LastModified     string              `json:"lastModified"`
	VulnStatus       string              `json:"vulnStatus"`
	CveTags          []any               `json:"cveTags"`
	Descriptions     []CVE2Description   `json:"descriptions"`
	Metrics          CVE2Metrics         `json:"metrics"`
	Weaknesses       []CVE2Weakness      `json:"weaknesses"`
	Configurations   []CVE2Configuration `json:"configurations"`
	References       []CVE2Reference     `json:"references"`
}

// CVE 2.0 描述信息
type CVE2Description struct {
	Lang  string `json:"lang"`
	Value string `json:"value"`
}

// CVE 2.0 指标信息
type CVE2Metrics struct {
	CvssMetricV40 []CVE2MetricV40 `json:"cvssMetricV40"`
	CvssMetricV31 []CVE2MetricV31 `json:"cvssMetricV31"`
	CvssMetricV30 []CVE2MetricV30 `json:"cvssMetricV30"`
	CvssMetricV2  []CVE2MetricV2  `json:"cvssMetricV2"`
}

// CVE 2.0 CVSS v4.0 指标
type CVE2MetricV40 struct {
	Source   string      `json:"source"`
	Type     string      `json:"type"`
	CvssData CVE2CvssV40 `json:"cvssData"`
}

// CVE 2.0 CVSS v3.1 指标
type CVE2MetricV31 struct {
	Source              string      `json:"source"`
	Type                string      `json:"type"`
	CvssData            CVE2CvssV31 `json:"cvssData"`
	ExploitabilityScore float64     `json:"exploitabilityScore"`
	ImpactScore         float64     `json:"impactScore"`
}

// CVE 2.0 CVSS v3.0 指标
type CVE2MetricV30 struct {
	Source              string      `json:"source"`
	Type                string      `json:"type"`
	CvssData            CVE2CvssV30 `json:"cvssData"`
	ExploitabilityScore float64     `json:"exploitabilityScore"`
	ImpactScore         float64     `json:"impactScore"`
}

// CVE 2.0 CVSS v2 指标
type CVE2MetricV2 struct {
	Source                  string     `json:"source"`
	Type                    string     `json:"type"`
	CvssData                CVE2CvssV2 `json:"cvssData"`
	BaseSeverity            string     `json:"baseSeverity"`
	ExploitabilityScore     float64    `json:"exploitabilityScore"`
	ImpactScore             float64    `json:"impactScore"`
	AcInsufInfo             bool       `json:"acInsufInfo"`
	ObtainAllPrivilege      bool       `json:"obtainAllPrivilege"`
	ObtainUserPrivilege     bool       `json:"obtainUserPrivilege"`
	ObtainOtherPrivilege    bool       `json:"obtainOtherPrivilege"`
	UserInteractionRequired bool       `json:"userInteractionRequired"`
}

// CVE 2.0 CVSS v3.1 数据
type CVE2CvssV31 struct {
	Version               string  `json:"version"`
	VectorString          string  `json:"vectorString"`
	BaseScore             float64 `json:"baseScore"`
	BaseSeverity          string  `json:"baseSeverity"`
	AttackVector          string  `json:"attackVector"`
	AttackComplexity      string  `json:"attackComplexity"`
	PrivilegesRequired    string  `json:"privilegesRequired"`
	UserInteraction       string  `json:"userInteraction"`
	Scope                 string  `json:"scope"`
	ConfidentialityImpact string  `json:"confidentialityImpact"`
	IntegrityImpact       string  `json:"integrityImpact"`
	AvailabilityImpact    string  `json:"availabilityImpact"`
}

// CVE 2.0 CVSS v3.0 数据
type CVE2CvssV30 struct {
	Version               string  `json:"version"`
	VectorString          string  `json:"vectorString"`
	BaseScore             float64 `json:"baseScore"`
	BaseSeverity          string  `json:"baseSeverity"`
	AttackVector          string  `json:"attackVector"`
	AttackComplexity      string  `json:"attackComplexity"`
	PrivilegesRequired    string  `json:"privilegesRequired"`
	UserInteraction       string  `json:"userInteraction"`
	Scope                 string  `json:"scope"`
	ConfidentialityImpact string  `json:"confidentialityImpact"`
	IntegrityImpact       string  `json:"integrityImpact"`
	AvailabilityImpact    string  `json:"availabilityImpact"`
}

// CVE 2.0 CVSS v4.0 数据
type CVE2CvssV40 struct {
	Version                           string  `json:"version"`
	VectorString                      string  `json:"vectorString"`
	BaseScore                         float64 `json:"baseScore"`
	BaseSeverity                      string  `json:"baseSeverity"`
	AttackVector                      string  `json:"attackVector"`
	AttackComplexity                  string  `json:"attackComplexity"`
	AttackRequirements                string  `json:"attackRequirements"`
	PrivilegesRequired                string  `json:"privilegesRequired"`
	UserInteraction                   string  `json:"userInteraction"`
	VulnConfidentialityImpact         string  `json:"vulnConfidentialityImpact"`
	VulnIntegrityImpact               string  `json:"vulnIntegrityImpact"`
	VulnAvailabilityImpact            string  `json:"vulnAvailabilityImpact"`
	SubConfidentialityImpact          string  `json:"subConfidentialityImpact"`
	SubIntegrityImpact                string  `json:"subIntegrityImpact"`
	SubAvailabilityImpact             string  `json:"subAvailabilityImpact"`
	ExploitMaturity                   string  `json:"exploitMaturity"`
	ConfidentialityRequirement        string  `json:"confidentialityRequirement"`
	IntegrityRequirement              string  `json:"integrityRequirement"`
	AvailabilityRequirement           string  `json:"availabilityRequirement"`
	ModifiedAttackVector              string  `json:"modifiedAttackVector"`
	ModifiedAttackComplexity          string  `json:"modifiedAttackComplexity"`
	ModifiedAttackRequirements        string  `json:"modifiedAttackRequirements"`
	ModifiedPrivilegesRequired        string  `json:"modifiedPrivilegesRequired"`
	ModifiedUserInteraction           string  `json:"modifiedUserInteraction"`
	ModifiedVulnConfidentialityImpact string  `json:"modifiedVulnConfidentialityImpact"`
	ModifiedVulnIntegrityImpact       string  `json:"modifiedVulnIntegrityImpact"`
	ModifiedVulnAvailabilityImpact    string  `json:"modifiedVulnAvailabilityImpact"`
	ModifiedSubConfidentialityImpact  string  `json:"modifiedSubConfidentialityImpact"`
	ModifiedSubIntegrityImpact        string  `json:"modifiedSubIntegrityImpact"`
	ModifiedSubAvailabilityImpact     string  `json:"modifiedSubAvailabilityImpact"`
	Safety                            string  `json:"Safety"`
	Automatable                       string  `json:"Automatable"`
	Recovery                          string  `json:"Recovery"`
	ValueDensity                      string  `json:"valueDensity"`
	VulnerabilityResponseEffort       string  `json:"vulnerabilityResponseEffort"`
	ProviderUrgency                   string  `json:"providerUrgency"`
}

// CVE 2.0 CVSS v2 数据
type CVE2CvssV2 struct {
	Version               string  `json:"version"`
	VectorString          string  `json:"vectorString"`
	BaseScore             float64 `json:"baseScore"`
	AccessVector          string  `json:"accessVector"`
	AccessComplexity      string  `json:"accessComplexity"`
	Authentication        string  `json:"authentication"`
	ConfidentialityImpact string  `json:"confidentialityImpact"`
	IntegrityImpact       string  `json:"integrityImpact"`
	AvailabilityImpact    string  `json:"availabilityImpact"`
}

// CVE 2.0 弱点信息
type CVE2Weakness struct {
	Source      string            `json:"source"`
	Type        string            `json:"type"`
	Description []CVE2Description `json:"description"`
}

// CVE 2.0 配置信息
type CVE2Configuration struct {
	Nodes []CVE2Node `json:"nodes"`
}

// CVE 2.0 节点信息
type CVE2Node struct {
	Operator string         `json:"operator"`
	Negate   bool           `json:"negate"`
	CpeMatch []CVE2CpeMatch `json:"cpeMatch"`
}

// CVE 2.0 CPE匹配信息
type CVE2CpeMatch struct {
	Vulnerable            bool   `json:"vulnerable"`
	Criteria              string `json:"criteria"`
	MatchCriteriaId       string `json:"matchCriteriaId"`
	VersionStartExcluding string `json:"versionStartExcluding"`
	VersionEndExcluding   string `json:"versionEndExcluding"`
	VersionStartIncluding string `json:"versionStartIncluding"`
	VersionEndIncluding   string `json:"versionEndIncluding"`
}

// CVE 2.0 参考信息
type CVE2Reference struct {
	URL    string   `json:"url"`
	Source string   `json:"source"`
	Tags   []string `json:"tags"`
}

// CVEVulnerability 转换方法：获取CVE ID
func (v *CVEVulnerability) CVEId() string {
	return v.Cve.ID
}

// CVEVulnerability 转换方法：获取CWE信息
func (v *CVEVulnerability) CWE() string {
	var cwe []string
	for _, weakness := range v.Cve.Weaknesses {
		for _, desc := range weakness.Description {
			if strings.HasPrefix(desc.Value, "CWE-") {
				cwe = append(cwe, desc.Value)
			}
		}
	}
	return strings.Join(cwe, " | ")
}

// CVEVulnerability 转换方法：获取主要描述
func (v *CVEVulnerability) DescriptionMain() string {
	data := v.Cve.Descriptions
	if len(data) <= 0 {
		return ""
	} else if len(data) == 1 {
		return data[0].Value
	} else {
		var (
			currentLength int
			currentData   string
		)
		for _, datum := range data {
			if len(datum.Value) > currentLength {
				currentLength = len(datum.Value)
				currentData = datum.Value
			}
		}
		return currentData
	}
}

// CVEVulnerability 转换方法：获取发布日期
func (v *CVEVulnerability) GetPublishedDate() time.Time {
	// CVE 2.0格式使用 RFC3339 时间格式
	t, err := time.Parse(time.RFC3339, v.Cve.Published)
	if err != nil {
		// 尝试其他格式
		t, err = time.Parse("2006-01-02T15:04:05.000", v.Cve.Published)
		if err != nil {
			log.Error(err)
		}
	}
	return t
}

// CVEVulnerability 转换方法：获取最后修改日期
func (v *CVEVulnerability) GetLastModifiedDate() time.Time {
	// CVE 2.0格式使用 RFC3339 时间格式
	t, err := time.Parse(time.RFC3339, v.Cve.LastModified)
	if err != nil {
		// 尝试其他格式
		t, err = time.Parse("2006-01-02T15:04:05.000", v.Cve.LastModified)
		if err != nil {
			log.Error(err)
		}
	}
	return t
}

// CVEVulnerability 转换为CVE数据库记录
func (v *CVEVulnerability) ToCVE(db *gorm.DB) (*CVE, error) {
	c := &CVE{}
	if strings.HasPrefix(v.DescriptionMain(), "** REJECT **") {
		return nil, fmt.Errorf("REJECT")
	}

	var Vendors []string
	var Products []string

	// 处理配置信息获取厂商和产品
	for _, config := range v.Cve.Configurations {
		for _, node := range config.Nodes {
			vendors, products := v.extractVendorProduct(&node)
			Vendors = append(Vendors, vendors...)
			Products = append(Products, products...)
		}
	}
	Vendors = Set(Vendors)
	Products = Set(Products)

	var titleZh string
	var descZh string
	var solution string
	if descGetter != nil && len(descGetter) > 0 {
		i, ok := descGetter[strings.TrimSpace(v.CVEId())]
		if ok {
			titleZh = i.TitleZh
			solution = i.Solution
			descZh = i.DescriptionMainZh
		}
	}

	// 优先使用 CVSS v4.0，然后是 v3.1，v3.0，最后是 v2
	if len(v.Cve.Metrics.CvssMetricV40) > 0 {
		metric := v.Cve.Metrics.CvssMetricV40[0] // 使用第一个主要评分
		cvss := metric.CvssData

		c = &CVE{
			Model:                   gorm.Model{},
			CVE:                     v.CVEId(),
			CWE:                     v.CWE(),
			TitleZh:                 titleZh,
			DescriptionMainZh:       descZh,
			Solution:                solution,
			ProblemType:             MarshalCheck(v.Cve.Weaknesses),
			References:              MarshalCheck(v.Cve.References),
			DescriptionMain:         v.DescriptionMain(),
			Descriptions:            MarshalCheck(v.Cve.Descriptions),
			CPEConfigurations:       MarshalCheck(v.Cve.Configurations),
			CVSSVersion:             cvss.Version,
			CVSSVectorString:        cvss.VectorString,
			AccessVector:            cvss.AttackVector,
			AccessComplexity:        cvss.AttackComplexity,
			Authentication:          cvss.PrivilegesRequired, // CVSSv4使用PrivilegesRequired替代Authentication
			ConfidentialityImpact:   cvss.VulnConfidentialityImpact,
			IntegrityImpact:         cvss.VulnIntegrityImpact,
			AvailabilityImpact:      cvss.VulnAvailabilityImpact,
			BaseCVSSv2Score:         cvss.BaseScore,
			Severity:                cvss.BaseSeverity,
			ExploitabilityScore:     0,     // CVSSv4中没有单独的exploitabilityScore
			ImpactScore:             0,     // CVSSv4中没有单独的impactScore
			ObtainAllPrivilege:      false, // CVSSv4中没有这些字段
			ObtainUserPrivilege:     false,
			ObtainOtherPrivilege:    false,
			UserInteractionRequired: cvss.UserInteraction == "REQUIRED",
			PublishedDate:           v.GetPublishedDate(),
			LastModifiedData:        v.GetLastModifiedDate(),
			Vendor:                  strings.Join(Vendors, ","),
			Product:                 strings.Join(Products, ","),
		}
	} else if len(v.Cve.Metrics.CvssMetricV31) > 0 {
		metric := v.Cve.Metrics.CvssMetricV31[0] // 使用第一个主要评分
		cvss := metric.CvssData

		c = &CVE{
			Model:                   gorm.Model{},
			CVE:                     v.CVEId(),
			CWE:                     v.CWE(),
			TitleZh:                 titleZh,
			DescriptionMainZh:       descZh,
			Solution:                solution,
			ProblemType:             MarshalCheck(v.Cve.Weaknesses),
			References:              MarshalCheck(v.Cve.References),
			DescriptionMain:         v.DescriptionMain(),
			Descriptions:            MarshalCheck(v.Cve.Descriptions),
			CPEConfigurations:       MarshalCheck(v.Cve.Configurations),
			CVSSVersion:             cvss.Version,
			CVSSVectorString:        cvss.VectorString,
			AccessVector:            cvss.AttackVector,
			AccessComplexity:        cvss.AttackComplexity,
			Authentication:          cvss.PrivilegesRequired, // CVSSv3使用PrivilegesRequired替代Authentication
			ConfidentialityImpact:   cvss.ConfidentialityImpact,
			IntegrityImpact:         cvss.IntegrityImpact,
			AvailabilityImpact:      cvss.AvailabilityImpact,
			BaseCVSSv2Score:         cvss.BaseScore,
			Severity:                cvss.BaseSeverity,
			ExploitabilityScore:     metric.ExploitabilityScore,
			ImpactScore:             metric.ImpactScore,
			ObtainAllPrivilege:      false, // CVSSv3中没有这些字段
			ObtainUserPrivilege:     false,
			ObtainOtherPrivilege:    false,
			UserInteractionRequired: cvss.UserInteraction == "REQUIRED",
			PublishedDate:           v.GetPublishedDate(),
			LastModifiedData:        v.GetLastModifiedDate(),
			Vendor:                  strings.Join(Vendors, ","),
			Product:                 strings.Join(Products, ","),
		}
	} else if len(v.Cve.Metrics.CvssMetricV30) > 0 {
		metric := v.Cve.Metrics.CvssMetricV30[0]
		cvss := metric.CvssData

		c = &CVE{
			Model:                   gorm.Model{},
			CVE:                     v.CVEId(),
			CWE:                     v.CWE(),
			TitleZh:                 titleZh,
			DescriptionMainZh:       descZh,
			Solution:                solution,
			ProblemType:             MarshalCheck(v.Cve.Weaknesses),
			References:              MarshalCheck(v.Cve.References),
			DescriptionMain:         v.DescriptionMain(),
			Descriptions:            MarshalCheck(v.Cve.Descriptions),
			CPEConfigurations:       MarshalCheck(v.Cve.Configurations),
			CVSSVersion:             cvss.Version,
			CVSSVectorString:        cvss.VectorString,
			AccessVector:            cvss.AttackVector,
			AccessComplexity:        cvss.AttackComplexity,
			Authentication:          cvss.PrivilegesRequired,
			ConfidentialityImpact:   cvss.ConfidentialityImpact,
			IntegrityImpact:         cvss.IntegrityImpact,
			AvailabilityImpact:      cvss.AvailabilityImpact,
			BaseCVSSv2Score:         cvss.BaseScore,
			Severity:                cvss.BaseSeverity,
			ExploitabilityScore:     metric.ExploitabilityScore,
			ImpactScore:             metric.ImpactScore,
			ObtainAllPrivilege:      false,
			ObtainUserPrivilege:     false,
			ObtainOtherPrivilege:    false,
			UserInteractionRequired: cvss.UserInteraction == "REQUIRED",
			PublishedDate:           v.GetPublishedDate(),
			LastModifiedData:        v.GetLastModifiedDate(),
			Vendor:                  strings.Join(Vendors, ","),
			Product:                 strings.Join(Products, ","),
		}
	} else if len(v.Cve.Metrics.CvssMetricV2) > 0 {
		metric := v.Cve.Metrics.CvssMetricV2[0]
		cvss := metric.CvssData

		c = &CVE{
			Model:                   gorm.Model{},
			CVE:                     v.CVEId(),
			CWE:                     v.CWE(),
			TitleZh:                 titleZh,
			DescriptionMainZh:       descZh,
			Solution:                solution,
			ProblemType:             MarshalCheck(v.Cve.Weaknesses),
			References:              MarshalCheck(v.Cve.References),
			DescriptionMain:         v.DescriptionMain(),
			Descriptions:            MarshalCheck(v.Cve.Descriptions),
			CPEConfigurations:       MarshalCheck(v.Cve.Configurations),
			CVSSVersion:             cvss.Version,
			CVSSVectorString:        cvss.VectorString,
			AccessVector:            cvss.AccessVector,
			AccessComplexity:        cvss.AccessComplexity,
			Authentication:          cvss.Authentication,
			ConfidentialityImpact:   cvss.ConfidentialityImpact,
			IntegrityImpact:         cvss.IntegrityImpact,
			AvailabilityImpact:      cvss.AvailabilityImpact,
			BaseCVSSv2Score:         cvss.BaseScore,
			Severity:                metric.BaseSeverity,
			ExploitabilityScore:     metric.ExploitabilityScore,
			ImpactScore:             metric.ImpactScore,
			ObtainAllPrivilege:      metric.ObtainAllPrivilege,
			ObtainUserPrivilege:     metric.ObtainUserPrivilege,
			ObtainOtherPrivilege:    metric.ObtainOtherPrivilege,
			UserInteractionRequired: metric.UserInteractionRequired,
			PublishedDate:           v.GetPublishedDate(),
			LastModifiedData:        v.GetLastModifiedDate(),
			Vendor:                  strings.Join(Vendors, ","),
			Product:                 strings.Join(Products, ","),
		}
	} else {
		// 没有CVSS评分的情况
		c = &CVE{
			Model:                   gorm.Model{},
			CVE:                     v.CVEId(),
			CWE:                     v.CWE(),
			TitleZh:                 titleZh,
			DescriptionMainZh:       descZh,
			Solution:                solution,
			ProblemType:             MarshalCheck(v.Cve.Weaknesses),
			References:              MarshalCheck(v.Cve.References),
			DescriptionMain:         v.DescriptionMain(),
			Descriptions:            MarshalCheck(v.Cve.Descriptions),
			CPEConfigurations:       MarshalCheck(v.Cve.Configurations),
			CVSSVersion:             "",
			CVSSVectorString:        "",
			AccessVector:            "",
			AccessComplexity:        "",
			Authentication:          "",
			ConfidentialityImpact:   "",
			IntegrityImpact:         "",
			AvailabilityImpact:      "",
			BaseCVSSv2Score:         0,
			Severity:                "",
			ExploitabilityScore:     0,
			ImpactScore:             0,
			ObtainAllPrivilege:      false,
			ObtainUserPrivilege:     false,
			ObtainOtherPrivilege:    false,
			UserInteractionRequired: false,
			PublishedDate:           v.GetPublishedDate(),
			LastModifiedData:        v.GetLastModifiedDate(),
			Vendor:                  strings.Join(Vendors, ","),
			Product:                 strings.Join(Products, ","),
		}
	}

	return c, nil
}

// 从CPE匹配信息中提取厂商和产品信息
func (v *CVEVulnerability) extractVendorProduct(node *CVE2Node) ([]string, []string) {
	var vendors []string
	var products []string

	for _, cpe := range node.CpeMatch {
		if cpe.Vulnerable {
			// 解析CPE格式：cpe:2.3:a:vendor:product:version:update:edition:language:sw_edition:target_sw:target_hw:other
			parts := strings.Split(cpe.Criteria, ":")
			if len(parts) >= 4 {
				vendor := parts[3]
				if len(parts) >= 5 {
					product := parts[4]
					if vendor != "*" && vendor != "" {
						vendors = append(vendors, vendor)
					}
					if product != "*" && product != "" {
						products = append(products, product)
					}
				}
			}
		}
	}

	return vendors, products
}
