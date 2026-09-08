package cveresources

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/gorm"
)

// CVE 5.0 Record 根结构
// 数据源: cveawg.mitre.org/api/cve/{cveId} 或 cvelistV5 GitHub Releases
type CVERecordV5 struct {
	DataType    string         `json:"dataType"`    // "CVE_RECORD"
	DataVersion string         `json:"dataVersion"` // "5.0" / "5.1" / "5.2"
	CVEMetadata CVE5Metadata   `json:"cveMetadata"`
	Containers  CVE5Containers `json:"containers"`
}

type CVE5Metadata struct {
	CVEID              string `json:"cveId"`
	AssignerOrgId      string `json:"assignerOrgId"`
	AssignerShortName  string `json:"assignerShortName"`
	State              string `json:"state"` // PUBLISHED / REJECTED
	DateReserved       string `json:"dateReserved"`
	DatePublished      string `json:"datePublished"`
	DateUpdated        string `json:"dateUpdated"`
}

// CVE5Containers 可能包含 cna 和 adp 两个容器
// cna: CNA 提交的原始数据 (title, affected, solutions, metrics 等)
// adp: ADP (Authorized Data Publisher) 补充数据，如 CISA 的 SSVC 分析
type CVE5Containers struct {
	CNA *CVE5CNA     `json:"cna"`
	ADP []CVE5ADP    `json:"adp"`
}

// CVE 5.0 CNA 容器 — 包含漏洞的核心信息
type CVE5CNA struct {
	ProviderMetadata CVE5ProviderMeta   `json:"providerMetadata"`
	Title            string             `json:"title"`        // ← 原始英文标题
	DatePublic       string             `json:"datePublic"`
	Descriptions     []CVE5Description  `json:"descriptions"`
	Affected         []CVE5Affected     `json:"affected"`     // ← 影响产品
	Solutions        []CVE5Solution     `json:"solutions"`    // ← 解决方案
	ProblemTypes     []CVE5ProblemType  `json:"problemTypes"`
	Metrics          []CVE5Metric       `json:"metrics"`
	References       []CVE5Reference    `json:"references"`
	Credits          []CVE5Credit       `json:"credits"`
	RejectedReasons  []CVE5Description  `json:"rejectedReasons"`
	// CPE 配置 (部分 CNA 如 Microsoft 会提交)
	CPEApplicability json.RawMessage    `json:"cpeApplicability,omitempty"`
}

type CVE5ADP struct {
	ProviderMetadata CVE5ProviderMeta  `json:"providerMetadata"`
	Title            string            `json:"title"`
	Metrics          []CVE5Metric      `json:"metrics"`
}

type CVE5ProviderMeta struct {
	OrgId       string `json:"orgId"`
	ShortName   string `json:"shortName"`
	DateUpdated string `json:"dateUpdated"`
}

type CVE5Description struct {
	Lang  string `json:"lang"`
	Value string `json:"value"`
}

type CVE5Affected struct {
	Vendor       string        `json:"vendor"`
	Product      string        `json:"product"`
	Platforms    []string      `json:"platforms"`
	DefaultStatus string       `json:"defaultStatus"`
	Versions     []CVE5Version `json:"versions"`
	CPEs         []string      `json:"cpes"`
	PackageURL   string        `json:"packageURL"`
}

type CVE5Version struct {
	Version         string `json:"version"`
	Status          string `json:"status"` // affected / unaffected / unknown
	VersionType     string `json:"versionType"`
	LessThan        string `json:"lessThan"`
	LessThanOrEqual string `json:"lessThanOrEqual"`
}

type CVE5Solution struct {
	Lang  string `json:"lang"`
	Value string `json:"value"`
}

type CVE5ProblemType struct {
	Descriptions []CVE5ProblemTypeDesc `json:"descriptions"`
}

type CVE5ProblemTypeDesc struct {
	Lang        string `json:"lang"`
	Description string `json:"description"`
	CWEId       string `json:"cweId"`
	Type        string `json:"type"` // CWE / text
}

