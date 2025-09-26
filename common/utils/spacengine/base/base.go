package base

import (
	"math/rand"
	"net/http"
	"net/url"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

type BaseSpaceEngineConfig struct {
	APIKey         string `app:"name:api_key,verbose:API Key,desc:APIKey / Token,id:1"`
	UserIdentifier string `app:"name:user_identifier,verbose:用户信息,desc:email / username,id:2"`
	Domain         string `app:"name:domain,verbose:域名,desc:域名,id:3"`
}

// QueryConfig 查询配置
type QueryConfig struct {
	RandomDelayRange int // 随机延迟范围（秒），0 表示无延迟
	RetryTimes       int // 重试次数，0 表示不重试
}

// ApplyRandomDelay 应用随机延迟
func ApplyRandomDelay(delayRange int) {
	if delayRange > 0 {
		// 生成 1 到 delayRange 秒之间的随机延迟
		delay := rand.Intn(delayRange) + 1
		log.Infof("应用随机延迟: %d 秒", delay)
		time.Sleep(time.Duration(delay) * time.Second)
	}
}

type BaseSpaceEngineClient struct {
	Key     string
	BaseUrl string
}

type SpaceEngineResponse struct {
	Request     *http.Request
	Response    *http.Response
	ResponseRaw []byte
	Body        []byte
	StatusCode  int
}

func NewBaseSpaceEngineClient(key string, host string) *BaseSpaceEngineClient {
	return &BaseSpaceEngineClient{
		Key:     key,
		BaseUrl: lowhttp.FixURLScheme(host),
	}
}

func (c *BaseSpaceEngineClient) Do(method, path string, opts ...poc.PocConfigOption) (*SpaceEngineResponse, error) {
	urlStr, err := url.JoinPath(c.BaseUrl, path)
	if err != nil {
		return nil, err
	}
	opts = append([]poc.PocConfigOption{poc.WithTimeout(60)}, opts...)

	rsp, req, err := poc.Do(method, urlStr, opts...)
	if err != nil {
		return nil, err
	}

	var (
		rspInst    *http.Response
		statusCode int
	)

	if len(rsp.MultiResponseInstances) == 0 {
		statusCode = lowhttp.GetStatusCodeFromResponse(rsp.RawPacket)
	} else {
		rspInst = rsp.MultiResponseInstances[0]
		statusCode = rspInst.StatusCode
	}
	_, body := lowhttp.SplitHTTPPacketFast(rsp.RawPacket)

	return &SpaceEngineResponse{
		Request:     req,
		Response:    rspInst,
		ResponseRaw: rsp.RawPacket,
		Body:        body,
		StatusCode:  statusCode,
	}, nil
}

func (c *BaseSpaceEngineClient) Get(path string, opts ...poc.PocConfigOption) (*SpaceEngineResponse, error) {
	return c.Do(http.MethodGet, path, opts...)
}

func (c *BaseSpaceEngineClient) Post(path string, opts ...poc.PocConfigOption) (*SpaceEngineResponse, error) {
	return c.Do(http.MethodPost, path, opts...)
}
