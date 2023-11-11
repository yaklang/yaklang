package yakit

import (
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"sort"
	"strings"
)

func FilterHTTPFlowBySchema(db *gorm.DB, schema string) *gorm.DB {
	if schema != "" {
		db = db.Where("url LIKE ?", schema+"://%")
	}
	return db
}

type WebsiteNextPart struct {
	Schema       string
	NextPart     string
	HaveChildren bool
	Count        int
}

func trimPathWithOneSlash(path string) string {
	path, _, _ = strings.Cut(path, "?")
	if strings.HasPrefix(path, "/") {
		path = path[1:]
	}
	if strings.HasSuffix(path, "/") {
		path = path[:len(path)-1]
	}
	return path
}

func GetHTTPFlowDomainsByDomainSuffix(db *gorm.DB, domainSuffix string) []*WebsiteNextPart {
	db = FilterHTTPFlowByDomain(db, domainSuffix)
	db = db.Select(
		"DISTINCT SUBSTR(url, INSTR(url, '://') + 3, INSTR(SUBSTR(url, INSTR(url, '://') + 3), '/') - 1) as next_part,\n" +
			"SUBSTR(url, 0, INSTR(url, '://'))",
	).Table("http_flows").Limit(1000) // .Debug()
	if rows, err := db.Rows(); err != nil {
		log.Error("query nextPart for website tree failed: %s", err)
		return nil
	} else {
		var resultMap = make(map[string]*WebsiteNextPart)
		for rows.Next() {
			var nextPart string
			var schema string
			rows.Scan(&nextPart, &schema)
			if nextPart == "" {
				continue
			}
			haveChildren := false
			nextPartItem, after, splited := strings.Cut(nextPart, "/")
			if splited && after != "" {
				haveChildren = true
			}
			if result, ok := resultMap[nextPartItem]; ok {
				result.Count++
			} else {
				resultMap[nextPartItem] = &WebsiteNextPart{
					NextPart: nextPartItem, HaveChildren: haveChildren, Count: 1,
					Schema: schema,
				}
			}
		}
		var data []*WebsiteNextPart
		for _, r := range resultMap {
			data = append(data, r)
		}
		sort.SliceStable(data, func(i, j int) bool {
			return data[i].NextPart > data[j].NextPart
		})
		return data
	}
}

func GetHTTPFlowNextPartPathByPathPrefix(db *gorm.DB, originPathPrefix string) []*WebsiteNextPart {
	pathPrefix := trimPathWithOneSlash(originPathPrefix)
	db = FilterHTTPFlowPathPrefix(db, originPathPrefix)
	db = db.Select(fmt.Sprintf(
		`DISTINCT SUBSTR(
   url,
   INSTR(url, '://') + 3 + INSTR(SUBSTR(url, INSTR(url, '://') + 3), '/') + %d,
   CASE
	   WHEN INSTR(SUBSTR(url, INSTR(url, '://') + 3), ?) > 0
		   THEN
			   INSTR(SUBSTR(url, INSTR(url, '://') + 3), ?) -
			   INSTR(SUBSTR(url, INSTR(url, '://') + 3), '/') -
			   1 - %d
	   ELSE LENGTH(url)
	   END
) as next_part`, len(pathPrefix), len(pathPrefix)), "?", "?").Table("http_flows").Limit(1000) // .Debug()
	if rows, err := db.Rows(); err != nil {
		log.Error("query nextPart for website tree failed: %s", err)
		return nil
	} else {
		var resultMap = make(map[string]*WebsiteNextPart)
		for rows.Next() {
			var nextPart string
			rows.Scan(&nextPart)
			if nextPart == "" {
				continue
			}
			if nextPart[0] == '/' {
				nextPart = nextPart[1:]
			}
			haveChildren := false
			nextPartItem, after, splited := strings.Cut(nextPart, "/")
			if splited && after != "" {
				haveChildren = true
			}
			if result, ok := resultMap[nextPartItem]; ok {
				result.Count++
			} else {
				resultMap[nextPartItem] = &WebsiteNextPart{
					NextPart: nextPartItem, HaveChildren: haveChildren, Count: 1,
				}
			}
		}
		var data []*WebsiteNextPart
		for _, r := range resultMap {
			data = append(data, r)
		}
		sort.SliceStable(data, func(i, j int) bool {
			return data[i].NextPart > data[j].NextPart
		})
		return data
	}
}

func FilterHTTPFlowPathPrefix(db *gorm.DB, pathPrefix string) *gorm.DB {
	if pathPrefix != "" {
		pathPrefix := trimPathWithOneSlash(pathPrefix)
		template := `SUBSTR(url,INSTR(url, '://') + 3 + INSTR(SUBSTR(url, INSTR(url, '://') + 3), '/'),CASE WHEN INSTR(SUBSTR(url, INSTR(url, '://') + 3), ?) > 0 THEN INSTR(SUBSTR(url, INSTR(url, '://') + 3), ?) - INSTR(SUBSTR(url, INSTR(url, '://') + 3), '/') - 1 ELSE LENGTH(url) END) LIKE ?`
		db = db.Where(template, "?", "?", pathPrefix+"%")
	}
	return db
}

func FilterHTTPFlowByDomain(db *gorm.DB, domain string) *gorm.DB {
	// query url
	// schema://domain
	// no '/' in domain and schema
	if strings.Contains(domain, "%") {
		domain = strings.ReplaceAll(domain, "%", "%%")
		domain = strings.Trim(domain, "%")
	}

	if domain != "" {
		db = db.Where(`SUBSTR(url, INSTR(url, '://') + 3, INSTR(SUBSTR(url, INSTR(url, '://') + 3), '/') - 1) LIKE ?`, "%"+domain)
	}
	return db
}
