package crawler

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"golang.org/x/net/html"
	"strings"
)

func NewHTTPRequest(https bool, req []byte, rsp []byte, url string) (bool, []byte, error) {
	reqBytes := lowhttp.UrlToRequestPacket(
		"GET", url, req, https,
		lowhttp.ExtractCookieJarFromHTTPResponse(rsp)...)
	if utils.IsHttpOrHttpsUrl(url) {
		return strings.HasPrefix(strings.ToLower(url), "https://"), reqBytes, nil
	}
	return https, reqBytes, nil
}

func Exec(https bool, req []byte, callback func(response *lowhttp.LowhttpResponse, https bool, req []byte)) error {
	rsp, err := lowhttp.HTTP(
		lowhttp.WithPacketBytes(req),
		lowhttp.WithHttps(https),
		lowhttp.WithConnPool(true),
	)
	if err != nil {
		return err
	}
	_, body := lowhttp.SplitHTTPPacketFast(rsp.RawPacket)
	return PageInformationWalker(
		lowhttp.GetHTTPPacketContentType(rsp.RawPacket), string(body),
		WithFetcher_HtmlTag(func(s string, node *html.Node) {
			switch s {
			case "a":
				var href string
				for _, i := range node.Attr {
					if strings.ToLower(i.Key) == "href" {
						href = i.Val
						newReqHttps, newReq, err := NewHTTPRequest(https, rsp.RawRequest, rsp.RawPacket, href)
						if err != nil {
							log.Errorf("new request error: %s", err.Error())
							break
						}
						callback(rsp, newReqHttps, newReq)
						break
					}
				}
			}
		}),
		WithFetcher_JavaScript(func(content *JavaScriptContent) {
			if content.IsCodeText {
				log.Errorf("javascript code not supported: %s", content.Code)
				return
			}
			newReqHttps, newReq, err := NewHTTPRequest(https, rsp.RawRequest, rsp.RawPacket, content.UrlPath)
			if err != nil {
				log.Errorf("new request error: %s", err.Error())
				return
			}
			callback(rsp, newReqHttps, newReq)
		}),
	)
}
