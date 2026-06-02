package netx

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	utls "github.com/refraction-networking/utls"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	tlsErrorKindUnknown     = "unknown"
	tlsErrorKindNotTLS      = "not_tls"
	tlsErrorKindHandshake   = "handshake"
	tlsErrorKindALPN        = "alpn"
	tlsErrorKindSNI         = "sni"
	tlsErrorKindVersion     = "version"
	tlsErrorKindCertificate = "certificate"
)

type tlsRetryAttempt struct {
	name       string
	reason     string
	suggestion string
	err        error
	success    bool
	duration   time.Duration
}

type tlsRetryError struct {
	target     string
	kind       string
	attempts   []tlsRetryAttempt
	suggestion string
}

func (e *tlsRetryError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("tls retry failed for %s: %v", e.target, e.LastError())
}

func (e *tlsRetryError) LastError() error {
	if e == nil {
		return nil
	}
	for i := len(e.attempts) - 1; i >= 0; i-- {
		if e.attempts[i].err != nil {
			return e.attempts[i].err
		}
	}
	return nil
}

func (e *tlsRetryError) Unwrap() error {
	return e.LastError()
}

func (e *tlsRetryError) RetrySummary() string {
	if e == nil {
		return ""
	}
	var retryNames []string
	for _, attempt := range e.attempts {
		if strings.Contains(attempt.name, "baseline") {
			continue
		}
		retryNames = append(retryNames, attempt.name)
	}
	if len(retryNames) == 0 {
		return "未进行 TLS 兼容重试"
	}
	return fmt.Sprintf("已进行 %d 次 TLS 兼容重试：%s", len(retryNames), strings.Join(retryNames, ", "))
}

func (e *tlsRetryError) Reason() string {
	if e == nil {
		return ""
	}
	switch e.kind {
	case tlsErrorKindNotTLS:
		return "目标端口疑似不是 TLS 服务"
	case tlsErrorKindCertificate:
		return "TLS 证书/客户端证书问题"
	case tlsErrorKindSNI:
		return "TLS SNI 可能不匹配"
	case tlsErrorKindVersion:
		return "TLS 版本不兼容"
	case tlsErrorKindALPN:
		return "TLS ALPN 不兼容"
	case tlsErrorKindHandshake:
		return "TLS 握手失败"
	default:
		return "TLS 连接失败"
	}
}

type tlsRetryCandidate struct {
	name       string
	reason     string
	tip        string
	suggestion string
	sni        string
	nextProtos []string
	spec       *utls.ClientHelloSpec
	config     func(*gmtls.Config)
}

