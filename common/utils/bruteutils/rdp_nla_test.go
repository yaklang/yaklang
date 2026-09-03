package bruteutils

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"errors"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/core"
	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/nla"
)

// 最小 CredSSP/NLA 模拟服务器：覆盖 X.224 协商(HYBRID) → TLS →
// NTLMv2 challenge/response 的完整认证路径。
//
// 背景：xrdp 官方主线不支持服务端 NLA（源码注明 "We don't yet support
// CredSSP"），真实 NLA 目标以 Windows 为主。本模拟服务器在服务端按
// 规范验证 NTProofStr，从而对 grdp 的 NTLMv2 客户端实现做确定性
// 正反测试（模拟器侧断言 + 客户端行为断言）。
//
// 服务端验证公式（与 [MS-NLMP] 一致）：
//
//	NTLMv2Hash = HMAC-MD5(MD4(UTF16LE(password)), UTF16LE(UPPER(user)+domain))
//	NTProofStr = HMAC-MD5(NTLMv2Hash, serverChallenge + blob)
type nlaTestServer struct {
	listener net.Listener
	tlsCert  tls.Certificate

	mu         sync.Mutex
	lastResult *nlaVerifyRecord
	conns      int
	knownUsers map[string]string // user -> password（小写 user 匹配）

	// credsspVersion is the version advertised in server TSRequests.
	// 0 means v2 (legacy mock). 6 enables errorCode on auth failure.
	credsspVersion int
	// berLongForm：边缘 BER 长度（82 00 xx）。Win7 风格 tlsDropOnFail
	// 不发 errorCode，直接拆 TLS。
	berLongForm    bool
	tlsDropOnFail  bool
}

type nlaVerifyRecord struct {
	Verified    bool
	User        string
	Domain      string
	GotAuth     bool // 客户端完成 Challenge 应答
	GotAuthInfo bool // 客户端发送了最终 AuthInfo（NLA 全通过的证据）
}

func startNLATestServer(t *testing.T, users map[string]string) *nlaTestServer {
	return startNLATestServerVer(t, users, 0)
}

func startNLATestServerBERLong(t *testing.T, users map[string]string) *nlaTestServer {
	s := startNLATestServerVer(t, users, 6)
	s.berLongForm = true
	return s
}

func startNLATestServerWin7(t *testing.T, users map[string]string) *nlaTestServer {
	s := startNLATestServerVer(t, users, 2)
	s.tlsDropOnFail = true
	return s
}

func startNLATestServerVer(t *testing.T, users map[string]string, ver int) *nlaTestServer {
	t.Helper()
	// 自签证书
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "nla-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &nlaTestServer{listener: ln, tlsCert: cert, knownUsers: users, credsspVersion: ver}
	go srv.serve()
	t.Cleanup(func() { ln.Close() })
	return srv
}

func (s *nlaTestServer) addr() string { return s.listener.Addr().String() }

func (s *nlaTestServer) protoVersion() int {
	if s.credsspVersion == 0 {
		return nla.CredSSPVersion2
	}
	return s.credsspVersion
}

func (s *nlaTestServer) rejectAuth(conn *tls.Conn) error {
	// Win7：Authenticate 之后直接 TLS alert/拆连接。Win10+：errorCode。
	if !s.tlsDropOnFail && s.credsspVersion >= nla.CredSSPVersion6 {
		_, _ = conn.Write(nla.EncodeTSRequestError(s.protoVersion(), 0xC000006D))
	}
	return errAuthFail
}

func (s *nlaTestServer) record(r *nlaVerifyRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastResult = r
	s.conns++
}

func (s *nlaTestServer) result() *nlaVerifyRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastResult
}

func (s *nlaTestServer) waitResult(t *testing.T, wantVerify bool) *nlaVerifyRecord {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		res := s.result()
		if res != nil && res.GotAuth && res.Verified == wantVerify {
			return res
		}
		time.Sleep(5 * time.Millisecond)
	}
	return s.result()
}

