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

// queryEx 使用默认 CVE 数据库按可选项流式查询 CVE（导出名为 cve.QueryEx）
// 与 cve.Query 类似，但自动获取默认 CVE 数据库连接，无需手动传入
// 参数:
//   - i: 查询可选项，如 cve.vendor、cve.product 等
//
// 返回值:
//   - CVE 记录的流式通道
//
// Example:
// ```
// // 示意性示例，需要本地 CVE 数据库
//
//	for c in cve.QueryEx(cve.vendor("apache")) {
//	    println(c.CVE)
//	}
//
// ```
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

// getCVE 按 CVE 编号查询单条 CVE 记录（导出名为 cve.GetCVE）
// 参数:
//   - cve: CVE 编号，如 CVE-2021-44228
//
// 返回值:
//   - 对应的 CVE 记录，未找到时为 nil
//
// Example:
// ```
// // 示意性示例，需要本地 CVE 数据库
// c = cve.GetCVE("CVE-2021-44228")
// if c != nil { println(c.CVE) }
// ```
func getCVE(cve string) *cveresources.CVE {
	var c cveresources.CVE
	consts.GetGormCVEDatabase().Where("cve = ?", cve).First(&c)
	if c.CVE == "" {
		return nil
	}
	return &c
}

// getCWE 按 CWE 编号查询单条 CWE 记录（导出名为 cwe.Get，可省略 CWE- 前缀）
// 参数:
//   - i: CWE 编号，如 "CWE-79" 或 "79"
//
// 返回值:
//   - 对应的 CWE 记录，未找到或出错时为 nil
//
// Example:
// ```
// // 示意性示例，需要本地 CWE 数据库
// c = cwe.Get("CWE-79")
// if c != nil { println(c.NameZh) }
// ```
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
	"Get":              getCWE,
	"Update":           cvequeryops.CWEUpdate,
	"ListAll":          cvequeryops.ListAllCWE,
	"AICompleteFields": cvequeryops.AICompleteFields,
	// Export and Import
	"Export": cvequeryops.ExportCWE,
	"Import": cvequeryops.ImportCWE,
	// Update options
	"proxy": cvequeryops.WithCWEProxy,
	"url":   cvequeryops.WithCWEURL,
	// AICompleteFields options
	"aiConcurrent": cvequeryops.WithAIConcurrent,
	"testLimit":    cvequeryops.WithTestLimit,
}

var CVEExports = map[string]interface{}{
	"Download":      cvequeryops.DownLoad,
	"LoadCVE":       cvequeryops.LoadCVE,
	"QueryEx":       queryEx,
	"Query":         cvequeryops.QueryCVEYields,
	"GetCVE":        getCVE,
	"NewStatistics": NewStatistics,

	// AI completion and import/export
	"AICompleteFields": cvequeryops.CVEAICompleteFields,
	"Export":           cvequeryops.ExportCVE,
	"Import":           cvequeryops.ImportCVE,
	"aiConcurrent":     cvequeryops.WithCVEAIConcurrent,
	"testLimit":        cvequeryops.WithCVETestLimit,

	//"LoadCNNVD":    cveAction.LoadCNNVD,
	"cwe":            cvequeryops.CWE,
	"cve":            cvequeryops.CVE,
	"after":          cvequeryops.After,
	"before":         cvequeryops.Before,
	"skipAnyVersion": cvequeryops.SkipUnboundedWildcard,
	"score":          cvequeryops.Score,
	"severity":       cvequeryops.Severity,
	"vendor":         cvequeryops.Vendor,
	"product":        cvequeryops.ProductWithVersion,
	"cpe":            cvequeryops.CPE,
	"parseToCpe":     webfingerprint.ParseToCPE,
	//"MakeCtScript": cvequeryops.MakeCtScript,
}
