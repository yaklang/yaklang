package bruteutils

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
	"github.com/yaklang/yaklang/common/utils/bruteutils/internal/oracleprobe"
)

var oracleServiceNames = []string{"orcl", "xe", "oracle", "orclpdb1", "xepdb1", "freepdb1", "free"}

var oracleAuth = &DefaultServiceAuthInfo{
	ServiceName: "oracle", DefaultPorts: "1521",
	DefaultUsernames: []string{"sys", "system", "oracle"},
	DefaultPasswords: []string{"sys", "sys123", "system", "password", "123qwe", "123456", "oracle", "oracle001", "oracle.com", "admin123..", "admin", "root"},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		return oracleBrutePassDetailed(i, func(ctx context.Context, o oracleprobe.Options) (oracleprobe.Details, error) {
			return oracleprobe.ProbeDetailed(ctx, defaultDialer, o)
		})
	},
}

// OracleAttempt is bounded diagnostic evidence, not a raw server error/log.
type OracleAttempt struct {
	Service string             `json:"service"`
	Status  oracleprobe.Status `json:"status"`
	Code    int                `json:"ora_code,omitempty"`
	Cached  bool               `json:"cached,omitempty"`
	oracleprobe.Details
}

// OracleLoginInfo is encoded in ExtraInfo for successes AND failures.
type OracleLoginInfo struct {
	Protocol           string             `json:"protocol"`
	RequestedTransport string             `json:"requested_transport"`
	SID                bool               `json:"sid"`
	SysDBA             bool               `json:"sysdba"`
	Status             oracleprobe.Status `json:"status"`
	Attempts           []OracleAttempt    `json:"attempts"`
}

type oracleDetailedProbe func(context.Context, oracleprobe.Options) (oracleprobe.Details, error)

// Keep the narrow error-only adapter for consumers/tests that inject a probe.
func oracleBrutePass(i *BruteItem, probe func(context.Context, oracleprobe.Options) error) *BruteItemResult {
	return oracleBrutePassDetailed(i, func(ctx context.Context, o oracleprobe.Options) (oracleprobe.Details, error) {
		return oracleprobe.Details{Stage: "auth-result"}, probe(ctx, o)
	})
}

