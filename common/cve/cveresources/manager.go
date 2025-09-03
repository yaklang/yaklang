package cveresources

import (
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/ysmood/leakless/pkg/utils"
)

type SqliteManager struct {
	*gorm.DB
}

func GetManager(path string, forceCreates ...bool) *SqliteManager {
	var (
		db  *gorm.DB
		err error
	)
	forceCreate := false
	if len(forceCreates) > 0 {
		forceCreate = forceCreates[0]
	}

	if utils.FileExists(path) && !forceCreate {
		db, err = gorm.Open(consts.SQLite, path)
	} else {
		db, err = consts.CreateCVEDatabase(path)
	}
	if err != nil {
		panic("failed to connect database")
	}
	return &SqliteManager{db}
}

type CVEDesc struct {
	TitleZh           string
	Solution          string
	DescriptionMainZh string
}

var descGetter = make(map[string]CVEDesc)

func RegisterDesc(d map[string]CVEDesc) {
	descGetter = d
}

func (record *CVERecord) ToCVE(db *gorm.DB) (*CVE, error) {
	c := &CVE{}
	if strings.HasPrefix(record.DescriptionMain(), "** REJECT **") {
		return nil, fmt.Errorf("REJECT")
	}

	var Vendors []string
	var Products []string

	for _, node := range record.Configurations.Nodes {
		node.insertProducts(db)
	}
	for _, node := range record.Configurations.Nodes {
		Vendors = append(Vendors, node.GetVendor()...)
		Products = append(Products, node.GetProduct()...)
	}
	Vendors = Set(Vendors)
	Products = Set(Products)

	var titleZh string
	var descZh string
	var solution string
	if descGetter != nil && len(descGetter) > 0 {
		i, ok := descGetter[strings.TrimSpace(record.CVEId())]
		if ok {
			titleZh = i.TitleZh
			solution = i.Solution
			descZh = i.DescriptionMainZh
		}
	}

	if record.Impact.BaseMetricV2.CvssV2.Version == "" {
		baseMetric := record.Impact.BaseMetricV3
		cvss := baseMetric.CvssV3

		c = &CVE{
			Model:                   gorm.Model{},
			CVE:                     record.CVEId(),
			CWE:                     record.CWE(),
			TitleZh:                 titleZh,
			DescriptionMainZh:       descZh,
			Solution:                solution,
			ProblemType:             MarshalCheck(record.Cve.Problemtype),
			References:              MarshalCheck(record.Cve.References),
			DescriptionMain:         record.DescriptionMain(),
			Descriptions:            MarshalCheck(record.Cve.DescriptionInfo),
			CPEConfigurations:       MarshalCheck(record.Configurations),
			CVSSVersion:             cvss.Version,
			CVSSVectorString:        cvss.VectorString,
			AccessVector:            cvss.AttackVector,
			AccessComplexity:        cvss.AttackComplexity,
			Authentication:          "",
			ConfidentialityImpact:   cvss.ConfidentialityImpact,
			IntegrityImpact:         cvss.IntegrityImpact,
			AvailabilityImpact:      cvss.AvailabilityImpact,
			BaseCVSSv2Score:         cvss.BaseScore,
			Severity:                cvss.BaseSeverity,
			ExploitabilityScore:     baseMetric.ExploitabilityScore,
			ImpactScore:             baseMetric.ImpactScore,
			ObtainAllPrivilege:      baseMetric.ObtainAllPrivilege,
			ObtainUserPrivilege:     baseMetric.ObtainUserPrivilege,
			ObtainOtherPrivilege:    baseMetric.ObtainOtherPrivilege,
			UserInteractionRequired: baseMetric.UserInteractionRequired,
			PublishedDate:           record.GetPublishedDate(),
			LastModifiedData:        record.GetLastModifiedDate(),
			Vendor:                  strings.Join(Vendors, ","),
			Product:                 strings.Join(Products, ","),
		}
	} else {
		baseMetric := record.Impact.BaseMetricV2
		cvss := baseMetric.CvssV2

		c = &CVE{
			Model:                   gorm.Model{},
			CVE:                     record.CVEId(),
			CWE:                     record.CWE(),
			TitleZh:                 titleZh,
			DescriptionMainZh:       descZh,
			Solution:                solution,
			ProblemType:             MarshalCheck(record.Cve.Problemtype),
			References:              MarshalCheck(record.Cve.References),
			DescriptionMain:         record.DescriptionMain(),
			Descriptions:            MarshalCheck(record.Cve.DescriptionInfo),
			CPEConfigurations:       MarshalCheck(record.Configurations),
			CVSSVersion:             cvss.Version,
			CVSSVectorString:        cvss.VectorString,
			AccessVector:            cvss.AccessVector,
			AccessComplexity:        cvss.AccessComplexity,
			Authentication:          cvss.Authentication,
			ConfidentialityImpact:   cvss.ConfidentialityImpact,
			IntegrityImpact:         cvss.IntegrityImpact,
			AvailabilityImpact:      cvss.AvailabilityImpact,
			BaseCVSSv2Score:         cvss.BaseScore,
			Severity:                baseMetric.Severity,
			ExploitabilityScore:     baseMetric.ExploitabilityScore,
			ImpactScore:             baseMetric.ImpactScore,
			ObtainAllPrivilege:      baseMetric.ObtainAllPrivilege,
			ObtainUserPrivilege:     baseMetric.ObtainUserPrivilege,
			ObtainOtherPrivilege:    baseMetric.ObtainOtherPrivilege,
			UserInteractionRequired: baseMetric.UserInteractionRequired,
			PublishedDate:           record.GetPublishedDate(),
			LastModifiedData:        record.GetLastModifiedDate(),
			Vendor:                  strings.Join(Vendors, ","),
			Product:                 strings.Join(Products, ","),
		}
	}
	return c, nil
}

func (m *SqliteManager) SaveCVERecord(record *CVERecord) {
	c, err := record.ToCVE(m.DB)
	if err != nil {
		log.Error(err)
	}
	if c != nil {
		if db := m.DB.Save(c); db.Error != nil {
			fmt.Printf("save cve %s failed: %s", c.CVE, db.Error)
		}
	}
}

func (m *SqliteManager) SaveCVEVulnerability(vuln *CVEVulnerability) {
	c, err := vuln.ToCVE(m.DB)
	if err != nil {
		log.Error(err)
	}
	if c != nil {
		if db := m.DB.Save(c); db.Error != nil {
			fmt.Printf("save cve %s failed: %s", c.CVE, db.Error)
		}
	}
}

func (n Nodes) insertProducts(db *gorm.DB) []string {
	var Vendors []string
	if len(n.Children) > 0 {
		for _, insideNode := range n.Children {
			insideNode.insertProducts(db)
		}
	} else {
		for _, match := range n.CpeMatch {
			if match.Vulnerable == true {
				cpe, err := ParseToCPE(match.Cpe23URI)
				if err != nil {
					fmt.Println(match.Cpe23URI)
				}
				if err != nil {
					log.Error(err)
				}
				db.Save(ProductsTable{
					Product: cpe.Product,
					Vendor:  cpe.Vendor,
				})
			}
		}
	}
	return Set(Vendors)
}
