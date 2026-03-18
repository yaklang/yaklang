package zone

import (
	"encoding/json"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/utils/spacengine/base"
)

var _ base.IUserProfile = (*ZoneClient)(nil)

const (
	defaultAPIHost = "https://0.zone"
	sessionKey     = "__YAK_BUILTIN_ZONE_CLIENT__"
)

// zoneQueryParam 0.zone API 请求参数
type zoneQueryParam struct {
	Query     string `json:"query"`
	QueryType string `json:"query_type"`
	Page      int    `json:"page"`
	PageSize  int    `json:"pagesize"`
	ZoneKeyID string `json:"zone_key_id"`
}

type ZoneClient struct {
	*base.BaseSpaceEngineClient
}

func NewClient(key string) *ZoneClient {
	return &ZoneClient{
		BaseSpaceEngineClient: base.NewBaseSpaceEngineClient(key, defaultAPIHost),
	}
}

func NewClientEx(key string, apiHost string) *ZoneClient {
	if apiHost == "" {
		apiHost = defaultAPIHost
	}

	return &ZoneClient{
		BaseSpaceEngineClient: base.NewBaseSpaceEngineClient(key, apiHost),
	}
}

// UserProfile 0.zone 无独立用户信息接口，通过一次轻量级查询验证 API Key
func (c *ZoneClient) UserProfile() ([]byte, error) {
	param := zoneQueryParam{
		Query:     "(status_code=200)",
		QueryType: "site",
		Page:      1,
		PageSize:  1,
		ZoneKeyID: c.Key,
	}
	raw, err := json.Marshal(param)
	if err != nil {
		return nil, utils.Wrap(err, "zone query param marshal failed")
	}

	rsp, err := c.Post("/api/data/", poc.WithReplaceAllHttpPacketHeaders(map[string]string{
		"Content-Type": "application/json",
	}), poc.WithReplaceHttpPacketBody(raw, false), poc.WithSession(sessionKey))
	if err != nil {
		return nil, err
	}

	result := gjson.ParseBytes(rsp.Body)
	if result.Get("code").Int() != 0 {
		return nil, utils.Errorf("zone api error: code=%d, message=%s", result.Get("code").Int(), result.Get("message").String())
	}

	return rsp.Body, nil
}

// Query 执行 0.zone 查询，queryType 支持 "site"（信息系统）等
func (c *ZoneClient) Query(query string, queryType string, page int, pageSize int) (*gjson.Result, error) {
	param := zoneQueryParam{
		Query:     query,
		QueryType: queryType,
		Page:      page,
		PageSize:  pageSize,
		ZoneKeyID: c.Key,
	}
	raw, err := json.Marshal(param)
	if err != nil {
		return nil, utils.Wrap(err, "zone query param marshal failed")
	}

	rsp, err := c.Post("/api/data/", poc.WithReplaceAllHttpPacketHeaders(map[string]string{
		"Content-Type": "application/json",
	}), poc.WithReplaceHttpPacketBody(raw, false), poc.WithSession(sessionKey))
	if err != nil {
		return nil, err
	}

	result := gjson.ParseBytes(rsp.Body)
	code := result.Get("code").Int()
	if code != 0 {
		return nil, utils.Errorf("zone api error: code=%d, message=%s", code, result.Get("message").String())
	}

	return &result, nil
}