func (s *nlaTestServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *nlaTestServer) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))

	// 1. X.224 Connection Request → Confirm（选择 HYBRID/CredSSP）
	if err := x224NegotiateNLA(conn); err != nil {
		return
	}

	// 2. TLS（服务器侧）
	tlsConn := tls.Server(conn, &tls.Config{
		Certificates: []tls.Certificate{s.tlsCert},
	})
	if err := tlsConn.Handshake(); err != nil {
		return
	}

	// 3. CredSSP/NTLM
	rec := &nlaVerifyRecord{}
	defer s.record(rec)
	if err := s.credssp(tlsConn, rec); err != nil {
		return
	}
}

// x224NegotiateNLA 读取 X.224 Connection Request，响应 HYBRID 被选中。
func x224NegotiateNLA(conn net.Conn) error {
	// TPKT(4) + X.224 CR（长度可变，含 RDP_NEG_REQ）
	head := make([]byte, 4)
	if _, err := ioReadFull(conn, head); err != nil {
		return err
	}
	pduLen := int(binary.BigEndian.Uint16(head[2:4]))
	body := make([]byte, pduLen-4)
	if _, err := ioReadFull(conn, body); err != nil {
		return err
	}
	// 响应：X.224 Connection Confirm + RDP_NEG_RSP(selected=HYBRID 0x02)
	cc := []byte{
		0x0e,       // LI
		0xd0,       // Code: CC | EOT
		0x00, 0x00, // dst-ref
		0x12, 0x34, // src-ref
		0x00, // class
		// NEG_RSP(0x02) flags=0 length=8(LE) selected=PROTOCOL_HYBRID(2, LE)
		0x02, 0x00, 0x08, 0x00,
		0x02, 0x00, 0x00, 0x00,
	}
	tpkt := append([]byte{0x03, 0x00, 0x00, byte(4 + len(cc))}, cc...)
	_, err := conn.Write(tpkt)
	return err
}

func ioReadFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func (s *nlaTestServer) credssp(conn *tls.Conn, rec *nlaVerifyRecord) error {
	// 3a. 收 TSRequest(Negotiate)
	negoReq, err := readTSRequest(conn)
	if err != nil {
		return err
	}
	if len(negoReq.NegoTokens) == 0 {
		return errAuthFail
	}
	// 3b. 发 TSRequest(Challenge)
	serverChallenge := make([]byte, 8)
	_, _ = rand.Read(serverChallenge)
	challengeMsg := buildNTLMChallenge(serverChallenge)
	ch := nla.EncodeDERTRequest(s.protoVersion(), []nla.Message{challengeMsg}, nil, nil, nil)
	if s.berLongForm {
		if padded, err := nla.PadBERLongForm(ch); err == nil {
			ch = padded
		}
	}
	if _, err := conn.Write(ch); err != nil {
		return err
	}
	// 3c. 收 TSRequest(Authenticate)
	authReq, err := readTSRequest(conn)
	if err != nil {
		return err
	}
	rec.GotAuth = true
	if len(authReq.NegoTokens) == 0 {
		return errAuthFail
	}
	user, domain, ntResp, err := parseNTLMAuthenticate(authReq.NegoTokens[0].Data)
	if err != nil {
		return err
	}
	rec.User, rec.Domain = user, domain

	// 3d. 服务端验证 NTProofStr
	password, known := s.knownUsers[user]
	if !known {
		return s.rejectAuth(conn)
	}
	if !verifyNTLMv2(password, user, domain, serverChallenge, ntResp) {
		return s.rejectAuth(conn)
	}
	rec.Verified = true

	// 3e. 发 PubKeyAuth（客户端不校验内容）→ 收最终 AuthInfo
	if _, err := conn.Write(nla.EncodeDERTRequest(s.protoVersion(), nil, nil, []byte("pubkey-placeholder"), nil)); err != nil {
		return err
	}
	final, err := readTSRequest(conn)
	if err != nil {
		return err
	}
	if len(final.AuthInfo) > 0 {
		rec.GotAuthInfo = true
	}
	// NLA 完成；模拟器不实现后续 MCS/GCC，主动断开
	return nil
}

var errAuthFail = &nlaAuthError{}

type nlaAuthError struct{}

func (*nlaAuthError) Error() string { return "ntlm authentication failed" }

