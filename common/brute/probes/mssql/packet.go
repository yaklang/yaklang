// Package mssql 实现最小化 MSSQL (TDS 7.4) 认证探针。
//
// 只包含 PRELOGIN 协商（含 TDS 内嵌 TLS 升级）、LOGIN7 构造
// （SQL 认证，密码按协议做半字节交换混淆）与响应 token 分类。
// 不引入 go-mssqldb 驱动、连接池与查询能力。
package mssql

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/yaklang/yaklang/common/brute/core"
	"github.com/yaklang/yaklang/common/brute/dicts"
)

// TDS 包类型。
const (
	packPreLogin = 0x12
	packLogin7   = 0x10
	packReply    = 0x04
	packSSPI     = 0x11
)

// PRELOGIN 选项 token。
const (
	preloginVERSION    = 0x00
	preloginENCRYPTION = 0x01
	preloginINSTOPT    = 0x02
	preloginTHREADID   = 0x03
	preloginMARS       = 0x04
)

// 加密协商值。
const (
	encryptOff    = 0 // 可用但未启用
	encryptOn     = 1 // 全程加密
	encryptNotSup = 2 // 不支持
	encryptReq    = 3 // 必须加密
)

// LOGIN7 flag。
const (
	fUseDB       = 0x20
	fSetLang     = 0x80
	fODBC        = 2
	fIntSecurity = 0x80
)

// 响应 token。
const (
	tokenError         = 0xAA
	tokenInfo          = 0xAB
	tokenLoginAck      = 0xAD
	tokenFeatureExtAck = 0xAE
	tokenEnvChange     = 0xE3
	tokenSSPI          = 0xED
	tokenDone          = 0xFD
)

const tdsVersion74 = 0x74000004
const defaultPacketSize = 4096

var errTDS = errors.New("tds: malformed packet")

// serverError 是 ERROR token 的解析结果。
type serverError struct {
	Number  int32
	State   uint8
	Class   uint8
	Message string
}

func (e *serverError) Error() string {
	return fmt.Sprintf("mssql error %d state %d class %d: %s", e.Number, e.State, e.Class, e.Message)
}

// ---- TDS 流层 ----

// tdsStream 处理 TDS 分包读写与可选的 TLS 层。
type tdsStream struct {
	conn      net.Conn // 底层（TLS 升级后为 TLS 连接）
	tlsActive bool

	pendingType byte
	pending     []byte

	readRemain []byte // 当前 TDS 包剩余载荷
}

func newTDSStream(conn net.Conn) *tdsStream {
	return &tdsStream{conn: conn}
}

// beginPacket 开始一个新 TDS 包的写入。
func (s *tdsStream) beginPacket(pktType byte) {
	s.pendingType = pktType
	s.pending = nil
}

// write 追加到待发载荷。
func (s *tdsStream) write(b []byte) { s.pending = append(s.pending, b...) }

// writeByte 追加单字节。
func (s *tdsStream) writeByte(b byte) { s.pending = append(s.pending, b) }

// finishPacket 按 4KB 分片发出（末包 status=EOM）。
func (s *tdsStream) finishPacket() error {
	if s.pending == nil {
		s.pending = []byte{}
	}
	for {
		chunk := s.pending
		if len(chunk) > defaultPacketSize-8 {
			chunk = chunk[:defaultPacketSize-8]
		}
		s.pending = s.pending[len(chunk):]
		eom := byte(1)
		if len(s.pending) > 0 {
			eom = 0
		}
		header := make([]byte, 8, 8+len(chunk))
		header[0] = s.pendingType
		header[1] = eom
		binary.BigEndian.PutUint16(header[2:4], uint16(len(chunk)+8))
		// spid/packetID/window 置零
		packet := append(header, chunk...)
		if _, err := s.conn.Write(packet); err != nil {
			return err
		}
		if eom == 1 {
			break
		}
	}
	s.pending = nil
	return nil
}

// readPacket 读取下一个完整 TDS 包（载荷拼接连续分片）。
func (s *tdsStream) readPacket() (pktType byte, payload []byte, err error) {
	var full []byte
	for {
		var header [8]byte
		if _, err := io.ReadFull(s.conn, header[:]); err != nil {
			return 0, nil, err
		}
		pktType = header[0]
		length := int(binary.BigEndian.Uint16(header[2:4]))
		if length < 8 || length > 32768 {
			return 0, nil, fmt.Errorf("%w: bad packet length %d", errTDS, length)
		}
		chunk := make([]byte, length-8)
		if _, err := io.ReadFull(s.conn, chunk); err != nil {
			return 0, nil, err
		}
		full = append(full, chunk...)
		if header[1]&1 == 1 { // EOM
			return pktType, full, nil
		}
		if len(full) > 1<<20 {
			return 0, nil, fmt.Errorf("%w: packet too large", errTDS)
		}
	}
}

