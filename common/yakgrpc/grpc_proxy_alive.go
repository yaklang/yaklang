package yakgrpc

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) CheckProxyAlive(ctx context.Context, req *ypb.CheckProxyAliveRequest) (*ypb.CheckProxyAliveResponse, error) {
	target := strings.TrimSpace(req.GetTarget())
	resp := &ypb.CheckProxyAliveResponse{
		Target: target,
	}
	if target == "" {
		resp.Reason = "target 不能为空"
		return resp, nil
	}

	proxy := strings.TrimSpace(req.GetProxy())
	if proxy == "" {
		id := strings.TrimSpace(req.GetEndpointId())
		if id == "" {
			resp.Reason = "endpointId 或 proxy 不能为空"
			return resp, nil
		}
		endpoint, err := resolveProxyEndpoint(id)
		if err != nil {
			resp.Reason = err.Error()
			return resp, nil
		}
		proxy = yakit.BuildProxyEndpointURL(endpoint)
	}
	proxy = strings.Trim(proxy, `":`)
	resp.Proxy = proxy

	timeout := pickProxyCheckTimeout(ctx, req.GetTimeoutSeconds())
	begin := time.Now()

	u, isHttps, err := parseProbeTarget(target)
	packet, err := lowhttp.UrlToHTTPRequest(u.String())
	if err != nil {
		resp.Reason = err.Error()
		return resp, nil
	}
	if isCipHost(u.Host) {
		packet = lowhttp.ReplaceAllHTTPPacketHeaders(
			packet,
			map[string]string{"User-Agent": "curl/7.88.1"},
		)
	}

	rsp, err := lowhttp.HTTPWithoutRedirect(
		lowhttp.WithRequest(packet),
		lowhttp.WithProxy(proxy),
		lowhttp.WithHttps(isHttps),
		lowhttp.WithTimeout(timeout),
	)
	if err != nil {
		if errors.Is(err, netx.ErrorProxyAuthFailed) {
			resp.Reason = "代理认证失败"
		} else {
			resp.Reason = err.Error()
		}
		return resp, nil
	}
	statusCode := 0
	if rsp != nil {
		statusCode = lowhttp.GetStatusCodeFromResponse(rsp.RawPacket)
		if statusCode == 407 {
			resp.Reason = "代理认证失败"
			return resp, nil
		}
	}

	if isCipHost(u.Host) && rsp != nil {
		body := rsp.GetBody()
		if len(body) > 0 {
			resp.Ok = true
			resp.Reason = strings.TrimSpace(string(body))
			resp.CostMs = time.Since(begin).Milliseconds()
			return resp, nil
		}
	}

	resp.Ok = true
	if statusCode > 0 {
		resp.Reason = fmt.Sprintf("ok (status %d)", statusCode)
	} else {
		resp.Reason = "ok"
	}
	resp.CostMs = time.Since(begin).Milliseconds()
	return resp, nil
}

func pickProxyCheckTimeout(ctx context.Context, timeoutSeconds int64) time.Duration {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeoutSeconds <= 0 {
		timeout = 5 * time.Second
	}
	if ddl, ok := ctx.Deadline(); ok {
		if remaining := time.Until(ddl); remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}
	return timeout
}

func resolveProxyEndpoint(id string) (*ypb.ProxyEndpoint, error) {
	cfg, err := yakit.GetGlobalProxyRulesConfig()
	if err != nil {
		return nil, err
	}
	for _, ep := range cfg.GetEndpoints() {
		if ep != nil && ep.GetId() == id {
			if ep.GetDisabled() {
				return nil, utils.Errorf("proxy endpoint disabled: %s", id)
			}
			return ep, nil
		}
	}
	return nil, utils.Errorf("未找到代理节点: %s", id)
}

func parseProbeTarget(raw string) (*url.URL, bool, error) {
	target := strings.TrimSpace(raw)
	if target == "" {
		return nil, false, utils.Error("目标地址为空")
	}
	if !strings.Contains(target, "://") {
		target = "http://" + target
	}
	u, err := url.Parse(target)
	if err != nil {
		return nil, false, err
	}
	return u, u.Scheme == "https", nil

}

func isCipHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	return h == "cip.cc" || strings.HasSuffix(h, ".cip.cc")
}
