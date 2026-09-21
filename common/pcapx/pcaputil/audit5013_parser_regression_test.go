package pcaputil

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

func TestAudit5013RESPIntegerPrefixInvariant(t *testing.T) {
	for _, wire := range []string{":+1\r\n", "(+123456789012345678901234567890\r\n"} {
		n, err := redisFrameLength([]byte(wire), 0)
		if err != nil || n != len(wire) {
			t.Fatalf("complete %q: n=%d err=%v", wire, n, err)
		}
		for split := 1; split < len(wire); split++ {
			_, err := redisFrameLength([]byte(wire[:split]), 0)
			if err != nil {
				t.Errorf("valid frame %q rejected at prefix length %d (%q): %v", wire, split, wire[:split], err)
				break
			}
		}
	}
}

func TestAudit5013PcapngCaplenMustFitBlockBeforeAllocation(t *testing.T) {
	var b bytes.Buffer
	w := func(x uint32) { binary.Write(&b, binary.LittleEndian, x) }
	w(0x0a0d0d0a)
	w(28)
	w(0x1a2b3c4d)
	w(1)
	w(0xffffffff)
	w(0xffffffff)
	w(28)
	w(1)
	w(20)
	w(1)
	w(65535)
	w(20)
	w(6)
	w(32)
	w(0)
	w(0)
	w(0)
	w(32 << 20)
	w(32 << 20)
	w(32)
	// Safe: only execute the original guard. Do NOT invoke upstream pcapgo's allocator.
	raw := append([]byte(nil), b.Bytes()...)
	accepted, err := io.ReadAll(&boundedNgInput{input: bytes.NewReader(raw)})
	if err == nil {
		t.Fatalf("guard forwarded all %d bytes; EPB has 0 packet bytes but caplen=32MiB", len(accepted))
	}
}
func TestAudit5013QUICUntrustedWireMustNotAutoBecomePlaintext(t *testing.T) {
	raw := quicLongPacket(2, 1, []byte{1, 2, 3, 4, 5, 6, 7, 8}, []byte{9, 10, 11, 12, 13, 14, 15, 16}, nil, 0, []byte{1})
	q := &binQUIC{}
	info, err := q.consume(0, raw, 64)
	if err == nil || info["Plaintext Input"] == true {
		t.Fatalf("wire Handshake bypassed packet authentication: info=%v err=%v", info, err)
	}
}
func TestAudit5013DNSValidSixteenLabels(t *testing.T) {
	msg := make([]byte, 12)
	for i := 0; i < 16; i++ {
		msg = append(msg, 1, 'a')
	}
	msg = append(msg, 0)
	_, _, err := dnsParseName(msg, 12)
	if err != nil {
		t.Fatalf("valid 16-label, 33-octet QNAME rejected: %v", err)
	}
}
