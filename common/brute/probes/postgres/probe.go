// Package postgres 实现最小化 PostgreSQL 认证探针。
//
// 支持：cleartext、MD5、SCRAM-SHA-256（无通道绑定）三种服务端认证方式；
// 只做 StartupMessage + PasswordMessage 交互，不引入 SQL 执行与驱动。
package postgres

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
	"github.com/yaklang/yaklang/common/brute/dicts"
	"github.com/yaklang/yaklang/common/brute/probes/internal/scram"
)

// Prober 是 PostgreSQL 最小认证探针。
type Prober struct{}

const protocolVersion = 196608 // 3.0

// 服务端消息类型。
const (
	msgAuth            = 'R'
	msgPassword        = 'p'
	msgError           = 'E'
	msgNotice          = 'N'
	msgParameterStatus = 'S'
	msgBackendKeyData  = 'K'
	msgReadyForQuery   = 'Z'
)

// 认证请求子类型。
const (
	authOK           = 0
	authCleartext    = 3
	authMD5          = 5
	authSASL         = 10
	authSASLContinue = 11
	authSASLFinal    = 12
	authKerberos     = 2
	authSCM          = 6
	authGSS          = 7
	authGSSContinue  = 8
	authSSPI         = 9
)

// pgError 是 ErrorResponse 的解析结果。
type pgError struct {
	Severity string
	Code     string // SQLSTATE
	Message  string
}

func (e *pgError) Error() string {
	return fmt.Sprintf("postgres %s %s: %s", e.Severity, e.Code, e.Message)
}

// Probe 执行一次 PostgreSQL 认证。
func (p Prober) Probe(ctx context.Context, target core.Target, cred core.Credential, opts core.Options) core.Result {
	timeout := timeoutOf(opts)
	addr := withDefaultPort(target, 5432)

	// PG 的 TLS 是协议级 SSLRequest 协商，TCP 层恒为明文拨号；
	// 协议级策略在下方处理（Strict/Opportunistic/Plaintext）。
	conn, transport, err := core.Dialer(ctx, addr, core.PlaintextAllowed, timeout)
	if err != nil {
		return dialFailure(ctx, err, transport)
	}
	defer core.SafeClose(conn)
	unwatch := core.WatchConn(ctx, conn)
	defer unwatch()
	core.SetDeadline(conn, ctx, timeout)

	// PostgreSQL TLS 走协议内 SSLRequest 协商（非 TCP 层探测式 TLS）。
	// Opportunistic：协商失败/不支持则回退明文并记录；Strict：不支持即终止。
	if opts.TLSPolicy != core.PlaintextAllowed {
		sslSupported := writeSSLRequest(conn)
		if !sslSupported {
			if opts.TLSPolicy == core.TLSStrict {
				return core.Result{
					Outcome:   core.OutcomeTLSRequired,
					Transport: transport,
					Err:       core.ErrTLSPolicyBlocked,
					ErrDetail: "postgres server does not support SSL",
				}
			}
			// Opportunistic：服务器拒绝 SSL 时连接仍可复用（未污染），
			// 继续明文协议并如实记录传输方式。
		} else {
			tlsConn, err := upgradeTLS(ctx, conn, target.Host, timeout)
			if err == nil {
				conn = tlsConn
				transport = core.TransportTLS
				core.SetDeadline(conn, ctx, timeout)
			} else if opts.TLSPolicy == core.TLSStrict {
				return core.Result{
					Outcome:   core.OutcomeTLSRequired,
					Transport: transport,
					Err:       core.ErrTLSPolicyBlocked,
					ErrDetail: "postgres TLS handshake failed: " + sanitize(err),
				}
			} else {
				// Opportunistic：握手失败，重建明文连接（原连接已污染）。
				core.SafeClose(conn)
				conn, transport, err = core.Dialer(ctx, addr, core.PlaintextAllowed, timeout)
				if err != nil {
					return dialFailure(ctx, err, transport)
				}
				defer core.SafeClose(conn)
				unwatch := core.WatchConn(ctx, conn)
				defer unwatch()
				core.SetDeadline(conn, ctx, timeout)
			}
		}
	}

	// StartupMessage
	if err := writeStartupMessage(conn, cred.Username, "postgres"); err != nil {
		return readFailure(ctx, err, transport)
	}

	return p.authExchange(ctx, conn, transport, cred)
}

