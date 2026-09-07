package mysql

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
)

// Prober 是 MySQL 最小认证探针。
type Prober struct{}

// Probe 执行一次 MySQL 认证。
//
// 传输策略：
//   - 明文 TCP 建立后读取 Initial Handshake；若服务端声明支持 CLIENT_SSL
//     且策略不禁止 TLS，则发送 SSLRequest 升级 TLS（证书校验按模块约定跳过）；
//   - TLS 升级失败时，若策略为 Opportunistic 则整轮明文重试（显式回退，
//     结果中 transport 如实记录）；TLSStrict 下直接以 TLSRequired 结束。
func (p Prober) Probe(ctx context.Context, target core.Target, cred core.Credential, opts core.Options) core.Result {
	hostport := withDefaultPort(target, 3306)

	res, usedTLS, tlsFailed := p.attempt(ctx, hostport, target, cred, opts, true /*tryMySQLTLS*/)
	if !tlsFailed || opts.TLSPolicy == core.TLSStrict || opts.TLSPolicy == core.PlaintextAllowed {
		if tlsFailed && opts.TLSPolicy == core.TLSStrict {
			res.Outcome = core.OutcomeTLSRequired
			res.Err = core.ErrTLSPolicyBlocked
		}
		return res
	}
	_ = usedTLS
	// TLS 升级失败：显式明文重试（Opportunistic 策略）。
	res, _, _ = p.attempt(ctx, hostport, target, cred, opts, false)
	return res
}

// attempt 执行完整握手。返回 (结果, 是否使用TLS, TLS是否升级失败)。
func (p Prober) attempt(ctx context.Context, hostport string, target core.Target, cred core.Credential, opts core.Options, tryMySQLTLS bool) (core.Result, bool, bool) {
	timeout := timeoutOf(opts)
	conn, transport, err := core.Dialer(ctx, hostport, core.PlaintextAllowed, timeout)
	if err != nil {
		return dialFailure(ctx, err, transport), false, false
	}
	defer core.SafeClose(conn)
	unwatch := core.WatchConn(ctx, conn)
	defer unwatch()
	core.SetDeadline(conn, ctx, timeout)

	pc := newPacketConn(conn)

	// 1. Initial Handshake
	greetPayload, err := pc.readPacket()
	if err != nil {
		return readFailure(ctx, err, transport), false, false
	}
	greet, err := parseGreeting(greetPayload)
	if err != nil {
		return core.Result{
			Outcome:   core.OutcomeProtocolMismatch,
			Transport: transport,
			Err:       core.ErrProtocolParse,
			ErrDetail: sanitizeErr(err),
		}, false, false
	}

	// 2. MySQL 原生 TLS 升级（凭证加密传输）。
	if tryMySQLTLS && greet.CapabilityFlags&clientSSL != 0 && opts.TLSPolicy != core.PlaintextAllowed {
		if err := pc.writePacket(buildSSLRequest(clientBaseCaps(greet.CapabilityFlags))); err != nil {
			return readFailure(ctx, err, transport), false, false
		}
		tlsConn := tls.Client(conn, &tls.Config{InsecureSkipVerify: true, ServerName: target.Host}) // #nosec G402 -- 弱口令探测约定跳过证书校验
		hctx, hcancel := context.WithTimeout(ctx, timeout)
		tlsErr := tlsConn.HandshakeContext(hctx)
		hcancel()
		if tlsErr != nil {
			// 连接已被 TLS 握手污染，交由上层明文整轮重试。
			return core.Result{
				Outcome:   core.OutcomeUnknown,
				Transport: core.TransportTLS,
				Err:       core.ErrHandshake,
				ErrDetail: "mysql TLS upgrade failed: " + sanitizeErr(tlsErr),
			}, false, true
		}
		conn = tlsConn
		transport = core.TransportTLS
		pc.setStream(tlsConn) // 序号必须跨 TLS 连续
		core.SetDeadline(conn, ctx, timeout)
	}

	// 3. HandshakeResponse
	plugin := greet.AuthPluginName
	if plugin == "" {
		plugin = pluginDefaultFallback
	}
	authResp, err := authResponse(plugin, greet.AuthPluginData, cred.Password, true)
	if err != nil {
		return core.Result{
			Outcome:   core.OutcomeProtocolMismatch,
			Transport: transport,
			Err:       core.ErrProtocolParse,
			ErrDetail: sanitizeErr(err),
		}, transport == core.TransportTLS, false
	}
	if err := pc.writePacket(buildHandshakeResponse(clientBaseCaps(greet.CapabilityFlags), cred.Username, authResp, plugin)); err != nil {
		return readFailure(ctx, err, transport), transport == core.TransportTLS, false
	}

	// 4. 认证交互循环
	res := p.authLoop(ctx, pc, transport, greet, plugin, cred)
	if res.Outcome == core.OutcomeAuthSuccess && len(res.Extra) == 0 {
		res.Extra = []byte("server " + greet.ServerVersion)
	}
	return res, transport == core.TransportTLS, false
}

