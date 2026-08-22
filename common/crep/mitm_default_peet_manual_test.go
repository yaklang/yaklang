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

	proxy, err := NewMITMServer(MITM_SetHTTP2(true), MITM_SetDisableSystemProxy(true))
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
		lowhttp.WithHttp2(true),
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
		HTTP2 struct {
			Akamai string `json:"akamai_fingerprint"`
		} `json:"http2"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode Peet response: %v\n%s", err, body)
	}
	const wantJA4 = "t13d1516h2_8daaf6152771_806a8c22fdea"
	const wantAkamai = "1:65536;2:0;4:6291456;6:262144|15663105|0|m,a,s,p"
	if result.TLS.JA4 != wantJA4 {
		t.Fatalf("JA4 = %q, want %q", result.TLS.JA4, wantJA4)
	}
	if result.HTTP2.Akamai != wantAkamai {
		t.Fatalf("HTTP/2 fingerprint = %q, want %q", result.HTTP2.Akamai, wantAkamai)
	}
	t.Logf("profile=%s JA3=%s JA3Hash=%s JA4=%s Akamai=%s", proxy.tlsFingerprint, result.TLS.JA3, result.TLS.JA3Hash, result.TLS.JA4, result.HTTP2.Akamai)
}
