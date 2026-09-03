package mysql

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
)

// mysqlSim 模拟 MySQL 服务端的认证交互，用于探针的行为测试。
type mysqlSim struct {
	t        *testing.T
	listener net.Listener
	addr     string

	// greeting 行为
	plugin        string // greeting 中声明的认证插件
	forceSSLCap   bool   // greeting 是否声明 CLIENT_SSL
	tlsServer     bool   // 是否真正接受 TLS 升级
	serverVersion string

	// 验证模式：按 scramble 判定成败
	verify           bool
	validUser        string
	validPass        string
	unicodeSensitive bool

	// 脚本模式：读到 handshake response 后按步骤应答
	script []scriptStep

	mu         sync.Mutex
	handshakes []handshakeRecord
	connCount  int
	wg         sync.WaitGroup
}

type handshakeRecord struct {
	nonce      []byte // 服务端下发的挑战值
	username   string
	plugin     string
	scramble   []byte // 客户端发送的认证响应
	tlsUsed    bool
	decrypted  string // RSA 全量认证解密出的明文密码
	clearPlain string // TLS 信道明文密码
	raw        []byte
}

type scriptStep struct {
	kind   string // ok | err | switch | moredata | close | delay
	errNum uint16
	errMsg string
	plugin string
	nonce  []byte
	data   byte
	d      time.Duration
}

func newMySQLSim(t *testing.T) *mysqlSim {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return &mysqlSim{
		t:             t,
		listener:      l,
		addr:          l.Addr().String(),
		serverVersion: "8.0.36-test",
	}
}

func (s *mysqlSim) start() {
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

func (s *mysqlSim) stop() {
	_ = s.listener.Close()
	s.wg.Wait()
}

func (s *mysqlSim) attempts() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connCount
}

func (s *mysqlSim) lastRecord() handshakeRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.handshakes) == 0 {
		return handshakeRecord{}
	}
	return s.handshakes[len(s.handshakes)-1]
}

var (
	simRSALock sync.Mutex
	simRSA     *rsa.PrivateKey
)

func simRSAKey() *rsa.PrivateKey {
	simRSALock.Lock()
	defer simRSALock.Unlock()
	if simRSA == nil {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		simRSA = key
	}
	return simRSA
}

var (
	certOnce sync.Once
	simCert  tls.Certificate
)

func selfSignedCert() tls.Certificate {
	certOnce.Do(func() {
		key := simRSAKey()
		tmpl := &x509.Certificate{
			SerialNumber:          big.NewInt(1),
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
		simCert = tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	})
	return simCert
}

// buildGreeting 构造协议版本 10 的 Initial Handshake Packet。
func (s *mysqlSim) buildGreeting() (payload []byte, nonce []byte) {
	nonce = make([]byte, 20)
	_, _ = rand.Read(nonce)
	plugin := s.plugin
	if plugin == "" {
		plugin = pluginNative
	}
	var caps uint32 = clientProtocol41 | clientSecureConn | clientPluginAuth
	if s.forceSSLCap {
		caps |= clientSSL
	}
	buf := make([]byte, 0, 128)
	buf = append(buf, 10)                             // protocol version
	buf = append(buf, s.serverVersion...)             // server version
	buf = append(buf, 0)                              // NUL
	buf = append(buf, 1, 0, 0, 0)                     // thread id
	buf = append(buf, nonce[:8]...)                   // auth data part 1
	buf = append(buf, 0)                              // filler
	buf = append(buf, byte(caps), byte(caps>>8))      // caps lower
	buf = append(buf, 45)                             // charset
	buf = append(buf, 2, 0)                           // status
	buf = append(buf, byte(caps>>16), byte(caps>>24)) // caps upper
	buf = append(buf, 21)                             // auth plugin data len
	buf = append(buf, make([]byte, 10)...)            // reserved
	buf = append(buf, nonce[8:]...)                   // auth data part 2 (12 字节)
	buf = append(buf, 0)                              // NUL
	buf = append(buf, plugin...)
	buf = append(buf, 0)
	return buf, nonce
}

func (s *mysqlSim) handleConn(conn net.Conn) {
	s.mu.Lock()
	s.connCount++
	s.mu.Unlock()

	pc := newPacketConn(conn)
	greet, nonce := s.buildGreeting()
	if err := pc.writePacket(greet); err != nil {
		return
	}

	rec := handshakeRecord{nonce: append([]byte{}, nonce...)}

	payload, err := pc.readPacket()
	if err != nil {
		return
	}

	// SSLRequest?
	if len(payload) == 32 {
		caps := uint32(payload[0]) | uint32(payload[1])<<8 | uint32(payload[2])<<16 | uint32(payload[3])<<24
		if caps&clientSSL != 0 {
			rec.tlsUsed = true
			if !s.tlsServer {
				return // 不支持 TLS：关闭，触发客户端明文重试
			}
			tlsConn := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{selfSignedCert()}})
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			conn = tlsConn
			pc.setStream(conn) // 序号跨 TLS 连续
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			if payload, err = pc.readPacket(); err != nil {
				return
			}
		}
	}

	// 解析 HandshakeResponse41
	pos := 4 + 4 + 1 + 23
	if pos >= len(payload) {
		return
	}
	end := indexByte(payload[pos:], 0)
	if end < 0 {
		return
	}
	rec.username = string(payload[pos : pos+end])
	pos += end + 1
	if pos >= len(payload) {
		s.mu.Lock()
		s.handshakes = append(s.handshakes, rec)
		s.mu.Unlock()
		_ = pc.writePacket(errPacket(1045, "28000", "Access denied"))
		return
	}
	respLen := int(payload[pos])
	pos++
	rec.scramble = append([]byte{}, payload[pos:pos+respLen]...)
	pos += respLen
	if pos < len(payload) {
		pend := indexByte(payload[pos:], 0)
		if pend >= 0 {
			rec.plugin = string(payload[pos : pos+pend])
		}
	}
	rec.raw = append([]byte{}, payload...)

	s.mu.Lock()
	s.handshakes = append(s.handshakes, rec)
	s.mu.Unlock()

	if s.verify && len(s.script) == 0 {
		if s.verifyScramble(rec) {
			_ = pc.writePacket(okPacket())
		} else {
			_ = pc.writePacket(errPacket(1045, "28000", "Access denied for user"))
		}
		return
	}

	for _, step := range s.script {
		switch step.kind {
		case "ok":
			_ = pc.writePacket(okPacket())
			return
		case "err":
			_ = pc.writePacket(errPacket(step.errNum, "HY000", step.errMsg))
			return
		case "switch":
			p := append([]byte{iEOF}, step.plugin...)
			p = append(p, 0)
			p = append(p, step.nonce...)
			if err := pc.writePacket(p); err != nil {
				return
			}
			resp, err := pc.readPacket()
			if err != nil {
				return
			}
			s.mu.Lock()
			if n := len(s.handshakes); n > 0 {
				s.handshakes[n-1].scramble = append([]byte{}, resp...)
				s.handshakes[n-1].plugin = step.plugin
			}
			s.mu.Unlock()
		case "moredata":
			p := append([]byte{iAuthMoreData}, step.data)
			if err := pc.writePacket(p); err != nil {
				return
			}
			if step.data == cachingFullAuth {
				if err := s.fullAuthExchange(pc, nonce, rec.username); err != nil {
					return
				}
				return
			}
		case "close":
			return
		case "raw-old-auth":
			// Protocol::OldAuthSwitchRequest：单字节 0xFE
			_ = pc.writePacket([]byte{iEOF})
			return
		case "delay":
			time.Sleep(step.d)
			_ = pc.writePacket(okPacket())
			return
		}
	}
	_ = pc.writePacket(errPacket(1045, "28000", "Access denied"))
}

