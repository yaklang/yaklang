package oracleprobe

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

// mockLoginServer implements the four request/response exchanges independently
// of Probe. It decrypts the submitted password using the server's verifier;
// it never treats an arbitrary authentication request as success.
func mockLoginServer(conn net.Conn, fault string) error {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	s := &session{conn: conn, sdu: 8192, ClrChunkSize: 64}
	if fault == "fragmented" {
		s.sdu = 160
	}
	p, err := s.packet()
	if err != nil {
		return err
	}
	if p[4] != 1 || !bytes.Contains(p, []byte("SERVICE_NAME=MOCK")) {
		return errors.New("expected CONNECT")
	}
	if fault == "stall_connect" {
		_, err := io.Copy(io.Discard, conn)
		return err
	}
	if fault == "disconnect_connect" {
		return nil
	}
	if _, err = conn.Write(acceptPacket()); err != nil {
		return err
	}
	s.version = 314
	p, err = s.data()
	if err != nil {
		return err
	}
	if len(p) < 3 || p[0] != 1 {
		return errors.New("expected protocol request")
	}
	if fault == "stall_protocol" {
		_, err := io.Copy(io.Discard, conn)
		return err
	}
	if fault == "disconnect_protocol" {
		return nil
	}
	// No timezone, extended CLR, EOS or FSAP capabilities are negotiated.
	proto := append([]byte{1, 6, 0}, []byte("Oracle mock\x00")...)
	proto = append(proto, 0x69, 0x03, 0, 0, 0, 0, 11)
	charset := make([]byte, 11)
	binary.BigEndian.PutUint16(charset[9:], 2000)
	proto = append(proto, charset...)
	proto = append(proto, 8, 0, 0, 0, 0, 0x22, 0, 0, 8, 0)
	if err = s.writeData(proto); err != nil {
		return err
	}
	p, err = s.data()
	if err != nil {
		return err
	}
	if len(p) < 1 || p[0] != 2 {
		return errors.New("expected datatype request")
	}
	if fault == "stall_datatype" {
		_, err := io.Copy(io.Discard, conn)
		return err
	}
	if fault == "disconnect_datatype" {
		return nil
	}
	if err = s.writeData([]byte{2, 0}); err != nil {
		return err
	}
	p, err = s.data()
	if err != nil {
		return err
	}
	if len(p) < 3 || p[0] != 3 || p[1] != 0x76 {
		return errors.New("expected password challenge request")
	}
	if fault == "stall_challenge" {
		_, err := io.Copy(io.Discard, conn)
		return err
	}
	if fault == "disconnect_challenge" {
		return nil
	}
	if fault == "premature_success" {
		return s.writeData([]byte{9})
	}
	password := "Mock_密码!123"
	salt := []byte("0123456789")
	checkSalt := bytes.Repeat([]byte{0x63}, 16)
	speedy := pbkdf2.Key([]byte(password), append(bytes.Clone(salt), []byte("AUTH_PBKDF2_SPEEDY_KEY")...), 4096, 64, sha512.New)
	hash := sha512.Sum512(append(bytes.Clone(speedy), salt...))
	passwordKey := hash[:32]
	serverNonce := bytes.Repeat([]byte{0x53}, 48)
	challenge := &session{ClrChunkSize: 64}
	challenge.PutBytes(8)
	challenge.PutInt(5, 4, true, true)
	challenge.PutKeyValString("AUTH_SESSKEY", mockEncrypt(passwordKey, serverNonce, false), 0)
	challenge.PutInt(len("AUTH_VFR_DATA"), 4, true, true)
	challenge.PutString("AUTH_VFR_DATA")
	challenge.PutInt(len(salt)*2, 4, true, true)
	challenge.PutString(hex.EncodeToString(salt))
	challenge.PutInt(18453, 4, true, true)
	challenge.PutKeyValString("AUTH_PBKDF2_CSK_SALT", hex.EncodeToString(checkSalt), 0)
	challenge.PutKeyValString("AUTH_PBKDF2_VGEN_COUNT", "4096", 0)
	challenge.PutKeyValString("AUTH_PBKDF2_SDER_COUNT", "3", 0)
	challenge.PutBytes(mockSummary(0)...)
	if err = s.writeData(challenge.out.Bytes()); err != nil {
		return err
	}
	p, err = s.data()
	if err != nil {
		return err
	}
	if len(p) < 3 || p[0] != 3 || p[1] != 0x73 {
		return errors.New("expected password response")
	}
	if fault == "stall_result" {
		_, err := io.Copy(io.Discard, conn)
		return err
	}
	if fault == "disconnect_result" {
		return nil
	}
	request := wireSession(nil)
	request.in.Write(p[3:])
	_, _ = request.GetByte()
	_, _ = request.GetInt(4, true, true)
	mode, _ := request.GetInt(4, true, true)
	_, _ = request.GetByte()
	count, _ := request.count(32)
	_, _ = request.GetBytes(2)
	username, _ := request.GetClr()
	props := map[string]string{}
	for n := 0; n < count; n++ {
		k, v, _, e := request.GetKeyVal()
		if e != nil {
			return e
		}
		props[string(k)] = string(v)
	}
	if request.err != nil {
		return request.err
	}
	if string(username) != "PROBE" || mode&int(UserAndPass) == 0 {
		return errors.New("missing password login mode")
	}
	if _, ok := props["AUTH_ALTER_SESSION"]; ok {
		return errors.New("probe must not issue ALTER SESSION")
	}
	clientNonce, e := mockDecrypt(passwordKey, props["AUTH_SESSKEY"], false)
	if e != nil {
		return e
	}
	material := append(bytes.Clone(clientNonce), serverNonce...)
	combined := pbkdf2.Key([]byte(strings.ToUpper(hex.EncodeToString(material))), checkSalt, 3, 32, sha512.New)
	candidate, e := mockDecrypt(combined, props["AUTH_PASSWORD"], true)
	if e != nil || len(candidate) < 16 || string(candidate[16:]) != password {
		return s.writeData(mockSummary(1017))
	}
	clearSpeedy, e := mockDecrypt(combined, props["AUTH_PBKDF2_SPEEDY_KEY"], false)
	if e != nil || len(clearSpeedy) < 16 || !bytes.Equal(clearSpeedy[16:], speedy) {
		return errors.New("invalid speedy response")
	}
	proof := append(bytes.Repeat([]byte{0x71}, 16), []byte("SERVER_TO_CLIENT")...)
	result := map[string]string{"AUTH_SESSION_ID": "42", "AUTH_SERIAL_NUM": "3", "AUTH_SVR_RESPONSE": mockEncrypt(combined, proof, true)}
	switch fault {
	case "missing_proof":
		delete(result, "AUTH_SVR_RESPONSE")
	case "invalid_proof":
		result["AUTH_SVR_RESPONSE"] = mockEncrypt(combined, bytes.Repeat([]byte{0x42}, 32), true)
	case "invalid_proof_padding":
		result["AUTH_SVR_RESPONSE"] = mockEncrypt(combined, append(proof, make([]byte, 16)...), false)
	case "missing_identity":
		delete(result, "AUTH_SESSION_ID")
	case "bare_success":
		result = nil
	}
	response := authResult(result, true)[10:]
	if fault == "error_after_properties" {
		response = append(response[:len(response)-1], mockSummary(28000)...)
	}
	if fault == "error_after_completion" {
		response = append(response, mockSummary(1017)...)
	}
	if fault == "truncated_proof" {
		response = response[:len(response)/2]
	}
	if fault == "unexpected_message" {
		response = []byte{99}
	}
	if err = s.writeData(response); err != nil {
		return err
	}
	if fault == "success" || fault == "fragmented" {
		// The probe must close here, without SQL, version, NLS or ping requests.
		var one [1]byte
		_, err = conn.Read(one[:])
		if !errors.Is(err, io.EOF) {
			return fmt.Errorf("expected close after login, got %v", err)
		}
	}
	return nil
}