func oracleBrutePassDetailed(i *BruteItem, probe oracleDetailedProbe) *BruteItemResult {
	res := i.Result()
	config := i.OracleConfig
	services, policy, timeout, err := config.normalize()
	info := OracleLoginInfo{Protocol: "oracle", RequestedTransport: "tcp", SysDBA: strings.EqualFold(i.Username, "sys")}
	if config != nil {
		if config.TLS != nil {
			info.RequestedTransport = "tcps"
		}
		info.SID = config.SID
		if config.SysDBA != nil {
			info.SysDBA = *config.SysDBA
		}
	}
	if err != nil {
		info.Status = oracleprobe.InvalidOptions
		res.Finished = true
		return finishOracleResult(res, info)
	}
	ctx, cancel := context.WithTimeout(itemCtx(i), timeout)
	defer cancel()
	cache, _ := ctx.Value(oracleCacheKey{}).(*oracleServiceCache)
	address := appendDefaultPort(i.Target, 1521)
	// Prioritize known services without assuming the listener has only one.
	ordered := make([]string, 0, len(services))
	remaining := make([]string, 0, len(services))
	for _, service := range services {
		// Read each observation once: another worker can discover a service or
		// its TTL can expire while we build this ordering. Two independent passes
		// could otherwise omit or duplicate a candidate between those changes.
		v, ok := cache.observation(oracleServiceKey{i.Target, service, info.SID})
		if ok && !v.unknown {
			ordered = append(ordered, service)
		} else {
			remaining = append(remaining, service)
		}
	}
	ordered = append(ordered, remaining...)
	allFinal, allUserBlocked, sawUser := true, true, false
	retryBudget := 1
	info.Status = oracleprobe.ServiceUnknown
	for _, service := range ordered {
		if ctx.Err() != nil {
			info.Status = oracleprobe.Classify(ctx.Err())
			allFinal = false
			allUserBlocked = false
			break
		}
		key := oracleServiceKey{i.Target, service, info.SID}
		if v, ok := cache.observation(key); ok && v.unknown {
			info.Attempts = append(info.Attempts, OracleAttempt{Service: service, Status: oracleprobe.ServiceUnknown, Cached: true, Details: oracleprobe.Details{Stage: "connect"}})
			continue
		}
		o := oracleprobe.Options{Address: address, Service: service, Username: i.Username, Password: i.Password, SysDBA: info.SysDBA, SID: info.SID, Timeout: timeout, Encryption: policy}
		if config != nil {
			o.TLS = config.TLS
		}
		var status oracleprobe.Status
		for {
			details, e := probe(ctx, o)
			status = oracleprobe.Classify(e)
			attempt := OracleAttempt{Service: service, Status: status, Details: details}
			var ora *oracleprobe.Error
			if errors.As(e, &ora) {
				attempt.Code = ora.Code
			}
			info.Attempts = append(info.Attempts, attempt)
			// Retry transport/capacity failures once per credential, never a password
			// rejection. Waiting and retrying consume the same service-list deadline.
			if status != oracleprobe.Unavailable || retryBudget == 0 || ctx.Err() != nil {
				break
			}
			retryBudget--
			timer := time.NewTimer(100 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
			case <-timer.C:
			}
			if ctx.Err() != nil {
				status = oracleprobe.Classify(ctx.Err())
				break
			}
		}
		switch status {
		case oracleprobe.Success:
			cache.remember(key, false)
			info.Status = status
			res.Ok = true
			res.Finished = true
			return finishOracleResult(res, info)
		case oracleprobe.ServiceUnknown:
			cache.remember(key, true)
			continue
		case oracleprobe.AccountLocked, oracleprobe.PasswordExpired:
			cache.remember(key, false)
			sawUser = true
			allFinal = false
		case oracleprobe.AuthRejected:
			cache.remember(key, false)
			allFinal = false
			allUserBlocked = false
		case oracleprobe.Unavailable, oracleprobe.Cancelled, oracleprobe.ServerError, oracleprobe.CredentialUnsupported, oracleprobe.AuthIncomplete:
			allFinal = false
			allUserBlocked = false
		default:
			allUserBlocked = false
		}
		// Preserve a credential-level result over failures of another service; the
		// per-service evidence still records every failure and its ORA code.
		if info.Status != oracleprobe.AuthRejected {
			info.Status = status
		}
	}
	if ctx.Err() != nil {
		info.Status = oracleprobe.Classify(ctx.Err())
		allFinal, allUserBlocked = false, false
	}
	res.Finished = allFinal || info.Status == oracleprobe.Cancelled
	res.UserEliminated = sawUser && allUserBlocked
	return finishOracleResult(res, info)
}

func finishOracleResult(res *BruteItemResult, info OracleLoginInfo) *BruteItemResult {
	res.ExtraInfo, _ = json.Marshal(info)
	r := &core.Result{Outcome: core.OutcomeUnknown, Transport: core.TransportUnknown, Extra: res.ExtraInfo, Err: core.ErrHandshake, UserEliminated: res.UserEliminated}
	switch {
	case res.Ok:
		r.Outcome = core.OutcomeAuthSuccess
		r.Err = core.ErrNone
	case info.Status == oracleprobe.Cancelled:
		r.Outcome = core.OutcomeCancelled
		r.Err = core.ErrCancelled
	case res.Finished:
		r.Outcome = core.OutcomeProtocolMismatch
		r.Err = core.ErrProtocolParse
	case info.Status == oracleprobe.AuthRejected:
		r.Outcome = core.OutcomeAuthFailed
		r.Err = core.ErrAuthRejected
	case info.Status == oracleprobe.Unavailable:
		r.Err = core.ErrIO
		r.RetryAfter = 200 * time.Millisecond
	}
	for j := len(info.Attempts) - 1; j >= 0; j-- {
		transport := info.Attempts[j].Transport
		if transport == "tcps" {
			r.Transport = core.TransportTLS
			break
		}
		if transport == "tcp" {
			r.Transport = core.TransportPlainTCP
			break
		}
	}
	res.ProbeResult = r
	return res
}