func readTSRequest(conn *tls.Conn) (*nla.TSRequest, error) {
	// TSRequest 是 DER 编码；先按 TPKT 读取（CredSSP 走 TLS 后无 TPKT，
	// 直接读 DER：读 1 字节头 + 长度再读体）
	head := make([]byte, 2)
	if _, err := ioReadFull(conn, head); err != nil {
		return nil, err
	}
	// head[0]=0x30 SEQUENCE；长度可能是短格式或长格式
	var bodyLen int
	if head[1]&0x80 == 0 {
		bodyLen = int(head[1])
		rest := make([]byte, bodyLen)
		if _, err := ioReadFull(conn, rest); err != nil {
			return nil, err
		}
		return decodeTS(append(head, rest...))
	}
	numLenBytes := int(head[1] & 0x7f)
	lenBytes := make([]byte, numLenBytes)
	if _, err := ioReadFull(conn, lenBytes); err != nil {
		return nil, err
	}
	for _, b := range lenBytes {
		bodyLen = bodyLen<<8 | int(b)
	}
	rest := make([]byte, bodyLen)
	if _, err := ioReadFull(conn, rest); err != nil {
		return nil, err
	}
	out := append(head, lenBytes...)
	return decodeTS(append(out, rest...))
}

func decodeTS(der []byte) (*nla.TSRequest, error) {
	return nla.DecodeDERTRequest(der)
}

// buildNTLMChallenge 构造 NTLM type2 挑战（手工序列化，避开 struc 变长问题）。
// 布局 [MS-NLMP] 2.2.2.2：
//
//	sig(8) type(4) target(8) flags(4) challenge(8) reserved(8)
//	targetInfo(8) version(8) payload(targetName+targetInfo)
func buildNTLMChallenge(serverChallenge []byte) nla.Message {
	const (
		flagUnicode                 = 0x00000001
		flagRequestTarget           = 0x00000004
		flagNTLM                    = 0x00000200
		flagExtendedSessionSecurity = 0x00080000
		flagTargetInfo              = 0x00800000
		flagVersion                 = 0x02000000
	)
	var flags uint32 = flagUnicode | flagRequestTarget | flagNTLM |
		flagExtendedSessionSecurity | flagTargetInfo | flagVersion

	targetName := core.UnicodeEncode("NLA-SRV")
	// TargetInfo AV pairs
	var ti bytes.Buffer
	appendAV := func(id uint16, val []byte) {
		var idb [2]byte
		binary.LittleEndian.PutUint16(idb[:], id)
		ti.Write(idb[:])
		binary.LittleEndian.PutUint16(idb[:], uint16(len(val)))
		ti.Write(idb[:])
		ti.Write(val)
	}
	appendAV(0x0001, core.UnicodeEncode("NLA-SRV")) // NbComputerName
	appendAV(0x0002, core.UnicodeEncode("NLADOM"))  // NbDomainName
	appendAV(0x0007, make([]byte, 8))               // Timestamp
	appendAV(0x0000, nil)                           // EOL

	headerLen := 8 + 4 + 8 + 4 + 8 + 8 + 8 + 8 // 含 version
	targetOff := uint32(headerLen)
	tiOff := targetOff + uint32(len(targetName))

	le16 := func(v uint16) []byte { b := make([]byte, 2); binary.LittleEndian.PutUint16(b, v); return b }
	le32 := func(v uint32) []byte { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, v); return b }

	buf := &bytes.Buffer{}
	buf.Write([]byte{0x4e, 0x54, 0x4c, 0x4d, 0x53, 0x53, 0x50, 0x00}) // "NTLMSSP\0"
	buf.Write(le32(2))                                                // MessageType
	buf.Write(le16(uint16(len(targetName))))                          // TargetNameLen
	buf.Write(le16(uint16(len(targetName))))                          // TargetNameMaxLen
	buf.Write(le32(targetOff))
	buf.Write(le32(flags))
	buf.Write(serverChallenge)
	buf.Write(make([]byte, 8))        // reserved
	buf.Write(le16(uint16(ti.Len()))) // TargetInfoLen
	buf.Write(le16(uint16(ti.Len()))) // TargetInfoMaxLen
	buf.Write(le32(tiOff))
	buf.Write([]byte{6, 1, 0xb1, 0x1f, 0, 0, 0, 0x0f}) // version
	buf.Write(targetName)
	buf.Write(ti.Bytes())
	return &rawNTLMMessage{data: buf.Bytes()}
}

