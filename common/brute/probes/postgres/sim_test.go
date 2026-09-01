package postgres

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/binary"
	"math/big"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
	"github.com/yaklang/yaklang/common/brute/probes/internal/scram"
)

// pgSim 模拟 PostgreSQL 服务端。
type pgSim struct {
	t        *testing.T
	listener net.Listener
	addr     string

	authMethod string // cleartext | md5 | scram
	validUser  string
	validPass  string
	sslMode    string // off | on | reject(明确拒绝 SSL)
	requireSSL bool   // 客户端不使用 SSL 时报错

	scriptErr         *pgError // 每次认证直接回该错误（分类测试）
	closeAfterStartup bool

	mu       sync.Mutex
	attempts int
	startups []string // 记录用户名
	lastTLS  bool
	scramOK  bool // SCRAM 校验通过
	wg       sync.WaitGroup
}

func newPGSim(t *testing.T) *pgSim {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return &pgSim{t: t, listener: l, addr: l.Addr().String()}
}

func (s *pgSim) start() {
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

func (s *pgSim) stop() {
	_ = s.listener.Close()
	s.wg.Wait()
}

var (
	pgCertOnce sync.Once
	pgCert     tls.Certificate
)

func pgSelfSignedCert() tls.Certificate {
	pgCertOnce.Do(func() {
		key := simRSAKeyFor()
		tmpl := &x509.Certificate{
			SerialNumber:          big.NewInt(2),
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
		pgCert = tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	})
	return pgCert
}

func (s *pgSim) handleConn(conn net.Conn) {
	s.mu.Lock()
	s.attempts++
	s.mu.Unlock()

	// 先读 1 字节窥探：SSLRequest 以长度开头（首字节为长度高字节 = 0），Startup 首字节也为 0…
	// PG 用长度区分：SSLRequest 长度恒为 8，Startup 长度 >= 16。
	var lenBuf [4]byte
	if _, err := readFull(conn, lenBuf[:]); err != nil {
		return
	}
	pktLen := int(binary.BigEndian.Uint32(lenBuf[:]))
	if pktLen == 8 {
		// SSLRequest
		var code [4]byte
		if _, err := readFull(conn, code[:]); err != nil {
			return
		}
		switch s.sslMode {
		case "on":
			_, _ = conn.Write([]byte{'S'})
			tlsConn := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{pgSelfSignedCert()}})
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			conn = tlsConn
			s.mu.Lock()
			s.lastTLS = true
			s.mu.Unlock()
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			s.handleStartupAuth(conn)
		case "reject":
			_, _ = conn.Write([]byte{'N'})
			s.handleStartupAuth(conn) // 继续明文
		default: // off：视为不支持
			_, _ = conn.Write([]byte{'N'})
			s.handleStartupAuth(conn)
		}
		return
	}
	if pktLen < 16 || pktLen > 1<<20 {
		return
	}
	// StartupMessage：回退读取
	body := make([]byte, pktLen-4)
	if _, err := readFull(conn, body); err != nil {
		return
	}
	version := binary.BigEndian.Uint32(body[:4])
	if version != protocolVersion {
		_ = writeMsg(conn, msgError, errorResponsePayload("FATAL", "0A000", "unsupported frontend protocol"))
		return
	}
	username := parseStartupUser(body[4:])
	s.mu.Lock()
	s.startups = append(s.startups, username)
	s.mu.Unlock()

	if s.closeAfterStartup {
		return
	}
	s.authenticate(conn, username)
}

// handleStartupAuth 读取 StartupMessage 后进入认证。
func (s *pgSim) handleStartupAuth(conn net.Conn) {
	var lenBuf [4]byte
	if _, err := readFull(conn, lenBuf[:]); err != nil {
		return
	}
	pktLen := int(binary.BigEndian.Uint32(lenBuf[:]))
	if pktLen < 16 || pktLen > 1<<20 {
		return
	}
	body := make([]byte, pktLen-4)
	if _, err := readFull(conn, body); err != nil {
		return
	}
	username := parseStartupUser(body[4:])
	s.mu.Lock()
	s.startups = append(s.startups, username)
	s.mu.Unlock()
	if s.closeAfterStartup {
		return
	}
	s.authenticate(conn, username)
}

func parseStartupUser(body []byte) string {
	for len(body) > 0 {
		end := indexOfByte(body, 0)
		if end < 0 {
			return ""
		}
		key := string(body[:end])
		body = body[end+1:]
		end = indexOfByte(body, 0)
		if end < 0 {
			return ""
		}
		val := string(body[:end])
		body = body[end+1:]
		if key == "user" {
			return val
		}
	}
	return ""
}

