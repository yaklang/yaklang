package yakit

import (
	"context"
	"net/url"
	"sort"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
)

func FilterHTTPFlowBySchema(db *gorm.DB, schema string) *gorm.DB {
	if schema != "" {
		db = db.Where("SUBSTR(url, 1, ?) = ?", len(schema+"://"), schema+"://") //.Debug()
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
	RawQueryKey  string
	RawNextPart  string
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
	).Table("http_flows").Limit(1000) //.Debug()
	if rows, err := db.Rows(); err != nil {
		log.Errorf("query nextPart for website tree failed: %s", err)
		return nil
	} else {
		resultMap := make(map[string]*WebsiteNextPart)
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
			// 创建一个包含schema和nextPart的唯一键
			uniqueKey := schema + "://" + nextPartItem
			if result, ok := resultMap[uniqueKey]; ok {
				result.Count++
			} else {
				resultMap[uniqueKey] = &WebsiteNextPart{
					NextPart: uniqueKey, HaveChildren: haveChildren, Count: 1,
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

func matchURL(u string, searchPath string) bool {
	var err error
	if strings.Contains(searchPath, "%") {
		searchPath, err = url.PathUnescape(searchPath)
		if err != nil {
			return false
		}
	}

	// 解析 URL
	parsedURL, _ := url.Parse(u)

	normalizedPath := strings.Join(strings.FieldsFunc(parsedURL.Path, func(r rune) bool {
		return r == '/'
	}), "/")

	normalizedSearchPath := strings.Join(strings.FieldsFunc(searchPath, func(r rune) bool {
		return r == '/'
	}), "/")

	// 确保搜索路径以 "/" 开头
	searchPath = "/" + strings.TrimLeft(normalizedSearchPath, "/")
	searchPath = strings.TrimRight(searchPath, "/")
	// 获取路径并确保它以 "/" 开头
	path := "/" + strings.TrimLeft(normalizedPath, "/")

	// 分割搜索路径和 URL 路径
	searchSegments := strings.Split(searchPath, "/")
	pathSegments := strings.Split(path, "/")

	// 检查路径是否以搜索路径的段开头
	match := true
	for i := 1; i < len(searchSegments); i++ {
		if i >= len(pathSegments) || searchSegments[i] != pathSegments[i] {
			match = false
			break
		}
	}

	return match
}

// findNextPathSegment 返回 url 中紧随 target 之后的路径段，包含正确数量的斜杠
func findNextPathSegment(url, target string) string {
	// 分割 url 和 target
	urlSegments := strings.Split(url, "/")
	targetSegments := strings.Split(target, "/")

	targetIndex, slashCount := 0, 0

	// 遍历 urlSegments 查找 targetSegments
	for _, segment := range urlSegments {
		targetIndex++

		if segment == "" {
			slashCount++
			continue
		}

		if targetIndex-1 < len(targetSegments) && segment == targetSegments[targetIndex-1] {
			slashCount = 1 // 重置斜杠计数
			continue
		}

		// 找到 target，返回下一个非空段，连同之前计算的斜杠
		if segment != "" {
			return strings.Repeat("/", slashCount) + segment
		}

	}

	return ""
}

func GetHTTPFlowNextPartPathByPathPrefix(db *gorm.DB, originPathPrefix string) []*WebsiteNextPart {
	//pathPrefix := strings.Join(strings.FieldsFunc(originPathPrefix, func(r rune) bool {
	//	return r == '/'
	//}), "/")
	pathPrefix := strings.TrimLeft(originPathPrefix, "/")
	db = db.Select("url").Table("http_flows").Where("url LIKE ?", `%`+pathPrefix+`%`).Limit(1000) //.Debug()
	urlsMap := make(map[string]bool)
	var urls []string
	for u := range YieldHTTPUrl(db, context.Background()) {
		if _, exists := urlsMap[u.Url]; !exists {
			urlsMap[u.Url] = true
			if matchURL(u.Url, originPathPrefix) {
				urls = append(urls, u.Url)
			}
		}
	}

	// 初始化一个映射，用于存储网站结构
	resultMap := make(map[string]*WebsiteNextPart)

	// 假设 urls 是您的 URL 列表
	for _, us := range urls {
		usC := strings.SplitN(us, "?", 2)[0] + "%2f"
		uc, _ := url.Parse(usC)
		u, _ := url.Parse(us)
		if u.Path == "" || uc.RawPath == "" {
			continue
		}
		// 寻找目标字符串，为了解决多个 / 的问题
		rawNextPart := findNextPathSegment(strings.TrimSuffix(uc.RawPath, "%2f"), originPathPrefix)
		// 去除URL路径中多余的斜线
		normalizedPath := strings.Join(strings.FieldsFunc(u.Path, func(r rune) bool {
			return r == '/'
		}), "/")

		normalizedOriginPathPrefix := strings.Join(strings.FieldsFunc(pathPrefix, func(r rune) bool {
			return r == '/'
		}), "/")

		path := strings.Trim(normalizedPath, "/")

		pathPrefix, err := url.PathUnescape(normalizedOriginPathPrefix)
		if err != nil {
			continue
		}

		suffix := strings.TrimPrefix(path, pathPrefix)

		suffix = strings.Trim(suffix, "/")

		nextSegment, after, splited := strings.Cut(suffix, "/")

		// 根据路径是否分割，决定是否有子路径
		haveChildren := splited && after != ""

		if nextSegment == "" && u.RawQuery == "" {
			continue
		}

		if nextSegment != "" {
			node, ok := resultMap[nextSegment]
			// 检查根路径段是否已经在resultMap中
			if !ok {
				if u.RawQuery != "" {
					haveChildren = true
				}
				node = &WebsiteNextPart{
					NextPart:     nextSegment,
					HaveChildren: haveChildren, // 如果有多个路径段，说明有子节点
					Count:        1,            // 初始化计数为1
					Schema:       u.Scheme,
					RawNextPart:  rawNextPart,
				}
				if strings.Contains(nextSegment, ".") {
					node.IsFile = true
				}
				resultMap[nextSegment] = node

			} else {
				// 如果已经存在，且不是文件，则增加计数
				if !strings.Contains(nextSegment, ".") && haveChildren {
					node.Count++
				}

				if u.RawQuery != "" {
					haveChildren = true
				}
				node.HaveChildren = haveChildren
			}
		}

		// 如果存在查询参数，将其添加到根路径段
		if u.RawQuery != "" && path == pathPrefix {
			for key := range u.Query() {
				if len(key) == 0 {
					continue
				}
				queryNode, ok := resultMap[key]
				if !ok {
					queryNode = &WebsiteNextPart{
						NextPart:     key,
						HaveChildren: false, // 如果有多个路径段，说明有子节点
						RawQueryKey:  key,
						IsQuery:      true,
						Schema:       u.Scheme,
					}
					//if strings.Contains(key, ".") {
					//	queryNode.HaveChildren = true
					//}
					resultMap[key] = queryNode
				} else {
					queryNode.Count++
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
		//db = db.Where(`SUBSTR(url, INSTR(url, '://') + 3, INSTR(SUBSTR(url, INSTR(url, '://') + 3), '/') - 1) LIKE ?`, "%"+domain)
		db = db.Where(`SUBSTR(url, INSTR(url, '://') + 3, INSTR(SUBSTR(url, INSTR(url, '://') + 3), '/') - 1) LIKE ?`, "%"+domain+"%")
		// db = db.Where(`SUBSTR(url, INSTR(url, '://') + 3, INSTR(SUBSTR(url, INSTR(url, '://') + 3), '/') - 1) = ?`, domain)

	}
	return db
}

func FilterHTTPFlowByRuntimeID(db *gorm.DB, runtimeID string) *gorm.DB {
	runtimeID = strings.TrimSpace(runtimeID)
	if runtimeID == "" {
		return db
	}

	// 支持使用逗号分隔的多个 runtime_id
	if strings.Contains(runtimeID, ",") {
		var cleaned []string
		for _, id := range strings.Split(runtimeID, ",") {
			id = strings.TrimSpace(id)
			if id != "" {
				cleaned = append(cleaned, id)
			}
		}
		if len(cleaned) == 0 {
			return db
		}
		return db.Where("runtime_id IN (?)", cleaned)
	}

	return db.Where("runtime_id = ?", runtimeID)
}
