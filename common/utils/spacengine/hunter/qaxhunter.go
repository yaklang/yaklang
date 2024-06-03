package hunter

import (
	"errors"
	"fmt"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/utils/spacengine/base"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/yaklang/yaklang/common/utils"
)

var _ base.IUserProfile = (*HunterClient)(nil)

const (
	defaultAPIHost = "https://hunter.qianxin.com"
	sessionKey     = "__YAK_BUILTIN_HUNTER_CLIENT__"
)

type HunterClient struct {
	*base.BaseSpaceEngineClient
}

func NewClient(key string) *HunterClient {
	return &HunterClient{
		BaseSpaceEngineClient: base.NewBaseSpaceEngineClient(key, defaultAPIHost),
	}
}

func NewClientEx(key string, apiHost string) *HunterClient {
	if apiHost == "" {
		apiHost = defaultAPIHost
	}

	return &HunterClient{
		BaseSpaceEngineClient: base.NewBaseSpaceEngineClient(key, apiHost),
	}
}

func (c *HunterClient) UserProfile() ([]byte, error) {
	// can only search to get user info
	params := map[string]string{
		"api-key":   c.Key,
		"search":    "YXBhY2hl",
		"page":      "1",
		"page_size": "1",
	}

	rsp, err := c.Get("/openApi/search", poc.WithReplaceAllHttpPacketQueryParams(params), poc.WithSession(sessionKey))
	if err != nil {
		return nil, err
	}

	return rsp.Body, nil
}

func (c *HunterClient) Query(query string, page, pageSize int) (*gjson.Result, error) {
	params := map[string]string{
		"api-key":   c.Key,
		"search":    codec.EncodeBase64Url(query),
		"page":      fmt.Sprint(page),
		"page_size": fmt.Sprint(pageSize),
	}

	rsp, err := c.Get("/openApi/search", poc.WithReplaceAllHttpPacketQueryParams(params), poc.WithSession(sessionKey))
	if err != nil {
		return nil, err
	}

	return checkErrorAndResult(rsp.StatusCode, rsp.Body)
}

func checkErrorAndResult(statusCode int, data []byte) (*gjson.Result, error) {
	if statusCode != 200 {
		return nil, errors.New("invalid status code")
	}

	result := gjson.ParseBytes(data)
	code, errmsg := result.Get("code").Int(), result.Get("message").String()
	if code != 200 {
		return nil, utils.Errorf("[%v]: %v", code, errmsg)
	}

	return &result, nil
}
