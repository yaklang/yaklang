package mongodb

import (
	"context"
	"crypto/md5"
	"crypto/rand"
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

// MongoDB OP_MSG opCode。
const opMsg = 2013

// Mongo 错误码。
const (
	codeAuthenticationFailed = 18
	codeUnauthorized         = 13
	codeMechanismUnavailable = 334 // SASL 机制不支持
	codeLoginFailed          = 18
)

// mongoError 是命令失败信息。
type mongoError struct {
	Code    int32
	Name    string
	Message string
}

func (e *mongoError) Error() string {
	return fmt.Sprintf("mongodb %d %s: %s", e.Code, e.Name, e.Message)
}

// Prober 是 MongoDB 最小认证探针。
type Prober struct{}

// Probe 执行一次 MongoDB SCRAM 认证。
//
// 传输策略（SCRAM 本身是质询响应协议，不传输明文密码）：
//   - 默认明文 TCP（与 mongo-driver 默认一致）；
//   - hello 在协议层失败（连接被关/响应不可解析）时，若策略允许则以 TLS 重试
//     （服务端可能只接受 TLS），回退被如实记录；
//   - TLSStrict 只走 TLS，绝不发送明文；
//   - PlaintextAllowed 只走明文。
func (p Prober) Probe(ctx context.Context, target core.Target, cred core.Credential, opts core.Options) core.Result {
	addr := withDefaultPort(target, 27017)

	if opts.TLSPolicy != core.TLSStrict {
		res, helloDone := p.attempt(ctx, addr, target, cred, opts, false /*plaintext*/)
		if helloDone || ctx.Err() != nil {
			return res
		}
		// hello 未完成（传输层/解析失败）：可能是 TLS-only 服务端，显式 TLS 重试。
		res, _ = p.attempt(ctx, addr, target, cred, opts, true /*tls*/)
		return res
	}
	res, _ := p.attempt(ctx, addr, target, cred, opts, true)
	return res
}

// attempt 单轮尝试：plaintext=false 明文 / true TLS。
// 返回 (结果, hello 是否完成)。hello 完成意味着对端确实是 MongoDB（无需 TLS 重试）。
func (p Prober) attempt(ctx context.Context, addr string, target core.Target, cred core.Credential, opts core.Options, useTLS bool) (core.Result, bool) {
	timeout := timeoutOf(opts)
	dialPolicy := core.PlaintextAllowed
	transport := core.TransportPlainTCP
	if useTLS {
		dialPolicy = core.TLSStrict
		transport = core.TransportTLS
	}
	conn, gotTransport, err := core.Dialer(ctx, addr, dialPolicy, timeout)
	if err != nil {
		if useTLS {
			return dialFailure(ctx, err, transport), false
		}
		return dialFailure(ctx, err, gotTransport), false
	}
	defer core.SafeClose(conn)
	unwatch := core.WatchConn(ctx, conn)
	defer unwatch()
	core.SetDeadline(conn, ctx, timeout)
	transport = gotTransport

	// 1. hello：确认 OP_MSG 可用并获取机制列表。
	hello := D{S("hello", int32(1)), S("helloOk", true), S("$db", "admin")}
	resp, err := roundTrip(conn, hello)
	if err != nil {
		return readFailure(ctx, err, transport), false
	}
	if _, ok := resp.GetInt("ok"); !ok {
		return core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: "no ok field in hello response"}, true
	}
	if !resp.IsOK() {
		// 有效 MongoDB 错误响应（无需 TLS 重试）。
		if code, hasCode := resp.GetInt("code"); hasCode {
			res := classifyMongoError(&mongoError{Code: int32(code), Message: "hello rejected"}, transport)
			return res, true
		}
		return core.Result{Outcome: core.OutcomeUnknown, Transport: transport, Err: core.ErrIO, ErrDetail: "hello not ok"}, true
	}

	mechanism := pickMechanism(resp, cred)
	return p.scram(ctx, conn, transport, cred, mechanism), true
}

// pingProbe 检测未授权访问（无凭证）：明文优先，失败再 TLS。
// 注意：ping 命令在开启认证的 MongoDB 上也允许匿名执行（会导致误报），
// 因此使用需要权限的 listDatabases 判定真实的未授权访问。
func (p Prober) pingProbe(ctx context.Context, addr string, opts core.Options) core.Result {
	tryOnce := func(useTLS bool) core.Result {
		timeout := timeoutOf(opts)
		policy := core.PlaintextAllowed
		if useTLS {
			policy = core.TLSStrict
		}
		conn, transport, err := core.Dialer(ctx, addr, policy, timeout)
		if err != nil {
			return dialFailure(ctx, err, transport)
		}
		defer core.SafeClose(conn)
		unwatch := core.WatchConn(ctx, conn)
		defer unwatch()
		core.SetDeadline(conn, ctx, timeout)

		resp, err := roundTrip(conn, D{S("listDatabases", int32(1)), S("$db", "admin")})
		if err != nil {
			return readFailure(ctx, err, transport)
		}
		if resp.IsOK() {
			return core.Result{Outcome: core.OutcomeAuthSuccess, Transport: transport}
		}
		if code, has := resp.GetInt("code"); has {
			return classifyMongoError(&mongoError{Code: int32(code), Message: "unauthorized"}, transport)
		}
		return core.Result{Outcome: core.OutcomeAuthFailed, Transport: transport, Err: core.ErrAuthRejected}
	}

	if opts.TLSPolicy == core.TLSStrict {
		return tryOnce(true)
	}
	res := tryOnce(false)
	if res.Outcome == core.OutcomeProtocolMismatch || res.Outcome == core.OutcomeTargetUnavailable {
		if ctx.Err() == nil {
			return tryOnce(true)
		}
	}
	return res
}