// authExchange 处理服务端认证交互。
func (p Prober) authExchange(ctx context.Context, conn net.Conn, transport core.Transport, cred core.Credential) core.Result {
	for {
		msgType, payload, err := readMessage(conn)
		if err != nil {
			return readFailure(ctx, err, transport)
		}
		switch msgType {
		case msgError:
			pgErr := parseErrorResponse(payload)
			return classifyPgError(pgErr, transport)
		case msgNotice, msgParameterStatus, msgBackendKeyData:
			continue // 认证过程中可忽略的附加信息
		case msgReadyForQuery:
			// ReadyForQuery 即认证完成（SCRAM 路径已消费 authOK）。
			return core.Result{Outcome: core.OutcomeAuthSuccess, Transport: transport}
		case msgAuth:
			if len(payload) < 4 {
				return core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: "malformed auth message"}
			}
			authType := int(binary.BigEndian.Uint32(payload[:4]))
			switch authType {
			case authOK:
				// 认证成功：等待 ReadyForQuery 确认（跳过参数状态）。
				return p.awaitReady(ctx, conn, transport)
			case authCleartext:
				if err := writePasswordMessage(conn, cred.Password+"\x00"); err != nil {
					return readFailure(ctx, err, transport)
				}
			case authMD5:
				if len(payload) < 8 {
					return core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: "malformed md5 salt"}
				}
				if cred.Username == "" {
					// MD5 摘要绑定用户名；空用户名仍可发送（服务端会拒绝）。
				}
				resp := md5AuthResponse(cred.Username, cred.Password, payload[4:8])
				if err := writePasswordMessage(conn, resp+"\x00"); err != nil {
					return readFailure(ctx, err, transport)
				}
			case authSASL:
				res := p.doSCRAM(ctx, conn, transport, cred, payload[4:])
				if res != nil {
					return *res
				}
				// SCRAM 完成后循环继续等 authOK / error
			case authKerberos, authSCM, authGSS, authGSSContinue, authSSPI:
				return core.Result{
					Outcome:   core.OutcomeAuthFailed,
					Transport: transport,
					Err:       core.ErrAuthRejected,
					ErrDetail: fmt.Sprintf("unsupported server auth method %d", authType),
				}
			default:
				return core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: fmt.Sprintf("unknown auth type %d", authType)}
			}
		default:
			return core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: fmt.Sprintf("unexpected message type %q", msgType)}
		}
	}
}

