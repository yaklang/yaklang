package cveresources

import (
	"fmt"
	"github.com/antchfx/xmlquery"
	"yaklang/common/log"
	"time"
)

type CNNVD struct {
	CnnvdID     string `gorm:"primary_key"`
	Name        string
	Published   time.Time
	Modified    time.Time
	Source      string
	Severity    string
	VulnType    string
	Thrtype     string
	Description string
	CveId       string
	Refs        string
	Solution    string
}

func (m SqliteManager) SaveCNNVDRecord(node xmlquery.Node) {
	cnnvdData := CNNVD{
		Name:        "",
		CnnvdID:     "",
		Published:   time.Time{},
		Modified:    time.Time{},
		Source:      "",
		Severity:    "",
		VulnType:    "",
		Thrtype:     "",
		Description: "",
		CveId:       "",
		Refs:        "",
		Solution:    "",
	}

	if n := node.SelectElement("name"); n != nil {
		cnnvdData.Name = n.InnerText()
	}

	if n := node.SelectElement("vuln-id"); n != nil {
		cnnvdData.CnnvdID = n.InnerText()
	}

	if n := node.SelectElement("published"); n != nil {
		PublishedTime, err := time.Parse("2006-01-02", n.InnerText())
		if err != nil {
			log.Error(err)
		}
		cnnvdData.Published = PublishedTime
	}

	if n := node.SelectElement("modified"); n != nil {
		ModifiedTime, err := time.Parse("2006-01-02", n.InnerText())
		if err != nil {
			log.Error(err)
		}
		cnnvdData.Published = ModifiedTime
	}

	if n := node.SelectElement("source"); n != nil {
		cnnvdData.Source = n.InnerText()
	}

	if n := node.SelectElement("severity"); n != nil {
		cnnvdData.Severity = n.InnerText()
	}

	if n := node.SelectElement("vuln-type"); n != nil {
		cnnvdData.VulnType = n.InnerText()
	}

	if n := node.SelectElement("thrtype"); n != nil {
		cnnvdData.Thrtype = n.InnerText()
	}

	if n := node.SelectElement("vuln-descript"); n != nil {
		cnnvdData.Description = n.InnerText()
	}

	if n := node.SelectElement("other-id").SelectElement("cve-id"); n != nil {
		cnnvdData.CveId = n.InnerText()
	}

	if n := node.SelectElement("refs").SelectElements("ref"); n != nil {
		for _, insideNode := range n {
			cnnvdData.Refs += insideNode.SelectElement("ref-url").InnerText() + "\n"
		}
	}

	if n := node.SelectElement("vuln-solution"); n != nil {
		cnnvdData.Solution = n.InnerText()
	}

	if db := m.DB.Save(cnnvdData); db.Error != nil {
		fmt.Printf("save cve %s failed: %s", cnnvdData.CnnvdID, db.Error)
	}

}
