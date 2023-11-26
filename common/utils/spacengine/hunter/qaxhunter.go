package hunter

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net/url"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/utils"
)

var defaultHttpClient = utils.NewDefaultHTTPClient()

func HunterQuery(username, key, query string, page, pageSize int) (*gjson.Result, error) {
	if key == "" {
		return nil, utils.Error("empty api key")
	}
	values := make(url.Values)
	values.Set("api-key", key)
	values.Set("search", codec.EncodeBase64Url(query))
	values.Set("page", fmt.Sprint(page))
	values.Set("page_size", fmt.Sprint(pageSize))

	packet := []byte(`GET /openApi/search HTTP/1.1
Host: hunter.qianxin.com
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:91.0) Gecko/20100101 Firefox/91.0

`)
	for _, kv := range [][]string{
		{"api-key", key},
		{"search", codec.EncodeBase64Url(query)},
		{"page", fmt.Sprint(page)},
		{"page_size", fmt.Sprint(pageSize)},
	} {
		packet = lowhttp.ReplaceHTTPPacketQueryParam(packet, kv[0], kv[1])
	}
	rsp, err := lowhttp.HTTP(lowhttp.WithPacketBytes(packet), lowhttp.WithHttps(true))
	if err != nil {
		spew.Dump(rsp.RawPacket)
		spew.Dump(rsp.RawRequest)
		return nil, err
	}
	_, raw := lowhttp.SplitHTTPPacketFast(rsp.RawPacket)
	if lowhttp.GetStatusCodeFromResponse(rsp.RawPacket) != 200 {
		spew.Dump(rsp.RawPacket)
		return nil, utils.Errorf("invalid status code")
	}

	result := gjson.ParseBytes(raw)
	code, errmsg := result.Get("code").Int(), result.Get("message").String()

	if result.Get("code").Int() != 200 {
		log.Warnf("met error! %v", string(raw))
		fmt.Println(string(rsp.RawRequest))
		return &result, utils.Errorf("[%v]: %v", code, errmsg)
	}

	return &result, nil
}