// fullAuthExchange 处理 caching_sha2 全量认证的服务端侧。
func (s *mysqlSim) fullAuthExchange(pc *packetConn, nonce []byte, username string) error {
	req, err := pc.readPacket()
	if err != nil {
		return err
	}
	plain := ""
	switch {
	case len(req) == 1 && req[0] == cachingReqKey:
		pubDER, _ := x509.MarshalPKIXPublicKey(&simRSAKey().PublicKey)
		pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
		pd := append([]byte{iAuthMoreData}, pubPEM...)
		if err := pc.writePacket(pd); err != nil {
			return err
		}
		enc, err := pc.readPacket()
		if err != nil {
			return err
		}
		dec, err := rsa.DecryptOAEP(sha1.New(), rand.Reader, simRSAKey(), enc, nil)
		if err != nil {
			return err
		}
		for i := range dec {
			dec[i] ^= nonce[i%len(nonce)]
		}
		plain = strings.TrimRight(string(dec), "\x00")
	case len(req) > 0 && req[len(req)-1] == 0:
		plain = strings.TrimRight(string(req), "\x00")
	default:
		return errMalformedPacket
	}
	s.mu.Lock()
	if n := len(s.handshakes); n > 0 {
		s.handshakes[n-1].decrypted = plain
	}
	s.mu.Unlock()
	if plain == s.validPass && username == s.validUser {
		_ = pc.writePacket(okPacket())
	} else {
		_ = pc.writePacket(errPacket(1045, "28000", "Access denied"))
	}
	return nil
}

// verifyScramble 服务端独立计算 scramble 与客户端响应比对。
func (s *mysqlSim) verifyScramble(rec handshakeRecord) bool {
	if rec.username != s.validUser {
		return false
	}
	if s.validPass == "" {
		return len(rec.scramble) == 0
	}
	plugin := rec.plugin
	if plugin == "" {
		plugin = s.plugin
		if plugin == "" {
			plugin = pluginNative
		}
	}
	switch plugin {
	case pluginNative:
		want := scrambleNative(rec.nonce, s.validPass)
		return string(want) == string(rec.scramble)
	case pluginCachingSHA2:
		want := scrambleCachingSHA2(rec.nonce, s.validPass)
		return string(want) == string(rec.scramble)
	}
	return false
}

func okPacket() []byte {
	return []byte{iOK, 0, 0, 2, 0, 0, 0}
}

func errPacket(code uint16, sqlstate, msg string) []byte {
	p := []byte{iERR, byte(code), byte(code >> 8), '#'}
	p = append(p, sqlstate...)
	p = append(p, msg...)
	return p
}

// ---- 测试辅助 ----

func probeOnce(t *testing.T, addr, user, pass string) core.Result {
	t.Helper()
	return probeOnceCtx(t, context.Background(), addr, user, pass, core.Options{Timeout: 3 * time.Second})
}

func probeOnceCtx(t *testing.T, ctx context.Context, addr, user, pass string, opts core.Options) core.Result {
	t.Helper()
	target, err := core.ParseTarget(addr)
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}
	var prober Prober
	return prober.Probe(ctx, target, core.Credential{Username: user, Password: pass}, opts)
}
