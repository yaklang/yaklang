package cveresources

import (
	"context"
	"encoding/json"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strconv"
	"strings"
	"time"
)

type CVE struct {
	gorm.Model

	CVE               string `gorm:"uniqueIndex"`
	CWE               string
	ProblemType       []byte
	References        []byte
	TitleZh           string
	Solution          string
	DescriptionMain   string
	DescriptionMainZh string
	Descriptions      []byte
	Vendor            string
	Product           string

	CPEConfigurations []byte

	CVSSVersion      string
	CVSSVectorString string

	// 攻击路径
	AccessVector string
	// 攻击复杂度
	AccessComplexity string
	// 需要认证
	Authentication string
	// 机密性影响（泄密）
	ConfidentialityImpact string
	// 完整性影响（破坏程度）
	IntegrityImpact string
	// 可用性影响（导致服务不可用）
	AvailabilityImpact string
	// 基础评分
	BaseCVSSv2Score float64

	// 严重等级
	Severity string
	// 漏洞利用评分
	ExploitabilityScore float64
	// 漏洞影响评分
	ImpactScore float64

	// 可获取所有权限
	ObtainAllPrivilege bool
	// 可获取用户权限
	ObtainUserPrivilege bool
	// 可获取其他权限
	ObtainOtherPrivilege bool

	// 是否需要用户交互
	UserInteractionRequired bool

	PublishedDate    time.Time
	LastModifiedData time.Time
}

func (c *CVE) ToGPRCModel() *ypb.CVEDetail {
	return &ypb.CVEDetail{
		CVE:                     c.CVE,
		DescriptionZh:           utils.EscapeInvalidUTF8Byte([]byte(c.DescriptionMainZh)),
		DescriptionOrigin:       utils.EscapeInvalidUTF8Byte([]byte(c.DescriptionMain)),
		Title:                   utils.EscapeInvalidUTF8Byte([]byte(c.TitleZh)),
		Solution:                utils.EscapeInvalidUTF8Byte([]byte(c.Solution)),
		AccessVector:            utils.EscapeInvalidUTF8Byte([]byte(c.AccessVector)),
		References:              utils.EscapeInvalidUTF8Byte(c.References),
		AccessComplexity:        AccessComplexityVerbose(utils.EscapeInvalidUTF8Byte([]byte(c.AccessComplexity))),
		Authentication:          utils.EscapeInvalidUTF8Byte([]byte(c.Authentication)),
		ConfidentialityImpact:   utils.EscapeInvalidUTF8Byte([]byte(c.ConfidentialityImpact)),
		IntegrityImpact:         utils.EscapeInvalidUTF8Byte([]byte(c.IntegrityImpact)),
		AvailabilityImpact:      utils.EscapeInvalidUTF8Byte([]byte(c.AvailabilityImpact)),
		Severity:                SeverityVerbose(utils.EscapeInvalidUTF8Byte([]byte(c.Severity))),
		PublishedAt:             c.PublishedDate.Unix(),
		CWE:                     c.CWE,
		CVSSVersion:             c.CVSSVersion,
		CVSSVectorString:        c.CVSSVectorString,
		BaseCVSSv2Score:         c.BaseCVSSv2Score,
		ExploitabilityScore:     c.ExploitabilityScore,
		ObtainAllPrivileged:     c.ObtainAllPrivilege,
		ObtainUserPrivileged:    c.ObtainUserPrivilege,
		ObtainOtherPrivileged:   c.ObtainOtherPrivilege,
		UserInteractionRequired: c.UserInteractionRequired,
		Product:                 c.Product,
		UpdatedAt:               c.UpdatedAt.Unix(),
		LastModifiedData:        c.LastModifiedData.Unix(),
	}
}

func AccessComplexityVerbose(i string) string {
	i = strings.ToLower(i)
	switch i {
	case "low":
		return "容易"
	case "medium":
		return "一般"
	case "high":
		return "困难"
	default:
		return "-"
	}
}

func SeverityVerbose(i string) string {
	i = strings.ToLower(i)
	switch i {
	case "trace", "debug", "note":
		return "调试信息"
	case "info", "fingerprint", "infof", "default":
		return "信息/指纹"
	case "low":
		return "低危"
	case "middle", "warn", "warning", "medium":
		return "中危"
	case "high":
		return "高危"
	case "fatal", "critical", "panic":
		return "严重"
	default:
		return "-"
	}
}

func (c *CVE) Year() int {
	if !c.PublishedDate.IsZero() {
		return c.PublishedDate.Year()
	}
	results := strings.Split(c.CVE, "-")
	if len(results) > 1 {
		i, _ := strconv.Atoi(results[1])
		return i
	}
	return 0
}