// rawNTLMMessage 包装手工序列化的 NTLM 消息以满足 nla.Message 接口。
type rawNTLMMessage struct{ data []byte }

func (m *rawNTLMMessage) Serialize() []byte { return m.data }

// parseNTLMAuthenticate 手工解析 NTLM type3，提取 user/domain/NT 响应。
func parseNTLMAuthenticate(msg []byte) (user, domain string, ntResp []byte, err error) {
	if len(msg) < 64 || string(msg[:7]) != "NTLMSSP" {
		return "", "", nil, errAuthFail
	}
	le16 := func(o int) uint16 { return binary.LittleEndian.Uint16(msg[o:]) }
	le32 := func(o int) uint32 { return binary.LittleEndian.Uint32(msg[o:]) }
	cut := func(lenOff int) []byte {
		l := int(le16(lenOff))
		off := int(le32(lenOff + 4))
		if off+l > len(msg) {
			return nil
		}
		return msg[off : off+l]
	}
	// 布局：sig(8) type(4)，随后每组 8 字节：
	// LM(12) NT(20) Domain(28) User(36) Workstation(44) EncRandomSession(52) Flags(60)
	ntResp = cut(20)
	domRaw := cut(28)
	userRaw := cut(36)
	decode := func(b []byte) string {
		if len(b)%2 == 1 {
			b = b[:len(b)-1]
		}
		return core.UnicodeDecode(b)
	}
	_ = le32
	return decode(userRaw), decode(domRaw), ntResp, nil
}

// verifyNTLMv2 服务端按 [MS-NLMP] 验证 NTLMv2 NTProofStr。
func verifyNTLMv2(password, user, domain string, serverChallenge, ntResp []byte) bool {
	if len(ntResp) < 16 {
		return false
	}
	ntProof := ntResp[:16]
	blob := ntResp[16:]

	v2hash := nla.HMAC_MD5(nla.MD4(core.UnicodeEncode(password)),
		core.UnicodeEncode(upper(user)+domain))
	expect := hmacMD5(v2hash, append(append([]byte{}, serverChallenge...), blob...))
	return hmac.Equal(expect, ntProof)
}

func hmacMD5(key, data []byte) []byte {
	h := hmac.New(md5.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func upper(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'a' && b[i] <= 'z' {
			b[i] -= 0x20
		}
	}
	return string(b)
}

// TestRDPNLAAuthentication CredSSP/NTLMv2 认证路径正反测试：
// 模拟器侧断言 NTProofStr 验证结果，客户端侧断言连接行为。
func TestRDPNLAAuthentication(t *testing.T) {
	users := map[string]string{
		"rdpuser":   "RdpPass123!",
		"unicode用户": "密码🔐123",
	}
	srv := startNLATestServer(t, users)

	cases := []struct {
		name         string
		user         string
		pass         string
		wantVerify   bool
		wantAuthInfo bool
	}{
		{"correct-creds", "rdpuser", "RdpPass123!", true, true},
		{"wrong-password", "rdpuser", "WRONG-pass", false, false},
		{"unknown-user", "no-such-user", "whatever", false, false},
		{"unicode-correct", "unicode用户", "密码🔐123", true, true},
		{"unicode-wrong", "unicode用户", "错误密码", false, false},
		{"empty-password", "rdpuser", "", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			host, portStr, _ := splitHostPort(srv.addr())
			port := 0
			for _, ch := range portStr {
				port = port*10 + int(ch-'0')
			}
			ok, err := rdpLoginContext(context.Background(), host, host, c.user, c.pass, port)
			gotErr := err != nil
			res := srv.waitResult(t, c.wantVerify)
			if res == nil {
				t.Fatalf("no connection reached NLA stage (err=%v)", err)
			}
			if res.Verified != c.wantVerify {
				t.Errorf("server verify=%v want %v (user=%q err=%v)",
					res.Verified, c.wantVerify, res.User, err)
			}
			if res.GotAuthInfo != c.wantAuthInfo {
				t.Errorf("client AuthInfo=%v want %v", res.GotAuthInfo, c.wantAuthInfo)
			}
			if c.wantVerify {
				if !ok || gotErr {
					t.Errorf("correct NLA creds must report brute success (ok=%v err=%v)", ok, err)
				}
			} else if ok || !gotErr {
				t.Errorf("client reported success for failed NLA auth (ok=%v err=%v)", ok, err)
			}
			t.Logf("case=%s server: verified=%v gotAuth=%v gotAuthInfo=%v user=%q domain=%q; client: ok=%v err=%v",
				c.name, res.Verified, res.GotAuth, res.GotAuthInfo, res.User, res.Domain, ok, err)
		})
	}
}