func classifyTLSError(err error) (kind string, retryable bool, suggestion string) {
	if err == nil {
		return tlsErrorKindUnknown, false, ""
	}
	msg := strings.ToLower(err.Error())

	switch {
	case strings.Contains(msg, "first record does not look like a tls handshake"),
		strings.Contains(msg, "server gave http response to https client"),
		strings.Contains(msg, "not tls conn"),
		strings.Contains(msg, "plain http"),
		strings.Contains(msg, "http/1.0"),
		strings.Contains(msg, "http/1.1"),
		strings.Contains(msg, "http/2"):
		return tlsErrorKindNotTLS, false, "目标端口疑似不是 TLS 服务，请确认协议是否应为 HTTP/明文 TCP，或检查端口是否正确；如果是通过代理访问，也可能是代理返回了明文响应，请检查代理配置"
	case strings.Contains(msg, "certificate required"),
		strings.Contains(msg, "bad certificate"),
		strings.Contains(msg, "unknown ca"),
		strings.Contains(msg, "alert(116)"):
		return tlsErrorKindCertificate, false, "目标可能要求客户端证书，请配置 P12/客户端证书后重试；如果是证书校验问题，请检查 CA 或跳过证书校验配置"
	case strings.Contains(msg, "unrecognized_name"),
		strings.Contains(msg, "unrecognized name"),
		strings.Contains(msg, "certificate is valid for"),
		strings.Contains(msg, "hostname"):
		return tlsErrorKindSNI, true, "TLS SNI 可能不匹配，请检查目标域名、Host 与 SNI 配置；如果使用 IP 直连，可尝试指定正确 SNI"
	case strings.Contains(msg, "protocol version"),
		strings.Contains(msg, "unsupported protocol"),
		strings.Contains(msg, "no supported versions"),
		strings.Contains(msg, "server selected unsupported protocol version"):
		return tlsErrorKindVersion, true, "TLS 版本不兼容，请检查全局 TLS 版本配置，目标可能只支持较旧 TLS 版本"
	case strings.Contains(msg, "no application protocol"),
		strings.Contains(msg, "no_application_protocol"),
		strings.Contains(msg, "alpn"):
		return tlsErrorKindALPN, true, "目标可能不兼容当前 ALPN，可尝试仅使用 http/1.1 或关闭 ALPN"
	case strings.Contains(msg, "handshake failure"),
		strings.Contains(msg, "decode error"),
		strings.Contains(msg, "illegal parameter"),
		strings.Contains(msg, "unexpected eof"),
		strings.Contains(msg, "connection reset"):
		return tlsErrorKindHandshake, true, "TLS 握手失败，可能是服务端不兼容 Go 默认 TLS 指纹，可尝试开启/调整 TLS 指纹；如果目标是国密站点，可尝试启用 GMTLS"
	case errors.Is(err, io.EOF):
		return tlsErrorKindHandshake, true, "TLS 握手阶段连接被关闭，可能是服务端不兼容当前 TLS 参数；请确认协议、端口、代理与 TLS 指纹配置"
	case strings.Contains(msg, "certificate"), strings.Contains(msg, "x509"):
		return tlsErrorKindCertificate, false, "目标可能要求客户端证书，请配置 P12/客户端证书后重试；如果是证书校验问题，请检查 CA 或跳过证书校验配置"
	default:
		return tlsErrorKindUnknown, true, "TLS 握手失败，可尝试调整 TLS 指纹、ALPN、SNI 或 TLS 版本配置"
	}
}

func dialTLSWithRetry(
	target string,
	config *dialXConfig,
	tlsConfig *gmtls.Config,
	sni string,
	tlsTimeout time.Duration,
	clientHelloSpec *utls.ClientHelloSpec,
	strategy TLSStrategy,
) (net.Conn, time.Duration, error) {
	var attempts []tlsRetryAttempt
	var totalHandshakeDuration time.Duration

	baseConn, err := dialPlainTCPConnWithRetry(target, config)
	if err != nil {
		return nil, 0, err
	}
	baseConfig := tlsConfig.Clone()
	baseConfig.GMSupport = nil

	startUpgrade := time.Now()
	tlsConn, err := UpgradeToTLSConnectionWithTimeout(baseConn, sni, baseConfig, tlsTimeout, clientHelloSpec, config.TLSNextProto...)
	handshakeDuration := time.Since(startUpgrade)
	totalHandshakeDuration += handshakeDuration
	if err == nil {
		return tlsConn, totalHandshakeDuration, nil
	}
	_ = baseConn.Close()
	kind, retryable, suggestion := classifyTLSError(err)
	attempts = append(attempts, tlsRetryAttempt{
		name:       fmt.Sprintf("%s-baseline", strategy),
		reason:     "baseline tls handshake",
		suggestion: suggestion,
		err:        err,
		duration:   handshakeDuration,
	})

	if !retryable {
		addTLSNoRetryTip(config.TraceInfo, kind, suggestion)
		return nil, totalHandshakeDuration, &tlsRetryError{target: target, kind: kind, attempts: attempts, suggestion: suggestion}
	}

	candidates := buildTLSRetryCandidates(target, config, tlsConfig, sni, clientHelloSpec, kind, suggestion)
	if len(candidates) == 0 {
		addTLSNoRetryTip(config.TraceInfo, kind, suggestion)
		return nil, totalHandshakeDuration, &tlsRetryError{target: target, kind: kind, attempts: attempts, suggestion: suggestion}
	}

	for _, candidate := range candidates {
		if config.Debug {
			log.Infof("dial %v tls retry candidate %s: %s", target, candidate.name, candidate.reason)
		}
		if config.TraceInfo != nil {
			config.TraceInfo.TLSRetryCount++
		}
		config.TraceInfo.AddTLSRetryTip(candidate.tip)

		conn, err := dialPlainTCPConnWithRetry(target, config)
		if err != nil {
			attempts = append(attempts, tlsRetryAttempt{
				name:       candidate.name,
				reason:     candidate.reason,
				suggestion: candidate.suggestion,
				err:        err,
			})
			continue
		}

		tempTlsConfig := tlsConfig.Clone()
		tempTlsConfig.GMSupport = nil
		if candidate.config != nil {
			candidate.config(tempTlsConfig)
		}

		startUpgrade := time.Now()
		tlsConn, err = UpgradeToTLSConnectionWithTimeout(conn, candidate.sni, tempTlsConfig, tlsTimeout, candidate.spec, candidate.nextProtos...)
		handshakeDuration = time.Since(startUpgrade)
		totalHandshakeDuration += handshakeDuration
		attempt := tlsRetryAttempt{
			name:       candidate.name,
			reason:     candidate.reason,
			suggestion: candidate.suggestion,
			err:        err,
			success:    err == nil,
			duration:   handshakeDuration,
		}
		attempts = append(attempts, attempt)
		if err == nil {
			return tlsConn, totalHandshakeDuration, nil
		}
		_ = conn.Close()
	}

	return nil, totalHandshakeDuration, &tlsRetryError{target: target, kind: kind, attempts: attempts, suggestion: suggestion}
}

