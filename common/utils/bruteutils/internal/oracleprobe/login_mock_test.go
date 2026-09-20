package oracleprobe

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha1"
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
	verifier := 18453
	if strings.HasPrefix(fault, "short_nonce_") {
		verifier = 6949
	}
	password := "Mock_密码!123"
	salt := []byte("0123456789")
	checkSalt := bytes.Repeat([]byte{0x63}, 16)
	speedy := pbkdf2.Key([]byte(password), append(bytes.Clone(salt), []byte("AUTH_PBKDF2_SPEEDY_KEY")...), 4096, 64, sha512.New)
	hash := sha512.Sum512(append(bytes.Clone(speedy), salt...))
	passwordKey := hash[:32]
	serverNonce := bytes.Repeat([]byte{0x53}, 48)
	if verifier == 6949 {
		sum := sha1.Sum(append([]byte(password), salt...))
		passwordKey = append(sum[:], 0, 0, 0, 0)
		serverNonce = serverNonce[:32]
	}
	challenge := &session{ClrChunkSize: 64}
	challenge.PutBytes(8)
	challenge.PutInt(5, 4, true, true)
	challenge.PutKeyValString("AUTH_SESSKEY", mockEncrypt(passwordKey, serverNonce, false), 0)
	challenge.PutInt(len("AUTH_VFR_DATA"), 4, true, true)
	challenge.PutString("AUTH_VFR_DATA")
	challenge.PutInt(len(salt)*2, 4, true, true)
	challenge.PutString(hex.EncodeToString(salt))
	challenge.PutInt(verifier, 4, true, true)
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
	if mode&int(UserAndPass) == 0 {
		return errors.New("missing password login mode")
	}
	if !strings.EqualFold(string(username), "PROBE") {
		return s.writeData(mockSummary(1017))
	}
	if _, ok := props["AUTH_ALTER_SESSION"]; ok {
		return errors.New("probe must not issue ALTER SESSION")
	}
	clientNonce, e := mockDecrypt(passwordKey, props["AUTH_SESSKEY"], false)
	if e != nil {
		return e
	}
	nonceSize, keySize := len(clientNonce), 32
	if verifier == 6949 {
		nonceSize, keySize = 24, 24
	}
	material := append(bytes.Clone(clientNonce[:nonceSize]), serverNonce[:nonceSize]...)
	combined := pbkdf2.Key([]byte(strings.ToUpper(hex.EncodeToString(material))), checkSalt, 3, keySize, sha512.New)
	candidate, e := mockDecrypt(combined, props["AUTH_PASSWORD"], true)
	if e != nil || len(candidate) < 16 || string(candidate[16:]) != password || fault == "short_nonce_rejected" {
		return s.writeData(mockSummary(1017))
	}
	if verifier == 18453 {
		clearSpeedy, e := mockDecrypt(combined, props["AUTH_PBKDF2_SPEEDY_KEY"], false)
		if e != nil || len(clearSpeedy) < 16 || !bytes.Equal(clearSpeedy[16:], speedy) {
			return errors.New("invalid speedy response")
		}
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
	if fault == "success" || fault == "username_lowercase" || fault == "fragmented" {
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
	for _, fault := range []string{"success", "username_lowercase", "username_nul", "password_case", "password_space", "fragmented", "short_nonce_success", "short_nonce_rejected", "wrong_password", "missing_proof", "invalid_proof", "invalid_proof_padding", "missing_identity", "bare_success", "error_after_properties", "error_after_completion", "truncated_proof", "unexpected_message", "premature_success", "disconnect_connect", "disconnect_protocol", "disconnect_datatype", "disconnect_challenge", "disconnect_result"} {
		t.Run(fault, func(t *testing.T) {
			client, server := net.Pipe()
			serverResult := make(chan error, 1)
			go func() { serverResult <- mockLoginServer(server, fault) }()
			username, password := "PROBE", "Mock_密码!123"
			switch fault {
			case "username_lowercase":
				username = "probe"
			case "username_nul":
				username += "\x00suffix"
			case "password_case":
				password = strings.ToLower(password)
			case "password_space":
				password += " "
			}
			if fault == "wrong_password" {
				password += "wrong"
			}
			err := Probe(context.Background(), dialFunc(func(context.Context, string, string) (net.Conn, error) { return client, nil }), Options{Address: "127.0.0.1:1521", Service: "MOCK", Username: username, Password: password, Timeout: 2 * time.Second})
			if (err == nil) != (fault == "success" || fault == "username_lowercase" || fault == "fragmented" || fault == "short_nonce_success") {
				t.Fatalf("unexpected probe result: %v", err)
			}
			if fault == "username_nul" || fault == "password_case" || fault == "password_space" || fault == "wrong_password" || fault == "error_after_properties" || fault == "short_nonce_rejected" {
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
	for _, scenario := range []string{"trusted", "untrusted", "handoff", "handoff_untrusted", "handoff_stall", "resend_limit"} {
		t.Run(scenario, func(t *testing.T) {
			trusted := scenario != "untrusted"
			wantSuccess := scenario == "trusted" || scenario == "handoff"
			client, server := tlsTCPPair(t)
			done := make(chan error, 1)
			go func() {
				_ = server.SetDeadline(time.Now().Add(3 * time.Second))
				defer server.Close()
				secured := tls.Server(server, serverConfig)
				if e := secured.Handshake(); e != nil {
					server.Close()
					done <- e
					return
				}
				if strings.HasPrefix(scenario, "handoff") || scenario == "resend_limit" {
					for n := 0; ; n++ {
						wire := &session{conn: secured}
						p, e := wire.packet()
						if e != nil {
							done <- e
							return
						}
						if p[4] != 1 {
							done <- errors.New("expected CONNECT before TLS handoff")
							return
						}
						resend := wirePacket(0, 11, nil)
						resend[5] = 8
						if _, e = secured.Write(resend); e != nil {
							done <- e
							return
						}
						if scenario == "handoff_stall" {
							_, e = io.Copy(io.Discard, server)
							done <- e
							return
						}
						cfg := serverConfig.Clone()
						if scenario == "handoff_untrusted" {
							// Initial handshake is trusted; only the replacement certificate is invalid.
							other := *template
							other.DNSNames = []string{"wrong.test"}
							other.SerialNumber = big.NewInt(2)
							der, e := x509.CreateCertificate(rand.Reader, &other, &other, pub, key)
							if e != nil {
								done <- e
								return
							}
							cfg.Certificates = []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}
						}
						secured = tls.Server(server, cfg)
						if e = secured.Handshake(); e != nil {
							done <- e
							return
						}
						if scenario != "resend_limit" || n >= 3 {
							break
						}
					}
				}
				done <- mockLoginServer(secured, "success")
			}()
			config := &tls.Config{ServerName: "oracle.test", MinVersion: tls.VersionTLS12, RootCAs: x509.NewCertPool()}
			if trusted {
				config.RootCAs = roots
			}
			err := Probe(context.Background(), dialFunc(func(context.Context, string, string) (net.Conn, error) { return client, nil }), Options{Address: "127.0.0.1:1521", Service: "MOCK", Username: "PROBE", Password: "Mock_密码!123", TLS: config, Timeout: 500 * time.Millisecond})
			if (err == nil) != wantSuccess {
				t.Fatalf("unexpected TLS result: %v", err)
			}
			if scenario == "handoff_untrusted" {
				var verification *tls.CertificateVerificationError
				if !errors.As(err, &verification) {
					t.Fatalf("replacement certificate accepted: %v", err)
				}
			}
			if scenario == "handoff_stall" {
				var timeout net.Error
				if !errors.Is(err, context.DeadlineExceeded) && !(errors.As(err, &timeout) && timeout.Timeout()) {
					t.Fatalf("lost handshake deadline: %v", err)
				}
			}
			if scenario == "resend_limit" && (err == nil || !strings.Contains(err.Error(), "resend limit")) {
				t.Fatalf("lost resend bound: %v", err)
			}
			if e := <-done; wantSuccess && e != nil {
				t.Fatal(e)
			}
		})
	}
	for _, flag := range []string{"data", "header", "restart_limit"} {
		t.Run(flag, func(t *testing.T) {
			client, server := tlsTCPPair(t)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = client.SetDeadline(time.Now().Add(time.Second))
			_ = server.SetDeadline(time.Now().Add(time.Second))
			done := make(chan error, 1)
			go func() {
				defer server.Close()
				secured := tls.Server(server, serverConfig)
				if e := secured.Handshake(); e != nil {
					done <- e
					return
				}
				for n := 0; ; n++ {
					packet := wirePacket(0, 6, []byte{0, 0})
					if flag == "header" {
						packet[5] = 0x80
					} else {
						packet[8] = 0x80
					}
					if _, e := secured.Write(packet); e != nil {
						done <- e
						return
					}
					secured = tls.Server(server, serverConfig)
					if e := secured.Handshake(); e != nil {
						done <- e
						return
					}
					if flag != "restart_limit" || n >= 4 {
						break
					}
				}
				_, e := secured.Write(wirePacket(0, 6, []byte{0, 0, 42}))
				done <- e
			}()
			s := &session{conn: client, transport: client, ctx: ctx, tlsConfig: &tls.Config{ServerName: "oracle.test", RootCAs: roots, MinVersion: tls.VersionTLS12}}
			if e := s.startTLS(); e != nil {
				t.Fatal(e)
			}
			data, e := s.data()
			if flag == "restart_limit" {
				if e == nil || !strings.Contains(e.Error(), "TLS restart limit") {
					t.Fatalf("unbounded TLS handoff: %v", e)
				}
			} else if e != nil || !bytes.Equal(data, []byte{42}) {
				t.Fatalf("lost data after TLS handoff: %x %v", data, e)
			}
			client.Close()
			if e := <-done; flag != "restart_limit" && e != nil {
				t.Fatal(e)
			}
		})
	}

}

// Real TCP buffering lets TLS peers exchange an alert while the other peer is
// finishing its certificate flight. An unbuffered net.Pipe can deadlock there.
func tlsTCPPair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	client, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	server, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	return client, server
}
