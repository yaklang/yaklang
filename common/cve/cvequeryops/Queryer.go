package cvequeryops

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/log"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type CVEOption func(info *CVEQueryInfo)

type CVEQueryInfo struct {
	CVE          string
	CPE          []cveresources.CPE
	CWE          []string
	Vendors      []string
	Products     []string
	Severity     []string
	ExploitScore float64
	After        time.Time
	Before       time.Time
	Start        int
	Quantity     int
	OrderBy      string
	Desc         bool
	Strict       bool
}

func QueryCVEYields(db *gorm.DB, opts ...CVEOption) chan *cveresources.CVE {
	db, queryConfig := Filter(db, opts...)
	if queryConfig != nil && len(queryConfig.CPE) > 0 {
		ch := make(chan *cveresources.CVE)
		go func() {
			defer close(ch)
			for c := range cveresources.YieldCVEs(db, context.Background()) {
				var level = 0.0
				var config cveresources.Configurations

				err := json.Unmarshal(c.CPEConfigurations, &config)
				if err != nil {
					continue
				}

				for _, node := range config.Nodes {
					if level < node.Result(queryConfig.CPE) {
						level = node.Result(queryConfig.CPE)
					}
				}

				if level > 0 {
					ch <- c
				}
			}
		}()
		return ch
	}
	return cveresources.YieldCVEs(db, context.Background())
}

func Filter(db *gorm.DB, opts ...CVEOption) (*gorm.DB, *CVEQueryInfo) {
	if len(opts) <= 0 {
		return db, &CVEQueryInfo{}
	}
	cveQuery := &CVEQueryInfo{}
	cveQuery.Strict = false
	for _, opt := range opts {
		opt(cveQuery)
	}

	if (len(cveQuery.Products) > 0 || len(cveQuery.CPE) > 0) && !cveQuery.Strict {
		cveQuery = FixCVEProduct(cveQuery, db)
	}
	var sqlSentence string
	var param []interface{}
	sqlSentence, param = MakeSqlSentence(cveQuery)
	db = db.Where(sqlSentence, param...)
	if cveQuery.Quantity != 0 {
		if cveQuery.OrderBy != "" {
			if cveQuery.Desc {
				db = db.Offset(cveQuery.Start).Order(cveQuery.OrderBy + "desc").Limit(cveQuery.Quantity)
			} else {
				db = db.Offset(cveQuery.Start).Order(cveQuery.OrderBy).Limit(cveQuery.Quantity)
			}
		} else {
			db = db.Offset(cveQuery.Start).Limit(cveQuery.Quantity)
		}
	}
	return db, cveQuery
}

func FixCVEProduct(cveQuery *CVEQueryInfo, db *gorm.DB) *CVEQueryInfo {
	var fixName []string
	var fixCPE []cveresources.CPE
	for i := 0; i < len(cveQuery.Products); i++ {
		if info, ok := cveresources.CommonFix[cveQuery.Products[i]]; ok { //查询基础修复里有没有对应的畸形名
			if info.Vendor != "" {
				fixCPE = append(fixCPE, cveresources.CPE{
					Part:    "*",
					Vendor:  info.Vendor,
					Product: info.ProductName,
					Version: "*",
					Edition: "*",
				})
			}
			fixName = append(fixName, cveresources.CommonFix[cveQuery.Products[i]].ProductName)
			continue
		}
		fixRes, err := cveresources.FixProductName(cveQuery.Products[i], db) //尝试通用方法修复
		if err != nil {
			log.Warningf("find product name failed: %s[%s]", err, cveQuery.Products[i])
		} else { //修复好的所有产品名放入查询条件里
			fixName = append(fixName, fixRes...)
		}
	}
	if fixName != nil {
		cveQuery.Products = fixName
	}

	for i := 0; i < len(cveQuery.CPE); i++ {
		if info, ok := cveresources.CommonFix[cveQuery.CPE[i].Product]; ok { //查询基础修复里有没有对应的畸形名
			vendorStr := cveQuery.CPE[i].Vendor
			if info.Vendor != "" {
				vendorStr = info.Vendor
			}
			fixCPE = append(fixCPE, cveresources.CPE{
				Part:    cveQuery.CPE[i].Part,
				Vendor:  vendorStr,
				Product: info.ProductName,
				Version: cveQuery.CPE[i].Version,
				Edition: cveQuery.CPE[i].Edition,
			})
			continue
		}
		fixRes, err := cveresources.FixProductName(cveQuery.CPE[i].Product, db)
		if err != nil {
			log.Warningf("find product name failed: %s[%s]", err, cveQuery.CPE[i].Product)
		} else {
			//修复后所有可能的产品名放入CPE中
			for _, name := range fixRes {
				cpeItem := cveresources.CPE{
					Part:    cveQuery.CPE[i].Part,
					Vendor:  cveQuery.CPE[i].Vendor,
					Product: name,
					Version: cveQuery.CPE[i].Version,
					Edition: cveQuery.CPE[i].Edition,
				}
				fixCPE = append(fixCPE, cpeItem)
			}
		}
	}
	if fixCPE != nil {
		cveQuery.CPE = fixCPE
	}
	return cveQuery
}