func mockSummary(code int) []byte {
	w := &session{ClrChunkSize: 64}
	w.PutBytes(4)
	w.PutBytes(make([]byte, 26)...)
	w.PutInt(code, 4, true, true)
	w.PutBytes(0)
	if code != 0 {
		w.PutString(fmt.Sprintf("ORA-%05d: mock rejection", code))
	}
	return w.out.Bytes()
}
func mockEncrypt(key, clear []byte, padding bool) string {
	clear = bytes.Clone(clear)
	if padding {
		n := 16 - len(clear)%16
		clear = append(clear, bytes.Repeat([]byte{byte(n)}, n)...)
	}
	blk, _ := aes.NewCipher(key)
	b := make([]byte, len(clear))
	cipher.NewCBCEncrypter(blk, make([]byte, 16)).CryptBlocks(b, clear)
	return hex.EncodeToString(b)
}
func mockDecrypt(key []byte, text string, padding bool) ([]byte, error) {
	b, e := hex.DecodeString(text)
	if e != nil || len(b) == 0 || len(b)%16 != 0 {
		return nil, errors.New("invalid client cipher")
	}
	blk, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	out := make([]byte, len(b))
	cipher.NewCBCDecrypter(blk, make([]byte, 16)).CryptBlocks(out, b)
	if padding {
		n := int(out[len(out)-1])
		if n < 1 || n > 16 || n > len(out) || !bytes.Equal(out[len(out)-n:], bytes.Repeat([]byte{byte(n)}, n)) {
			return nil, errors.New("invalid client padding")
		}
		out = out[:len(out)-n]
	}
	return out, nil
}

