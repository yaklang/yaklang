package nla_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"testing"

	"github.com/yaklang/yaklang/common/utils/bruteutils/grdp/protocol/nla"
)

func TestReadTSRequestRoundtrip(t *testing.T) {
	ntlm := nla.NewNTLMv2("DOM", "user", "pass")
	nonce := bytes.Repeat([]byte{0x11}, 32)
	encoded := nla.EncodeDERTRequest(nla.CredSSPVersion6,
		[]nla.Message{ntlm.GetNegotiateMessage()}, nil, nil, nonce)

	got, err := nla.ReadTSRequest(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != nla.CredSSPVersion6 {
		t.Fatalf("version=%d want 6", got.Version)
	}
	if len(got.NegoTokens) != 1 {
		t.Fatalf("negoTokens=%d", len(got.NegoTokens))
	}
	if !bytes.Equal(got.ClientNonce, nonce) {
		t.Fatalf("nonce mismatch")
	}
}

func TestReadTSRequestFragmented(t *testing.T) {
	ntlm := nla.NewNTLMv2("", "", "")
	encoded := nla.EncodeDERTRequest(nla.CredSSPVersion6,
		[]nla.Message{ntlm.GetNegotiateMessage()}, nil, nil, bytes.Repeat([]byte{0x22}, 32))

	// One-byte-at-a-time reader: Windows NTLM Challenge TSRequests are
	// frequently split across TLS records; a single Conn.Read used to truncate.
	got, err := nla.ReadTSRequest(&byteReader{buf: encoded})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 6 || len(got.NegoTokens) != 1 {
		t.Fatalf("decoded version=%d tokens=%d", got.Version, len(got.NegoTokens))
	}
}

func TestPadBERLongFormRoundTrip(t *testing.T) {
	ntlm := nla.NewNTLMv2("DOM", "user", "pass")
	der := nla.EncodeDERTRequest(nla.CredSSPVersion6,
		[]nla.Message{ntlm.GetNegotiateMessage()}, nil, nil, bytes.Repeat([]byte{0x11}, 32))
	ber, err := nla.PadBERLongForm(der)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(ber, der) {
		t.Fatal("BER pad should change encoding")
	}
	if ber[0] != 0x30 || ber[1] != 0x82 {
		t.Fatalf("want SEQUENCE 82 .. got %02x %02x", ber[0], ber[1])
	}
	got, err := nla.ReadTSRequest(bytes.NewReader(ber))
	if err != nil {
		t.Fatalf("padded BER: %v", err)
	}
	if got.Version != 6 || len(got.NegoTokens) != 1 {
		t.Fatalf("version=%d tokens=%d", got.Version, len(got.NegoTokens))
	}
}

func TestReadTSRequestBERLeadingZeros(t *testing.T) {
	// 边缘情况：SEQUENCE 长度用 82 00 f3（前导零，BER 合法 DER 非法）。
	raw, err := hex.DecodeString("308200f3a003020106a18200ea308200e6308200e2a08200de048200da4e544c4d53535000020000001a001a003800000035828ae275b01fa7791e3ca3000000000000000088008800520000000a0063450000000f4400450053004b0054004f0050002d0046004100500052004f0001001a004400450053004b0054004f0050002d0046004100500052004f0002001a004400450053004b0054004f0050002d0046004100500052004f0003001a004400450053004b0054004f0050002d0046004100500052004f0004001a004400450053004b0054004f0050002d0046004100500052004f0007000800b0a8bc8b803bdd0100000000")
	if err != nil {
		t.Fatal(err)
	}
	got, err := nla.ReadTSRequest(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("BER TSRequest: %v", err)
	}
	if got.Version != 6 {
		t.Fatalf("version=%d want 6", got.Version)
	}
	if len(got.NegoTokens) != 1 || len(got.NegoTokens[0].Data) < 8 {
		t.Fatalf("negoTokens=%+v", got.NegoTokens)
	}
	if string(got.NegoTokens[0].Data[:7]) != "NTLMSSP" {
		t.Fatalf("not NTLMSSP: %q", got.NegoTokens[0].Data[:7])
	}
}

func TestReadTSRequestRejectsHugeLength(t *testing.T) {
	// SEQUENCE, long-form length 0x84 (4 length bytes) claiming 16MiB.
	frame := []byte{0x30, 0x84, 0x01, 0x00, 0x00, 0x00}
	_, err := nla.ReadTSRequest(bytes.NewReader(frame))
	if err == nil {
		t.Fatal("expected error for huge DER length")
	}
}

func TestTSRequestErrorCode(t *testing.T) {
	req := &nla.TSRequest{Version: 6, ErrorCode: int64(0xC000006D)}
	err := req.AuthError()
	var cssp *nla.CredSSPError
	if !errors.As(err, &cssp) {
		t.Fatalf("want CredSSPError got %v", err)
	}
	if !cssp.AuthFailed() {
		t.Fatal("STATUS_LOGON_FAILURE must be AuthFailed")
	}
	if cssp.AccountLocked() {
		t.Fatal("STATUS_LOGON_FAILURE is not lockout")
	}

	locked := &nla.TSRequest{Version: 6, ErrorCode: int64(0xC0000234)}
	err = locked.AuthError()
	if !errors.As(err, &cssp) || !cssp.AccountLocked() {
		t.Fatalf("want locked CredSSPError got %v", err)
	}
}

func TestComputePubKeyHashStable(t *testing.T) {
	nonce := bytes.Repeat([]byte{0x01}, 32)
	pub := []byte("subject-public-key")
	a := nla.ComputePubKeyHash(true, nonce, pub)
	b := nla.ComputePubKeyHash(true, nonce, pub)
	if !bytes.Equal(a, b) || len(a) != 32 {
		t.Fatalf("hash len=%d a=%s b=%s", len(a), hex.EncodeToString(a), hex.EncodeToString(b))
	}
	c2s := nla.ComputePubKeyHash(true, nonce, pub)
	s2c := nla.ComputePubKeyHash(false, nonce, pub)
	if bytes.Equal(c2s, s2c) {
		t.Fatal("client-to-server and server-to-client hashes must differ")
	}
}

func TestEffectiveVersion(t *testing.T) {
	if nla.EffectiveVersion(6, 2) != 2 {
		t.Fatal("min(6,2) should be 2")
	}
	if nla.EffectiveVersion(6, 6) != 6 {
		t.Fatal("min(6,6) should be 6")
	}
	if nla.EffectiveVersion(5, 6) != 5 {
		t.Fatal("min(5,6) should be 5")
	}
}

type byteReader struct {
	buf []byte
	off int
}

func (r *byteReader) Read(p []byte) (int, error) {
	if r.off >= len(r.buf) {
		return 0, io.EOF
	}
	p[0] = r.buf[r.off]
	r.off++
	return 1, nil
}