// authLoop 处理认证结果包：OK / ERR / AuthSwitch / AuthMoreData。
func (p Prober) authLoop(ctx context.Context, pc *packetConn, transport core.Transport, greet *greeting, plugin string, cred core.Credential) core.Result {
	nonce := greet.AuthPluginData
	switches := 0
	for {
		payload, err := pc.readPacket()
		if err != nil {
			return readFailure(ctx, err, transport)
		}
		if len(payload) == 0 {
			return core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: "empty auth packet"}
		}
		switch payload[0] {
		case iOK:
			return core.Result{
				Outcome:   core.OutcomeAuthSuccess,
				Transport: transport,
				Extra:     []byte("server " + greet.ServerVersion),
			}
		case iERR:
			srvErr, perr := parseErrPacket(payload)
			if perr != nil {
				return core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: sanitizeErr(perr)}
			}
			return classifyServerError(srvErr, transport)
		case iAuthMoreData: // 0x01：data（快路径/全量认证指示）已在 payload 中
			res, err := p.handleAuthMoreData(ctx, pc, transport, nonce, plugin, cred, payload[1:])
			if err != nil {
				if errors.Is(err, errOutcomeReady) {
					return res
				}
				return readFailure(ctx, err, transport)
			}
			continue
		case iEOF: // 0xFE
			if len(payload) == 1 {
				// Protocol::OldAuthSwitchRequest：pre-4.1 认证。
				return core.Result{
					Outcome:   core.OutcomeAuthFailed,
					Transport: transport,
					Err:       core.ErrAuthRejected,
					ErrDetail: "server requires pre-4.1 auth (unsupported)",
				}
			}
			// AuthSwitchRequest: plugin NUL + nonce
			if switches >= 2 {
				return core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: "too many auth switches"}
			}
			switches++
			end := indexByte(payload[1:], 0)
			if end < 0 {
				return core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: "malformed auth switch"}
			}
			newPlugin := string(payload[1 : 1+end])
			newNonce := payload[1+end+1:]
			if len(newNonce) > 0 && newNonce[len(newNonce)-1] == 0 {
				newNonce = newNonce[:len(newNonce)-1]
			}
			resp, err := authResponse(newPlugin, newNonce, cred.Password, true)
			if err != nil {
				return core.Result{
					Outcome:   core.OutcomeProtocolMismatch,
					Transport: transport,
					Err:       core.ErrProtocolParse,
					ErrDetail: sanitizeErr(err),
				}
			}
			nonce, plugin = newNonce, newPlugin
			if err := pc.writePacket(resp); err != nil {
				return readFailure(ctx, err, transport)
			}
		default:
			return core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: "unexpected packet header"}
		}
	}
}

// errOutcomeReady 表示 handleAuthMoreData 已产出最终结果。
var errOutcomeReady = errors.New("mysql: outcome ready")

// handleAuthMoreData 处理 caching_sha2_password / sha256_password 的中间包。
// data 是 AuthMoreData 包的载荷（首字节为快路径/全量认证指示）。
// 返回 errOutcomeReady 时 res 为最终结果。
func (p Prober) handleAuthMoreData(ctx context.Context, pc *packetConn, transport core.Transport, nonce []byte, plugin string, cred core.Credential, data []byte) (core.Result, error) {
	final := func(payload []byte) (core.Result, error) {
		if len(payload) == 0 {
			return core.Result{}, errMalformedPacket
		}
		switch payload[0] {
		case iOK:
			return core.Result{Outcome: core.OutcomeAuthSuccess, Transport: transport}, errOutcomeReady
		case iERR:
			srvErr, perr := parseErrPacket(payload)
			if perr != nil {
				return core.Result{}, perr
			}
			return classifyServerError(srvErr, transport), errOutcomeReady
		default:
			return core.Result{}, errMalformedPacket
		}
	}

	if plugin != pluginCachingSHA2 && plugin != pluginSHA256Password {
		return core.Result{}, errMalformedPacket
	}
	if len(data) == 0 {
		// 空载荷：某些实现直接回 OK/ERR。
		next, err := pc.readPacket()
		if err != nil {
			return core.Result{}, err
		}
		return final(next)
	}
	switch data[0] {
	case cachingFastAuth: // 0x03 快路径确认 → 等 OK/ERR
		next, err := pc.readPacket()
		if err != nil {
			return core.Result{}, err
		}
		return final(next)
	case cachingFullAuth: // 0x04 全量认证
		if transport == core.TransportTLS {
			// TLS 信道：明文密码。
			if err := pc.writePacket(append([]byte(cred.Password), 0)); err != nil {
				return core.Result{}, err
			}
		} else {
			// 明文信道：请求公钥 → RSA-OAEP 加密。
			if len(nonce) < 20 {
				return core.Result{}, errMalformedPacket
			}
			if err := pc.writePacket([]byte{cachingReqKey}); err != nil {
				return core.Result{}, err
			}
			keyPayload, err := pc.readPacket()
			if err != nil {
				return core.Result{}, err
			}
			if len(keyPayload) == 0 || keyPayload[0] != iAuthMoreData {
				return core.Result{}, errMalformedPacket
			}
			pub, err := parseRSAPublicKey(keyPayload[1:])
			if err != nil {
				return core.Result{}, err
			}
			enc, err := encryptPasswordRSA(cred.Password, nonce[:20], pub)
			if err != nil {
				return core.Result{}, err
			}
			if err := pc.writePacket(enc); err != nil {
				return core.Result{}, err
			}
		}
		next, err := pc.readPacket()
		if err != nil {
			return core.Result{}, err
		}
		return final(next)
	default:
		return core.Result{}, errMalformedPacket
	}
}