// ---- PRELOGIN ----

// buildPrelogin 构造 PRELOGIN 请求。encrypt 为协商的加密偏好。
func buildPrelogin(encrypt byte, instance string) []byte {
	type preloginField struct {
		token byte
		data  []byte
	}
	fields := []preloginField{
		{token: preloginVERSION, data: []byte{0, 0, 0, 0, 0, 0}},
		{token: preloginENCRYPTION, data: []byte{encrypt}},
		{token: preloginINSTOPT, data: append([]byte(instance), 0)},
		{token: preloginTHREADID, data: []byte{0, 0, 0, 0}},
		{token: preloginMARS, data: []byte{0}},
	}
	headerLen := len(fields)*5 + 1
	var out []byte
	var data []byte
	for _, f := range fields {
		out = append(out, f.token)
		out = binary.BigEndian.AppendUint16(out, uint16(headerLen+len(data)))
		out = binary.BigEndian.AppendUint16(out, uint16(len(f.data)))
		data = append(data, f.data...)
	}
	out = append(out, 0xFF) // TERMINATOR
	out = append(out, data...)
	return out
}

// parsePrelogin 解析 PRELOGIN 响应为 token → 数据。
func parsePrelogin(payload []byte) (map[byte][]byte, error) {
	fields := make(map[byte][]byte)
	pos := 0
	for pos < len(payload) {
		token := payload[pos]
		if token == 0xFF { // TERMINATOR（VERSION token 本身是 0x00）
			break
		}
		if pos+5 > len(payload) {
			return nil, errTDS
		}
		offset := int(binary.BigEndian.Uint16(payload[pos+1 : pos+3]))
		length := int(binary.BigEndian.Uint16(payload[pos+3 : pos+5]))
		if offset+length > len(payload) {
			return nil, errTDS
		}
		fields[token] = payload[offset : offset+length]
		pos += 5
	}
	return fields, nil
}

// ---- TLS-in-TDS 握手 ----

// handshakeRW 把 TLS 握手记录封装进 TDS 包。
// 握手完成后（done=true）切换为 raw 连接直通：TLS 应用数据内部已含 TDS 帧。
// 剩余载荷缓存在 remain 中，严格遵守 io.Reader 契约（不丢字节）。
type handshakeRW struct {
	raw    net.Conn
	stream *tdsStream
	remain []byte
	done   bool
}

func (h *handshakeRW) Write(b []byte) (int, error) {
	fmt.Printf("[client-hs] WRITE %d bytes: %x\n", len(b), b[:min(len(b), 30)])
	if h.done {
		return h.raw.Write(b)
	}
	h.stream.write(b)
	return len(b), nil
}

func (h *handshakeRW) Read(b []byte) (int, error) {
	if h.done {
		return h.raw.Read(b)
	}
	if len(h.remain) == 0 {
		// 先把已缓冲的握手数据作为 TDS 包发出。
		if h.stream.pending != nil {
			if err := h.stream.finishPacket(); err != nil {
				return 0, err
			}
		}
		_, payload, err := h.stream.readPacket()
		if err != nil {
			return 0, err
		}
		h.remain = payload
	}
	n := copy(b, h.remain)
	h.remain = h.remain[n:]
	return n, nil
}

func (h *handshakeRW) markDone() { h.done = true }

// flushPending 发出残留在 TDS 缓冲中的握手记录。
func (h *handshakeRW) flushPending() error {
	if h.stream.pending != nil {
		return h.stream.finishPacket()
	}
	return nil
}

// upgradeTLS 在 TDS 流内完成 TLS 握手；成功后数据层切换为 TLS。
func (s *tdsStream) upgradeTLS(ctx context.Context, serverName string, timeout time.Duration) error {
	handshake := &handshakeRW{raw: s.conn, stream: newTDSStream(s.conn)}
	tlsConn := tlsClient(ctx, &netConnAdapter{rw: handshake, raw: s.conn}, serverName)
	hctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := tlsConn.HandshakeContext(hctx); err != nil {
		return err
	}
	// TLS 1.3 客户端在发出 Finished 后即认为握手完成，不再触发 Read 冲刷；
	// 残留在 TDS 缓冲中的握手记录必须在此显式发出。
	if err := handshake.flushPending(); err != nil {
		return err
	}
	handshake.markDone()
	s.conn = tlsConn
	s.tlsActive = true
	return nil
}

