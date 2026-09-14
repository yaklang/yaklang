package mssql

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"math/big"
	"net"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
)

// mssqlSim 模拟 MSSQL 服务端（PRELOGIN + LOGIN7 + token 响应）。
type mssqlSim struct {
	t        *testing.T
	listener net.Listener
	addr     string

	validUser, validPass string
	encrypt              byte // PRELOGIN 响应的加密值：0/1/2/3
	requireTLS           bool // encrypt=1/3 时做 TLS
	scriptErr            *serverError
	closeAfterPrelogin   bool
	noEncryptionField    bool

	mu       sync.Mutex
	attempts int
	logins   []loginRecord
	wg       sync.WaitGroup
}

type loginRecord struct {
	username string // 解码出的用户名（UCS2）
	password string // 反混淆出的密码
	raw      []byte
}

func newMssqlSim(t *testing.T) *mssqlSim {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return &mssqlSim{t: t, listener: l, addr: l.Addr().String(), encrypt: encryptNotSup}
}

func (s *mssqlSim) start() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			conn, err := s.listener.Accept()
			if err != nil {
				return
			}
			s.wg.Add(1)
			go func(c net.Conn) {
				defer s.wg.Done()
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(5 * time.Second))
				s.handleConn(c)
			}(conn)
		}
	}()
}

func (s *mssqlSim) stop() {
	_ = s.listener.Close()
	s.wg.Wait()
}

func (s *mssqlSim) lastLogin() loginRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.logins) == 0 {
		return loginRecord{}
	}
	return s.logins[len(s.logins)-1]
}

var (
	mssqlCertOnce sync.Once
	mssqlCert     tls.Certificate
)

func mssqlSelfSignedCert() tls.Certificate {
	mssqlCertOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		tmpl := &x509.Certificate{
			SerialNumber:          big.NewInt(3),
			NotBefore:             time.Now().Add(-time.Hour),
			NotAfter:              time.Now().Add(time.Hour),
			KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			BasicConstraintsValid: true,
			IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
			DNSNames:              []string{"localhost"},
			Subject:               pkix.Name{CommonName: "localhost"},
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
		if err != nil {
			panic(err)
		}
		mssqlCert = tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	})
	return mssqlCert
}

func (s *mssqlSim) handleConn(conn net.Conn) {
	s.mu.Lock()
	s.attempts++
	s.mu.Unlock()

	stream := newTDSStream(conn)

	// 1. 读 PRELOGIN
	pktType, _, err := stream.readPacket()
	if err != nil || pktType != packPreLogin {
		return
	}
	if s.closeAfterPrelogin {
		return
	}
	// 响应 PRELOGIN
	var resp []byte
	if s.noEncryptionField {
		resp = buildPreloginWithoutEncryption()
	} else {
		resp = buildPreloginResponse(s.encrypt)
	}
	stream.beginPacket(packPreLogin)
	stream.write(resp)
	if err := stream.finishPacket(); err != nil {
		return
	}

	// 2. TLS 升级（服务端要求加密时）
	if s.encrypt == encryptOn || s.encrypt == encryptReq {
		// TLS-in-TDS：把 TLS 握手记录封装在 TDS 包里
		handshake := &serverHandshakeRW{raw: conn, stream: newTDSStream(conn)}
		tlsServer := tls.Server(&netConnAdapter{rw: handshake, raw: conn},
			&tls.Config{Certificates: []tls.Certificate{mssqlSelfSignedCert()}})
		if err := tlsServer.Handshake(); err != nil {
			return
		}
		handshake.markDone()
		stream.conn = tlsServer
		stream.tlsActive = true
		_ = stream.conn.SetDeadline(time.Now().Add(5 * time.Second))
	}

	// 3. 读 LOGIN7
	pktType, loginPayload, err := stream.readPacket()
	if err != nil || pktType != packLogin7 {
		return
	}
	rec := parseLogin7Record(loginPayload)
	s.mu.Lock()
	s.logins = append(s.logins, rec)
	s.mu.Unlock()

	// 4. 响应
	if s.scriptErr != nil {
		writeReplyToken(stream, errorTokenBytes(s.scriptErr))
		return
	}
	if rec.username == s.validUser && rec.password == s.validPass {
		// LOGINACK
		writeReplyToken(stream, loginAckToken())
	} else {
		writeReplyToken(stream, errorTokenBytes(&serverError{
			Number:  18456,
			State:   1,
			Class:   14,
			Message: "Login failed for user '" + rec.username + "'.",
		}))
	}
}

// serverHandshakeRW 服务端侧 TLS-in-TDS（握手后切换直通）。
type serverHandshakeRW struct {
	raw    net.Conn
	stream *tdsStream
	remain []byte
	done   bool
}

func (h *serverHandshakeRW) Write(b []byte) (int, error) {
	if h.done {
		return h.raw.Write(b)
	}
	h.stream.beginPacket(packPreLogin)
	h.stream.write(b)
	if err := h.stream.finishPacket(); err != nil {
		return 0, err
	}
	return len(b), nil
}