func CVE(id string) CVEOption {
	return func(info *CVEQueryInfo) {
		info.CVE = id
	}
}

func CWE(cwe string) CVEOption {
	return func(info *CVEQueryInfo) {
		info.CWE = append(info.CWE, cwe)
	}
}

func After(year int, data ...int) CVEOption {
	dataStringFormat := "%d-%02d-%02d"
	dataString := ""
	if len(data) == 2 && data[0] > 0 && data[0] <= 12 && data[1] > 0 && data[1] <= 31 {
		dataString = fmt.Sprintf(dataStringFormat, year, data[0], data[1])
	} else if len(data) == 1 && data[0] > 0 && data[0] <= 12 {
		dataString = fmt.Sprintf(dataStringFormat, year, data[0], 1)
	} else if len(data) == 0 {
		dataString = fmt.Sprintf(dataStringFormat, year, 1, 1)
	} else {
		log.Error("time args error:", data)
		panic("time args error")
	}
	afterTime, err := time.Parse("2006-01-02", dataString)
	if err != nil {
		log.Error(err)
		panic(err)
	}
	return func(info *CVEQueryInfo) {
		info.After = afterTime
	}
}

func AfterByTimeStamp(timeStamp int64) CVEOption {
	AfterTime := time.Unix(timeStamp, 0)
	return func(info *CVEQueryInfo) {
		info.After = AfterTime
	}

}

func Before(year int, data ...int) CVEOption {
	dataStringFormat := "%d-%02d-%02d"
	dataString := ""
	if len(data) == 2 && data[0] > 0 && data[0] <= 12 && data[1] > 0 && data[1] <= 31 {
		dataString = fmt.Sprintf(dataStringFormat, year, data[0], data[1])
	} else if len(data) == 1 && data[0] > 0 && data[0] <= 12 {
		dataString = fmt.Sprintf(dataStringFormat, year, data[0], 1)
	} else if len(data) == 0 {
		dataString = fmt.Sprintf(dataStringFormat, year, 1, 1)
	} else {
		log.Error("time args error:", data)
		panic("time args error")
	}
	beforeTime, err := time.Parse("2006-01-02", dataString)
	if err != nil {
		log.Error(err)
		panic(err)
	}
	return func(info *CVEQueryInfo) {
		info.Before = beforeTime
	}
}

func BeforeByTimeStamp(timeStamp int64) CVEOption {
	beforeTime := time.Unix(timeStamp, 0)
	return func(info *CVEQueryInfo) {
		info.Before = beforeTime
	}

}

func Score(score float64) CVEOption {
	return func(info *CVEQueryInfo) {
		info.ExploitScore = score
	}
}

func Severity(level string) CVEOption {
	formatLevel := strings.ToUpper(level)
	if strings.Contains(formatLevel, "HIGH") || strings.Contains(formatLevel, "HIG") {
		level = "HIGH"
	} else if strings.Contains(formatLevel, "MEDIUM") || strings.Contains(formatLevel, "MID") {
		level = "MEDIUM"
	} else if strings.Contains(formatLevel, "LOW") {
		level = "LOW"
	} else {
		log.Error("Unknown Severity level")
		panic("Unknown Severity level")
	}

	return func(info *CVEQueryInfo) {
		info.Severity = append(info.Severity, level)
	}
}

func Vendor(v string) CVEOption {
	return func(info *CVEQueryInfo) {
		info.Vendors = append(info.Vendors, v)
	}
}

func Product(p string) CVEOption {
	return func(info *CVEQueryInfo) {
		info.Products = append(info.Products, p)
	}
}

func ProductWithVersion(p string, v ...string) CVEOption {
	if len(v) == 0 {
		return Product(p)
	} else if len(v) == 1 {
		return func(info *CVEQueryInfo) {
			info.Products = append(info.Products, p)
			info.CPE = append(info.CPE, cveresources.CPE{
				Part:    "*",
				Vendor:  "*",
				Product: p,
				Version: v[0],
				Edition: "",
			})
		}
	} else {
		log.Error("The number of parameters does not match")
		panic("The number of parameters does not match")
	}
}

