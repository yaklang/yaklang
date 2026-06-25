package loop_ssa_api_discovery

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

const probeTimeout = 5 * time.Second

// TargetProbeResult records how we tested target reachability.
type TargetProbeResult struct {
	Reachable   bool
	ProbeMethod string
	Detail      string
	Host        string
	Port        string
	Scheme      string
}

// ProbeTarget checks TCP or HTTP reachability depending on target shape.
func ProbeTarget(ctx context.Context, targetRaw string) *TargetProbeResult {
	res := &TargetProbeResult{Reachable: false, ProbeMethod: "none", Detail: "empty target"}
	targetRaw = NormalizeTargetString(strings.TrimSpace(targetRaw))
	if targetRaw == "" {
		return res
	}

	if strings.Contains(targetRaw, "://") {
		u, err := url.Parse(targetRaw)
		if err != nil {
			res.Detail = err.Error()
			return res
		}
		res.Scheme = u.Scheme
		res.Host = u.Hostname()
		res.Port = u.Port()
		return probeHTTP(ctx, targetRaw, res)
	}

	host, port, err := utils.ParseStringToHostPort(targetRaw)
	if err != nil {
		res.Detail = err.Error()
		return res
	}
	res.Host = host
	if port > 0 {
		res.Port = strconv.Itoa(port)
	}
	return probeTCP(ctx, host, port, res)
}

func probeTCP(ctx context.Context, host string, port int, res *TargetProbeResult) *TargetProbeResult {
	res.ProbeMethod = "tcp_connect"
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	d := net.Dialer{Timeout: probeTimeout}
	c, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		res.Reachable = false
		res.Detail = err.Error()
		return res
	}
	_ = c.Close()
	res.Reachable = true
	res.Detail = "tcp ok"
	return res
}

func probeHTTP(ctx context.Context, rawURL string, res *TargetProbeResult) *TargetProbeResult {
	res.ProbeMethod = "http_head"
	client := &http.Client{Timeout: probeTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		res.Detail = err.Error()
		return res
	}
	resp, err := client.Do(req)
	if err != nil {
		res.ProbeMethod = "http_get"
		req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		resp2, err2 := client.Do(req2)
		if err2 != nil {
			res.Reachable = false
			res.Detail = err2.Error()
			return res
		}
		defer resp2.Body.Close()
		res.Reachable = true
		res.Detail = resp2.Status
		return res
	}
	defer resp.Body.Close()
	res.Reachable = true
	res.Detail = resp.Status
	return res
}
