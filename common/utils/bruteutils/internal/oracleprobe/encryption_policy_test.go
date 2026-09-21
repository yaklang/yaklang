package oracleprobe

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"strings"
	"testing"
)

// Minimal server selections, independent of the client's advertised list. These
// exercise an attempted downgrade before any password challenge can be sent.
func policyResponse(encryption byte) []byte {
	b := []byte{0xde, 0xad, 0xbe, 0xef, 0, 0, 0x17, 0, 0, 0, 0, 4, 0}
	for _, svc := range []struct {
		id     uint16
		fields [][]byte
	}{
		{4, [][]byte{{0, 4, 0, 5, 0x17, 0, 0, 0}, {0, 2, 0, 6, 0, 31}, {0, 0, 0, 1}}},
		{1, [][]byte{{0, 4, 0, 5, 0x17, 0, 0, 0}, {0, 2, 0, 6, 0xfb, 0xff}}},
		{2, [][]byte{{0, 4, 0, 5, 0x17, 0, 0, 0}, {0, 1, 0, 2, encryption}}},
		{3, [][]byte{{0, 4, 0, 5, 0x17, 0, 0, 0}, {0, 1, 0, 2, 0}}},
	} {
		b = binary.BigEndian.AppendUint16(b, svc.id)
		b = binary.BigEndian.AppendUint16(b, uint16(len(svc.fields)))
		b = append(b, 0, 0, 0, 0)
		for _, field := range svc.fields {
			b = append(b, field...)
		}
	}
	binary.BigEndian.PutUint16(b[4:], uint16(len(b)))
	return wirePacket(0, 6, append([]byte{0, 0}, b...))
}

func TestNativeEncryptionPolicy(t *testing.T) {
	for _, tc := range []struct {
		name     string
		policy   EncryptionPolicy
		selected byte
		rejected bool
	}{
		{"accepted_clear", EncryptionAccepted, 0, false},
		{"requested_clear", EncryptionRequested, 0, false},
		{"rejected_clear", EncryptionRejected, 0, false},
		{"required_downgrade", EncryptionRequired, 0, true},
		{"rejected_encrypted", EncryptionRejected, 17, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := wireSession(policyResponse(tc.selected))
			s.encryption = tc.policy
			err := s.negotiateAdvanced()
			if tc.rejected {
				if err == nil || !strings.Contains(err.Error(), "encryption policy") {
					t.Fatalf("policy not enforced: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			// Exactly one ANO request; policy failure must not send credentials or DH.
			sent := s.conn.(*memoryConn).written.Bytes()
			if len(sent) < 10 || int(binary.BigEndian.Uint16(sent)) != len(sent) || sent[4] != 6 {
				t.Fatal("unexpected follow-up request")
			}
		})
	}
}

func TestRequiredEncryptionWithoutANO(t *testing.T) {
	conn := &memoryConn{Reader: bytes.NewReader(acceptPacket())}
	err := Probe(context.Background(), dialFunc(func(context.Context, string, string) (net.Conn, error) { return conn, nil }), Options{Address: "127.0.0.1:1521", Service: "MOCK", Username: "PROBE", Password: "secret", Encryption: EncryptionRequired})
	if err == nil || !strings.Contains(err.Error(), "required but unavailable") {
		t.Fatalf("accepted unencrypted listener: %v", err)
	}
	if !conn.closed {
		t.Fatal("connection leaked")
	}
	sent := conn.written.Bytes()
	if len(sent) < 8 || sent[4] != 1 || int(binary.BigEndian.Uint16(sent)) != len(sent) {
		t.Fatal("sent authentication after encryption refusal")
	}
}

func TestInvalidEncryptionPolicyBeforeDial(t *testing.T) {
	called := false
	err := Probe(context.Background(), dialFunc(func(context.Context, string, string) (net.Conn, error) { called = true; return nil, nil }), Options{Address: "127.0.0.1:1521", Service: "MOCK", Encryption: EncryptionPolicy(255)})
	if err == nil || called {
		t.Fatalf("invalid policy reached network: %v", err)
	}
}
