package yakgrpc

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"mime"
	"regexp"
	"strings"

	"github.com/gobwas/glob"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type MITMFilterManager struct {
	db               *gorm.DB `json:"-"`
	IncludeHostnames []string `json:"includeHostnames"`
	ExcludeHostnames []string `json:"excludeHostnames"`
	IncludeSuffix    []string `json:"includeSuffix"`
	ExcludeSuffix    []string `json:"excludeSuffix"`
	ExcludeMethods   []string `json:"excludeMethods"`
	ExcludeMIME      []string `json:"excludeMIME"`
	ExcludeUri       []string `json:"excludeUri"`
	IncludeUri       []string `json:"includeUri"`

	ExcludeHostnamesMatcher *httptpl.YakMatcher
	ExcludeSuffixMatcher    *httptpl.YakMatcher
	ExcludeMethodsMatcher   *httptpl.YakMatcher
	ExcludeMIMEMatcher      *httptpl.YakMatcher
}

var (
	defaultExcludeHostnamesMatcher = &httptpl.YakMatcher{
		MatcherType: "glob",
		Scope:       "raw",
		Group:       []string{"google.com", "*gstatic.com", "*bdstatic.com", "*google*.com"},
	}
	defaultExcludeSuffixMatcher = &httptpl.YakMatcher{
		MatcherType: "word",
		Scope:       "raw",
		Group: []string{
			".css",
			".jpg", ".jpeg", ".png",
			".mp3", ".mp4", ".ico", ".bmp",
			".flv", ".aac", ".ogg", "avi",
			".svg", ".gif", ".woff", ".woff2",
			".doc", ".docx", ".pptx",
			".ppt", ".pdf",
		},
	}

	defaultExcludeMethodsMatcher = &httptpl.YakMatcher{
		MatcherType: "glob",
		Scope:       "raw",
		Group:       []string{"OPTIONS", "CONNECT"},
	}
	defaultExcludeMIMEMatcher = &httptpl.YakMatcher{
		MatcherType: "glob",
		Scope:       "raw",
		Group: []string{
			"image/*",
			"audio/*", "video/*", // "*octet-stream*",
			"application/ogg", "application/pdf", "application/msword",
			"application/x-ppt", "video/avi", "application/x-ico",
			"*zip",
		},
	}
)

var (
	defaultExcludeHostnames = []string{"google.com", "*gstatic.com", "*bdstatic.com", "*google*.com"}
	defaultExcludeSuffix    = []string{
		".css",
		".jpg", ".jpeg", ".png",
		".mp3", ".mp4", ".ico", ".bmp",
		".flv", ".aac", ".ogg", "avi",
		".svg", ".gif", ".woff", ".woff2",
		".doc", ".docx", ".pptx",
		".ppt", ".pdf",
	}
)

var (
	defaultExcludeMethods = []string{"OPTIONS", "CONNECT"}
	defaultExcludeMIME    = []string{
		"image/*",
		"audio/*", "video/*", // "*octet-stream*",
		"application/ogg", "application/pdf", "application/msword",
		"application/x-ppt", "video/avi", "application/x-ico",
		"*zip",
	}
)

func _exactChecker(includes, excludes []string, target string) bool {
	excludes = utils.StringArrayFilterEmpty(excludes)
	includes = utils.StringArrayFilterEmpty(includes)

	for _, exclude := range excludes {
		if match, err := regexp.MatchString(exclude, target); err == nil && match {
			return false
		} else if exclude == target {
			return false
		}
	}

	if includes == nil {
		return true
	}

	for _, include := range includes {
		if match, err := regexp.MatchString(include, target); err == nil && match {
			return true
		} else if include == target {
			return true
		}
	}

	return false
}

func fixSuffix(suf string) string {
	if strings.HasPrefix(suf, ".") {
		return suf
	} else {
		return "." + suf
	}
}

