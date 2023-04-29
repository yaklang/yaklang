package cveresources

import (
	"strings"
)

type CVERes struct {
	CVE
	ConfidenceLevel float64
}

func (c CVERes) Year() int {
	return c.CVE.PublishedDate.Year()
}

func (c CVERes) CWE(rule string) bool {
	if strings.Contains(c.CVE.CWE, rule) {
		return true
	}
	return false
}

func (c CVERes) CNNVD(dir string) (CNNVD, error) {
	var res CNNVD
	m := GetManager(dir)
	resDb := m.DB.Where("cve_id = ?", c.CVE.CVE).Find(&res)
	if resDb.Error != nil {
		return res, resDb.Error
	}
	return res, nil
}
