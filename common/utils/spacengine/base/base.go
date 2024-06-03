package base

import (
	"net/http"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

type BaseSpaceEngineClient struct {
	Key     string
	APIHost string
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
		APIHost: lowhttp.FixURLScheme(host),
	}
}

func (c *BaseSpaceEngineClient) Do(method, path string, opts ...poc.PocConfigOption) (*SpaceEngineResponse, error) {
	urlStr, err := utils.UrlJoin(c.APIHost, path)
	if err != nil {
		return nil, err
	}

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