// TestRDPNLAv6LogonFailure 覆盖 Windows 10/2016+ 的 CredSSP v6 失败信号：
// 服务端在 NTLM 失败后发 TSRequest.errorCode=STATUS_LOGON_FAILURE，
// 客户端必须在数秒内返回 CredSSPError 而不是 15s 超时或 tls 误分类。
func TestRDPNLAv6LogonFailure(t *testing.T) {
	users := map[string]string{"rdpuser": "RdpPass123!"}
	srv := startNLATestServerVer(t, users, nla.CredSSPVersion6)

	host, portStr, _ := splitHostPort(srv.addr())
	port := atoiDefault(portStr, 0)

	ok, err := rdpLoginContext(context.Background(), host, host, "rdpuser", "WRONG", port)
	if ok {
		t.Fatal("wrong password must not succeed")
	}
	if err == nil {
		t.Fatal("wrong password must return an error")
	}
	var cssp *nla.CredSSPError
	if !errors.As(err, &cssp) {
		t.Fatalf("want CredSSPError, got %v", err)
	}
	if !cssp.AuthFailed() {
		t.Fatalf("want AuthFailed, got %v", err)
	}

	// 正确凭证：NLA 完成即爆破成功，不必再开 MCS。
	ok, err = rdpLoginContext(context.Background(), host, host, "rdpuser", "RdpPass123!", port)
	res := srv.waitResult(t, true)
	if res == nil || !res.Verified {
		t.Fatalf("correct creds: server verify=%v client ok=%v err=%v", res, ok, err)
	}
	if !ok || err != nil {
		t.Fatalf("correct NLA creds must report brute success, ok=%v err=%v", ok, err)
	}
}

// TestRDPBrutePassClassification 走 BrutePass 兼容层：正确凭证 Ok+Finished，
// 错误凭证不得 Finished（调度器要继续字典），不可达必须 Finished。
func TestRDPBrutePassClassification(t *testing.T) {
	users := map[string]string{"rdpuser": "RdpPass123!"}
	srv := startNLATestServerVer(t, users, nla.CredSSPVersion6)

	hit := func(user, pass string) *BruteItemResult {
		return rdpAuth.BrutePass(&BruteItem{
			Type:     "rdp",
			Target:   srv.addr(),
			Username: user,
			Password: pass,
			Context:  context.Background(),
		})
	}

	okRes := hit("rdpuser", "RdpPass123!")
	if !okRes.Ok || !okRes.Finished {
		t.Errorf("correct creds: want ok+finished, got ok=%v finished=%v", okRes.Ok, okRes.Finished)
	}

	bad := hit("rdpuser", "WRONG")
	if bad.Ok {
		t.Errorf("wrong password must not be ok")
	}
	if bad.Finished {
		t.Errorf("wrong password must not mark target finished (scheduler would skip the rest of the dict)")
	}

	unknown := hit("no-such-user", "x")
	if unknown.Ok || unknown.Finished {
		t.Errorf("unknown user: want !ok !finished, got ok=%v finished=%v", unknown.Ok, unknown.Finished)
	}

	dead := rdpAuth.BrutePass(&BruteItem{
		Type: "rdp", Target: "127.0.0.1:1", Username: "a", Password: "b",
		Context: context.Background(),
	})
	if dead.Ok || !dead.Finished {
		t.Errorf("unreachable: want finished+!ok, got ok=%v finished=%v", dead.Ok, dead.Finished)
	}
}