// clientBaseCaps 依据服务端能力协商客户端能力位。
func clientBaseCaps(serverCaps uint32) uint32 {
	caps := clientProtocol41 |
		clientSecureConn |
		clientLongPassword |
		clientLongFlag |
		clientTransactions |
		clientPluginAuth |
		clientMultiResults |
		clientCanHandleExpiredPasswords |
		clientLocalFiles
	// 客户端核心位必须保留（4.1 协议 / 安全连接），其余只保留服务端支持的。
	return (caps & serverCaps) | clientProtocol41 | clientSecureConn | clientLongPassword
}

// classifyServerError 把 MySQL ERR 包映射为结构化结果。
func classifyServerError(srvErr *serverError, transport core.Transport) core.Result {
	res := core.Result{Transport: transport, ErrDetail: sanitizeErr(srvErr)}
	switch srvErr.Code {
	case 1045, 1044, 1698, 3118: // Access denied / auth_socket / SRP 拒绝
		res.Outcome = core.OutcomeAuthFailed
		res.Err = core.ErrAuthRejected
	case 1129: // Host blocked because of many connection errors
		res.Outcome = core.OutcomeRateLimited
		res.Err = core.ErrAuthRejected
		res.RetryAfter = 30 * time.Second
	case 1040, 1203, 1226: // Too many connections / max_user_connections
		res.Outcome = core.OutcomeRateLimited
		res.Err = core.ErrAuthRejected
		res.RetryAfter = 5 * time.Second
	case 1130: // Host is not allowed to connect
		res.Outcome = core.OutcomeTargetUnavailable
		res.Err = core.ErrAuthRejected
	case 1251, 2061, 2062, 2065, 2066: // 认证协议/插件协商错误
		res.Outcome = core.OutcomeProtocolMismatch
		res.Err = core.ErrHandshake
	default:
		res.Outcome = core.OutcomeUnknown
		res.Err = core.ErrAuthRejected
	}
	return res
}

// ---- 工具函数 ----

func withDefaultPort(target core.Target, port int) string {
	if target.Port > 0 {
		return net.JoinHostPort(target.Host, strconv.Itoa(target.Port))
	}
	return net.JoinHostPort(target.Host, strconv.Itoa(port))
}

func timeoutOf(opts core.Options) time.Duration {
	if opts.Timeout > 0 {
		return opts.Timeout
	}
	return core.DefaultTimeout
}

func dialFailure(ctx context.Context, err error, transport core.Transport) core.Result {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return core.Result{Outcome: core.OutcomeCancelled, Transport: transport, Err: core.ErrCancelled}
	}
	return core.Result{Outcome: core.OutcomeTargetUnavailable, Transport: transport, Err: core.ErrDial, ErrDetail: sanitizeErr(err)}
}

func readFailure(ctx context.Context, err error, transport core.Transport) core.Result {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return core.Result{Outcome: core.OutcomeCancelled, Transport: transport, Err: core.ErrDeadline}
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return core.Result{Outcome: core.OutcomeTargetUnavailable, Transport: transport, Err: core.ErrIO, ErrDetail: "server closed during handshake"}
	}
	if errors.Is(err, errMalformedPacket) || strings.Contains(err.Error(), errMalformedPacket.Error()) {
		return core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: sanitizeErr(err)}
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return core.Result{Outcome: core.OutcomeTargetUnavailable, Transport: transport, Err: core.ErrDeadline, ErrDetail: "i/o timeout"}
	}
	return core.Result{Outcome: core.OutcomeTargetUnavailable, Transport: transport, Err: core.ErrIO, ErrDetail: sanitizeErr(err)}
}

// sanitizeErr 确保错误文本不含凭证且长度可控。探针自身不注入密码，此处为防御。
func sanitizeErr(err error) string {
	msg := err.Error()
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return msg
}
