package mongodb

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"hash"
	"io"
	"net"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
	"github.com/yaklang/yaklang/common/brute/probes/internal/scram"
)

// mongoSim 模拟 MongoDB 服务端。
type mongoSim struct {
	t        *testing.T
	listener net.Listener
	addr     string

	authRequired bool
	mechanism    string // SCRAM-SHA-1 | SCRAM-SHA-256
	validUser    string
	validPass    string

	scriptErr       *mongoError // 每次命令回该错误
	closeAfterHello bool
	noSaslMechs     bool // hello 不带 saslSupportedMechs

	mu        sync.Mutex
	attempts  int
	users     []string
	convState *scramConv
	wg        sync.WaitGroup
}

func newMongoSim(t *testing.T) *mongoSim {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return &mongoSim{t: t, listener: l, addr: l.Addr().String(), authRequired: true, mechanism: "SCRAM-SHA-256"}
}

func (s *mongoSim) start() {
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

func (s *mongoSim) stop() {
	_ = s.listener.Close()
	s.wg.Wait()
}

func (s *mongoSim) handleConn(conn net.Conn) {
	s.mu.Lock()
	s.attempts++
	s.mu.Unlock()

	for {
		cmd, err := readCommand(conn)
		if err != nil {
			return
		}
		resp, closeConn := s.handleCommand(cmd)
		if err := writeResponse(conn, resp); err != nil {
			return
		}
		if closeConn {
			return
		}
	}
}

// handleCommand 返回（响应文档, 是否关闭连接）。
func (s *mongoSim) handleCommand(cmd D) (D, bool) {
	if s.scriptErr != nil {
		if _, isSasl := cmd.Get("saslStart"); isSasl {
			return errDoc(s.scriptErr), false
		}
		if _, isSaslCont := cmd.Get("saslContinue"); isSaslCont {
			return errDoc(s.scriptErr), false
		}
	}
	if helloVal, ok := cmd.Get("hello"); ok {
		_ = helloVal
		if s.closeAfterHello {
			return nil, true
		}
		doc := D{
			S("ok", doubleOrInt(1)),
			S("isWritablePrimary", true),
			S("maxWireVersion", int32(21)),
			S("maxBsonObjectSize", int32(16<<20)),
		}
		if !s.noSaslMechs && s.authRequired {
			doc = append(doc, S("saslSupportedMechs", D{S("0", "SCRAM-SHA-256"), S("1", "SCRAM-SHA-1")}))
		}
		return doc, false
	}
	if _, ok := cmd.Get("ping"); ok {
		// ping 无需认证（真实 MongoDB 行为）
		return D{S("ok", doubleOrInt(1))}, false
	}
	if _, ok := cmd.Get("listDatabases"); ok {
		if s.authRequired {
			return errDoc(&mongoError{Code: codeUnauthorized, Name: "Unauthorized", Message: "command listDatabases requires authentication"}), false
		}
		return D{S("databases", D{}), S("ok", doubleOrInt(1))}, false
	}
	if _, ok := cmd.Get("saslStart"); ok {
		return s.handleSaslStart(cmd), false
	}
	if _, ok := cmd.Get("saslContinue"); ok {
		return s.handleSaslContinue(cmd), false
	}
	return errDoc(&mongoError{Code: 59, Name: "CommandNotFound", Message: "no such command"}), false
}

func (s *mongoSim) handleSaslStart(cmd D) D {
	mechanism, _ := cmd.GetString("mechanism")
	payload, _ := cmd.GetBinary("payload")
	if mechanism != s.mechanism {
		if s.authRequired {
			return errDoc(&mongoError{Code: codeMechanismUnavailable, Name: "MechanismUnavailable", Message: "Unsupported mechanism '" + mechanism + "' on database 'admin'"})
		}
		return errDoc(&mongoError{Code: codeAuthenticationFailed, Name: "AuthenticationFailed", Message: "Authentication failed."})
	}
	clientFirst := string(payload)
	if !strings.HasPrefix(clientFirst, "n,,") {
		return errDoc(&mongoError{Code: codeAuthenticationFailed, Name: "AuthenticationFailed", Message: "Authentication failed."})
	}
	clientFirstBare := clientFirst[3:]

	// server-first
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	iterations := 15000
	clientNonce := nonceFrom(clientFirstBare)
	serverNonce := clientNonce + base64.StdEncoding.EncodeToString([]byte("srv"))
	serverFirst := "r=" + serverNonce + ",s=" + base64.StdEncoding.EncodeToString(salt) + ",i=" + strconv.Itoa(iterations)

	s.mu.Lock()
	s.users = append(s.users, usernameFrom(clientFirstBare))
	s.convState = &scramConv{
		clientFirstBare: clientFirstBare,
		serverFirst:     serverFirst,
		salt:            salt,
		iterations:      iterations,
		serverNonce:     serverNonce,
	}
	s.mu.Unlock()

	return D{
		S("conversationId", int32(1)),
		S("done", false),
		S("payload", []byte(serverFirst)),
		S("ok", doubleOrInt(1)),
	}
}

type scramConv struct {
	clientFirstBare string
	serverFirst     string
	salt            []byte
	iterations      int
	serverNonce     string
}

func (s *mongoSim) handleSaslContinue(cmd D) D {
	s.mu.Lock()
	conv := s.convState
	s.mu.Unlock()
	if conv == nil {
		return errDoc(&mongoError{Code: codeAuthenticationFailed, Name: "AuthenticationFailed", Message: "Authentication failed."})
	}
	payload, _ := cmd.GetBinary("payload")
	clientFinal := string(payload)

	if !strings.HasPrefix(clientFinal, "c=biws,r="+conv.serverNonce) {
		return errDoc(&mongoError{Code: codeAuthenticationFailed, Name: "AuthenticationFailed", Message: "Authentication failed."})
	}
	proofIdx := strings.LastIndex(clientFinal, ",p=")
	if proofIdx < 0 {
		return errDoc(&mongoError{Code: codeAuthenticationFailed, Name: "AuthenticationFailed", Message: "Authentication failed."})
	}
	withoutProof := clientFinal[:proofIdx]
	authMessage := conv.clientFirstBare + "," + conv.serverFirst + "," + withoutProof

	// 用户名校验（MongoDB 对不存在用户一律 AuthenticationFailed）
	if s.validUser != usernameOf(conv.clientFirstBare) {
		return errDoc(&mongoError{Code: codeAuthenticationFailed, Name: "AuthenticationFailed", Message: "Authentication failed."})
	}

	// 服务端独立验证（按声明的机制计算）
	var salted, clientKey, storedKey []byte
	if s.mechanism == "SCRAM-SHA-256" {
		pw, _ := scram.SASLprep(s.validPass)
		salted = scram.PBKDF2(sha256.New, []byte(pw), conv.salt, conv.iterations, 32)
		clientKey = hmacSum(salted, "Client Key", sha256.New)
		storedKey = hashBytes(clientKey, sha256.New)
	} else {
		sum := md5.Sum([]byte(s.validUser + ":mongo:" + s.validPass))
		salted = scram.PBKDF2(sha1.New, []byte(hexEncode(sum[:])), conv.salt, conv.iterations, 20)
		clientKey = hmacSum(salted, "Client Key", sha1.New)
		storedKey = hashBytes(clientKey, sha1.New)
	}
	clientSig := hmacSum(storedKey, authMessage, s.hashFn())
	proofB64 := clientFinal[proofIdx+3:]
	proof, err := base64.StdEncoding.DecodeString(proofB64)
	if err != nil {
		return errDoc(&mongoError{Code: codeAuthenticationFailed, Name: "AuthenticationFailed", Message: "Authentication failed."})
	}
	calcKey := make([]byte, len(proof))
	for i := range proof {
		calcKey[i] = proof[i] ^ clientSig[i]
	}
	if !hmac.Equal(calcKey, clientKey) {
		return errDoc(&mongoError{Code: codeAuthenticationFailed, Name: "AuthenticationFailed", Message: "Authentication failed."})
	}

	// server-final
	serverKey := hmacSum(salted, "Server Key", s.hashFn())
	serverSig := hmacSum(serverKey, authMessage, s.hashFn())
	return D{
		S("conversationId", int32(1)),
		S("done", true),
		S("payload", []byte("v="+base64.StdEncoding.EncodeToString(serverSig))),
		S("ok", doubleOrInt(1)),
	}
}

func (s *mongoSim) hashFn() func() hash.Hash {
	if s.mechanism == "SCRAM-SHA-256" {
		return sha256.New
	}
	return sha1.New
}

func hmacSum(key []byte, data string, h func() hash.Hash) []byte {
	m := hmac.New(h, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

func hashBytes(b []byte, h func() hash.Hash) []byte {
	m := h()
	m.Write(b)
	return m.Sum(nil)
}

func nonceFrom(bare string) string {
	for _, part := range strings.Split(bare, ",") {
		if strings.HasPrefix(part, "r=") {
			return part[2:]
		}
	}
	return ""
}

func usernameOf(bare string) string { return usernameFrom(bare) }

func usernameFrom(bare string) string {
	for _, part := range strings.Split(bare, ",") {
		if strings.HasPrefix(part, "n=") {
			return part[2:]
		}
	}
	return ""
}

func doubleOrInt(v int32) interface{} {
	return float64(v)
}

func errDoc(e *mongoError) D {
	return D{
		S("ok", doubleOrInt(0)),
		S("errmsg", e.Message),
		S("code", e.Code),
		S("codeName", e.Name),
	}
}

// ---- OP_MSG 服务端读写 ----

func readCommand(conn net.Conn) (D, error) {
	var header [16]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return nil, err
	}
	length := int(binary.LittleEndian.Uint32(header[0:4]))
	// 模拟器只接受小命令；TLS ClientHello 伪装长度（~66KB）应立即失败断开。
	if length < 16 || length > 65536 {
		return nil, errShortReadM
	}
	rest := make([]byte, length-16)
	if _, err := io.ReadFull(conn, rest); err != nil {
		return nil, err
	}
	if len(rest) < 5 || rest[4] != 0 {
		return nil, errShortReadM
	}
	return DecodeD(rest[5:])
}

var errShortReadM = &simpleErr{"short read"}

type simpleErr struct{ s string }

func (e *simpleErr) Error() string { return e.s }

func writeResponse(conn net.Conn, doc D) error {
	if doc == nil {
		return io.EOF
	}
	body := EncodeD(doc)
	buf := make([]byte, 0, 21+len(body))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(21+len(body)))
	buf = binary.LittleEndian.AppendUint32(buf, 999) // responseTo 占位（客户端不校验）
	buf = binary.LittleEndian.AppendUint32(buf, 1)   // requestID
	buf = binary.LittleEndian.AppendUint32(buf, opMsg)
	buf = binary.LittleEndian.AppendUint32(buf, 0)
	buf = append(buf, 0)
	buf = append(buf, body...)
	_, err := conn.Write(buf)
	return err
}

// ---- 测试辅助 ----

func mongoProbeOnce(t *testing.T, addr, user, pass string) core.Result {
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
