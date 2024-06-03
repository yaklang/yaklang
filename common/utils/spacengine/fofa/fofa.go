package fofa

import (
	"errors"
	"fmt"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/utils/spacengine/base"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var _ base.IUserProfile = (*FofaClient)(nil)

// FofaClient a fofa client can be used to make queries
type FofaClient struct {
	*base.BaseSpaceEngineClient
	email string
}

// User struct for fofa user
type User struct {
	Email          string `json:"email,omitempty"`
	Avatar         string `json:"avatar,omitempty"`
	Err            string `json:"errmsg,omitempty"`
	Fcoin          int    `json:"fcoin,omitempty"`
	RemainApiQuery int64  `json:"remain_api_query,omitempty"`
	Vip            bool   `json:"bool,omitempty"`
}

const (
	defaultAPIHost = "https://fofa.info"
	sessionKey     = "__YAK_BUILTIN_FOFA_CLIENT__"
)

var (
	errFofaReplyWrongFormat = errors.New("Fofa Reply With Wrong Format")
	errFofaReplyNoData      = errors.New("No Data In Fofa Reply")
)

func NewClient(email, key string) *FofaClient {
	return &FofaClient{
		email:                 email,
		BaseSpaceEngineClient: base.NewBaseSpaceEngineClient(key, defaultAPIHost),
	}
}

func NewClientEx(email, key string, apiHost string) *FofaClient {
	if apiHost == "" {
		apiHost = defaultAPIHost
	}

	return &FofaClient{
		email:                 email,
		BaseSpaceEngineClient: base.NewBaseSpaceEngineClient(key, apiHost),
	}
}

func (c *FofaClient) Query(page, pageSize int, args ...string) (*gjson.Result, error) {
	var (
		query  = ""
		fields = "host,title,ip,domain,port,country,city"
	)
	switch {
	case len(args) == 1:
		query = args[0]
	case len(args) == 2:
		query = args[0]
		fields = args[1]
	}
	params := map[string]string{
		"email":   c.email,
		"key":     c.Key,
		"qbase64": codec.EncodeBase64(query),
		"fields":  fields,
		"page":    fmt.Sprint(page),
		"size":    fmt.Sprint(pageSize),
	}
	rsp, err := c.Get("/api/v1/search/all", poc.WithReplaceAllHttpPacketQueryParams(params), poc.WithSession(sessionKey))
	if err != nil {
		return nil, err
	}

	errmsg := gjson.GetBytes(rsp.Body, "errmsg").String()
	if errmsg != "" {
		err = errors.New(errmsg)
	}
	result := gjson.ParseBytes(rsp.Body)
	return &result, err
}

func (c *FofaClient) UserProfile() (raw []byte, err error) {
	params := map[string]string{
		"email": c.email,
		"key":   c.Key,
	}

	rsp, err := c.Get("/api/v1/info/my", poc.WithReplaceAllHttpPacketQueryParams(params), poc.WithSession(sessionKey))
	if err != nil {
		return nil, err
	}

	return rsp.Body, nil
}