func TestProbeCompleteHandshakeMock(t *testing.T) {
	for _, fault := range []string{"success", "fragmented", "wrong_password", "missing_proof", "invalid_proof", "invalid_proof_padding", "missing_identity", "bare_success", "error_after_properties", "error_after_completion", "truncated_proof", "unexpected_message", "premature_success", "disconnect_connect", "disconnect_protocol", "disconnect_datatype", "disconnect_challenge", "disconnect_result"} {
		t.Run(fault, func(t *testing.T) {
			client, server := net.Pipe()
			serverResult := make(chan error, 1)
			go func() { serverResult <- mockLoginServer(server, fault) }()
			password := "Mock_密码!123"
			if fault == "wrong_password" {
				password += "wrong"
			}
			err := Probe(context.Background(), dialFunc(func(context.Context, string, string) (net.Conn, error) { return client, nil }), Options{Address: "127.0.0.1:1521", Service: "MOCK", Username: "PROBE", Password: password, Timeout: 2 * time.Second})
			if (err == nil) != (fault == "success" || fault == "fragmented") {
				t.Fatalf("unexpected probe result: %v", err)
			}
			if fault == "wrong_password" || fault == "error_after_properties" {
				var ora *Error
				if !errors.As(err, &ora) {
					t.Fatalf("missing server error: %v", err)
				}
			}
			if e := <-serverResult; e != nil && !errors.Is(e, io.ErrClosedPipe) {
				t.Fatalf("mock server: %v", e)
			}
		})
	}
}

func TestProbeTLSVerification(t *testing.T) {
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "oracle.test"}, DNSNames: []string{"oracle.test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	serverConfig := &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12}
	for _, trusted := range []bool{true, false} {
		t.Run(fmt.Sprintf("trusted=%v", trusted), func(t *testing.T) {
			client, server := net.Pipe()
			done := make(chan error, 1)
			go func() {
				_ = server.SetDeadline(time.Now().Add(3 * time.Second))
				secured := tls.Server(server, serverConfig)
				if e := secured.Handshake(); e != nil {
					server.Close()
					done <- e
					return
				}
				done <- mockLoginServer(secured, "success")
			}()
			config := &tls.Config{ServerName: "oracle.test", MinVersion: tls.VersionTLS12, RootCAs: x509.NewCertPool()}
			if trusted {
				config.RootCAs = roots
			}
			err := Probe(context.Background(), dialFunc(func(context.Context, string, string) (net.Conn, error) { return client, nil }), Options{Address: "127.0.0.1:1521", Service: "MOCK", Username: "PROBE", Password: "Mock_密码!123", TLS: config, Timeout: 2 * time.Second})
			if (err == nil) != trusted {
				t.Fatalf("unexpected TLS result: %v", err)
			}
			if e := <-done; trusted && e != nil {
				t.Fatal(e)
			}
		})
	}
}
