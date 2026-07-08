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

func TestGlobalReverseServerResultConfiguredVsEffective(t *testing.T) {
	t.Setenv("YAK_BRIDGE_LOCAL_REVERSE_ADDR", "127.0.0.1:62075")

	rsp := &ypb.GetGlobalReverseServerResponse{
		LocalReverseAddr: "",
		LocalReversePort: 62075,
	}
	out := globalReverseServerResult(rsp)
	require.Equal(t, "", out["ConfiguredLocalAddr"])
	require.Equal(t, "", out["LocalReverseAddr"])
	require.Equal(t, "127.0.0.1", out["EffectiveLocalReverseAddr"])
	require.Equal(t, "127.0.0.1:62075", out["LocalReverseListener"])
	require.Contains(t, out["note"], "ConfiguredLocalAddr is empty")
}

func TestGlobalReverseServerResultConfiguredSet(t *testing.T) {
	t.Setenv("YAK_BRIDGE_LOCAL_REVERSE_ADDR", "192.168.1.8:62075")

	rsp := &ypb.GetGlobalReverseServerResponse{
		LocalReverseAddr: "192.168.1.8",
		LocalReversePort: 62075,
	}
	out := globalReverseServerResult(rsp)
	require.Equal(t, "192.168.1.8", out["ConfiguredLocalAddr"])
	require.Equal(t, "192.168.1.8", out["EffectiveLocalReverseAddr"])
	require.NotContains(t, out, "note")
}
