package cybertunnel

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func TestValidateDNSLogQueryPathMismatch(t *testing.T) {
	RememberDNSLogTokenIssuance("remote-token", false, "")
	err := ValidateDNSLogQueryPath("remote-token", true, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "path mismatch")
}

func TestValidateDNSLogQueryPathMatch(t *testing.T) {
	RememberDNSLogTokenIssuance("remote-token-2", false, "")
	require.NoError(t, ValidateDNSLogQueryPath("remote-token-2", false, ""))
}

func TestValidateDNSLogQueryPathUnknownToken(t *testing.T) {
	require.NoError(t, ValidateDNSLogQueryPath("never-seen-token", true, ""))
}

func TestValidateDNSLogQueryDNSModeMismatch(t *testing.T) {
	RememberDNSLogTokenIssuance("local-token", true, "dnslog.cn")
	err := ValidateDNSLogQueryPath("local-token", true, "other.cn")
	require.Error(t, err)
	require.Contains(t, err.Error(), "dnsMode mismatch")
}

func TestDNSLogTokenIssuedUseLocal(t *testing.T) {
	RememberDNSLogTokenIssuance("t-remote", false, "")
	issuedLocal, ok := DNSLogTokenIssuedUseLocal("t-remote")
	require.True(t, ok)
	require.False(t, issuedLocal)

	_, ok = DNSLogTokenIssuedUseLocal("missing")
	require.False(t, ok)
}

func TestShouldFallbackDNSLogBroker(t *testing.T) {
	brokerErr := utils.Errorf("require[dnslog.cn] dnslog failed: fetch dnslog.cn token failed: EOF")
	require.True(t, shouldFallbackDNSLogBroker(brokerErr, true, "dnslog.cn"))
	require.True(t, shouldFallbackDNSLogBroker(brokerErr, false, "dnslog.cn"))
	require.False(t, shouldFallbackDNSLogBroker(brokerErr, false, ""))
	require.False(t, shouldFallbackDNSLogBroker(utils.Errorf("connection refused"), true, "dnslog.cn"))
}

func TestShouldFallbackFromLocalDNSLogBroker(t *testing.T) {
	require.True(t, ShouldFallbackFromLocalDNSLogBroker(utils.Errorf("fetch dnslog.cn token failed: EOF")))
	require.True(t, ShouldFallbackFromLocalDNSLogBroker(utils.Errorf("get dnslog broker by local failed: dnsbroker [x] no existed")))
	require.False(t, ShouldFallbackFromLocalDNSLogBroker(utils.Errorf("connection refused")))
}
