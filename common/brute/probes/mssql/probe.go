package mssql

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
)

// Prober 是 MSSQL 最小认证探针。
type Prober struct{}

// netConnAdapter 把 io.ReadWriter 适配成 net.Conn（TLS-in-TDS 握手用）。
type netConnAdapter struct {
	rw interface {
		Read(p []byte) (int, error)
		Write(p []byte) (int, error)
	}
	raw net.Conn
}

func (a *netConnAdapter) Read(p []byte) (int, error)  { return a.rw.Read(p) }
func (a *netConnAdapter) Write(p []byte) (int, error) { return a.rw.Write(p) }
func (a *netConnAdapter) Close() error                { return a.raw.Close() }
func (a *netConnAdapter) LocalAddr() net.Addr         { return a.raw.LocalAddr() }
func (a *netConnAdapter) RemoteAddr() net.Addr        { return a.raw.RemoteAddr() }
func (a *netConnAdapter) SetDeadline(t time.Time) error {
	return a.raw.SetDeadline(t)
}
func (a *netConnAdapter) SetReadDeadline(t time.Time) error {
	return a.raw.SetReadDeadline(t)
}
func (a *netConnAdapter) SetWriteDeadline(t time.Time) error {
	return a.raw.SetWriteDeadline(t)
}

// tlsClient 创建跳过证书校验的 TLS 客户端（模块约定：弱口令探测不校验证书）。
func tlsClient(ctx context.Context, conn net.Conn, serverName string) *tls.Conn {
	_ = ctx
	return tls.Client(conn, &tls.Config{InsecureSkipVerify: true, ServerName: serverName, DynamicRecordSizingDisabled: true}) // #nosec G402 -- 探测场景刻意跳过证书校验
}

// Probe 执行一次 MSSQL SQL 认证。
//
// 流程：PRELOGIN（加密协商）→ [TDS 内嵌 TLS 升级，若服务端要求] → LOGIN7 → token 分类。
// 旧实现固定 encrypt=disable；本探针提议 encryptNotSup 但在服务端要求加密时
// 升级 TLS（InsecureSkipVerify），凭证不再明文暴露给强制加密的服务端。
func (p Prober) Probe(ctx context.Context, target core.Target, cred core.Credential, opts core.Options) core.Result {
	timeout := timeoutOf(opts)
	addr := withDefaultPort(target, 1433)

	conn, transport, err := core.Dialer(ctx, addr, core.PlaintextAllowed, timeout)
	if err != nil {
		return dialFailure(ctx, err, transport)
	}
	defer core.SafeClose(conn)
	unwatch := core.WatchConn(ctx, conn)
	defer unwatch()
	core.SetDeadline(conn, ctx, timeout)

	stream := newTDSStream(conn)

	// 1. PRELOGIN：提议明文（encryptNotSup），服务端要求时升级 TLS。
	stream.beginPacket(packPreLogin)
	stream.write(buildPrelogin(encryptNotSup, ""))
	if err := stream.finishPacket(); err != nil {
		return readFailure(ctx, err, transport)
	}
	_, payload, err := stream.readPacket()
	if err != nil {
		return readFailure(ctx, err, transport)
	}
	// 注意：真实 SQL Server 的 PRELOGIN 响应使用 REPLY(0x04) 包类型
	// （与 go-mssqldb 一致，不校验包类型，直接解析字段）。
	fields, err := parsePrelogin(payload)
	if err != nil {
		return core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: sanitize(err)}
	}
	encryptBytes, ok := fields[preloginENCRYPTION]
	if !ok || len(encryptBytes) == 0 {
		return core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: "server prelogin without encryption field"}
	}
	serverEncrypt := encryptBytes[0]

	// 2. TLS 升级（服务端要求加密时）。
	//    encryptOn(1)/encryptReq(3)：TDS 内嵌 TLS 握手；encryptOff(0)：登录加密。
	//    encryptNotSup(2)：纯明文。
	if serverEncrypt != encryptNotSup {
		if opts.TLSPolicy == core.PlaintextAllowed {
			// 策略禁止 TLS 而服务端要求：不得发送明文凭证。
			return core.Result{
				Outcome:   core.OutcomeTLSRequired,
				Transport: transport,
				Err:       core.ErrTLSPolicyBlocked,
				ErrDetail: "server requires encryption but plaintext policy set",
			}
		}
		if err := stream.upgradeTLS(ctx, target.Host, timeout); err != nil {
			return core.Result{
				Outcome:   core.OutcomeTLSRequired,
				Transport: transport,
				Err:       core.ErrTLSPolicyBlocked,
				ErrDetail: "TDS TLS upgrade failed: " + sanitize(err),
			}
		}
		transport = core.TransportTLS
		core.SetDeadline(stream.conn, ctx, timeout)
	}

	// 3. LOGIN7
	stream.beginPacket(packLogin7)
	stream.write(buildLogin7(cred.Username, cred.Password, target.Host))
	if err := stream.finishPacket(); err != nil {
		return readFailure(ctx, err, transport)
	}

	// 4. 读取响应 token 流
	return p.readAuthResult(ctx, stream, transport)
}

// readAuthResult 读取登录响应并分类。
func (p Prober) readAuthResult(ctx context.Context, stream *tdsStream, transport core.Transport) core.Result {
	for {
		pktType, payload, err := stream.readPacket()
		if err != nil {
			return readFailure(ctx, err, transport)
		}
		switch pktType {
		case packReply:
			srvErr, loginAck, err := parseTokens(payload)
			if err != nil {
				return core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: sanitize(err)}
			}
			if srvErr != nil {
				return classifyServerError(srvErr, transport)
			}
			if loginAck {
				return core.Result{Outcome: core.OutcomeAuthSuccess, Transport: transport}
			}
			// 无 LOGINACK 无 ERROR（INFO/ENVCHANGE 等）：继续读下一包。
		case packSSPI:
			return core.Result{
				Outcome:   core.OutcomeAuthFailed,
				Transport: transport,
				Err:       core.ErrAuthRejected,
				ErrDetail: "server requires Windows integrated auth (SSPI/NTLM)",
			}
		default:
			return core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: "unexpected response packet type"}
		}
	}
}

// unauthProbe 无凭证探测：MSSQL 不存在真正的匿名访问；PRELOGIN+空凭证登录探测可达性。
type unauthProbe struct{}

func (unauthProbe) Probe(ctx context.Context, target core.Target, cred core.Credential, opts core.Options) core.Result {
	// 空凭证 SQL 登录：服务端可达且允许 SQL 认证时会回 18456（凭证错误），
	// 而不是网络错误——这可以区分"服务可达"与"不可达"。
	empty := Prober{}.Probe(ctx, target, core.Credential{Username: "", Password: ""}, opts)
	switch empty.Outcome {
	case core.OutcomeAuthSuccess:
		return empty // 匿名可登（极罕见）
	case core.OutcomeAuthFailed, core.OutcomeMFARequired, core.OutcomeAccountLocked:
		// 服务可达：无未授权访问。
		return core.Result{Outcome: core.OutcomeAuthFailed, Transport: empty.Transport, Err: core.ErrAuthRejected}
	default:
		return empty
	}
}

// ServiceInfoFull 返回带未授权探测的服务描述。
func ServiceInfoFull() core.ServiceInfo {
	info := ServiceInfo()
	info.UnAuthProber = unauthProbe{}
	return info
}

// RegisterFull 注册带未授权探测的探针。
func RegisterFull() { core.Register(ServiceInfoFull()) }