// CVE5Metric 是一个数组，每项可能包含 cvssV2_0 / cvssV3_0 / cvssV3_1 / cvssV4_0
// 也可能包含 other (非标准评分)
type CVE5Metric struct {
	Format  string          `json:"format"`
	CvssV2  *CVE5CvssV2     `json:"cvssV2_0,omitempty"`
	CvssV30 *CVE5CvssV30    `json:"cvssV3_0,omitempty"`
	CvssV31 *CVE5CvssV31    `json:"cvssV3_1,omitempty"`
	CvssV40 *CVE5CvssV40    `json:"cvssV4_0,omitempty"`
	Other   json.RawMessage `json:"other,omitempty"`
}

type CVE5CvssV2 struct {
	Version               string  `json:"version"`
	VectorString          string  `json:"vectorString"`
	BaseScore             float64 `json:"baseScore"`
	AccessVector          string  `json:"accessVector"`
	AccessComplexity      string  `json:"accessComplexity"`
	Authentication        string  `json:"authentication"`
	ConfidentialityImpact string  `json:"confidentialityImpact"`
	IntegrityImpact       string  `json:"integrityImpact"`
	AvailabilityImpact    string  `json:"availabilityImpact"`
	BaseSeverity          string  `json:"baseSeverity"`
}

type CVE5CvssV30 struct {
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

type CVE5CvssV31 struct {
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

type CVE5CvssV40 struct {
	Version                   string  `json:"version"`
	VectorString              string  `json:"vectorString"`
	BaseScore                 float64 `json:"baseScore"`
	BaseSeverity              string  `json:"baseSeverity"`
	AttackVector              string  `json:"attackVector"`
	AttackComplexity          string  `json:"attackComplexity"`
	AttackRequirements        string  `json:"attackRequirements"`
	PrivilegesRequired        string  `json:"privilegesRequired"`
	UserInteraction           string  `json:"userInteraction"`
	VulnConfidentialityImpact string  `json:"vulnConfidentialityImpact"`
	VulnIntegrityImpact       string  `json:"vulnIntegrityImpact"`
	VulnAvailabilityImpact    string  `json:"vulnAvailabilityImpact"`
	SubConfidentialityImpact  string  `json:"subConfidentialityImpact"`
	SubIntegrityImpact        string  `json:"subIntegrityImpact"`
	SubAvailabilityImpact     string  `json:"subAvailabilityImpact"`
}

type CVE5Reference struct {
	URL   string   `json:"url"`
	Name  string   `json:"name"`
	Tags  []string `json:"tags"`
}

type CVE5Credit struct {
	Lang  string `json:"lang"`
	Value string `json:"value"`
	Type  string `json:"type"`
}

// CVEId 返回 CVE 编号
func (r *CVERecordV5) CVEId() string {
	return r.CVEMetadata.CVEID
}

// CWE 提取 CWE 信息
func (r *CVERecordV5) CWE() string {
	if r.Containers.CNA == nil {
		return ""
	}
	var cwe []string
	for _, pt := range r.Containers.CNA.ProblemTypes {
		for _, desc := range pt.Descriptions {
			if desc.CWEId != "" && strings.HasPrefix(desc.CWEId, "CWE-") {
				cwe = append(cwe, desc.CWEId)
			}
		}
	}
	return strings.Join(cwe, " | ")
}

// DescriptionMain 提取主要描述（优先英文，否则取最长的）
func (r *CVERecordV5) DescriptionMain() string {
	if r.Containers.CNA == nil {
		return ""
	}
	data := r.Containers.CNA.Descriptions
	if len(data) <= 0 {
		return ""
	} else if len(data) == 1 {
		return data[0].Value
	} else {
		// 优先英文
		for _, d := range data {
			if strings.HasPrefix(d.Lang, "en") {
				return d.Value
			}
		}
		// 否则取最长的
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

// GetPublishedDate 解析发布日期
func (r *CVERecordV5) GetPublishedDate() time.Time {
	t, err := time.Parse(time.RFC3339, r.CVEMetadata.DatePublished)
	if err != nil {
		// 尝试无时区后缀的格式
		t, err = time.Parse("2006-01-02T15:04:05.000", r.CVEMetadata.DatePublished)
		if err != nil {
			return time.Time{}
		}
	}
	return t
}

// GetLastModifiedDate 解析最后修改日期
func (r *CVERecordV5) GetLastModifiedDate() time.Time {
	t, err := time.Parse(time.RFC3339, r.CVEMetadata.DateUpdated)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05.000", r.CVEMetadata.DateUpdated)
		if err != nil {
			return time.Time{}
		}
	}
	return t
}

// extractTitle 提取英文标题
func (r *CVERecordV5) extractTitle() string {
	if r.Containers.CNA != nil && r.Containers.CNA.Title != "" {
		return r.Containers.CNA.Title
	}
	return ""
}

// extractSolution 提取解决方案（优先英文）
func (r *CVERecordV5) extractSolution() string {
	if r.Containers.CNA == nil || len(r.Containers.CNA.Solutions) == 0 {
		return ""
	}
	for _, s := range r.Containers.CNA.Solutions {
		if strings.HasPrefix(s.Lang, "en") {
			return s.Value
		}
	}
	return r.Containers.CNA.Solutions[0].Value
}

// extractVendorProduct 从 affected[] 提取厂商和产品
func (r *CVERecordV5) extractVendorProduct() ([]string, []string) {
	if r.Containers.CNA == nil {
		return nil, nil
	}
	var vendors, products []string
	for _, aff := range r.Containers.CNA.Affected {
		if aff.Vendor != "" && aff.Vendor != "*" {
			vendors = append(vendors, aff.Vendor)
		}
		if aff.Product != "" && aff.Product != "*" {
			products = append(products, aff.Product)
		}
	}
	return Set(vendors), Set(products)
}

// extractCVSS 从 metrics[] 提取 CVSS 评分（优先 v4 > v3.1 > v3.0 > v2）
func (r *CVERecordV5) extractCVSS() (version, vectorString string, cvssData *cvssExtracted) {
	if r.Containers.CNA == nil {
		return "", "", nil
	}
	for _, m := range r.Containers.CNA.Metrics {
		if m.CvssV40 != nil {
			return m.CvssV40.Version, m.CvssV40.VectorString, &cvssExtracted{
				attackVector:           m.CvssV40.AttackVector,
				attackComplexity:       m.CvssV40.AttackComplexity,
				privilegesRequired:     m.CvssV40.PrivilegesRequired,
				userInteraction:        m.CvssV40.UserInteraction,
				confidentialityImpact:  m.CvssV40.VulnConfidentialityImpact,
				integrityImpact:        m.CvssV40.VulnIntegrityImpact,
				availabilityImpact:     m.CvssV40.VulnAvailabilityImpact,
				baseScore:              m.CvssV40.BaseScore,
				baseSeverity:           m.CvssV40.BaseSeverity,
			}
		}
	}
	for _, m := range r.Containers.CNA.Metrics {
		if m.CvssV31 != nil {
			return m.CvssV31.Version, m.CvssV31.VectorString, &cvssExtracted{
				attackVector:           m.CvssV31.AttackVector,
				attackComplexity:       m.CvssV31.AttackComplexity,
				privilegesRequired:     m.CvssV31.PrivilegesRequired,
				userInteraction:        m.CvssV31.UserInteraction,
				confidentialityImpact:  m.CvssV31.ConfidentialityImpact,
				integrityImpact:        m.CvssV31.IntegrityImpact,
				availabilityImpact:     m.CvssV31.AvailabilityImpact,
				baseScore:              m.CvssV31.BaseScore,
				baseSeverity:           m.CvssV31.BaseSeverity,
			}
		}
	}
	for _, m := range r.Containers.CNA.Metrics {
		if m.CvssV30 != nil {
			return m.CvssV30.Version, m.CvssV30.VectorString, &cvssExtracted{
				attackVector:           m.CvssV30.AttackVector,
				attackComplexity:       m.CvssV30.AttackComplexity,
				privilegesRequired:     m.CvssV30.PrivilegesRequired,
				userInteraction:        m.CvssV30.UserInteraction,
				confidentialityImpact:  m.CvssV30.ConfidentialityImpact,
				integrityImpact:        m.CvssV30.IntegrityImpact,
				availabilityImpact:     m.CvssV30.AvailabilityImpact,
				baseScore:              m.CvssV30.BaseScore,
				baseSeverity:           m.CvssV30.BaseSeverity,
			}
		}
	}
	for _, m := range r.Containers.CNA.Metrics {
		if m.CvssV2 != nil {
			return m.CvssV2.Version, m.CvssV2.VectorString, &cvssExtracted{
				attackVector:           m.CvssV2.AccessVector,
				attackComplexity:       m.CvssV2.AccessComplexity,
				privilegesRequired:     m.CvssV2.Authentication,
				userInteraction:        "",
				confidentialityImpact:  m.CvssV2.ConfidentialityImpact,
				integrityImpact:        m.CvssV2.IntegrityImpact,
				availabilityImpact:     m.CvssV2.AvailabilityImpact,
				baseScore:              m.CvssV2.BaseScore,
				baseSeverity:           m.CvssV2.BaseSeverity,
			}
		}
	}
	return "", "", nil
}

// cvssExtracted 是从 CVSS 数据中提取的统一字段
type cvssExtracted struct {
	attackVector          string
	attackComplexity      string
	privilegesRequired    string
	userInteraction       string
	confidentialityImpact string
	integrityImpact       string
	availabilityImpact    string
	baseScore             float64
	baseSeverity          string
}

// ToCVE 将 CVE 5.0 Record 转换为数据库 CVE 记录
func (r *CVERecordV5) ToCVE(db *gorm.DB) (*CVE, error) {
	// REJECTED 状态跳过
	if r.CVEMetadata.State == "REJECTED" {
		return nil, fmt.Errorf("REJECTED")
	}

	cna := r.Containers.CNA
	if cna == nil {
		return nil, fmt.Errorf("no CNA container")
	}

	// 检查描述中是否有 REJECT 标记
	if strings.HasPrefix(r.DescriptionMain(), "** REJECT **") {
		return nil, fmt.Errorf("REJECT")
	}

	// 提取厂商和产品
	vendors, products := r.extractVendorProduct()

	// 提取 CVSS
	cvssVersion, vectorString, cvss := r.extractCVSS()

	// 提取 title 和 solution
	title := r.extractTitle()
	solution := r.extractSolution()

	// 中文翻译仍然来自 descGetter（AI 翻译）
	var titleZh string
	var descZh string
	var aiSolution string
	if descGetter != nil && len(descGetter) > 0 {
		i, ok := descGetter[strings.TrimSpace(r.CVEId())]
		if ok {
			titleZh = i.TitleZh
			aiSolution = i.Solution
			descZh = i.DescriptionMainZh
		}
	}

	// solution 优先使用 5.0 原生数据，fallback 到 AI 翻译
	finalSolution := solution
	if finalSolution == "" {
		finalSolution = aiSolution
	}

	c := &CVE{
		Model:             gorm.Model{},
		CVE:               r.CVEId(),
		CWE:               r.CWE(),
		Title:             title,       // 5.0 原始英文标题
		TitleZh:           titleZh,     // AI 中文标题
		Solution:          finalSolution,
		DescriptionMain:   r.DescriptionMain(),
		DescriptionMainZh: descZh,
		ProblemType:       MarshalCheck(cna.ProblemTypes),
		References:        MarshalCheck(cna.References),
		Descriptions:      MarshalCheck(cna.Descriptions),
		Vendor:            strings.Join(vendors, ","),
		Product:           strings.Join(products, ","),
		PublishedDate:     r.GetPublishedDate(),
		LastModifiedData:  r.GetLastModifiedDate(),
	}

	// 填充 CVSS 字段
	if cvss != nil {
		c.CVSSVersion = cvssVersion
		c.CVSSVectorString = vectorString
		c.AccessVector = cvss.attackVector
		c.AccessComplexity = cvss.attackComplexity
		c.Authentication = cvss.privilegesRequired
		c.ConfidentialityImpact = cvss.confidentialityImpact
		c.IntegrityImpact = cvss.integrityImpact
		c.AvailabilityImpact = cvss.availabilityImpact
		c.BaseCVSSv2Score = cvss.baseScore
		c.Severity = cvss.baseSeverity
		// userInteraction 在 v2 中没有
		c.UserInteractionRequired = cvss.userInteraction == "REQUIRED"
	}

	return c, nil
}

// SaveCVERecordV5 将 5.0 Record 保存到数据库
func (m *SqliteManager) SaveCVERecordV5(record *CVERecordV5) {
	c, err := record.ToCVE(m.DB)
	if err != nil {
		return
	}
	if c != nil {
		if db := m.DB.Save(c); db.Error != nil {
			fmt.Printf("save cve %s failed: %s", c.CVE, db.Error)
		}
	}
}

// FetchCVEV5FromAPI 通过 cveawg API 实时查询单个 CVE 的 5.0 Record
func FetchCVEV5FromAPI(cveID string) (*CVERecordV5, error) {
	// 这个函数在 grpc_cve.go 中使用，为了避免循环依赖
	// 实际的 HTTP 调用放在调用方
	// 此处仅作为接口声明
	return nil, fmt.Errorf("not implemented in cveresources, use FetchCVEV5Record instead")
}

// MergeCVEV5Fields 只合并 5.0 独有的字段到已有 CVE 记录
// 不覆盖 NVD 2.0 已有的 CPE/CVSS 数据，只补充缺失的 title/vendor/product/solution
func MergeCVEV5Fields(db *gorm.DB, v5 *CVE) error {
	var existing CVE
	if err := db.Where("cve = ?", v5.CVE).First(&existing).Error; err != nil {
		// 记录不存在，直接创建
		return db.Save(v5).Error
	}

	updates := map[string]interface{}{}
	if existing.Title == "" && v5.Title != "" {
		updates["title"] = v5.Title
	}
	if existing.Solution == "" && v5.Solution != "" {
		updates["solution"] = v5.Solution
	}
	if existing.Vendor == "" && v5.Vendor != "" {
		updates["vendor"] = v5.Vendor
	}
	if existing.Product == "" && v5.Product != "" {
		updates["product"] = v5.Product
	}
	if existing.DescriptionMain == "" && v5.DescriptionMain != "" {
		updates["description_main"] = v5.DescriptionMain
	}
	if existing.CWE == "" && v5.CWE != "" {
		updates["cwe"] = v5.CWE
	}
	if existing.CVSSVersion == "" && v5.CVSSVersion != "" {
		updates["cvss_version"] = v5.CVSSVersion
		updates["cvss_vector_string"] = v5.CVSSVectorString
		updates["access_vector"] = v5.AccessVector
		updates["access_complexity"] = v5.AccessComplexity
		updates["authentication"] = v5.Authentication
		updates["confidentiality_impact"] = v5.ConfidentialityImpact
		updates["integrity_impact"] = v5.IntegrityImpact
		updates["availability_impact"] = v5.AvailabilityImpact
		updates["base_cvs_sv2_score"] = v5.BaseCVSSv2Score
		updates["severity"] = v5.Severity
	}
	if existing.References == nil && v5.References != nil {
		updates["references"] = v5.References
	}
	if existing.PublishedDate.IsZero() && !v5.PublishedDate.IsZero() {
		updates["published_date"] = v5.PublishedDate
	}
	if existing.LastModifiedData.IsZero() && !v5.LastModifiedData.IsZero() {
		updates["last_modified_data"] = v5.LastModifiedData
	}

	if len(updates) == 0 {
		return nil // 没有需要更新的字段
	}

	return db.Model(&CVE{}).Where("cve = ?", v5.CVE).Updates(updates).Error
}

// ExtractVendorProductExported 导出版本，供外部包调用
func (r *CVERecordV5) ExtractVendorProductExported() ([]string, []string) {
	return r.extractVendorProduct()
}