// ---- LOGIN7 ----

// buildLogin7 构造 LOGIN7（SQL 认证）。
func buildLogin7(username, password, serverName string) []byte {
	hostname := toUCS2("")
	appName := toUCS2("yak-brute")
	ctlIntName := toUCS2("")
	language := toUCS2("")
	database := toUCS2("")
	userBytes := toUCS2(username)
	passBytes := manglePassword(password)
	serverBytes := toUCS2(serverName)

	type field struct {
		offset uint16
		length uint16 // 字符数（UCS2 双字节）
	}

	// 头部字节数（与 go-mssqldb loginHeader 结构一致，小端序）：
	// Length(4) + 5×uint32(20) + 4 flag(4) + TimeZone/LCID(8) +
	// Host/User/Pass/App/Server 5对(20) + Extension对(4) + Ctl/Lang/DB 3对(12) +
	// ClientID(6) + SSPI对(4) + AtchDB/ChangePwd 2对(8) + SSPILongLength(4) = 94
	const headerSize = 94

	offset := uint16(headerSize)
	put := func(b []byte) uint16 {
		off := offset
		offset += uint16(len(b))
		return off
	}

	hostOff := put(hostname)
	userOff := put(userBytes)
	passOff := put(passBytes)
	appOff := put(appName)
	srvOff := put(serverBytes)
	ctlOff := put(ctlIntName)
	langOff := put(language)
	dbOff := put(database)
	// SSPI 空
	sspiOff := offset
	// AtchDBFile / ChangePassword 空
	atchOff := offset
	chgOff := offset

	length := offset

	buf := make([]byte, 0, length)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(length))
	buf = binary.LittleEndian.AppendUint32(buf, tdsVersion74)
	buf = binary.LittleEndian.AppendUint32(buf, defaultPacketSize)
	buf = binary.LittleEndian.AppendUint32(buf, 0) // ClientProgVer
	buf = binary.LittleEndian.AppendUint32(buf, 0) // ClientPID
	buf = binary.LittleEndian.AppendUint32(buf, 0) // ConnectionID
	buf = append(buf, fUseDB|fSetLang)             // OptionFlags1
	buf = append(buf, byte(fODBC))                 // OptionFlags2
	buf = append(buf, 0)                           // TypeFlags: SQL_DFLT
	buf = append(buf, 0)                           // OptionFlags3
	buf = binary.LittleEndian.AppendUint32(buf, 0) // ClientTimeZone
	buf = binary.LittleEndian.AppendUint32(buf, 0) // ClientLCID
	appendField := func(off uint16, charCount int) {
		buf = binary.LittleEndian.AppendUint16(buf, off)
		buf = binary.LittleEndian.AppendUint16(buf, uint16(charCount))
	}
	// 头部字段顺序（[MS-TDS] 2.2.6.4）：
	// Host/User/Pass/App/Server 各一对 → Extension 对 → Ctl/Lang/DB 各一对
	// → ClientID(6) → SSPI/AtchDB/ChangePwd 各一对 → SSPILongLength
	appendField(hostOff, ucs2Count(""))
	appendField(userOff, ucs2Count(username))
	appendField(passOff, ucs2Count(password))
	appendField(appOff, ucs2Count("yak-brute"))
	appendField(srvOff, ucs2Count(serverName))
	// Extension（未使用）
	buf = binary.LittleEndian.AppendUint16(buf, 0)
	buf = binary.LittleEndian.AppendUint16(buf, 0)
	appendField(ctlOff, ucs2Count(""))
	appendField(langOff, ucs2Count(""))
	appendField(dbOff, ucs2Count(""))
	buf = append(buf, 0, 0, 0, 0, 0, 0) // ClientID
	appendField(sspiOff, 0)
	appendField(atchOff, 0)
	appendField(chgOff, 0)
	buf = binary.LittleEndian.AppendUint32(buf, 0) // SSPILongLength

	buf = append(buf, hostname...)
	buf = append(buf, userBytes...)
	buf = append(buf, passBytes...)
	buf = append(buf, appName...)
	buf = append(buf, serverBytes...)
	buf = append(buf, ctlIntName...)
	buf = append(buf, language...)
	buf = append(buf, database...)
	return buf
}