func CPE(c string) CVEOption {
	return func(info *CVEQueryInfo) {
		rule, err := regexp.Compile("\\[(\\d+)-(\\d+)]")
		if err != nil {
			log.Error(err)
			panic(err)
		}
		cpeStruct, err := cveresources.ParseToCPE(c)
		if err != nil {
			log.Error(err)
			panic(err)
		}

		//var cpeInfo cmd.CPE
		if rule.MatchString(cpeStruct.Version) {
			scope := rule.FindSubmatch([]byte(cpeStruct.Version))
			start, err := strconv.Atoi(string(scope[len(scope)-2]))
			if err != nil {
				log.Error(err)
				panic(err)
			}
			end, err := strconv.Atoi(string(scope[len(scope)-1]))
			if err != nil {
				log.Error(err)
				panic(err)
			}

			for i := start; i <= end; i++ {
				info.CPE = append(info.CPE, cveresources.CPE{
					Part:    cpeStruct.Part,
					Vendor:  cpeStruct.Vendor,
					Product: cpeStruct.Product,
					Version: strings.Replace(cpeStruct.Version, string(scope[0]), strconv.Itoa(i), 1),
					Edition: cpeStruct.Edition,
				})
			}
			//cpeInfo = *cpeStruct
		}

	}
}

func Limit(quantity int) CVEOption {
	return func(info *CVEQueryInfo) {
		info.Quantity = quantity
	}
}

func Offset(start int) CVEOption {
	return func(info *CVEQueryInfo) {
		info.Start = start
	}
}

func OrderBy(name string) CVEOption {
	return func(info *CVEQueryInfo) {
		info.OrderBy = name
	}
}

func Desc(flag bool) CVEOption {
	return func(info *CVEQueryInfo) {
		info.Desc = flag
	}
}

func Strict(flag bool) CVEOption {
	return func(info *CVEQueryInfo) {
		info.Strict = flag
	}
}

func MakeSqlSentence(info *CVEQueryInfo) (string, []interface{}) {
	zeroTime := time.Time{}

	var SqlSentences []string
	var param []interface{}

	if info.CVE != "" {
		SqlSentences = append(SqlSentences, " cve = ? ")
		param = append(param, info.CVE)
	}

	if len(info.CPE) > 0 {
		for _, cpe := range info.CPE {
			if cpe.Vendor != "*" {
				info.Vendors = append(info.Vendors, cpe.Vendor)
			}
			if cpe.Product != "*" {
				info.Products = append(info.Products, cpe.Product)
			}
		}
	}

	if len(info.Vendors) > 0 || len(info.Products) > 0 {
		info.Vendors = cveresources.Set(info.Vendors)
		info.Products = cveresources.Set(info.Products)
		clause := " ("

		//构造Vendor和Product查询子句
		var inSideSql []string
		for _, vendor := range info.Vendors {
			inSideSql = append(inSideSql, " vendor LIKE ? ")
			param = append(param, "%,"+vendor)
			inSideSql = append(inSideSql, " vendor LIKE ? ")
			param = append(param, "%,"+vendor+",%")
			inSideSql = append(inSideSql, " vendor LIKE ? ")
			param = append(param, vendor+",%")
			inSideSql = append(inSideSql, " vendor = ? ")
			param = append(param, vendor)
		}

		for _, product := range info.Products {
			inSideSql = append(inSideSql, " product LIKE ? ")
			param = append(param, "%,"+product)
			inSideSql = append(inSideSql, " product LIKE ? ")
			param = append(param, "%,"+product+",%")
			inSideSql = append(inSideSql, " product LIKE ? ")
			param = append(param, product+",%")
			inSideSql = append(inSideSql, " product = ? ")
			param = append(param, product)
		}

		clause += strings.Join(inSideSql, "OR")

		clause += ") "

		SqlSentences = append(SqlSentences, clause)
	}

	if len(info.CWE) > 0 {
		for i := 0; i < len(info.CWE); i++ {
			SqlSentences = append(SqlSentences, " cwe LIKE ? ")
			param = append(param, "%"+info.CWE[i]+"%")
		}
	}

	if len(info.Severity) > 0 {

		clause := " ("
		//构造Vendor和Product查询子句
		var inSideSql []string
		for _, level := range info.Severity {
			inSideSql = append(inSideSql, " severity == ? ")
			param = append(param, level)
		}
		clause += strings.Join(inSideSql, "OR")
		clause += ") "

		SqlSentences = append(SqlSentences, clause)
	}

	if info.ExploitScore > 0 {
		//ScoreStr := fmt.Sprintf("%2.1f", info.ExploitScore)
		//SqlSentences = append(SqlSentences, " base_cvs_sv2_score >= "+ScoreStr+" ")
		SqlSentences = append(SqlSentences, " base_cvs_sv2_score >= ? ")
		param = append(param, info.ExploitScore)
	}

	if info.Before != zeroTime {
		SqlSentences = append(SqlSentences, " published_date < ? ")
		param = append(param, info.Before)
	}

	if info.After != zeroTime {
		SqlSentences = append(SqlSentences, " published_date > ? ")
		param = append(param, info.After)
	}

	SqlSentence := strings.Join(SqlSentences, "AND")

	return SqlSentence, param
}
