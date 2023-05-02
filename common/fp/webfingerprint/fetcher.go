package webfingerprint

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func FetchBannerFromHostPortEx(baseCtx context.Context, packet2 []byte, host string, port interface{}, bufferSize int64, proxy ...string) (bool, []*HTTPResponseInfo, error) {
	ctx, cancel := context.WithTimeout(baseCtx, 10*time.Second)
	defer cancel()

	timeout := 10 * time.Second
	if ddl, ok := ctx.Deadline(); ok {
		timeout = ddl.Sub(time.Now())
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
	}

	portInt, _ := strconv.Atoi(fmt.Sprint(port))
	target := utils.HostPort(host, port)
	isTls := utils.IsTLSService(target)

	var redirectResponse []struct {
		Url     *url.URL
		Raw     []byte
		Request []byte
		IsHttps bool
	}

	packet := []byte(fmt.Sprintf(`GET / HTTP/1.1
Host: %v
User-Agent: Mozilla/5.0 (Windows NT 10.0; rv:68.0) Gecko/20100101 Firefox/68.0
`, target))
	if packet2 != nil {
		packet = packet2
	}

	originUrl, _ := lowhttp.ExtractURLFromHTTPRequestRaw(packet, isTls)
	rsp, _, isOpen, err := lowhttp.SendHTTPRequestWithRawPacketWithRedirectWithStateFullEx(isTls, host, portInt, packet, timeout, 5, true, func(isHttps bool, req []byte, rsp []byte) bool {
		urlRaw, _ := lowhttp.ExtractURLFromHTTPRequestRaw(req, isHttps)
		if urlRaw != nil {
			redirectResponse = append(redirectResponse, struct {
				Url     *url.URL
				Raw     []byte
				Request []byte
				IsHttps bool
			}{Url: urlRaw, Raw: rsp, Request: req, IsHttps: isHttps})
		}
		return true
	}, false, false, utils.StringArrayFilterEmpty(proxy)...)
	if err != nil {
		return isOpen, nil, utils.Errorf("send request failed: %s", err)
	}

	var infos []*HTTPResponseInfo
	for _, rspRaw := range append([]struct {
		Url     *url.URL
		Raw     []byte
		Request []byte
		IsHttps bool
	}{{Url: originUrl, Raw: rsp, Request: packet, IsHttps: isTls}}, redirectResponse...) {
		rsp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(rspRaw.Raw)), nil)
		if err != nil {
			//log.Errorf("read response failed: %s", err)
			continue
		}
		info := &HTTPResponseInfo{
			StatusCode: rsp.StatusCode,
			Status:     rsp.Status,
			Header:     &rsp.Header,
			URL:        rspRaw.Url,
			RequestRaw: rspRaw.Request,
			IsHttps:    rspRaw.IsHttps,
		}
		if info.URL == nil {
			urlFinal, err := lowhttp.ExtractURLFromHTTPRequestRaw(rspRaw.Request, rspRaw.IsHttps)
			if err != nil {
				return isOpen, nil, err
			}
			info.URL = urlFinal
		}
		info.Body, _ = ioutil.ReadAll(io.LimitReader(rsp.Body, bufferSize))
		infos = append(infos, info)
	}

	return isOpen, infos, nil
}