func (s *pgSim) authenticate(conn net.Conn, username string) {
	fail := func(code, msg string) {
		_ = writeMsg(conn, msgError, errorResponsePayload("FATAL", code, msg))
	}
	if s.scriptErr != nil {
		_ = writeMsg(conn, msgError, errorResponsePayload(s.scriptErr.Severity, s.scriptErr.Code, s.scriptErr.Message))
		return
	}
	if s.requireSSL {
		s.mu.Lock()
		tlsUsed := s.lastTLS
		s.mu.Unlock()
		if !tlsUsed {
			fail("28000", "no pg_hba.conf entry for host, SSL off")
			return
		}
	}

	switch s.authMethod {
	case "cleartext":
		payload := make([]byte, 4)
		binary.BigEndian.PutUint32(payload, authCleartext)
		if err := writeMsg(conn, msgAuth, payload); err != nil {
			return
		}
		msgType, pw, err := readMsg(conn)
		if err != nil || msgType != msgPassword {
			return
		}
		if username == s.validUser && strings.TrimRight(string(pw), "\x00") == s.validPass {
			s.authOK(conn)
		} else {
			fail("28P01", "password authentication failed for user \""+username+"\"")
		}
	case "md5":
		salt := make([]byte, 4)
		_, _ = rand.Read(salt)
		payload := make([]byte, 8)
		binary.BigEndian.PutUint32(payload, authMD5)
		copy(payload[4:], salt)
		if err := writeMsg(conn, msgAuth, payload); err != nil {
			return
		}
		msgType, pw, err := readMsg(conn)
		if err != nil || msgType != msgPassword {
			return
		}
		want := md5AuthResponse(username, s.validPass, salt)
		if username == s.validUser && strings.TrimRight(string(pw), "\x00") == want {
			s.authOK(conn)
		} else {
			fail("28P01", "password authentication failed for user \""+username+"\"")
		}
	case "scram":
		s.doSCRAMServer(conn, username)
	default:
		// trust：直接成功
		s.authOK(conn)
	}
}

// doSCRAMServer 实现 SCRAM-SHA-256 服务端。
func (s *pgSim) doSCRAMServer(conn net.Conn, username string) {
	fail := func(code, msg string) {
		_ = writeMsg(conn, msgError, errorResponsePayload("FATAL", code, msg))
	}
	// 发 SASL 机制列表：int32 类型 + "SCRAM-SHA-256\0\0"
	payload := make([]byte, 4, 4+len("SCRAM-SHA-256")+2)
	binary.BigEndian.PutUint32(payload, authSASL)
	payload = append(payload, "SCRAM-SHA-256"...)
	payload = append(payload, 0, 0)
	if err := writeMsg(conn, msgAuth, payload); err != nil {
		return
	}
	msgType, init, err := readMsg(conn)
	if err != nil || msgType != msgPassword {
		return
	}
	// SASLInitialResponse: mechanism\0 + int32 len + data
	zero := indexOfByte(init, 0)
	if zero < 0 || string(init[:zero]) != "SCRAM-SHA-256" {
		fail("28P01", "unsupported SASL mechanism")
		return
	}
	respLen := int(int32(binary.BigEndian.Uint32(init[zero+1:])))
	if len(init) < zero+5+respLen {
		return
	}
	clientFirst := string(init[zero+5 : zero+5+respLen])
	if !strings.HasPrefix(clientFirst, "n,,") {
		fail("28P01", "malformed SCRAM message")
		return
	}
	clientFirstBare := clientFirst[3:]

	// server-first
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	iterations := 4096
	clientNonce := scramNonceFrom(clientFirstBare)
	serverNonce := clientNonce + base64.StdEncoding.EncodeToString([]byte("srv"))
	serverFirst := "r=" + serverNonce + ",s=" + base64.StdEncoding.EncodeToString(salt) + ",i=" + strconv.Itoa(iterations)
	cont := make([]byte, 4, 4+len(serverFirst))
	binary.BigEndian.PutUint32(cont, authSASLContinue)
	cont = append(cont, serverFirst...)
	if err := writeMsg(conn, msgAuth, cont); err != nil {
		return
	}

	msgType, finalMsg, err := readMsg(conn)
	if err != nil || msgType != msgPassword {
		s.t.Logf("sim: final read err=%v type=%q", err, msgType)
		return
	}
	clientFinal := string(finalMsg)
	if !strings.HasPrefix(clientFinal, "c=biws,r="+serverNonce) {
		fail("28P01", "malformed SCRAM final message")
		return
	}
	proofIdx := strings.LastIndex(clientFinal, ",p=")
	if proofIdx < 0 {
		return
	}
	withoutProof := clientFinal[:proofIdx]
	authMessage := clientFirstBare + "," + serverFirst + "," + withoutProof

	// 服务端独立验证 proof
	password := s.validPass
	salted := scram.PBKDF2(sha256.New, []byte(password), salt, iterations, 32)
	clientKey := hmacSHA256(salted, []byte("Client Key"))
	storedKey := sha256.Sum256(clientKey)
	clientSig := hmacSHA256(storedKey[:], []byte(authMessage))
	proofB64 := clientFinal[proofIdx+3:]
	proof, err := base64.StdEncoding.DecodeString(proofB64)
	if err != nil || len(proof) != len(clientKey) {
		fail("28P01", "invalid proof encoding")
		return
	}
	calcKey := make([]byte, len(proof))
	for i := range proof {
		calcKey[i] = proof[i] ^ clientSig[i]
	}
	if !hmac.Equal(calcKey, clientKey) || username != s.validUser {
		fail("28P01", "password authentication failed for user \""+username+"\"")
		return
	}
	s.mu.Lock()
	s.scramOK = true
	s.mu.Unlock()

	// server-final
	serverKey := hmacSHA256(salted, []byte("Server Key"))
	serverSig := hmacSHA256(serverKey, []byte(authMessage))
	finalPayload := make([]byte, 4, 4+len("v="+base64.StdEncoding.EncodeToString(serverSig)))
	binary.BigEndian.PutUint32(finalPayload, authSASLFinal)
	finalPayload = append(finalPayload, "v="+base64.StdEncoding.EncodeToString(serverSig)...)
	if err := writeMsg(conn, msgAuth, finalPayload); err != nil {
		return
	}
	s.authOK(conn)
}

