package yakit

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	WebFuzzerConfigTypePage      = "page"
	WebFuzzerConfigTypePageGroup = "pageGroup"
	WebFuzzerCacheProjectKey     = "fuzzer-list-cache"
	WebFuzzerGroupNameMaxLength  = 24
	WebFuzzerDefaultGroupColor   = "purple"
)

var webFuzzerGroupColors = []string{"purple", "blue", "lakeBlue", "green", "red", "orange", "bluePurple", "grey"}
var webFuzzerGroupHexColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// WebFuzzerParamItem mirrors FuzzerParamItem in frontend advancedConfigValue.params.
type WebFuzzerParamItem struct {
	Key   string `json:"Key"`
	Value string `json:"Value"`
	Type  string `json:"Type"`
}

// WebFuzzerPageParams mirrors pageParams in WebFuzzerConfig.Config JSON.
// Schema source: yakit getFuzzerProcessedCacheData() + per-tab fields restored via fetchFuzzerList().
//
// MCP create_web_fuzzer_tab maps: request, isHttps, actualAddr->actualHost, proxy, concurrent, hotPatchCode.
type WebFuzzerPageParams struct {
	ActualHost      string               `json:"actualHost,omitempty"`
	ID              string               `json:"id"`
	IsHttps         bool                 `json:"isHttps"`
	Request         string               `json:"request"`
	Params          []WebFuzzerParamItem `json:"params"`
	Extractors      []json.RawMessage    `json:"extractors,omitempty"`
	Matchers        []json.RawMessage    `json:"matchers,omitempty"`
	RepeatTimes     int64                `json:"repeatTimes,omitempty"`
	Concurrent      int64                `json:"concurrent,omitempty"`
	MinDelaySeconds float64              `json:"minDelaySeconds,omitempty"`
	MaxDelaySeconds float64              `json:"maxDelaySeconds,omitempty"`
	HotPatchCode    string               `json:"hotPatchCode,omitempty"`
	Proxy           []string             `json:"proxy,omitempty"`
}

// WebFuzzerPageCacheItem is the JSON object stored in FuzzerConfig.Config for Type=page.
type WebFuzzerPageCacheItem struct {
	GroupChildren []any                `json:"groupChildren"`
	GroupID       string               `json:"groupId"`
	ID            string               `json:"id"`
	PageParams    *WebFuzzerPageParams `json:"pageParams"`
	SortField     int64                `json:"sortFieId"`
	Verbose       string               `json:"verbose,omitempty"`
	Expand        bool                 `json:"expand"`
	Color         string               `json:"color,omitempty"`
}

type WebFuzzerPageBuildOptions struct {
	PageID    string
	TabName   string
	GroupID   string
	SortField int64
}

type WebFuzzerGroupBuildOptions struct {
	GroupID   string
	GroupName string
	SortField int64
	Expand    bool
	Color     string
}

func WebFuzzerGroupColors() []string {
	return append([]string(nil), webFuzzerGroupColors...)
}

func WebFuzzerGroupColorPattern() string {
	return "^(" + strings.Join(webFuzzerGroupColors, "|") + `|#[0-9A-Fa-f]{6})$`
}

func NormalizeWebFuzzerGroupName(groupName string) (string, error) {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		return "", utils.Error("group name is required")
	}
	if utf8.RuneCountInString(groupName) > WebFuzzerGroupNameMaxLength {
		return "", utils.Errorf("group name must not exceed %d characters", WebFuzzerGroupNameMaxLength)
	}
	return groupName, nil
}

func NormalizeWebFuzzerGroupColor(color string) (string, error) {
	color = strings.TrimSpace(color)
	if color == "" {
		return WebFuzzerDefaultGroupColor, nil
	}
	if webFuzzerGroupHexColorPattern.MatchString(color) {
		return strings.ToUpper(color), nil
	}
	for _, allowed := range webFuzzerGroupColors {
		if color == allowed {
			return color, nil
		}
	}
	return "", utils.Errorf("unsupported group color %q; use a preset (%s) or a custom #RRGGBB color", color, strings.Join(webFuzzerGroupColors, ", "))
}

func CreateOrUpdateWebFuzzerConfig(db *gorm.DB, config *schema.WebFuzzerConfig) (int64, error) {
	db = db.Model(&schema.WebFuzzerConfig{})
	if db := db.Where("page_id = ?", config.PageId).Assign(config).FirstOrCreate(&schema.WebFuzzerConfig{}); db.Error != nil {
		return 0, utils.Errorf("create/update WebFuzzerLabel failed: %s", db.Error)
	}
	return db.RowsAffected, nil
}

func QueryWebFuzzerConfig(db *gorm.DB, params *ypb.QueryFuzzerConfigRequest) ([]*schema.WebFuzzerConfig, error) {
	var result []*schema.WebFuzzerConfig
	db = db.Model(&schema.WebFuzzerConfig{})
	db = bizhelper.ExactOrQueryStringArrayOr(db, "page_id", params.GetPageId())
	_, db = bizhelper.PagingByPagination(db, params.Pagination, &result)
	if db.Error != nil {
		return nil, utils.Errorf("query webFuzzerConfig failed: %s", db.Error)
	}
	return result, nil
}

func DeleteWebFuzzerConfig(db *gorm.DB, pageIds []string, deleteAll bool) (int64, error) {
	if deleteAll {
		db = db.Unscoped().Delete(&schema.WebFuzzerConfig{})
	} else if len(pageIds) > 0 {
		db = bizhelper.ExactOrQueryStringArrayOr(db, "page_id", pageIds).Unscoped().Delete(&schema.WebFuzzerConfig{})
	}
	if db.Error != nil {
		return 0, utils.Errorf("delete web fuzzer failed: %s", db.Error)
	}
	return db.RowsAffected, nil
}

