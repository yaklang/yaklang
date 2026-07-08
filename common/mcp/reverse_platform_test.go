package mcp

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestDNSLogDomainResult(t *testing.T) {
	rsp := &ypb.DNSLogRootDomain{Domain: "a.example.com", Token: "abc"}
	out := dnsLogDomainResult(rsp, false, true, true, "dnslog.cn")
	require.Equal(t, "a.example.com", out["Domain"])
	require.Equal(t, "abc", out["Token"])
	require.Equal(t, true, out["fallbackToRemote"])
	require.Equal(t, false, out["useLocal"])
	require.Equal(t, true, out["useRemote"])
	require.Equal(t, "dnslog.cn", out["dnsMode"])
}

func TestDNSLogQueryResult(t *testing.T) {
	rsp := &ypb.QueryDNSLogByTokenResponse{}
	out := dnsLogQueryResult(rsp, false, true, true)
	require.Equal(t, true, out["fallbackToRemote"])
	require.Contains(t, out, "Events")
}