func (s *pgSim) authOK(conn net.Conn) {
	okPayload := make([]byte, 4)
	binary.BigEndian.PutUint32(okPayload, authOK)
	if err := writeMsg(conn, msgAuth, okPayload); err != nil {
		return
	}
	_ = writeMsg(conn, msgParameterStatus, statusPayload("server_version", "15.4-sim"))
	_ = writeMsg(conn, msgBackendKeyData, []byte{1, 0, 0, 0, 2, 0, 0, 0})
	_ = writeMsg(conn, msgReadyForQuery, []byte{'I'})
}

func scramNonceFrom(clientFirstBare string) string {
	for _, part := range strings.Split(clientFirstBare, ",") {
		if strings.HasPrefix(part, "r=") {
			return part[2:]
		}
	}
	return ""
}

func hmacSHA256(key, data []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(data)
	return m.Sum(nil)
}

func statusPayload(k, v string) []byte {
	return append(append([]byte(k), 0), append([]byte(v), 0)...)
}

func errorResponsePayload(severity, code, msg string) []byte {
	p := []byte{'S'}
	p = append(p, severity...)
	p = append(p, 0, 'C')
	p = append(p, code...)
	p = append(p, 0, 'M')
	p = append(p, msg...)
	p = append(p, 0, 0)
	return p
}

func writeMsg(conn net.Conn, msgType byte, payload []byte) error {
	msg := make([]byte, 5+len(payload))
	msg[0] = msgType
	binary.BigEndian.PutUint32(msg[1:5], uint32(len(payload)+4))
	copy(msg[5:], payload)
	_, err := conn.Write(msg)
	return err
}

func readMsg(conn net.Conn) (byte, []byte, error) {
	var header [5]byte
	if _, err := readFull(conn, header[:]); err != nil {
		return 0, nil, err
	}
	length := int(binary.BigEndian.Uint32(header[1:5]))
	if length < 4 || length > 1<<20 {
		return 0, nil, errShortRead
	}
	payload := make([]byte, length-4)
	if _, err := readFull(conn, payload); err != nil {
		return 0, nil, err
	}
	return header[0], payload, nil
}

var errShortRead = &netError{}

type netError struct{}

func (*netError) Error() string   { return "short read" }
func (*netError) Timeout() bool   { return false }
func (*netError) Temporary() bool { return false }

func readFull(conn net.Conn, buf []byte) (int, error) {
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

var (
	simRSAOnce sync.Once
	simRSAKey  *rsa.PrivateKey
)

// simRSAKeyFor 生成模拟器自签证书用的 RSA 密钥。
func simRSAKeyFor() *rsa.PrivateKey {
	simRSAOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		simRSAKey = key
	})
	return simRSAKey
}

// ---- 测试辅助 ----

func pgProbeOnce(t *testing.T, addr, user, pass string) core.Result {
	t.Helper()
	return pgProbeOnceOpts(t, context.Background(), addr, user, pass, core.Options{Timeout: 3 * time.Second})
}

func pgProbeOnceOpts(t *testing.T, ctx context.Context, addr, user, pass string, opts core.Options) core.Result {
	t.Helper()
	target, err := core.ParseTarget(addr)
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}
	var p Prober
	return p.Probe(ctx, target, core.Credential{Username: user, Password: pass}, opts)
}