// doSCRAM 执行 SCRAM-SHA-256 认证。
// 返回 nil 表示完成（外层继续读取结果），非 nil 为最终结果。
func (p Prober) doSCRAM(ctx context.Context, conn net.Conn, transport core.Transport, cred core.Credential, mechanisms []byte) *core.Result {
	found := false
	for _, m := range strings.Split(strings.TrimRight(string(mechanisms), "\x00"), "\x00") {
		if m == "SCRAM-SHA-256" {
			found = true
			break
		}
	}
	if !found {
		return &core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: "server does not offer SCRAM-SHA-256"}
	}

	// SASLprep 密码（PG 14+ 对 SCRAM 密码规范化；无法规范化时退回原文，与 libpq 一致）。
	password := cred.Password
	if prep, err := scram.SASLprep(cred.Password); err == nil {
		password = prep
	}

	client := scram.NewClient(true /* sha256 */)
	nonce := make([]byte, 18)
	if _, err := rand.Read(nonce); err != nil {
		return &core.Result{Outcome: core.OutcomeUnknown, Transport: transport, Err: core.ErrIO, ErrDetail: sanitize(err)}
	}
	clientNonce := base64.StdEncoding.EncodeToString(nonce)
	clientFirstBare := scram.BuildClientFirst(saslprepUsername(cred.Username), clientNonce)

	// SASLInitialResponse: 机制名 + NUL + int32 长度 + (gs2 头 + client-first-bare)
	var init []byte
	init = append(init, "SCRAM-SHA-256"...)
	init = append(init, 0)
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len("n,,")+len(clientFirstBare)))
	init = append(init, lenBuf[:]...)
	init = append(init, "n,,"...) // gs2 头：无通道绑定
	init = append(init, clientFirstBare...)
	if err := writeMessage(conn, msgPassword, init); err != nil {
		return &core.Result{Outcome: core.OutcomeTargetUnavailable, Transport: transport, Err: core.ErrIO, ErrDetail: sanitize(err)}
	}

	// AuthenticationSASLContinue
	msgType, payload, err := readMessage(conn)
	if err != nil {
		return &core.Result{Outcome: core.OutcomeTargetUnavailable, Transport: transport, Err: core.ErrIO, ErrDetail: sanitize(err)}
	}
	if msgType == msgError {
		pgErr := parseErrorResponse(payload)
		res := classifyPgError(pgErr, transport)
		return &res
	}
	if msgType != msgAuth || len(payload) < 4 || int(binary.BigEndian.Uint32(payload[:4])) != authSASLContinue {
		return &core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: "expected SASLContinue"}
	}
	serverFirst := string(payload[4:])
	combinedNonce, salt, iterations, err := scram.ParseServerFirst(serverFirst)
	if err != nil {
		return &core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: sanitize(err)}
	}
	if !strings.HasPrefix(string(combinedNonce), clientNonce) {
		return &core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: "server nonce mismatch"}
	}

	if err := client.SetCredentials(password, salt, iterations); err != nil {
		return &core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: sanitize(err)}
	}
	clientFinal := client.BuildClientFinal("biws", string(combinedNonce), clientFirstBare, serverFirst)
	if err := writeMessage(conn, msgPassword, []byte(clientFinal)); err != nil {
		return &core.Result{Outcome: core.OutcomeTargetUnavailable, Transport: transport, Err: core.ErrIO, ErrDetail: sanitize(err)}
	}

	// AuthenticationSASLFinal + authOK
	msgType, payload, err = readMessage(conn)
	if err != nil {
		return &core.Result{Outcome: core.OutcomeTargetUnavailable, Transport: transport, Err: core.ErrIO, ErrDetail: sanitize(err)}
	}
	if msgType == msgError {
		pgErr := parseErrorResponse(payload)
		res := classifyPgError(pgErr, transport)
		return &res
	}
	if msgType != msgAuth || len(payload) < 4 {
		return &core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: "expected SASLFinal"}
	}
	switch int(binary.BigEndian.Uint32(payload[:4])) {
	case authSASLFinal:
		if err := client.VerifyServerFinal(string(payload[4:])); err != nil {
			// 服务器签名校验失败：目标可能不是真实 PG（中间人/蜜罐）。
			return &core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: sanitize(err)}
		}
		// 等 authOK
		msgType, payload, err = readMessage(conn)
		if err != nil {
			return &core.Result{Outcome: core.OutcomeTargetUnavailable, Transport: transport, Err: core.ErrIO, ErrDetail: sanitize(err)}
		}
		if msgType == msgError {
			pgErr := parseErrorResponse(payload)
			res := classifyPgError(pgErr, transport)
			return &res
		}
		if msgType != msgAuth || len(payload) < 4 || int(binary.BigEndian.Uint32(payload[:4])) != authOK {
			return &core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: "expected authOK after SASLFinal"}
		}
		return nil
	case authOK:
		return nil
	default:
		return &core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: "unexpected auth state in SASL"}
	}
}

// awaitReady 等待 ReadyForQuery（认证成功的最终确认）。
func (p Prober) awaitReady(ctx context.Context, conn net.Conn, transport core.Transport) core.Result {
	for {
		msgType, payload, err := readMessage(conn)
		if err != nil {
			return readFailure(ctx, err, transport)
		}
		switch msgType {
		case msgReadyForQuery:
			return core.Result{Outcome: core.OutcomeAuthSuccess, Transport: transport}
		case msgParameterStatus, msgBackendKeyData, msgNotice:
			_ = payload
			continue
		case msgError:
			pgErr := parseErrorResponse(payload)
			return classifyPgError(pgErr, transport)
		default:
			return core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: fmt.Sprintf("unexpected message %q", msgType)}
		}
	}
}