// pickMechanism 选择认证机制：优先 saslSupportedMechs 中的 SCRAM-SHA-256，
// 否则 SCRAM-SHA-1（与 mongo-driver 默认协商一致）。
func pickMechanism(helloResp D, cred core.Credential) string {
	if mechs, ok := helloResp.Get("saslSupportedMechs"); ok {
		if arr, ok := mechs.(D); ok {
			has256, has1 := false, false
			for _, e := range arr {
				if s, ok := e.Value.(string); ok {
					if s == "SCRAM-SHA-256" {
						has256 = true
					}
					if s == "SCRAM-SHA-1" {
						has1 = true
					}
				}
			}
			if has256 {
				return "SCRAM-SHA-256"
			}
			if has1 {
				return "SCRAM-SHA-1"
			}
		}
	}
	return "SCRAM-SHA-1"
}

// scram 执行 SCRAM-SHA-1/256 认证交换。
func (p Prober) scram(ctx context.Context, conn net.Conn, transport core.Transport, cred core.Credential, mechanism string) core.Result {
	sha256 := mechanism == "SCRAM-SHA-256"

	username := cred.Username
	password := cred.Password
	if sha256 {
		if u, err := scram.SASLprep(cred.Username); err == nil {
			username = u
		}
		if pw, err := scram.SASLprep(cred.Password); err == nil {
			password = pw
		}
	} else {
		// SCRAM-SHA-1 的密码是 MD5(user:mongo:pass) 十六进制。
		sum := md5.Sum([]byte(cred.Username + ":mongo:" + cred.Password))
		password = hexEncode(sum[:])
		username = strings.NewReplacer("=", "=3D", ",", "=2C").Replace(username)
	}

	client := scram.NewClient(sha256)
	nonceBytes := make([]byte, 18)
	if _, err := rand.Read(nonceBytes); err != nil {
		return core.Result{Outcome: core.OutcomeUnknown, Transport: transport, Err: core.ErrIO, ErrDetail: sanitize(err)}
	}
	clientNonce := base64.StdEncoding.EncodeToString(nonceBytes)
	clientFirstBare := scram.BuildClientFirst(username, clientNonce)

	// saslStart
	startResp, err := roundTrip(conn, D{
		S("saslStart", int32(1)),
		S("mechanism", mechanism),
		S("payload", []byte("n,,"+clientFirstBare)),
		S("options", D{S("skipEmptyExchange", true)}),
		S("$db", "admin"),
	})
	if err != nil {
		return readFailure(ctx, err, transport)
	}
	if !startResp.IsOK() {
		return mongoCommandError(startResp, transport)
	}
	serverPayload, ok := startResp.GetBinary("payload")
	if !ok {
		return core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: "saslStart: no payload"}
	}
	serverFirst := string(serverPayload)
	combinedNonce, salt, iterations, err := scram.ParseServerFirst(serverFirst)
	if err != nil {
		return core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: sanitize(err)}
	}
	if !strings.HasPrefix(string(combinedNonce), clientNonce) {
		return core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: "server nonce mismatch"}
	}
	if err := client.SetCredentials(password, salt, iterations); err != nil {
		return core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: sanitize(err)}
	}
	clientFinal := client.BuildClientFinal("biws", string(combinedNonce), clientFirstBare, serverFirst)

	// saslContinue
	contResp, err := roundTrip(conn, D{
		S("saslContinue", int32(1)),
		S("conversationId", int32(1)),
		S("payload", []byte(clientFinal)),
		S("$db", "admin"),
	})
	if err != nil {
		return readFailure(ctx, err, transport)
	}
	if !contResp.IsOK() {
		return mongoCommandError(contResp, transport)
	}

	// skipEmptyExchange：done 直接为 true，payload 是 server-final。
	finalPayload, hasPayload := contResp.GetBinary("payload")
	if !hasPayload {
		// 某些服务器需要再一轮 saslContinue（空 payload 的第三阶段）。
		doneResp, err := roundTrip(conn, D{
			S("saslContinue", int32(1)),
			S("conversationId", int32(1)),
			S("payload", []byte{}),
			S("$db", "admin"),
		})
		if err != nil {
			return readFailure(ctx, err, transport)
		}
		if !doneResp.IsOK() {
			return mongoCommandError(doneResp, transport)
		}
		finalPayload, hasPayload = doneResp.GetBinary("payload")
	}
	if hasPayload && len(finalPayload) > 0 {
		if err := client.VerifyServerFinal(string(finalPayload)); err != nil {
			// 签名不符：目标可能不是真实 MongoDB（中间人/蜜罐）。
			return core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: sanitize(err)}
		}
	}
	return core.Result{Outcome: core.OutcomeAuthSuccess, Transport: transport}
}

