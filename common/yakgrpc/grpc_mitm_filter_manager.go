package yakgrpc

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"mime"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type MITMFilterManager struct { // legacy
	db               *gorm.DB `json:"-"`
	IncludeHostnames []string `json:"includeHostnames"`
	ExcludeHostnames []string `json:"excludeHostnames"`

	IncludeSuffix []string `json:"includeSuffix"`
	ExcludeSuffix []string `json:"excludeSuffix"`

	IncludeUri []string `json:"includeUri"`
	ExcludeUri []string `json:"excludeUri"`

	ExcludeMethods []string `json:"excludeMethods"`
	ExcludeMIME    []string `json:"excludeMIME"`
}

func LegacyFilter2FilterMatcherData(m *MITMFilterManager) *ypb.MITMFilterData {
	var result = &ypb.MITMFilterData{}
	result.IncludeHostnames = []*ypb.FilterDataItem{{
		MatcherType: httptpl.MATCHER_TYPE_GLOB,
		Group:       m.IncludeHostnames,
	}}

	result.ExcludeHostnames = []*ypb.FilterDataItem{{
		MatcherType: httptpl.MATCHER_TYPE_GLOB,
		Group:       m.ExcludeHostnames,
	}}

	result.IncludeSuffix = []*ypb.FilterDataItem{{
		MatcherType: httptpl.MATCHER_TYPE_SUFFIX,
		Group:       m.IncludeSuffix,
	}}

	result.ExcludeSuffix = []*ypb.FilterDataItem{{
		MatcherType: httptpl.MATCHER_TYPE_SUFFIX,
		Group:       m.ExcludeSuffix,
	}}

	result.IncludeUri = []*ypb.FilterDataItem{{
		MatcherType: httptpl.MATCHER_TYPE_GLOB,
		Group:       m.IncludeUri,
	}}

	result.ExcludeUri = []*ypb.FilterDataItem{{
		MatcherType: httptpl.MATCHER_TYPE_GLOB,
		Group:       m.ExcludeUri,
	}}

	result.ExcludeMethods = []*ypb.FilterDataItem{{
		MatcherType: httptpl.MATCHER_TYPE_GLOB,
		Group:       m.ExcludeMethods,
	}}

	result.ExcludeMIME = []*ypb.FilterDataItem{{
		MatcherType: httptpl.MATCHER_TYPE_MIME,
		Group:       m.ExcludeMIME,
	}}
	return result
}

type FilterMatcher struct {
	IncludeSuffixMatcher *httptpl.YakMatcher
	ExcludeSuffixMatcher *httptpl.YakMatcher

	IncludeHostnamesMatcher *httptpl.YakMatcher
	ExcludeHostnamesMatcher *httptpl.YakMatcher

	IncludeUriMatcher *httptpl.YakMatcher
	ExcludeUriMatcher *httptpl.YakMatcher

	ExcludeMethodsMatcher *httptpl.YakMatcher
	ExcludeMIMEMatcher    *httptpl.YakMatcher
}

type MITMFilter struct {
	db      *gorm.DB
	Data    *ypb.MITMFilterData
	Filters *FilterMatcher
}

func NewMITMFilter(data *ypb.MITMFilterData) *MITMFilter {
	m := &MITMFilter{}
	m.Update(data)
	return m
}

func (m *MITMFilter) updateMatcher() {
	if m.Data == nil {
		m.Filters = nil
		return
	}
	m.Filters = &FilterMatcher{}
	m.Filters.ExcludeSuffixMatcher = FilterDataToMatchers(m.Data.ExcludeSuffix)
	m.Filters.IncludeSuffixMatcher = FilterDataToMatchers(m.Data.IncludeSuffix)

	m.Filters.ExcludeHostnamesMatcher = FilterDataToMatchers(m.Data.ExcludeHostnames)
	m.Filters.IncludeHostnamesMatcher = FilterDataToMatchers(m.Data.IncludeHostnames)

	m.Filters.ExcludeUriMatcher = FilterDataToMatchers(m.Data.ExcludeUri)
	m.Filters.IncludeUriMatcher = FilterDataToMatchers(m.Data.IncludeUri)

	m.Filters.ExcludeMethodsMatcher = FilterDataToMatchers(m.Data.ExcludeMethods)
	m.Filters.ExcludeMIMEMatcher = FilterDataToMatchers(m.Data.ExcludeMIME)
}

func (m *MITMFilter) Recover() {
	m.Update(defaultMITMFilterData)
}

func (m *MITMFilter) Update(data *ypb.MITMFilterData) {
	m.Data = data
	m.updateMatcher()
}

func FilterDataToMatchers(data []*ypb.FilterDataItem) *httptpl.YakMatcher {
	var matchers []*httptpl.YakMatcher
	for _, datum := range data {
		matcher := &httptpl.YakMatcher{
			MatcherType: datum.MatcherType,
			Group:       datum.Group,
		}
		matchers = append(matchers, matcher)
	}
	if len(matchers) == 0 {
		return nil
	}
	return &httptpl.YakMatcher{
		SubMatchers:         matchers,
		SubMatcherCondition: "or",
	}
}

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

