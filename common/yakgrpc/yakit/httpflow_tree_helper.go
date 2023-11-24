package yakit

import (
	"context"
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"net/url"
	"sort"
	"strings"
)

func FilterHTTPFlowBySchema(db *gorm.DB, schema string) *gorm.DB {
	if schema != "" {
		db = db.Where("url LIKE ?", schema+"://%").Debug()
	}
	return db
}

type WebsiteTree struct {
	Path         string
	NextParts    []*WebsiteNextPart
	HaveChildren bool
}

type WebsiteNextPart struct {
	Schema       string
	NextPart     string
	HaveChildren bool
	Count        int
	IsQuery      bool
	IsFile       bool
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
	pathPrefix2 := "/" + pathPrefix
	db = db.Select("url").Table("http_flows").Where("url LIKE ?", `%`+originPathPrefix+`%`).Limit(1000)
	var urls []string
	for u := range YieldHTTPUrl(db, context.Background()) {
		up, _ := url.Parse(u.Url)
		if strings.HasPrefix(up.Path, pathPrefix2) &&
			(strings.HasPrefix(up.RequestURI()[len(pathPrefix2):], "/") ||
				strings.HasPrefix(up.RequestURI()[len(pathPrefix2):], "?") ||
				pathPrefix == "") || up.Path == originPathPrefix || up.Path == pathPrefix2 {
			urls = append(urls, u.Url)
		}
	}

	// 初始化一个映射，用于存储网站结构
	resultMap := make(map[string]*WebsiteNextPart)

	// 假设 urls 是您的 URL 列表
	for _, us := range urls {
		u, _ := url.Parse(us)
		if u.Path == "" {
			continue
		}
		haveChildren := false

		path := strings.ReplaceAll(u.Path, "//", "/")

		suffix := strings.TrimPrefix(strings.TrimPrefix(path[1:], pathPrefix), "/")

		nextSegment, after, splited := strings.Cut(suffix, "/")
		if splited && after != "" {
			haveChildren = true
		}

		if nextSegment == "" && u.RawQuery == "" {
			continue
		}

		if nextSegment != "" {
			// 检查根路径段是否已经在resultMap中
			if _, ok := resultMap[nextSegment]; !ok {
				if u.RawQuery != "" {
					haveChildren = true
				}
				resultMap[nextSegment] = &WebsiteNextPart{
					NextPart:     nextSegment,
					HaveChildren: haveChildren, // 如果有多个路径段，说明有子节点
					Count:        1,            // 初始化计数为1
					Schema:       u.Scheme,
				}
				if strings.Contains(nextSegment, ".") {
					resultMap[nextSegment].IsFile = true
				}

			} else {
				if u.RawQuery != "" {
					haveChildren = true
				}
				resultMap[nextSegment].HaveChildren = haveChildren

				// 如果已经存在，且不是文件，则增加计数
				if !strings.Contains(nextSegment, ".") {
					resultMap[nextSegment].Count++
				}
			}
		}

		// 如果存在查询参数，将其添加到根路径段
		if u.RawQuery != "" && (u.Path == originPathPrefix || u.Path == "/"+originPathPrefix) {
			for key := range u.Query() {
				resultMap[key] = &WebsiteNextPart{
					NextPart:     key,
					HaveChildren: false, // 如果有多个路径段，说明有子节点
					IsQuery:      true,
					Schema:       u.Scheme,
				}
				resultMap[key].Count++

				if strings.Contains(key, ".") {
					resultMap[key].HaveChildren = true
				}
			}
		}
	}

	// resultMap 现在包含了所有的路径和查询参数，以及它们的层级关系和计数
	var data []*WebsiteNextPart
	for _, r := range resultMap {
		data = append(data, r)
	}
	sort.SliceStable(data, func(i, j int) bool {
		return data[i].NextPart > data[j].NextPart
	})
	return data
}

func GetHTTPFlowNextPartPathByPathPrefixb(db *gorm.DB, originPathPrefix string) []*WebsiteNextPart {
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
