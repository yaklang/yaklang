package cybertunnel

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"time"
)

func (s *TunnelServer) CheckServerReachable(ctx context.Context, req *tpb.CheckServerReachableRequest) (*tpb.CheckServerReachableResponse, error) {
	urlIns := utils.ParseStringToUrl(req.GetUrl())
	if !utils.IsValidHost(urlIns.Host) {
		return nil, fmt.Errorf("invalid host")
	}

	httpCheck := req.GetHttpCheck()
	if urlIns.Port() == "" {
		if urlIns.Scheme == "" || urlIns.Scheme == "http" {
			httpCheck = true
			urlIns.Host += ":80"
		} else if urlIns.Scheme == "https" {
			httpCheck = true
			urlIns.Host += ":443"
		}
	}
	var result = &tpb.CheckServerReachableResponse{}
	if httpCheck {
		if urlIns.Scheme == "" {
			if urlIns.Port() == "443" {
				urlIns.Scheme = "https"
			} else {
				urlIns.Scheme = "http"
			}
		}
		rsp, _, err := poc.DoGET(urlIns.String(), poc.WithForceHTTPS(urlIns.Scheme == "https"), poc.WithNoRedirect(true))
		if err != nil {
			result.Reachable = false
			result.Verbose = fmt.Sprintf("Try HTTP request %s fail: %s", req.GetUrl(), err.Error())
		} else {
			result.HTTPRequest = rsp.RawRequest
			result.HTTPResponse = rsp.RawPacket
			statusCode := lowhttp.GetStatusCodeFromResponse(rsp.RawPacket)
			if statusCode >= 200 && statusCode < 400 {
				result.Reachable = true
				result.Verbose = "HTTP check ok"
			} else {
				result.Reachable = false
				result.Verbose = fmt.Sprintf("HTTP request %s return status code %d", req.GetUrl(), statusCode)
			}
		}
	} else {
		conn, err := netx.DialX(urlIns.Host, netx.DialX_WithTimeout(3*time.Second), netx.DialX_WithDisableProxy(true))
		if err != nil {
			result.Reachable = false
			result.Verbose = fmt.Sprintf("Try dial %s fail: %s", req.GetUrl(), err.Error())
		} else {
			defer conn.Close()
			result.Reachable = true
			result.Verbose = "Dial check ok"
		}
	}

	return result, nil
}