func _suffixChecker(includes, excludes []string, target string) bool {
	excludes = utils.StringArrayFilterEmpty(excludes)
	includes = utils.StringArrayFilterEmpty(includes)

	for _, exclude := range excludes {
		if strings.HasSuffix(target, fixSuffix(exclude)) {
			return false
		} else if exclude == target {
			return false
		}
	}

	if includes == nil {
		return true
	}

	for _, include := range includes {
		if strings.HasSuffix(target, fixSuffix(include)) {
			return true
		} else if include == target {
			return true
		}
	}

	return false
}

func mimeCheckGlobRule(rule string, target string) bool {
	if strings.Contains(rule, "/") && strings.Contains(target, "/") { // 如果两个都包含/，则进行分割匹配
		ruleType := strings.SplitN(rule, "/", 2)
		targetType := strings.SplitN(target, "/", 2)
		for i := 0; i < 2; i++ {
			if strings.Contains(ruleType[i], "*") {
				rule, err := glob.Compile(ruleType[i])
				if err != nil || !rule.Match(targetType[i]) {
					return false // 任意部分匹配失败则 false,包括glob编译失败
				}
			} else {
				if ruleType[i] != targetType[i] {
					return false // 任意部分匹配失败则 false
				}
			}
		}
		return true // 全部通过 true
	}

	if !strings.Contains(target, "/") && !strings.Contains(rule, "/") { // 如果都不包含 /
		if strings.Contains(rule, "*") { // 尝试glob 匹配
			rule, err := glob.Compile(rule)
			if err == nil && rule.Match(target) {
				return true
			}
		} else { // 直接 contains
			if utils.IContains(target, rule) {
				return true
			}
		}
		return false
	}

	if strings.Contains(target, "/") && !strings.Contains(rule, "/") { // 仅rule不包含 /
		targetType := strings.SplitN(target, "/", 2)
		for i := 0; i < 2; i++ {
			if strings.Contains(rule, "*") {
				rule, err := glob.Compile(rule)
				if err != nil {
					continue
				}
				if rule.Match(targetType[i]) {
					return true // 任意部分匹配成功 则true
				}
			} else {
				if rule == targetType[i] {
					return true // 任意部分匹配成功 则true
				}
			}
		}
		return false // 全部失败 则false
	}

	return false // 仅 rule 有 / 则直接返回 false
}

func _mimeChecker(includes, excludes []string, target string) bool {
	excludes = utils.StringArrayFilterEmpty(excludes)
	includes = utils.StringArrayFilterEmpty(includes)

	if includes == nil {
		for _, rule := range excludes {
			if mimeCheckGlobRule(rule, target) {
				return false // 如果命中 excludes 则 false 即过滤
			}
		}
		return true
	}

	for _, rule := range excludes {
		if mimeCheckGlobRule(rule, target) {
			return false // 如果命中 excludes 则 false 即过滤
		}
	}

	for _, rule := range includes {
		if mimeCheckGlobRule(rule, target) {
			return true // 如果命中 includes 则 true 即放行
		}
	}
	return false
}

func (m *MITMFilterManager) Recover() {
	m.ExcludeMethods = defaultExcludeMethods
	m.ExcludeSuffix = defaultExcludeSuffix
	m.ExcludeHostnames = defaultExcludeHostnames
	m.ExcludeMIME = defaultExcludeMIME
	m.ExcludeUri = nil
	m.IncludeUri = nil
	m.IncludeHostnames = nil
	m.IncludeSuffix = nil
	m.Save()
}

var defaultMITMFilterManager = &MITMFilterManager{
	ExcludeHostnames: defaultExcludeHostnames,
	ExcludeSuffix:    defaultExcludeSuffix,
	ExcludeMethods:   defaultExcludeMethods,
	ExcludeMIME:      defaultExcludeMIME,
}