func addTLSNoRetryTip(traceInfo *DialXTraceInfo, kind, suggestion string) {
	switch kind {
	case tlsErrorKindNotTLS:
		traceInfo.AddTLSRetryTip("目标端口疑似非 TLS 服务，未继续进行 TLS 兼容重试；请确认协议或端口")
	case tlsErrorKindCertificate:
		traceInfo.AddTLSRetryTip("TLS 握手失败，目标可能要求客户端证书，未进行 TLS 指纹重试；请配置客户端证书")
	case tlsErrorKindSNI:
		traceInfo.AddTLSRetryTip("TLS SNI 可能不匹配，未静默覆盖用户配置；请检查目标域名、Host 与 SNI 配置")
	case tlsErrorKindVersion:
		traceInfo.AddTLSRetryTip("TLS 版本不兼容，未进行 TLS 版本重试；请检查全局 TLS 版本配置")
	default:
		if suggestion != "" {
			traceInfo.AddTLSRetryTip("TLS 握手失败，未进行 TLS 兼容重试；" + suggestion)
		}
	}
}

func buildTLSRetryCandidates(
	target string,
	config *dialXConfig,
	tlsConfig *gmtls.Config,
	sni string,
	clientHelloSpec *utls.ClientHelloSpec,
	kind string,
	suggestion string,
) []tlsRetryCandidate {
	candidates := make([]tlsRetryCandidate, 0, 3)
	added := make(map[string]struct{})
	add := func(candidate tlsRetryCandidate) {
		if len(candidates) >= 3 {
			return
		}
		key := tlsRetryCandidateKey(candidate)
		if _, ok := added[key]; ok {
			return
		}
		added[key] = struct{}{}
		candidates = append(candidates, candidate)
	}

	addTLS12Candidate := func() {
		if tlsConfig.MinVersion > gmtls.VersionTLS12 || tlsConfig.MaxVersion < gmtls.VersionTLS12 {
			config.TraceInfo.AddTLSRetryTip("TLS 版本不兼容，未尝试 TLS1.2 only；当前全局 TLS 版本配置不允许 TLS1.2")
			return
		}
		add(tlsRetryCandidate{
			name:       "tls12-only",
			reason:     "retry with TLS1.2 only",
			tip:        "TLS 版本不兼容，已尝试 TLS1.2 only",
			suggestion: suggestion,
			sni:        sni,
			nextProtos: cloneStringSlice(config.TLSNextProto),
			spec:       clientHelloSpec,
			config: func(c *gmtls.Config) {
				c.MinVersion = gmtls.VersionTLS12
				c.MaxVersion = gmtls.VersionTLS12
			},
		})
	}

	addChromeCandidate := func() {
		spec, err := utls.UTLSIdToSpec(utls.HelloChrome_120)
		if err != nil {
			config.TraceInfo.AddTLSRetryTip("TLS 握手失败，生成 Chrome TLS 指纹失败，未进行指纹重试")
			return
		}
		reason := "retry with Chrome TLS fingerprint"
		if clientHelloSpec != nil {
			reason = "fallback from custom ClientHello to Chrome TLS fingerprint"
		}
		add(tlsRetryCandidate{
			name:       "chrome-client-hello",
			reason:     reason,
			tip:        "TLS 握手失败，已尝试 Chrome TLS 指纹重试",
			suggestion: suggestion,
			sni:        sni,
			nextProtos: cloneStringSlice(config.TLSNextProto),
			spec:       &spec,
		})
	}

	addALPNCandidates := func() {
		if !containsString(config.TLSNextProto, "h2") {
			return
		}
		add(tlsRetryCandidate{
			name:       "alpn-http11",
			reason:     "retry with ALPN http/1.1 only",
			tip:        "TLS 握手失败，已尝试调整 ALPN 为 http/1.1",
			suggestion: "目标可能不兼容当前 ALPN，可尝试仅使用 http/1.1 或关闭 ALPN",
			sni:        sni,
			nextProtos: []string{"http/1.1"},
			spec:       clientHelloSpec,
			config: func(c *gmtls.Config) {
				c.NextProtos = []string{"http/1.1"}
			},
		})
		add(tlsRetryCandidate{
			name:       "alpn-disabled",
			reason:     "retry without ALPN",
			tip:        "TLS 握手失败，已尝试移除 ALPN",
			suggestion: "目标可能不兼容当前 ALPN，可尝试仅使用 http/1.1 或关闭 ALPN",
			sni:        sni,
			nextProtos: nil,
			spec:       clientHelloSpec,
			config: func(c *gmtls.Config) {
				c.NextProtos = nil
			},
		})
	}

	addSNICandidate := func() {
		if config.ShouldOverrideSNI {
			config.TraceInfo.AddTLSRetryTip("TLS SNI 可能不匹配，用户已显式设置 SNI，未静默覆盖；请检查目标域名、Host 与 SNI 配置")
			return
		}
		host := utils.ExtractHost(target)
		nextSNI := ""
		if sni == "" && host != "" {
			nextSNI = host
		} else if sni != "" {
			nextSNI = ""
		}
		if nextSNI == sni {
			return
		}
		add(tlsRetryCandidate{
			name:       "sni-adjusted",
			reason:     "retry with adjusted SNI",
			tip:        "TLS 握手失败，已尝试调整 SNI",
			suggestion: "TLS SNI 可能不匹配，请检查目标域名、Host 与 SNI 配置；如果使用 IP 直连，可尝试指定正确 SNI",
			sni:        nextSNI,
			nextProtos: cloneStringSlice(config.TLSNextProto),
			spec:       clientHelloSpec,
			config: func(c *gmtls.Config) {
				c.ServerName = nextSNI
			},
		})
	}

	switch kind {
	case tlsErrorKindVersion:
		addTLS12Candidate()
	case tlsErrorKindALPN:
		addALPNCandidates()
	case tlsErrorKindSNI:
		addSNICandidate()
	case tlsErrorKindHandshake, tlsErrorKindUnknown:
		addChromeCandidate()
		addALPNCandidates()
		addSNICandidate()
	}

	return candidates
}

func tlsRetryCandidateKey(candidate tlsRetryCandidate) string {
	specKey := "default"
	if candidate.spec != nil {
		specKey = "custom"
		if candidate.name == "chrome-client-hello" {
			specKey = "chrome120"
		}
	}
	return strings.Join([]string{
		candidate.sni,
		specKey,
		strings.Join(candidate.nextProtos, ","),
		candidate.name,
	}, "|")
}

func cloneStringSlice(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	ret := make([]string, len(v))
	copy(ret, v)
	return ret
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
