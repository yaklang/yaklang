package zoomeye

import (
	"fmt"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/utils/spacengine/base"

	"github.com/yaklang/yaklang/common/utils"
)

var _ base.IUserProfile = (*ZoomEyeClient)(nil)

const (
	defaultAPIHost = "https://api.zoomeye.org"
	sessionKey     = "__YAK_BUILTIN_ZOOMEYE_CLIENT__"
)

type ZoomEyeClient struct {
	*base.BaseSpaceEngineClient
}

func NewClient(key string) *ZoomEyeClient {
	return &ZoomEyeClient{
		BaseSpaceEngineClient: base.NewBaseSpaceEngineClient(key, defaultAPIHost),
	}
}

func NewClientEx(key string, apiHost string) *ZoomEyeClient {
	if apiHost == "" {
		apiHost = defaultAPIHost
	}

	return &ZoomEyeClient{
		BaseSpaceEngineClient: base.NewBaseSpaceEngineClient(key, apiHost),
	}
}

func (c *ZoomEyeClient) UserProfile() ([]byte, error) {
	rsp, err := c.Get("/resources-info", poc.WithReplaceHttpPacketHeader("API-KEY", c.Key), poc.WithSession(sessionKey))
	if err != nil {
		return nil, err
	}

	return rsp.Body, nil
}

func (c *ZoomEyeClient) Query(query string, page int) (*gjson.Result, error) {
	params := map[string]string{
		"query": query,
		"page":  fmt.Sprint(page),
	}
	rsp, err := c.Get("/host/search", poc.WithReplaceHttpPacketHeader("API-KEY", c.Key), poc.WithReplaceAllHttpPacketQueryParams(params), poc.WithSession(sessionKey))
	if err != nil {
		return nil, utils.Wrap(err, "query zoomeye search api failed")
	}

	result := gjson.ParseBytes(rsp.Body)
	if rsp.StatusCode != 200 {
		return &result, utils.Errorf("[%v]: invalid status code", rsp.StatusCode)
	}

	if !result.Get("matches").Exists() {
		return nil, utils.Errorf("no matches found")
	}

	return &result, nil
}
