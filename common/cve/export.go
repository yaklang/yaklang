package cve

import (
	"fmt"
	"strings"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cve/cvequeryops"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
)

func queryEx(i ...interface{}) chan *cveresources.CVE {
	db := consts.GetGormCVEDatabase()
	if db == nil {
		log.Info("no cve database resource found, you can download via yakit grpc intf")
		ch := make(chan *cveresources.CVE)
		close(ch)
		return ch
	}

	db = db.Model(&cveresources.CVE{})
	var opts []cvequeryops.CVEOption
	funk.ForEach(i, func(i interface{}) {
		r, ok := i.(cvequeryops.CVEOption)
		if ok {
			opts = append(opts, r)
		}
	})
	config := &cvequeryops.CVEQueryInfo{}
	for _, opt := range opts {
		opt(config)
	}

	return cvequeryops.QueryCVEYields(db, opts...)
}

func getCVE(cve string) *cveresources.CVE {
	var c cveresources.CVE
	consts.GetGormCVEDatabase().Where("cve = ?", cve).First(&c)
	if c.CVE == "" {
		return nil
	}
	return &c
}

func getCWE(i interface{}) *cveresources.CWE {
	s := fmt.Sprint(i)
	if strings.HasPrefix(s, "CWE-") {
		s = s[4:]
	}
	db := consts.GetGormCVEDatabase()
	if db == nil {
		log.Error("cannot found database (cve db)")
		return nil
	}
	cwe, err := cveresources.GetCWE(db, s)
	if err != nil {
		log.Errorf("get cwe %v failed: %s", i, err)
		return nil
	}
	return cwe
}

var CWEExports = map[string]interface{}{
	"Get": getCWE,
}

var CVEExports = map[string]interface{}{
	"Download":      cvequeryops.DownLoad,
	"Query":         cvequeryops.Query,
	"LoadCVE":       cvequeryops.LoadCVE,
	"QueryEx":       queryEx,
	"GetCVE":        getCVE,
	"NewStatistics": NewStatistics,

	//"LoadCNNVD":    cveAction.LoadCNNVD,
	"cwe":          cvequeryops.CWE,
	"cve":          cvequeryops.CVE,
	"after":        cvequeryops.After,
	"before":       cvequeryops.Before,
	"score":        cvequeryops.Score,
	"severity":     cvequeryops.Severity,
	"vendor":       cvequeryops.Vendor,
	"product":      cvequeryops.ProductWithVersion,
	"cpe":          cvequeryops.CPE,
	"parseToCpe":   webfingerprint.ParseToCPE,
	"MakeCtScript": cvequeryops.MakeCtScript,
}