// manglePassword 按 TDS 规范混淆密码：UCS2-LE 后每字节半字节交换并 XOR 0xA5。
func manglePassword(password string) []byte {
	ucs2 := toUCS2(password)
	out := make([]byte, len(ucs2))
	for i, ch := range ucs2 {
		out[i] = ((ch<<4)&0xff | ch>>4) ^ 0xA5
	}
	return out
}

// toUCS2 转换为 UTF-16LE 字节。
func toUCS2(s string) []byte {
	codes := utf16.Encode([]rune(s))
	out := make([]byte, len(codes)*2)
	for i, c := range codes {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(c))
	}
	return out
}

// ucs2ToString UTF-16LE 字节转字符串（错误消息解析用）。
func ucs2ToString(b []byte) string {
	if len(b)%2 != 0 {
		if len(b) > 0 {
			b = b[:len(b)-1]
		}
	}
	codes := make([]uint16, len(b)/2)
	for i := range codes {
		codes[i] = binary.LittleEndian.Uint16(b[i*2:])
	}
	return string(utf16.Decode(codes))
}

// ucs2Count 返回 UTF-16 码元数（TDS 长度字段的正确单位）：
// 星面字符（如 emoji）是代理对，占 2 个码元而非 1 个。
func ucs2Count(s string) int { return len(utf16.Encode([]rune(s))) }

// ---- 响应 token 解析 ----

// parseTokens 解析回复包中的 token 流，返回首个 ERROR 与是否出现 LOGINACK。
func parseTokens(payload []byte) (srvErr *serverError, loginAck bool, err error) {
	pos := 0
	for pos < len(payload) {
		token := payload[pos]
		pos++
		switch token {
		case tokenError:
			if pos+2 > len(payload) {
				return nil, false, errTDS
			}
			length := int(binary.LittleEndian.Uint16(payload[pos : pos+2]))
			pos += 2
			if pos+length > len(payload) {
				return nil, false, errTDS
			}
			e, err := parseErrorToken(payload[pos : pos+length])
			if err != nil {
				return nil, false, err
			}
			if srvErr == nil {
				srvErr = e
			}
			pos += length
		case tokenInfo, tokenEnvChange, tokenFeatureExtAck:
			if pos+2 > len(payload) {
				return nil, false, errTDS
			}
			length := int(binary.LittleEndian.Uint16(payload[pos : pos+2]))
			pos += 2 + length
		case tokenLoginAck:
			loginAck = true
			if pos+2 > len(payload) {
				return nil, false, errTDS
			}
			length := int(binary.LittleEndian.Uint16(payload[pos : pos+2]))
			pos += 2 + length
		case tokenDone, 0xFE, 0xFF:
			// DONE/DONEPROC/DONEINPROC：长度 2(+4 with COUNT if status&0x20? 简化为 2 字节长度)
			if pos+2 > len(payload) {
				return nil, false, errTDS
			}
			length := int(binary.LittleEndian.Uint16(payload[pos : pos+2]))
			pos += 2 + length
		case tokenSSPI:
			return nil, false, fmt.Errorf("tds: server requires SSPI (Windows integrated auth)")
		default:
			return nil, false, fmt.Errorf("%w: unknown token 0x%02x", errTDS, token)
		}
	}
	return srvErr, loginAck, nil
}

// parseErrorToken 解析 ERROR token 载荷。
func parseErrorToken(b []byte) (*serverError, error) {
	if len(b) < 9 {
		return nil, errTDS
	}
	e := &serverError{
		Number: int32(binary.LittleEndian.Uint32(b[0:4])),
		State:  b[4],
		Class:  b[5],
	}
	pos := 6
	// MsgText: US_VARCHAR（uint16 长度 + UCS2 字节）
	if pos+2 > len(b) {
		return nil, errTDS
	}
	msgLen := int(binary.LittleEndian.Uint16(b[pos : pos+2]))
	pos += 2
	if pos+msgLen > len(b) {
		return nil, errTDS
	}
	e.Message = ucs2ToString(b[pos : pos+msgLen])
	pos += msgLen
	// ServerName / ProcName: B_VARCHAR（uint8 长度 + 字节）
	for i := 0; i < 2 && pos < len(b); i++ {
		n := int(b[pos])
		pos += 1 + n
	}
	// LineNo (uint16) 忽略
	return e, nil
}

