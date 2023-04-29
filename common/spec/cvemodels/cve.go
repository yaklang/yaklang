package cvemodels

import (
	"encoding/json"
	"github.com/jinzhu/gorm/dialects/postgres"
	"github.com/pkg/errors"
	"strings"
	"time"
)

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

func (c *CVERecord) CVEId() string {
	return c.Cve.CVEDataMeta.ID
}

func (c *CVERecord) CWE() string {
	var cwe []string
	for _, data := range c.Cve.Problemtype.ProblemtypeData {
		for _, d := range data.Description {
			if strings.HasPrefix(d.Value, "CWE-") {
				cwe = append(cwe, d.Value)
			}
		}
	}
	return strings.Join(cwe, " | ")
}

func (c *CVERecord) ProblemTypeToJSONB() (postgres.Jsonb, error) {
	data, err := json.Marshal(c.Cve.Problemtype)
	if err != nil {
		return postgres.Jsonb{}, errors.Errorf("marshal failed: %s", err)
	}

	return postgres.Jsonb{data}, nil
}

func (c *CVERecord) ReferencesToJSONB() (postgres.Jsonb, error) {
	data, err := json.Marshal(c.Cve.References)
	if err != nil {
		return postgres.Jsonb{}, errors.Errorf("marshal failed: %s", err)
	}

	return postgres.Jsonb{data}, nil
}

func (c *CVERecord) DescriptionsToJSONB() (postgres.Jsonb, error) {
	data, err := json.Marshal(c.Cve.DescriptionInfo)
	if err != nil {
		return postgres.Jsonb{}, errors.Errorf("marshal failed: %s", err)
	}

	return postgres.Jsonb{data}, nil
}

func (c *CVERecord) CPEConfigurationsToJSONB() (postgres.Jsonb, error) {
	data, err := json.Marshal(c.Configurations)
	if err != nil {
		return postgres.Jsonb{}, errors.Errorf("marshal failed: %s", err)
	}

	return postgres.Jsonb{data}, nil
}

func (c *CVERecord) DescriptionMain() string {
	datas := c.Cve.DescriptionInfo.DescriptionData
	if len(datas) <= 0 {
		return ""
	} else if len(datas) == 1 {
		return datas[0].Value
	} else {
		var (
			currentLength int
			currentData   string
		)
		for _, data := range datas {
			if len(data.Value) > currentLength {
				currentLength = len(data.Value)
				currentData = data.Value
			}
		}
		return currentData
	}
}

func (c *CVERecord) GetPublishedDate() time.Time {
	t, err := time.Parse("2006-01-02T15:04Z", c.PublishedDate)
	if err != nil {
		return time.Time{}
	}
	return t
}

func (c *CVERecord) GetLastModifiedDate() time.Time {
	t, err := time.Parse("2006-01-02T15:04Z", c.LastModifiedDate)
	if err != nil {
		return time.Time{}
	}
	return t
}

type CVEDataMeta struct {
	ID       string `json:"ID"`
	ASSIGNER string `json:"ASSIGNER"`
}

type Description struct {
	Lang  string `json:"lang"`
	Value string `json:"value"`
}

type ProblemtypeData struct {
	Description []Description `json:"description"`
}

type Problemtype struct {
	ProblemtypeData []ProblemtypeData `json:"problemtype_data"`
}

type ReferenceData struct {
	URL       string        `json:"url"`
	Name      string        `json:"name"`
	Refsource string        `json:"refsource"`
	Tags      []interface{} `json:"tags"`
}

type References struct {
	ReferenceData []ReferenceData `json:"reference_data"`
}

type DescriptionData struct {
	Lang  string `json:"lang"`
	Value string `json:"value"`
}

type DescriptionInfo struct {
	DescriptionData []DescriptionData `json:"description_data"`
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

type CpeMatch struct {
	Vulnerable bool   `json:"vulnerable"`
	Cpe23URI   string `json:"cpe23Uri"`
}

type Nodes struct {
	Operator string     `json:"operator"`
	CpeMatch []CpeMatch `json:"cpe_match"`
	Children []Nodes    `json:"children"`
}

type Configurations struct {
	CVEDataVersion string  `json:"CVE_data_version"`
	Nodes          []Nodes `json:"nodes"`
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

type Impact struct {
	BaseMetricV2 BaseMetricV2 `json:"baseMetricV2"`
}