type CVEYearFile struct {
	CVEDataType         string      `json:"CVE_data_type"`
	CVEDataFormat       string      `json:"CVE_data_format"`
	CVEDataVersion      string      `json:"CVE_data_version"`
	CVEDataNumberOfCVEs string      `json:"CVE_data_numberOfCVEs"`
	CVEDataTimestamp    string      `json:"CVE_data_timestamp"`
	CVERecords          []CVERecord `json:"CVE_Items"`
}

type CVERecord struct {
	Cve              Cve            `json:"cve"`
	Configurations   Configurations `json:"configurations"`
	Impact           Impact         `json:"impact"`
	PublishedDate    string         `json:"publishedDate"`
	LastModifiedDate string         `json:"lastModifiedDate"`
}

type Cve struct {
	DataType        string          `json:"data_type"`
	DataFormat      string          `json:"data_format"`
	DataVersion     string          `json:"data_version"`
	CVEDataMeta     CVEDataMeta     `json:"CVE_data_meta"`
	Problemtype     Problemtype     `json:"problemtype"`
	References      References      `json:"references"`
	DescriptionInfo DescriptionInfo `json:"description"`
}

type CVEDataMeta struct {
	ID       string `json:"ID"`
	ASSIGNER string `json:"ASSIGNER"`
}

type Problemtype struct {
	ProblemtypeData []ProblemtypeData `json:"problemtype_data"`
}

type ProblemtypeData struct {
	Description []Description `json:"description"`
}

type Description struct {
	Lang  string `json:"lang"`
	Value string `json:"value"`
}

type References struct {
	ReferenceData []ReferenceData `json:"reference_data"`
}

type ReferenceData struct {
	URL       string        `json:"url"`
	Name      string        `json:"name"`
	Refsource string        `json:"refsource"`
	Tags      []interface{} `json:"tags"`
}

type Configurations struct {
	CVEDataVersion string  `json:"CVE_data_version"`
	Nodes          []Nodes `json:"nodes"`
}

type DescriptionInfo struct {
	DescriptionData []DescriptionData `json:"description_data"`
}

type DescriptionData struct {
	Lang  string `json:"lang"`
	Value string `json:"value"`
}

type Nodes struct {
	Operator string     `json:"operator"`
	CpeMatch []CpeMatch `json:"cpe_match"`
	Children []Nodes    `json:"children"`
}

type CpeMatch struct {
	Vulnerable            bool   `json:"vulnerable"`
	Cpe23URI              string `json:"cpe23Uri"`
	VersionStartExcluding string `json:"versionStartExcluding"`
	VersionEndExcluding   string `json:"versionEndExcluding"`
	VersionStartIncluding string `json:"versionStartIncluding"`
	VersionEndIncluding   string `json:"versionEndIncluding"`
}

type Impact struct {
	BaseMetricV2 BaseMetricV2 `json:"baseMetricV2"`
	BaseMetricV3 BaseMetricV3 `json:"baseMetricV3"`
}

type CvssV2 struct {
	Version               string  `json:"version"`
	VectorString          string  `json:"vectorString"`
	AccessVector          string  `json:"accessVector"`
	AccessComplexity      string  `json:"accessComplexity"`
	Authentication        string  `json:"authentication"`
	ConfidentialityImpact string  `json:"confidentialityImpact"`
	IntegrityImpact       string  `json:"integrityImpact"`
	AvailabilityImpact    string  `json:"availabilityImpact"`
	BaseScore             float64 `json:"baseScore"`
}

type BaseMetricV2 struct {
	CvssV2                  CvssV2  `json:"cvssV2"`
	Severity                string  `json:"severity"`
	ExploitabilityScore     float64 `json:"exploitabilityScore"`
	ImpactScore             float64 `json:"impactScore"`
	ObtainAllPrivilege      bool    `json:"obtainAllPrivilege"`
	ObtainUserPrivilege     bool    `json:"obtainUserPrivilege"`
	ObtainOtherPrivilege    bool    `json:"obtainOtherPrivilege"`
	UserInteractionRequired bool    `json:"userInteractionRequired"`
}

type CvssV3 struct {
	Version               string  `json:"version"`
	VectorString          string  `json:"vectorString"`
	AttackVector          string  `json:"attackVector"`
	AttackComplexity      string  `json:"attackComplexity"`
	PrivilegesRequired    string  `json:"privilegesRequired"`
	UserInteraction       string  `json:"userInteraction"`
	Scope                 string  `json:"scope"`
	ConfidentialityImpact string  `json:"confidentialityImpact"`
	IntegrityImpact       string  `json:"integrityImpact"`
	AvailabilityImpact    string  `json:"availabilityImpact"`
	BaseScore             float64 `json:"baseScore"`
	BaseSeverity          string  `json:"baseSeverity"`
}

