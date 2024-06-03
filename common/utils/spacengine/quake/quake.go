package quake

import (
	"encoding/json"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/utils/spacengine/base"
)

var _ base.IUserProfile = (*QuakeClient)(nil)

const (
	defaultAPIHost = "https://quake.360.net"
	sessionKey     = "__YAK_BUILTIN_QUAKE_CLIENT__"
)

type QuakeClient struct {
	*base.BaseSpaceEngineClient
}

type quakeQueryParam struct {
	Query string `json:"query"`
	Start int    `json:"start"`
	Size  int    `json:"size"`
}

type quakeUserInfo struct {
	Id      string `json:"id"`
	IsBaned bool   `json:"baned"`
	// 当月剩余
	MonthRemainingCredit int    `json:"month_remaining_credit"`
	TotalCredit          int    `json:"total_credit"`
	ConstantCredit       int    `json:"constant_credit"`
	BanStatus            string `json:"ban_status"`
}

func NewClient(key string) *QuakeClient {
	return &QuakeClient{
		BaseSpaceEngineClient: base.NewBaseSpaceEngineClient(key, defaultAPIHost),
	}
}

func NewClientEx(key string, apiHost string) *QuakeClient {
	if apiHost == "" {
		apiHost = defaultAPIHost
	}

	return &QuakeClient{
		BaseSpaceEngineClient: base.NewBaseSpaceEngineClient(key, apiHost),
	}
}

func (c *QuakeClient) UserProfile() ([]byte, error) {
	rsp, err := c.Get("/api/v3/user/info", poc.WithReplaceAllHttpPacketHeaders(map[string]string{
		"X-QuakeToken": c.Key,
		"Content-Type": "application/json",
	}), poc.WithSession(sessionKey))
	if err != nil {
		return nil, err
	}

	return rsp.Body, nil
}

func (c *QuakeClient) Query(query string, start, pageSize int) (*gjson.Result, error) {
	raw, err := json.Marshal(quakeQueryParam{
		Query: query,
		Start: start,
		Size:  pageSize,
	})
	if err != nil {
		return nil, utils.Wrap(err, "json marshal failed")
	}

	rsp, err := c.Post("/api/v3/search/quake_service", poc.WithReplaceAllHttpPacketHeaders(map[string]string{
		"X-QuakeToken": c.Key,
		"Content-Type": "application/json",
	}), poc.WithReplaceHttpPacketBody(raw, false), poc.WithSession(sessionKey))
	if err != nil {
		return nil, err
	}

	result := gjson.ParseBytes(rsp.Body)
	code := result.Get("code")
	if !code.Exists() || code.Type == gjson.String || code.Int() != 0 {
		return nil, utils.Errorf("quake error: %s", result.Get("message").String())
	}

	dataArray := result.Get("data").Array()
	if len(dataArray) <= 0 {
		return nil, utils.Errorf("empty services / results")
	}

	return &result, nil
}
