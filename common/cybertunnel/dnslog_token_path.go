package cybertunnel

import (
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

type dnsLogTokenPath struct {
	useLocal bool
	dnsMode  string
}

var dnsLogTokenPathCache = utils.NewTTLCache[dnsLogTokenPath](2 * time.Hour)

// RequireDNSLogDomain issues a DNSLog domain via local broker or remote Bridge.
// When a third-party broker (e.g. dnslog.cn) is unreachable, it falls back to Bridge-native domains.
func RequireDNSLogDomain(remoteAddr string, useLocal bool, dnsMode string) (domain, token, mode string, err error) {
	issuedLocal := false
	if useLocal {
		domain, token, mode, err = RequireDNSLogDomainByLocal(dnsMode)
		if err != nil && shouldFallbackDNSLogBroker(err, true, dnsMode) {
			domain, token, mode, err = RequireDNSLogDomainByRemote(remoteAddr, "")
		} else if err == nil {
			issuedLocal = true
		}
	} else {
		domain, token, mode, err = RequireDNSLogDomainByRemote(remoteAddr, dnsMode)
		if err != nil && shouldFallbackDNSLogBroker(err, false, dnsMode) {
			domain, token, mode, err = RequireDNSLogDomainByRemote(remoteAddr, "")
		}
	}
	if err != nil {
		return "", "", "", err
	}
	RememberDNSLogTokenIssuance(token, issuedLocal, mode)
	return domain, token, mode, nil
}

// RememberDNSLogTokenIssuance records how a DNSLog token was issued so later queries
// can reject remote/local path mismatches instead of returning silent empty Events.
func RememberDNSLogTokenIssuance(token string, useLocal bool, dnsMode string) {
	if token == "" {
		return
	}
	dnsLogTokenPathCache.Set(token, dnsLogTokenPath{useLocal: useLocal, dnsMode: dnsMode})
}

// DNSLogTokenIssuedUseLocal reports the effective issuance path for a remembered token.
func DNSLogTokenIssuedUseLocal(token string) (useLocal bool, ok bool) {
	meta, ok := dnsLogTokenPathCache.Get(token)
	if !ok {
		return false, false
	}
	return meta.useLocal, true
}

// ValidateDNSLogQueryPath returns an error when query useLocal/dnsMode disagrees with
// how the token was issued via RequireDNSLogDomain. Unknown tokens are allowed.
func ValidateDNSLogQueryPath(token string, queryUseLocal bool, queryDNSMode string) error {
	meta, ok := dnsLogTokenPathCache.Get(token)
	if !ok {
		return nil
	}
	if meta.useLocal != queryUseLocal {
		issued := "remote Bridge (useLocal=false)"
		queried := "local broker (useLocal=true)"
		if meta.useLocal {
			issued = "local broker (useLocal=true)"
			queried = "remote Bridge (useLocal=false)"
		}
		return utils.Errorf(
			"dnslog query path mismatch: token %q was issued via %s but query uses %s; use the same useLocal/dnsMode as require_dnslog_domain",
			token, issued, queried,
		)
	}
	if meta.useLocal && meta.dnsMode != "" && queryDNSMode != "" && meta.dnsMode != queryDNSMode {
		return utils.Errorf(
			"dnslog query dnsMode mismatch: token %q was issued with dnsMode %q but query uses %q",
			token, meta.dnsMode, queryDNSMode,
		)
	}
	return nil
}

// ShouldFallbackFromLocalDNSLogBroker reports whether a DNSLog broker failure should
// trigger fallback to remote Bridge (dnslog.cn EOF, missing broker, etc.).
func ShouldFallbackFromLocalDNSLogBroker(err error) bool {
	return isDNSLogBrokerFailure(err)
}

func shouldFallbackDNSLogBroker(err error, useLocal bool, dnsMode string) bool {
	if !isDNSLogBrokerFailure(err) {
		return false
	}
	if useLocal {
		return true
	}
	return dnsMode != ""
}

func isDNSLogBrokerFailure(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "dnsbroker") ||
		strings.Contains(msg, "fetch dnslog.cn") ||
		strings.Contains(msg, "require[dnslog.cn]") ||
		(strings.Contains(msg, "require[") && strings.Contains(msg, "dnslog failed"))
}
