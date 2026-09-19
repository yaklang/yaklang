package pcaputil

import (
	"encoding/binary"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func diameterTestMessage(request bool, hop uint32, avps []byte) []byte {
	w := make([]byte, 20)
	w[0] = 1
	n := 20 + len(avps)
	w[1] = byte(n >> 16)
	w[2] = byte(n >> 8)
	w[3] = byte(n)
	if request {
		w[4] = 128
	}
	w[6] = 1
	w[7] = 24
	binary.BigEndian.PutUint32(w[12:], hop)
	binary.BigEndian.PutUint32(w[16:], hop+1)
	return append(w, avps...)
}
func TestDiameterCorrelationAndBounds(t *testing.T) {
	req := diameterTestMessage(true, 7, nil)
	resp := diameterTestMessage(false, 7, []byte{0, 0, 1, 12, 0x40, 0, 0, 12, 0, 0, 7, 0xd1})
	for _, chunk := range []int{0, 1, 7} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/%v", chunk, deferred), func(t *testing.T) {
				events, _ := sessionTestFlow(t, "diameter", []sessionStep{{0, req}, {1, resp}}, chunk, deferred)
				require.Len(t, events, 2)
				assertSessionEvents(t, events, "diameter", deferred)
				require.Equal(t, true, events[1].Session["Matched"])
				require.Equal(t, 0, events[1].Session["Pending"])
			})
		}
	}
	s := &binDiameter{}
	_, err := s.consume(0, req, 1, 4)
	require.NoError(t, err)
	_, err = s.consume(0, diameterTestMessage(true, 8, nil), 1, 4)
	require.Error(t, err)
	bad := append([]byte(nil), resp...)
	bad[19]++
	_, err = s.consume(1, bad, 4, 4)
	require.Error(t, err)
	require.Len(t, s.pending, 1)
	for _, a := range [][]byte{{0, 0, 1, 12, 0, 0, 0, 7}, {0, 0, 1, 12, 0, 0, 0, 9, 0}} {
		_, err = s.consume(1, diameterTestMessage(false, 7, a), 4, 4)
		require.Error(t, err)
	}
}
func TestIEC104SequenceAndObjects(t *testing.T) {
	i := []byte{0x68, 25, 0, 0, 0, 0, 36, 1, 3, 0, 1, 0, 1, 0, 0, 0, 0, 0x80, 0x3f, 0, 0, 0, 0, 0, 1, 1, 26}
	for _, chunk := range []int{0, 1, 7} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/%v", chunk, deferred), func(t *testing.T) {
				events, _ := sessionTestFlow(t, "iec104", []sessionStep{{0, []byte{0x68, 4, 7, 0, 0, 0}}, {1, []byte{0x68, 4, 11, 0, 0, 0}}, {0, i}}, chunk, deferred)
				require.Len(t, events, 3)
				assertSessionEvents(t, events, "iec104", deferred)
				require.Equal(t, true, events[2].Session["Observed STARTDT Confirmation"])
				asdu := events[2].Session["ASDU"].(map[string]any)
				objs := asdu["Objects"].([]map[string]any)
				require.Equal(t, float32(1), objs[0]["Value"])
			})
		}
	}
	_, err := (&binIEC104{}).consume(0, i[:len(i)-1], 4)
	require.Error(t, err)
	_, err = iecObjects([]byte{36, 127, 3, 0, 1, 0}, 4)
	require.Error(t, err)
}
func s7TestPacket(p []byte) []byte {
	n := len(p) + 7
	return append([]byte{3, 0, byte(n >> 8), byte(n), 2, 0xf0, 0x80}, p...)
}
func TestS7NegotiationAndFragments(t *testing.T) {
	job := []byte{0x32, 1, 0, 0, 0, 1, 0, 8, 0, 0, 0xf0, 0, 0, 1, 0, 1, 1, 0xe0}
	ack := []byte{0x32, 3, 0, 0, 0, 1, 0, 8, 0, 0, 0, 0, 0xf0, 0, 0, 1, 0, 1, 1, 0xe0}
	for _, chunk := range []int{0, 1, 7} {
		events, _ := sessionTestFlow(t, "s7comm", []sessionStep{{0, s7TestPacket(job)}, {1, s7TestPacket(ack)}}, chunk, false)
		require.Len(t, events, 2)
		assertSessionEvents(t, events, "s7comm", false)
		require.Equal(t, true, events[1].Session["Matched"])
		require.Equal(t, uint16(480), events[1].Session["PDU Length"])
	}
	s := &binS7{}
	first := s7TestPacket(job[:12])
	first[6] = 0
	_, err := s.consume(0, first, 4, 128)
	require.NoError(t, err)
	out, err := s.consume(0, s7TestPacket(job[12:]), 4, 128)
	require.NoError(t, err)
	require.Equal(t, true, out["Reassembled"])
	require.Empty(t, s.fragments[0])
	_, err = s.consume(0, s7TestPacket(job), 4, 128)
	require.Error(t, err)
	_, err = s7Items([]byte{0x12}, 1, 4)
	require.Error(t, err)
}
func uaTestEnvelope(kind string, b []byte) []byte {
	w := append([]byte(kind), 'F', 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(w[4:], uint32(len(b)+8))
	return append(w, b...)
}
func TestOPCUANegotiationAndSecurityBoundary(t *testing.T) {
	b := make([]byte, 24)
	binary.LittleEndian.PutUint32(b[4:], 8192)
	binary.LittleEndian.PutUint32(b[8:], 8192)
	hello := uaTestEnvelope("HEL", b)
	ack := uaTestEnvelope("ACK", b[:20])
	for _, chunk := range []int{0, 1, 7} {
		for _, deferred := range []bool{false, true} {
			events, _ := sessionTestFlow(t, "opcua", []sessionStep{{0, hello}, {1, ack}}, chunk, deferred)
			require.Len(t, events, 2)
			assertSessionEvents(t, events, "opcua", deferred)
			require.Equal(t, uint32(8192), events[1].Session["Receive Buffer Size"])
		}
	}
	s := &binOPCUA{}
	out, err := s.consume(0, uaTestEnvelope("MSG", make([]byte, 16)), 4, 128)
	require.NoError(t, err)
	require.Equal(t, "security-context-required", out["Semantic Status"])
	bad := append([]byte(nil), hello...)
	bad[28] = 255
	_, err = s.consume(0, bad, 4, 128)
	require.Error(t, err)
}
func TestRFBStandardNoneHandshake(t *testing.T) {
	init := make([]byte, 24)
	binary.BigEndian.PutUint16(init, 16)
	binary.BigEndian.PutUint16(init[2:], 16)
	copy(init[4:], []byte{32, 24, 0, 1, 0, 255, 0, 255, 0, 255, 16, 8, 0, 0, 0, 0})
	steps := []sessionStep{{1, []byte("RFB 003.008\n")}, {0, []byte("RFB 003.008\n")}, {1, []byte{1, 1}}, {0, []byte{1}}, {1, []byte{0, 0, 0, 0}}, {0, []byte{1}}, {1, init}, {0, []byte{5, 1, 0, 2, 0, 3}}, {1, []byte{2}}}
	for _, chunk := range []int{0, 1, 7} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/%v", chunk, deferred), func(t *testing.T) {
				events, _ := sessionTestFlow(t, "vnc", steps, chunk, deferred)
				require.Len(t, events, 9)
				assertSessionEvents(t, events, "vnc", deferred)
				require.Equal(t, uint16(16), events[6].Session["Width"])
				require.Equal(t, uint16(2), events[7].Session["X"])
			})
		}
	}
	_, err := (&binRFB{server: 1, phase: "server-version"}).consume(1, []byte("RFB 004.001\n"), 4, 128)
	require.Error(t, err)
	_, _, err = rfbPixel(make([]byte, 16))
	require.Error(t, err)
}