func getInitFilterManager(db *gorm.DB) (*MITMFilterManager, error) {
	if db == nil {
		return nil, utils.Error("no database")
	}
	serializedFilter := ""
	if db.HasTable(&schema.ProjectGeneralStorage{}) {
		serializedFilter = yakit.GetProjectKey(db, MITMFilterKeyRecords)
	} else {
		serializedFilter = yakit.GetKey(db, MITMFilterKeyRecords)
	}

	var manager MITMFilterManager
	err := json.Unmarshal([]byte(serializedFilter), &manager)
	if err != nil {
		return nil, err
	}
	managerP := &manager
	return managerP, nil
}

func GetMITMFilterManager(projectDB, profileDB *gorm.DB) *MITMFilterManager {
	// project first
	for _, db := range []*gorm.DB{projectDB, profileDB} {
		result, err := getInitFilterManager(db)
		if err != nil || result == nil {
			continue
		}
		result.db = db
		return result
	}

	defaultMITMFilterManager.db = projectDB
	return defaultMITMFilterManager
}

func (m *MITMFilterManager) IsEmpty() bool {
	return len(m.ExcludeMIME) <= 0 && len(m.ExcludeMethods) <= 0 &&
		len(m.ExcludeSuffix) <= 0 && len(m.ExcludeHostnames) <= 0 &&
		len(m.IncludeHostnames) <= 0 && len(m.IncludeSuffix) <= 0 &&
		len(m.ExcludeUri) <= 0 && len(m.IncludeUri) <= 0
}

func (m *MITMFilterManager) Save() {
	db := m.db
	if db == nil {
		return
	}

	if m.IsEmpty() {
		m.Recover()
		return
	}

	result, err := json.Marshal(m)
	if err != nil {
		log.Errorf("marshal mitm filter failed: %s", err)
		return
	}
	// project first
	if db.HasTable(&schema.ProjectGeneralStorage{}) {
		err = yakit.SetProjectKey(db, MITMFilterKeyRecords, string(result))
	} else {
		err = yakit.SetKey(db, MITMFilterKeyRecords, string(result))
	}
	if err != nil {
		log.Errorf("set filter db key failed: %s", err)
	}
}

func (m *MITMFilterManager) IsMIMEPassed(ct string) bool {
	parsed, _, _ := mime.ParseMediaType(ct)
	if parsed != "" {
		ct = parsed
	}
	return _mimeChecker(nil, m.ExcludeMIME, ct)
}

func _FilterCheck(include *httptpl.YakMatcher, exclude *httptpl.YakMatcher, raw string) bool {
	if exclude != nil {
		excludeRes, err := exclude.ExecuteRaw([]byte(raw), nil)
		if err != nil {
			log.Errorf("filter exlude execute matcher failed: %s", err)
			return false
		} else if excludeRes {
			return false
		}
	}

	if include != nil {
		return true
	}

	includeRes, err := include.ExecuteRaw([]byte(raw), nil)
	if err != nil {
		log.Errorf("filter include execute matcher failed: %s", err)
		return false
	}
	return includeRes
}

// IsPassed return true if passed, false if filtered out
func (m *MITMFilterManager) IsPassed(method string, hostport, urlStr string, ext string, isHttps bool) bool {
	var passed bool

	passed = _exactChecker(nil, m.ExcludeMethods, method)
	if !passed {
		log.Debugf("[%v] url: %s is filtered via method", method, truncate(urlStr))
		return false
	}

	passed = _suffixChecker(m.IncludeSuffix, m.ExcludeSuffix, strings.ToLower(ext))
	if !passed {
		log.Debugf("url: %v is filtered via suffix(%v)", truncate(urlStr), ext)
		return false
	}

	passed = utils.IncludeExcludeChecker(m.IncludeHostnames, m.ExcludeHostnames, hostport)
	if !passed {
		log.Debugf("url: %s is filtered via hostnames(%v)", truncate(urlStr), hostport)
		return false
	}

	passed = utils.IncludeExcludeChecker(m.IncludeUri, m.ExcludeUri, urlStr)
	if !passed {
		log.Debugf("url: %s is filtered via uri(url)", truncate(urlStr))
		return false
	}
	return true
}