// classifyPgError 按 SQLSTATE 分类。
func classifyPgError(pgErr *pgError, transport core.Transport) core.Result {
	res := core.Result{Transport: transport, ErrDetail: sanitize(pgErr)}
	switch pgErr.Code {
	case "28P01": // password authentication failed
		res.Outcome = core.OutcomeAuthFailed
		res.Err = core.ErrAuthRejected
	case "28000": // invalid authorization specification
		msg := strings.ToLower(pgErr.Message)
		switch {
		case strings.Contains(msg, "no pg_hba.conf entry"):
			// 主机被 pg_hba 拒绝：对该目标没有继续意义。
			res.Outcome = core.OutcomeTargetUnavailable
			res.Err = core.ErrAuthRejected
		case strings.Contains(msg, "role") && strings.Contains(msg, "does not exist"):
			res.Outcome = core.OutcomeAuthFailed
			res.Err = core.ErrAuthRejected
			res.UserEliminated = true // 该用户名在此服务器不存在
		default:
			res.Outcome = core.OutcomeAuthFailed
			res.Err = core.ErrAuthRejected
		}
	case "28P02": // password was changed during authentication — 视为失败重试
		res.Outcome = core.OutcomeAuthFailed
		res.Err = core.ErrAuthRejected
	case "08001", "08004", "08006", "08000":
		res.Outcome = core.OutcomeTargetUnavailable
		res.Err = core.ErrIO
	case "53300": // too_many_connections
		res.Outcome = core.OutcomeRateLimited
		res.Err = core.ErrAuthRejected
		res.RetryAfter = 10 * time.Second
	case "57P03": // cannot_connect_now
		res.Outcome = core.OutcomeRateLimited
		res.Err = core.ErrIO
		res.RetryAfter = 10 * time.Second
	case "08000 ", "XX000":
		res.Outcome = core.OutcomeUnknown
		res.Err = core.ErrIO
	default:
		res.Outcome = core.OutcomeUnknown
		res.Err = core.ErrAuthRejected
	}
	return res
}

// ---- 报文层 ----

// writeStartupMessage 发送 StartupMessage：长度 + 协议版本 + 参数对 + 终止 NUL。
func writeStartupMessage(conn net.Conn, username, database string) error {
	body := make([]byte, 4, 64)
	binary.BigEndian.PutUint32(body, protocolVersion)
	for _, p := range []string{"user", username, "database", database, "client_encoding", "UTF8"} {
		body = append(body, p...)
		body = append(body, 0)
	}
	body = append(body, 0)
	msg := make([]byte, 4+len(body))
	binary.BigEndian.PutUint32(msg, uint32(len(body)+4))
	copy(msg[4:], body)
	_, err := conn.Write(msg)
	return err
}

// writeSSLRequest 发送 SSLRequest；返回服务端是否同意 SSL（'S'）。
func writeSSLRequest(conn net.Conn) bool {
	msg := make([]byte, 8)
	binary.BigEndian.PutUint32(msg[0:4], 8)
	binary.BigEndian.PutUint32(msg[4:8], 80877103)
	if _, err := conn.Write(msg); err != nil {
		return false
	}
	var reply [1]byte
	if _, err := io.ReadFull(conn, reply[:]); err != nil {
		return false
	}
	return reply[0] == 'S'
}

// upgradeTLS 在已有连接上完成 TLS 握手。
func upgradeTLS(ctx context.Context, conn net.Conn, serverName string, timeout time.Duration) (net.Conn, error) {
	tlsConn := tls.Client(conn, &tls.Config{InsecureSkipVerify: true, ServerName: serverName}) // #nosec G402 -- 探测场景跳过证书校验
	hctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := tlsConn.HandshakeContext(hctx); err != nil {
		return nil, err
	}
	return tlsConn, nil
}

// writePasswordMessage 发送 PasswordMessage / SASLResponse。
func writePasswordMessage(conn net.Conn, password string) error {
	return writeMessage(conn, msgPassword, []byte(password))
}