// classifyServerError 把 MSSQL 错误号映射为结构化结果。
func classifyServerError(srvErr *serverError, transport core.Transport) core.Result {
	res := core.Result{Transport: transport, ErrDetail: sanitize(srvErr)}
	switch srvErr.Number {
	case 18456: // Login failed for user
		if srvErr.State == 58 || srvErr.State == 38 { // windows-only / 拒绝 SQL 认证
			res.Outcome = core.OutcomeAuthFailed
			res.Err = core.ErrAuthRejected
			res.ErrDetail += " (server may not accept SQL authentication)"
			return res
		}
		res.Outcome = core.OutcomeAuthFailed
		res.Err = core.ErrAuthRejected
	case 18452, 18450: // not associated with trusted connection / permission
		res.Outcome = core.OutcomeAuthFailed
		res.Err = core.ErrAuthRejected
	case 18470: // account disabled
		res.Outcome = core.OutcomeAccountLocked
		res.Err = core.ErrAuthRejected
	case 18465, 18466, 18486: // locked out
		res.Outcome = core.OutcomeAccountLocked
		res.Err = core.ErrAuthRejected
	case 18487, 18488: // password expired / must change
		// 密码正确但需修改：凭证有效。
		res.Outcome = core.OutcomeMFARequired
		res.Err = core.ErrAuthRejected
		res.ErrDetail = "password valid but must change"
	case 4064: // cannot open default database（凭证有效）
		res.Outcome = core.OutcomeMFARequired
		res.Err = core.ErrAuthRejected
		res.ErrDetail = "credentials valid but default database unavailable"
	case 233: // connection init error
		res.Outcome = core.OutcomeTargetUnavailable
		res.Err = core.ErrIO
	case 10054, 10060, 232, 53: // 连接层
		res.Outcome = core.OutcomeTargetUnavailable
		res.Err = core.ErrIO
	case 17187, 1789, 1801: // server 拒绝/繁忙
		res.Outcome = core.OutcomeRateLimited
		res.Err = core.ErrIO
		res.RetryAfter = 10 * time.Second
	default:
		res.Outcome = core.OutcomeUnknown
		res.Err = core.ErrAuthRejected
	}
	return res
}

// ---- 工具 ----

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

func sanitize(err error) string {
	msg := err.Error()
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return msg
}

func isContextErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func readFailure(ctx context.Context, err error, transport core.Transport) core.Result {
	if ctx.Err() != nil || isContextErr(err) {
		return core.Result{Outcome: core.OutcomeCancelled, Transport: transport, Err: core.ErrDeadline}
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return core.Result{Outcome: core.OutcomeTargetUnavailable, Transport: transport, Err: core.ErrIO, ErrDetail: "server closed during handshake"}
	}
	if errors.Is(err, errTDS) || strings.Contains(err.Error(), "tds:") {
		return core.Result{Outcome: core.OutcomeProtocolMismatch, Transport: transport, Err: core.ErrProtocolParse, ErrDetail: sanitize(err)}
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return core.Result{Outcome: core.OutcomeTargetUnavailable, Transport: transport, Err: core.ErrDeadline, ErrDetail: "i/o timeout"}
	}
	return core.Result{Outcome: core.OutcomeTargetUnavailable, Transport: transport, Err: core.ErrIO, ErrDetail: sanitize(err)}
}

func dialFailure(ctx context.Context, err error, transport core.Transport) core.Result {
	if ctx.Err() != nil || isContextErr(err) {
		return core.Result{Outcome: core.OutcomeCancelled, Transport: transport, Err: core.ErrCancelled}
	}
	return core.Result{Outcome: core.OutcomeTargetUnavailable, Transport: transport, Err: core.ErrDial, ErrDetail: sanitize(err)}
}

// ServiceInfo 返回服务描述。
func ServiceInfo() core.ServiceInfo {
	return core.ServiceInfo{
		Name:             "mssql",
		DefaultPort:      1433,
		DefaultUsernames: []string{"administrator", "admin", "root", "mssql", "manager", "sa"},
		DefaultPasswords: dicts.CommonPasswords,
		Prober:           Prober{},
	}
}

// Register 注册探针。
func Register() { core.Register(ServiceInfo()) }

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ParsePreloginForFuzz / ParseTokensForFuzz 仅供模糊测试导出。
func ParsePreloginForFuzz(payload []byte) (map[byte][]byte, error) {
	return parsePrelogin(payload)
}

func ParseTokensForFuzz(payload []byte) (*serverError, bool, error) {
	return parseTokens(payload)
}