func fixSuffix(suf string) string {
	if strings.HasPrefix(suf, ".") {
		return suf
	} else {
		return "." + suf
	}
}

var defaultMITMFilterData = &ypb.MITMFilterData{
	ExcludeMethods: []*ypb.FilterDataItem{{
		MatcherType: httptpl.MATCHER_TYPE_GLOB,
		Group:       defaultExcludeMethods,
	}},
	ExcludeSuffix: []*ypb.FilterDataItem{{
		MatcherType: httptpl.MATCHER_TYPE_SUFFIX,
		Group:       defaultExcludeSuffix,
	}},
	ExcludeHostnames: []*ypb.FilterDataItem{{
		MatcherType: httptpl.MATCHER_TYPE_SUFFIX,
		Group:       defaultExcludeHostnames,
	}},
	ExcludeMIME: []*ypb.FilterDataItem{{
		MatcherType: httptpl.MATCHER_TYPE_MIME,
		Group:       defaultExcludeMIME,
	}},
}

func getInitFilterManager(db *gorm.DB) (*MITMFilter, error) {
	if db == nil {
		return nil, utils.Error("no database")
	}
	serializedFilter := ""
	if db.HasTable(&schema.ProjectGeneralStorage{}) {
		serializedFilter = yakit.GetProjectKey(db, MITMFilterKeyRecords)
	} else {
		serializedFilter = yakit.GetKey(db, MITMFilterKeyRecords)
	}

	var filter MITMFilter
	err := json.Unmarshal([]byte(serializedFilter), &filter)
	if err != nil {
		// legacy
		var manager MITMFilterManager
		err = json.Unmarshal([]byte(serializedFilter), &manager)
		if err != nil {
			return nil, err
		}
		return NewMITMFilter(LegacyFilter2FilterMatcherData(&manager)), nil
	}
	return &filter, nil
}

func GetMITMFilterManager(projectDB, profileDB *gorm.DB) *MITMFilter {
	// project first
	for _, db := range []*gorm.DB{projectDB, profileDB} {
		result, err := getInitFilterManager(db)
		if err != nil || result == nil {
			continue
		}
		result.db = db
		return result
	}
	filter := NewMITMFilter(defaultMITMFilterData)
	filter.db = projectDB
	return filter
}

func (m *MITMFilter) IsEmpty() bool {
	data := m.Data
	return len(data.ExcludeMIME) <= 0 && len(data.ExcludeMethods) <= 0 &&
		len(data.ExcludeSuffix) <= 0 && len(data.ExcludeHostnames) <= 0 &&
		len(data.IncludeHostnames) <= 0 && len(data.IncludeSuffix) <= 0 &&
		len(data.ExcludeUri) <= 0 && len(data.IncludeUri) <= 0
}

func (m *MITMFilter) Save() {
	db := m.db
	if db == nil {
		return
	}

	if m.IsEmpty() {
		m.Recover()
		return
	}
	result, err := json.Marshal(m.Data)
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

	if include == nil {
		return true
	}

	includeRes, err := include.ExecuteRaw([]byte(raw), nil)
	if err != nil {
		log.Errorf("filter include execute matcher failed: %s", err)
		return false
	}
	return includeRes
}

func (m *MITMFilter) IsMIMEPassed(ct string) bool {
	parsed, _, _ := mime.ParseMediaType(ct)
	if parsed != "" {
		ct = parsed
	}
	return _FilterCheck(nil, m.Filters.ExcludeMIMEMatcher, ct)
}

// IsPassed return true if passed, false if filtered out
func (m *MITMFilter) IsPassed(method string, hostport, urlStr string, ext string) bool {
	var passed bool
	matcher := m.Filters
	if matcher == nil {
		return true
	}

	passed = _FilterCheck(nil, matcher.ExcludeMethodsMatcher, method)
	if !passed {
		log.Debugf("[%v] url: %s is filtered via method", method, truncate(urlStr))
		return false
	}

	passed = _FilterCheck(matcher.IncludeSuffixMatcher, matcher.ExcludeSuffixMatcher, ext)
	if !passed {
		log.Debugf("url: %v is filtered via suffix(%v)", truncate(urlStr), ext)
		return false
	}

	passed = _FilterCheck(matcher.IncludeHostnamesMatcher, matcher.ExcludeHostnamesMatcher, hostport)
	if !passed {
		log.Debugf("url: %s is filtered via hostnames(%v)", truncate(urlStr), hostport)
		return false
	}

	passed = _FilterCheck(matcher.IncludeUriMatcher, matcher.ExcludeUriMatcher, utils.ExtractRawPath(urlStr))
	if !passed {
		log.Debugf("url: %s is filtered via uri(url)", truncate(urlStr))
		return false
	}
	return true
}