func (h *serverHandshakeRW) Read(b []byte) (int, error) {
	if h.done {
		return h.raw.Read(b)
	}
	if len(h.remain) == 0 {
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

func (h *serverHandshakeRW) markDone() { h.done = true }

func buildPreloginResponse(encrypt byte) []byte {
	fields := []struct {
		token byte
		data  []byte
	}{
		{preloginVERSION, []byte{0x0f, 0, 0, 0, 0, 0}},
		{preloginENCRYPTION, []byte{encrypt}},
		{preloginINSTOPT, []byte{0}},
		{preloginTHREADID, []byte{0, 0, 0, 0}},
		{preloginMARS, []byte{0}},
	}
	headerLen := len(fields)*5 + 1
	var out, data []byte
	for _, f := range fields {
		out = append(out, f.token)
		out = binary.BigEndian.AppendUint16(out, uint16(headerLen+len(data)))
		out = binary.BigEndian.AppendUint16(out, uint16(len(f.data)))
		data = append(data, f.data...)
	}
	out = append(out, 0xFF)
	out = append(out, data...)
	return out
}

func buildPreloginWithoutEncryption() []byte {
	// 去掉 ENCRYPTION 字段：重建（只剩 VERSION + MARS）
	fields := []struct {
		token byte
		data  []byte
	}{
		{preloginVERSION, []byte{0x0f, 0, 0, 0, 0, 0}},
		{preloginMARS, []byte{0}},
	}
	headerLen := len(fields)*5 + 1
	var out, data []byte
	for _, f := range fields {
		out = append(out, f.token)
		out = binary.BigEndian.AppendUint16(out, uint16(headerLen+len(data)))
		out = binary.BigEndian.AppendUint16(out, uint16(len(f.data)))
		data = append(data, f.data...)
	}
	out = append(out, 0xFF)
	out = append(out, data...)
	return out
}

// parseLogin7Record 从 LOGIN7 载荷提取用户名/密码（服务端反混淆）。
func parseLogin7Record(payload []byte) loginRecord {
	if len(payload) < 94 {
		return loginRecord{raw: payload}
	}
	// 头部偏移：UserName 对在 40-43，Password 对在 44-47
	userOff := int(binary.LittleEndian.Uint16(payload[40:42]))
	userLen := int(binary.LittleEndian.Uint16(payload[42:44]))
	passOff := int(binary.LittleEndian.Uint16(payload[44:46]))
	passLen := int(binary.LittleEndian.Uint16(payload[46:48]))
	rec := loginRecord{raw: payload}
	if userOff+userLen*2 <= len(payload) {
		rec.username = ucs2ToString(payload[userOff : userOff+userLen*2])
	}
	if passOff+passLen*2 <= len(payload) {
		mangled := payload[passOff : passOff+passLen*2]
		unmangled := make([]byte, len(mangled))
		for i, ch := range mangled {
			// 反混淆：先 XOR 再半字节交换（与混淆顺序相反）
			x := ch ^ 0xA5
			unmangled[i] = (x<<4)&0xff | x>>4
		}
		rec.password = ucs2ToString(unmangled)
	}
	return rec
}

func writeReplyToken(stream *tdsStream, token []byte) {
	stream.beginPacket(packReply)
	stream.write(token)
	_ = stream.finishPacket()
}

func loginAckToken() []byte {
	buf := []byte{tokenLoginAck}
	body := []byte{0x01}                  // interface
	body = append(body, 0x74, 0, 0, 0x04) // TDS 7.4
	body = append(body, 0x06, 0x00)       // prog name len
	body = append(body, toUCS2("yak")...) // prog name
	body = append(body, 0x0f, 0, 0, 0)    // version
	buf = binary.LittleEndian.AppendUint16(buf, uint16(len(body)))
	return append(buf, body...)
}

func errorTokenBytes(e *serverError) []byte {
	msgUCS2 := toUCS2(e.Message)
	body := binary.LittleEndian.AppendUint32(nil, uint32(e.Number))
	body = append(body, e.State, e.Class)
	body = binary.LittleEndian.AppendUint16(body, uint16(len(msgUCS2)))
	body = append(body, msgUCS2...)
	body = append(body, 4, 's', 'r', 'v', 0)         // server name BVARCHAR(4)
	body = append(body, 0)                           // proc name BVARCHAR(0)
	body = binary.LittleEndian.AppendUint16(body, 1) // line no
	buf := []byte{tokenError}
	buf = binary.LittleEndian.AppendUint16(buf, uint16(len(body)))
	return append(buf, body...)
}

// ---- 测试辅助 ----

func mssqlProbeOnce(t *testing.T, addr, user, pass string) core.Result {
	t.Helper()
	target, err := core.ParseTarget(addr)
	if err != nil {
		t.Fatal(err)
	}
	var p Prober
	return p.Probe(context.Background(), target, core.Credential{Username: user, Password: pass},
		core.Options{Timeout: 3 * time.Second})
}

var _ = runtime.NumGoroutine
var _ = strings.Contains