type BaseMetricV3 struct {
	CvssV3                  CvssV3  `json:"cvssV3"`
	ExploitabilityScore     float64 `json:"exploitabilityScore"`
	ImpactScore             float64 `json:"impactScore"`
	ObtainAllPrivilege      bool    `json:"obtainAllPrivilege"`
	ObtainUserPrivilege     bool    `json:"obtainUserPrivilege"`
	ObtainOtherPrivilege    bool    `json:"obtainOtherPrivilege"`
	UserInteractionRequired bool    `json:"userInteractionRequired"`
}

type ProductsTable struct {
	Product string `gorm:"primary_key"`
	Vendor  string
}

func (r *CVERecord) CVEId() string {
	return r.Cve.CVEDataMeta.ID
}

func (r *CVERecord) CWE() string {
	var cwe []string
	for _, data := range r.Cve.Problemtype.ProblemtypeData {
		for _, d := range data.Description {
			if strings.HasPrefix(d.Value, "CWE-") {
				cwe = append(cwe, d.Value)
			}
		}
	}
	return strings.Join(cwe, " | ")
}

func (r *CVERecord) DescriptionMain() string {
	data := r.Cve.DescriptionInfo.DescriptionData
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

func (r *CVERecord) GetPublishedDate() time.Time {
	t, err := time.Parse("2006-01-02T15:04Z", r.PublishedDate)
	if err != nil {
		log.Error(err)
	}
	return t
}

func (r *CVERecord) GetLastModifiedDate() time.Time {
	t, err := time.Parse("2006-01-02T15:04Z", r.LastModifiedDate)
	if err != nil {
		log.Error(err)
	}
	return t
}

func MarshalCheck(v any) []byte {
	jsonRes, err := json.Marshal(v)
	if err != nil {
		log.Error(err)
	}
	return jsonRes
}

func CreateOrUpdateCVE(db *gorm.DB, id string, cve *CVE) error {
	if db := db.Model(&CVE{}).Where("cve = ?", id).Assign(cve).FirstOrCreate(&CVE{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func YieldCVEs(db *gorm.DB, ctx context.Context) chan *CVE {
	outC := make(chan *CVE)
	go func() {
		defer close(outC)

		var page = 1
		for {
			var items []*CVE
			if _, b := bizhelper.NewPagination(&bizhelper.Param{
				DB:    db,
				Page:  page,
				Limit: 1000,
			}, &items); b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
			}

			page++

			for _, d := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- d:
				}
			}

			if len(items) < 1000 {
				return
			}
		}
	}()
	return outC
}

func GetCVE(db *gorm.DB, id string) (*CVE, error) {
	if db == nil {
		return nil, utils.Errorf("db is nil")
	}

	var req CVE
	if db := db.Model(&CVE{}).Where("cve = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get CVE failed: %s", db.Error)
	}

	return &req, nil
}

func FilterCVE(db *gorm.DB, req *ypb.QueryCVERequest) *gorm.DB {
	db = bizhelper.ExactQueryStringArrayOr(db, "access_vector", utils.PrettifyListFromStringSplited(req.GetAccessVector(), ","))
	db = bizhelper.ExactQueryStringArrayOr(db, "severity", utils.PrettifyListFromStringSplited(req.GetSeverity(), ","))
	db = bizhelper.ExactQueryStringArrayOr(db, "access_complexity", utils.PrettifyListFromStringSplited(req.GetAccessComplexity(), ","))
	db = bizhelper.FuzzQueryLike(db, "cwe", req.GetCWE())
	db = bizhelper.FuzzQueryLike(db, "product", req.GetProduct())
	if req.GetYear() != "" {
		db = bizhelper.FuzzQueryLike(db, "cve", req.GetYear())
	}
	if req.GetAfterYear() != "" {
		var i, _ = strconv.Atoi(req.GetAfterYear())
		if i > 0 {
			db = bizhelper.QueryDateTimeAfterTimestampOr(db, "published_date", time.Date(i, 1, 1, 0, 0, 0, 0, time.UTC).Unix())
		}
	}
	db = bizhelper.FuzzSearchEx(db, []string{
		"cve", "product", "title_zh",
	}, req.Keywords, false)
	db = bizhelper.QueryLargerThanFloatOr_AboveZero(db, "base_cvs_sv2_score", req.GetScore())
	return db
}

func QueryCVE(db *gorm.DB, req *ypb.QueryCVERequest) (*bizhelper.Paginator, []*CVE, error) {
	db = db.Model(&CVE{})

	params := req.GetPagination()
	if params.OrderBy == "" {
		params.OrderBy = "published_date"
	}
	if params.Order == "" {
		params.Order = "desc"
	}
	//db = bizhelper.QueryOrder(db, "published_date", "desc")
	db = bizhelper.QueryOrder(db, params.OrderBy, params.Order)

	if req.GetChineseTranslationFirst() {
		db = db.Where(`(cves.title_zh is not '' AND cves.description_main_zh is not '')`)
	}

	db = FilterCVE(db, req)

	var ret []*CVE
	paging, db := bizhelper.Paging(db, int(params.GetPage()), int(params.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}
