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
