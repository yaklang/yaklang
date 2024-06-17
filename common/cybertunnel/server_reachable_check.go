package cybertunnel

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"time"
)

func (s *TunnelServer) CheckServerReachable(ctx context.Context, req *tpb.CheckServerReachableRequest) (*tpb.CheckServerReachableResponse, error) {
	host, port, err := utils.ParseStringToHostPort(req.GetServer()) // check server string is valid
	if err != nil {
		return nil, err
	}

	var result = &tpb.CheckServerReachableResponse{}
	if req.GetHttpCheck() {
		httpFlow := req.GetHttpFlow()
		lowhttpRsp, err := lowhttp.HTTPWithoutRedirect(lowhttp.WithPacketBytes(httpFlow.GetHttpRequest()), lowhttp.WithHost(host), lowhttp.WithPort(port), lowhttp.WithHttps(httpFlow.GetIsHttps()))
		if err != nil {
			result.Reachable = false
			result.Verbose = fmt.Sprintf("Try HTTP request %s fail: %s", req.GetServer(), err.Error())
		} else {
			statusCode := lowhttp.GetStatusCodeFromResponse(lowhttpRsp.RawPacket)
			if statusCode != 200 {
				result.Reachable = false
				result.Verbose = fmt.Sprintf("HTTP request %s return status code %d, not 200 ok", req.GetServer(), statusCode)
				result.HttpFlow = &tpb.HTTPSimpleFlow{
					HttpResponse: lowhttpRsp.RawPacket,
					HttpRequest:  lowhttpRsp.RawRequest,
					IsHttps:      httpFlow.GetIsHttps(),
				}
			} else {
				result.Reachable = true
				result.Verbose = "HTTP check ok"
				result.HttpFlow = &tpb.HTTPSimpleFlow{
					HttpResponse: lowhttpRsp.RawPacket,
					HttpRequest:  lowhttpRsp.RawRequest,
					IsHttps:      httpFlow.GetIsHttps(),
				}
			}
		}
	} else {
		conn, err := netx.DialX(req.GetServer(), netx.DialX_WithTimeout(3*time.Second), netx.DialX_WithDisableProxy(true))
		if err != nil {
			result.Reachable = false
			result.Verbose = fmt.Sprintf("Try dial %s fail: %s", req.GetServer(), err.Error())
		} else {
			defer conn.Close()
			result.Reachable = true
			result.Verbose = "Dial check ok"
		}
	}

	return result, nil
}