// BuildWebFuzzerConfig builds ypb.FuzzerConfig aligned with SaveFuzzerConfig / QueryFuzzerConfig.
func BuildWebFuzzerConfig(req *ypb.FuzzerRequest, opts ...func(*WebFuzzerPageBuildOptions)) (*ypb.FuzzerConfig, error) {
	if req == nil {
		return nil, utils.Error("fuzzer request is nil")
	}

	options := WebFuzzerPageBuildOptions{
		PageID:    uuid.NewString(),
		TabName:   "MCP Web Fuzzer",
		GroupID:   "0",
		SortField: 1,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	if options.PageID == "" {
		options.PageID = uuid.NewString()
	}

	request := req.GetRequest()
	if request == "" && len(req.GetRequestRaw()) > 0 {
		request = string(req.GetRequestRaw())
	}
	if request == "" {
		return nil, utils.Error("request is required")
	}

	pageParams := buildWebFuzzerPageParamsFromRequest(req, options.PageID, request)

	cacheItem := &WebFuzzerPageCacheItem{
		GroupChildren: []any{},
		GroupID:       options.GroupID,
		ID:            options.PageID,
		PageParams:    pageParams,
		SortField:     options.SortField,
		Verbose:       options.TabName,
	}

	configJSON, err := json.Marshal(cacheItem)
	if err != nil {
		return nil, utils.Wrap(err, "marshal web fuzzer config failed")
	}

	return &ypb.FuzzerConfig{
		PageId: options.PageID,
		Type:   WebFuzzerConfigTypePage,
		Config: string(configJSON),
	}, nil
}

// BuildWebFuzzerGroupConfig builds the synthetic top-level node used by Yakit
// to represent a Web Fuzzer tab group. Group membership is stored on each
// child page through groupId; groupChildren is rebuilt by the frontend.
func BuildWebFuzzerGroupConfig(groupName string, opts ...func(*WebFuzzerGroupBuildOptions)) (*ypb.FuzzerConfig, error) {
	options := WebFuzzerGroupBuildOptions{
		GroupID:   uuid.NewString() + "-group",
		GroupName: groupName,
		SortField: 1,
		Expand:    true,
		Color:     WebFuzzerDefaultGroupColor,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	options.GroupID = strings.TrimSpace(options.GroupID)
	var err error
	options.GroupName, err = NormalizeWebFuzzerGroupName(options.GroupName)
	if err != nil {
		return nil, err
	}
	options.Color, err = NormalizeWebFuzzerGroupColor(options.Color)
	if err != nil {
		return nil, err
	}
	if options.GroupID == "" {
		options.GroupID = uuid.NewString() + "-group"
	}
	if !strings.HasSuffix(options.GroupID, "group") {
		return nil, utils.Error("group id must end with group")
	}
	if options.SortField <= 0 {
		options.SortField = 1
	}

	cacheItem := &WebFuzzerPageCacheItem{
		GroupChildren: []any{},
		GroupID:       "0",
		ID:            options.GroupID,
		SortField:     options.SortField,
		Verbose:       options.GroupName,
		Expand:        options.Expand,
		Color:         options.Color,
	}
	configJSON, err := json.Marshal(cacheItem)
	if err != nil {
		return nil, utils.Wrap(err, "marshal web fuzzer group config failed")
	}
	return &ypb.FuzzerConfig{
		PageId: options.GroupID,
		Type:   WebFuzzerConfigTypePageGroup,
		Config: string(configJSON),
	}, nil
}

// ParseWebFuzzerConfig decodes the frontend-compatible JSON payload and checks
// that its identity agrees with the enclosing FuzzerConfig record.
func ParseWebFuzzerConfig(config *ypb.FuzzerConfig) (*WebFuzzerPageCacheItem, error) {
	if config == nil {
		return nil, utils.Error("web fuzzer config is nil")
	}
	if strings.TrimSpace(config.GetPageId()) == "" {
		return nil, utils.Error("web fuzzer page id is empty")
	}
	var item WebFuzzerPageCacheItem
	if err := json.Unmarshal([]byte(config.GetConfig()), &item); err != nil {
		return nil, utils.Wrap(err, "unmarshal web fuzzer config failed")
	}
	if item.ID == "" {
		item.ID = config.GetPageId()
	}
	if item.ID != config.GetPageId() {
		return nil, utils.Errorf("web fuzzer config id %q does not match page id %q", item.ID, config.GetPageId())
	}
	if item.GroupID == "" {
		item.GroupID = "0"
	}
	return &item, nil
}

func buildWebFuzzerPageParamsFromRequest(req *ypb.FuzzerRequest, pageID, request string) *WebFuzzerPageParams {
	params := &WebFuzzerPageParams{
		ID:           pageID,
		IsHttps:      req.GetIsHTTPS(),
		Request:      request,
		Params:       defaultWebFuzzerParamItems(),
		Proxy:        parseWebFuzzerProxy(req.GetProxy()),
		ActualHost:   req.GetActualAddr(),
		HotPatchCode: req.GetHotPatchCode(),
	}
	if concurrent := req.GetConcurrent(); concurrent > 0 {
		params.Concurrent = concurrent
	}
	return params
}

func defaultWebFuzzerParamItems() []WebFuzzerParamItem {
	return []WebFuzzerParamItem{{Key: "", Value: "", Type: "raw"}}
}

func parseWebFuzzerProxy(proxy string) []string {
	proxy = strings.TrimSpace(proxy)
	if proxy == "" {
		return nil
	}
	parts := strings.Split(proxy, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