func writeMessage(conn net.Conn, msgType byte, payload []byte) error {
	msg := make([]byte, 5+len(payload))
	msg[0] = msgType
	binary.BigEndian.PutUint32(msg[1:5], uint32(len(payload)+4))
	copy(msg[5:], payload)
	_, err := conn.Write(msg)
	return err
}

// readMessage 读取一个 PG 消息：type + len + payload。
func readMessage(conn net.Conn) (byte, []byte, error) {
	var header [5]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return 0, nil, err
	}
	length := int(binary.BigEndian.Uint32(header[1:5]))
	if length < 4 || length > 1<<20 {
		return 0, nil, fmt.Errorf("postgres: invalid message length %d", length)
	}
	payload := make([]byte, length-4)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return 0, nil, err
	}
	return header[0], payload, nil
}

// parseErrorResponse 解析 ErrorResponse 的字段。
func parseErrorResponse(payload []byte) *pgError {
	e := &pgError{}
	for len(payload) > 0 {
		code := payload[0]
		if code == 0 {
			break
		}
		payload = payload[1:]
		end := indexOfByte(payload, 0)
		if end < 0 {
			break
		}
		val := string(payload[:end])
		payload = payload[end+1:]
		switch code {
		case 'S':
			e.Severity = val
		case 'C':
			e.Code = val
		case 'M':
			e.Message = val
		}
	}
	return e
}

func indexOfByte(b []byte, c byte) int {
	for i := range b {
		if b[i] == c {
			return i
		}
	}
	return -1
}

// md5AuthResponse 计算 md5 摘要：
//
//	"md5" + hex(md5(hex(md5(password + username)) + salt))
func md5AuthResponse(username, password string, salt []byte) string {
	inner := md5.Sum([]byte(password + username))
	combined := append([]byte{}, []byte(hexEncode(inner[:]))...)
	combined = append(combined, salt...)
	outer := md5.Sum(combined)
	return "md5" + hexEncode(outer[:])
}

func hexEncode(b []byte) string {
	const hexDigits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hexDigits[v>>4]
		out[i*2+1] = hexDigits[v&0xf]
	}
	return string(out)
}

// saslprepUsername 规范化 SCRAM 用户名（PG 对用户名不做 SASLprep，转义 = 和 ,）。
func saslprepUsername(username string) string {
	return strings.NewReplacer("=", "=3D", ",", "=2C").Replace(username)
}

// ---- 通用工具（与 MySQL 探针同构，独立维护避免跨包耦合） ----

func withDefaultPort(target core.Target, port int) string {
	p := target.Port
	if p <= 0 {
		p = port
	}
	return net.JoinHostPort(target.Host, strconv.Itoa(p))
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
	return core.Result{Outcome: core.OutcomeTargetUnavailable, Transport: transport, Err: core.ErrDial, ErrDetail: sanitize(err)}
}

func readFailure(ctx context.Context, err error, transport core.Transport) core.Result {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return core.Result{Outcome: core.OutcomeCancelled, Transport: transport, Err: core.ErrDeadline}
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return core.Result{Outcome: core.OutcomeTargetUnavailable, Transport: transport, Err: core.ErrIO, ErrDetail: "server closed during handshake"}
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return core.Result{Outcome: core.OutcomeTargetUnavailable, Transport: transport, Err: core.ErrDeadline, ErrDetail: "i/o timeout"}
	}
	return core.Result{Outcome: core.OutcomeTargetUnavailable, Transport: transport, Err: core.ErrIO, ErrDetail: sanitize(err)}
}

func sanitize(err error) string {
	msg := err.Error()
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return msg
}

// ServiceInfo 返回用于 core.Register 的服务描述。
func ServiceInfo() core.ServiceInfo {
	return core.ServiceInfo{
		Name:             "postgres",
		DefaultPort:      5432,
		DefaultUsernames: append([]string{"postgres"}, dicts.CommonUsernames...),
		DefaultPasswords: dicts.CommonPasswords,
		Prober:           Prober{},
	}
}

// Register 注册探针。
func Register() { core.Register(ServiceInfo()) }

// ParseErrorResponseForFuzz 仅供模糊测试导出。
func ParseErrorResponseForFuzz(payload []byte) *pgError {
	return parseErrorResponse(payload)
}