// mongoCommandError 把 {ok:0} 响应转结果。
func mongoCommandError(resp D, transport core.Transport) core.Result {
	code, _ := resp.GetInt("code")
	msg, _ := resp.GetString("errmsg")
	name, _ := resp.GetString("codeName")
	return classifyMongoError(&mongoError{Code: int32(code), Name: name, Message: msg}, transport)
}

// classifyMongoError 按错误码分类。
func classifyMongoError(mErr *mongoError, transport core.Transport) core.Result {
	res := core.Result{Transport: transport, ErrDetail: sanitize(mErr)}
	switch mErr.Code {
	case codeAuthenticationFailed:
		res.Outcome = core.OutcomeAuthFailed
		res.Err = core.ErrAuthRejected
	case codeUnauthorized:
		// 认证成功但无权限（对 admin 库 ping 的场景基本不出现）。
		res.Outcome = core.OutcomeAuthFailed
		res.Err = core.ErrAuthRejected
	default:
		lower := strings.ToLower(mErr.Message)
		switch {
		case strings.Contains(lower, "too many clients") || strings.Contains(lower, "connection pool"):
			res.Outcome = core.OutcomeRateLimited
			res.Err = core.ErrIO
			res.RetryAfter = 5 * time.Second
		case strings.Contains(lower, "unsupported mechanism") || strings.Contains(lower, "mechanism"):
			res.Outcome = core.OutcomeProtocolMismatch
			res.Err = core.ErrHandshake
		case strings.Contains(lower, "authentication"):
			res.Outcome = core.OutcomeAuthFailed
			res.Err = core.ErrAuthRejected
		default:
			res.Outcome = core.OutcomeUnknown
			res.Err = core.ErrIO
		}
	}
	return res
}

// ---- OP_MSG 组帧 ----

var requestCounter uint32

// roundTrip 发送一条命令并读取响应文档。
func roundTrip(conn net.Conn, cmd D) (D, error) {
	body := EncodeD(cmd)
	buf := make([]byte, 0, 16+5+len(body))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(16+5+len(body))) // length
	requestCounter++
	buf = binary.LittleEndian.AppendUint32(buf, requestCounter) // requestID
	buf = binary.LittleEndian.AppendUint32(buf, 0)              // responseTo
	buf = binary.LittleEndian.AppendUint32(buf, opMsg)          // opCode
	buf = binary.LittleEndian.AppendUint32(buf, 0)              // flagBits
	buf = append(buf, 0)                                        // section kind 0
	buf = append(buf, body...)
	if _, err := conn.Write(buf); err != nil {
		return nil, err
	}
	return readOpMsgResponse(conn)
}

// readOpMsgResponse 读取 OP_MSG 响应并解析首个 body 段。
func readOpMsgResponse(conn net.Conn) (D, error) {
	var header [16]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return nil, err
	}
	length := int(binary.LittleEndian.Uint32(header[0:4]))
	op := binary.LittleEndian.Uint32(header[12:16])
	if length < 16 || length > 64<<20 {
		return nil, fmt.Errorf("mongodb: invalid message length %d", length)
	}
	if op != opMsg {
		// 旧版响应 OP_REPLY(1)/OP_QUERY(2004)：视为协议不匹配。
		return nil, fmt.Errorf("mongodb: unexpected opCode %d (want OP_MSG)", op)
	}
	rest := make([]byte, length-16)
	if _, err := io.ReadFull(conn, rest); err != nil {
		return nil, err
	}
	if len(rest) < 5 {
		return nil, errors.New("mongodb: response too short")
	}
	// 跳过 flagBits(4)，取第一个 section。
	kind := rest[4]
	if kind != 0 {
		return nil, fmt.Errorf("mongodb: unsupported section kind %d", kind)
	}
	return DecodeD(rest[5:])
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

// ---- 通用工具 ----

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
	if errors.Is(err, errBSON) || strings.Contains(err.Error(), "mongodb:") {
		return core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: sanitize(err)}
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

// ServiceInfo 返回服务描述。
func ServiceInfo() core.ServiceInfo {
	return core.ServiceInfo{
		Name:             "mongodb",
		DefaultPort:      27017,
		DefaultUsernames: append([]string{"root", "admin", "mongodb"}, dicts.CommonUsernames...),
		DefaultPasswords: dicts.CommonPasswords,
		Prober:           Prober{},
		UnAuthProber:     UnauthProber{},
	}
}

// Register 注册探针。
func Register() { core.Register(ServiceInfo()) }

// UnauthProber 未授权访问检测：无凭证执行需要权限的命令。
type UnauthProber struct{}

func (UnauthProber) Probe(ctx context.Context, target core.Target, cred core.Credential, opts core.Options) core.Result {
	return Prober{}.pingProbe(ctx, withDefaultPort(target, 27017), opts)
}
