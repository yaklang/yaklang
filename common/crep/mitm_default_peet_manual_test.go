package crep

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestManualDefaultMITMPeet(t *testing.T) {
	t.Skip("manual integration test: comment out this line to verify against tls3.peet.ws")

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	proxy, err := NewMITMServer(MITM_SetHTTP2(false), MITM_SetDisableSystemProxy(true))
	if err != nil {
		t.Fatal(err)
	}
	if proxy.tlsFingerprint != netx.DefaultTLSFingerprint {
		t.Fatalf("default MITM TLS fingerprint = %q, want %q", proxy.tlsFingerprint, netx.DefaultTLSFingerprint)
	}
	proxyAddr := utils.HostPort("127.0.0.1", utils.GetRandomAvailableTCPPort())
	ready := make(chan struct{})
	go func() {
		_ = proxy.ServeWithListenedCallback(ctx, proxyAddr, func() { close(ready) })
	}()
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal("MITM did not start")
	}

	packet := []byte("GET /api/all HTTP/1.1\r\nHost: tls3.peet.ws\r\nUser-Agent: Mozilla/5.0\r\n\r\n")
	rsp, err := lowhttp.HTTPWithoutRedirect(
		lowhttp.WithPacketBytes(packet),
		lowhttp.WithHost("tls3.peet.ws"),
		lowhttp.WithPort(443),
		lowhttp.WithHttps(true),
		lowhttp.WithHttp2(false),
		lowhttp.WithProxy("http://"+proxyAddr),
		lowhttp.WithEnableSystemProxyFromEnv(false),
		lowhttp.WithTimeout(30*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	body := lowhttp.GetHTTPPacketBody(rsp.RawPacket)
	var result struct {
		TLS struct {
			JA3     string `json:"ja3"`
			JA3Hash string `json:"ja3_hash"`
			JA4     string `json:"ja4"`
		} `json:"tls"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode Peet response: %v\n%s", err, body)
	}
	if result.TLS.JA3 == "" || result.TLS.JA4 == "" {
		t.Fatalf("missing TLS fingerprint: JA3=%q JA4=%q", result.TLS.JA3, result.TLS.JA4)
	}
	t.Logf("profile=%s JA3=%s JA3Hash=%s JA4=%s", proxy.tlsFingerprint, result.TLS.JA3, result.TLS.JA3Hash, result.TLS.JA4)
}
