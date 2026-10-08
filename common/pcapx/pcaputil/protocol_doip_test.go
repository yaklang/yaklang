package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

func doipTestWire(kind uint16, body ...byte) []byte {
	w := make([]byte, 8+len(body))
	w[0], w[1] = 2, 0xfd
	binary.BigEndian.PutUint16(w[2:4], kind)
	binary.BigEndian.PutUint32(w[4:8], uint32(len(body)))
	copy(w[8:], body)
	return w
}

func doipTestActive(t *testing.T) *binDoIP {
	t.Helper()
	s := &binDoIP{clientDir: 0, maxPending: 2}
	_, err := s.consume(0, doipTestWire(5, 0x0e, 0x80, 0, 0, 0, 0, 0))
	require.NoError(t, err)
	_, err = s.consume(1, doipTestWire(6, 0x0e, 0x80, 0x40, 0x10, 0x10, 0, 0, 0, 0))
	require.NoError(t, err)
	return s
}

// Field values are handwritten from the valid Scapy DoIP flow in the v2
// delivery, not from its expected.json. Source: secdev/scapy test/pcaps/doip.pcap.
// The raw third-party PCAP stays outside the repository.
func TestDoIPScapyDiagnosticSessionControl(t *testing.T) {
	wires := []string{
		"02fd00050000000b0e80000000000000000000",
		"02fd0006000000090e8040101000000000",
		"02fd8001000000060e8040101002",
		"02fd80020000000540100e8000",
		"02fd80010000000740100e807f107e",
		"02fd8001000000060e8040101003",
		"02fd80020000000540100e8000",
		"02fd80010000000a40100e805003003201f4",
		"02fd000700000000",
	}
	dirs := []int{0, 1, 0, 1, 1, 0, 1, 1, 1}
	s := &binDoIP{clientDir: 0, maxPending: 2}
	var fields map[string]any
	for i, h := range wires {
		w, err := hex.DecodeString(h)
		require.NoError(t, err)
		if i == 0 {
			require.Equal(t, ProbeAccept, probeDoIP(w, 1024).Verdict)
		}
		fields, err = s.consume(dirs[i], w)
		require.NoError(t, err, "frame %d", i)
		switch i {
		case 1:
			require.Equal(t, 0x0e80, fields["Tester Address"])
			require.Equal(t, 0x4010, fields["Entity Address"])
			require.Equal(t, true, fields["Routing Active"])
		case 4:
			require.Equal(t, 0x7e, fields["Negative Response Code"])
			require.Equal(t, true, fields["Diagnostic Acknowledged"])
			require.Zero(t, s.outstanding())
		case 7:
			require.Equal(t, "matched", fields["Association"])
			require.Equal(t, 3, fields["Session Type"])
			require.Equal(t, 50, fields["P2 Server Max Milliseconds"])
			require.Equal(t, 5000, fields["P2 Star Server Max Milliseconds"])
			require.Equal(t, true, fields["Diagnostic Acknowledged"])
			require.Zero(t, s.outstanding())
		}
	}
	require.Equal(t, 1, s.outstanding())
	fields, err := s.consume(0, doipTestWire(8, 0x0e, 0x80))
	require.NoError(t, err)
	require.Equal(t, "matched", fields["Association"])
	require.Zero(t, s.outstanding())
}

func TestDoIPRejectDeliveredInvalidRoutingLengths(t *testing.T) {
	// The delivery's xe02-real-doip-routing-uds labels these as positive.
	// Its generator emits 10-byte routing bodies; ISO 13400 layouts require
	// request 7/11 and response 9/13. Preserve them as strict negative controls.
	for _, h := range []string{"02fd00050000000a0e800000000000000000", "02fd00060000000a0e800e00100000000000"} {
		w, err := hex.DecodeString(h)
		require.NoError(t, err)
		_, err = doipFrameSize(w, 1024)
		require.Error(t, err)
		require.Equal(t, ProbeReject, probeDoIP(w, 1024).Verdict)
	}
}

func TestDoIPStrictAssociationAndPendingResponse(t *testing.T) {
	s := doipTestActive(t)
	request := doipTestWire(0x8001, 0x0e, 0x80, 0x40, 0x10, 0x10, 3)
	_, err := s.consume(0, request)
	require.NoError(t, err)
	for _, tc := range []struct {
		dir  int
		wire []byte
	}{
		{0, doipTestWire(0x8002, 0x40, 0x10, 0x0e, 0x80, 0)},
		{1, doipTestWire(0x8002, 0x40, 0x11, 0x0e, 0x80, 0)},
		{1, doipTestWire(0x8002, 0x40, 0x10, 0x0e, 0x80, 1)},
		{1, doipTestWire(0x8002, 0x40, 0x10, 0x0e, 0x80, 0, 0x10, 2)},
		{1, doipTestWire(0x8001, 0x40, 0x10, 0x0e, 0x80, 0x50, 2, 0, 50, 1, 0xf4)},
		{1, doipTestWire(0x8001, 0x40, 0x10, 0x0e, 0x80, 0x7f, 0x11, 0x78)},
		{0, request},
	} {
		_, err = s.consume(tc.dir, tc.wire)
		require.Error(t, err)
		require.Equal(t, 1, s.outstanding())
	}
	fields, err := s.consume(1, doipTestWire(0x8001, 0x40, 0x10, 0x0e, 0x80, 0x7f, 0x10, 0x78))
	require.NoError(t, err)
	require.Equal(t, true, fields["Response Pending"])
	require.Equal(t, 1, s.outstanding())
	fields, err = s.consume(1, doipTestWire(0x8001, 0x40, 0x10, 0x0e, 0x80, 0x50, 3, 0, 50, 1, 0xf4))
	require.NoError(t, err)
	require.Equal(t, false, fields["Diagnostic Acknowledged"])
	require.Zero(t, s.outstanding())
	_, err = s.consume(1, doipTestWire(0x8001, 0x40, 0x10, 0x0e, 0x80, 0x50, 3, 0, 50, 1, 0xf4))
	require.Error(t, err)
}

func TestDoIPBudgetsOwnershipAndRoutingConfirmation(t *testing.T) {
	request := doipTestWire(5, 0x0e, 0x80, 0, 0, 0, 0, 0, 1, 2, 3, 4)
	s := &binDoIP{clientDir: 0, maxPending: 1}
	fields, err := s.consume(0, request)
	require.NoError(t, err)
	request[len(request)-1] = 99
	require.Equal(t, []byte{1, 2, 3, 4}, fields["OEM Data"])
	_, err = s.consume(1, doipTestWire(6, 0x0e, 0x80, 0x40, 0x10, 0x11, 0, 0, 0, 0))
	require.NoError(t, err)
	require.False(t, s.active)
	_, err = s.consume(0, doipTestWire(0x8001, 0x0e, 0x80, 0x40, 0x10, 0x10, 3))
	require.Error(t, err)
	s = doipTestActive(t)
	s.maxPending = 1
	_, err = s.consume(0, doipTestWire(0x8001, 0x0e, 0x80, 0x40, 0x10, 0x10, 3))
	require.NoError(t, err)
	_, err = s.consume(1, doipTestWire(7))
	require.Error(t, err)
	require.Equal(t, 1, s.outstanding())
	_, err = doipFrameSize([]byte{2, 0xfd, 0x80, 1, 0xff, 0xff, 0xff, 0xff}, 1024)
	require.Error(t, err)
	_, err = doipFrameSize(doipTestWire(5, 0x0e, 0x80, 0, 0, 0, 0, 0), 14)
	require.Error(t, err)
	require.Equal(t, int64(256), s.sessionBytes())
}

func TestDoIPUnsupportedAndMalformedControls(t *testing.T) {
	for _, w := range [][]byte{
		doipTestWire(1), doipTestWire(5, 0x0e, 0x80, 0, 0, 0, 0, 1),
		doipTestWire(5, 0x0e, 0x80, 1, 0, 0, 0, 0), doipTestWire(5, 0, 0, 0, 0, 0, 0, 0),
	} {
		require.Equal(t, ProbeReject, probeDoIP(w, 1024).Verdict)
	}
	for _, data := range [][]byte{{0x22, 0xf1, 0x90}, {0x10}, {0x10, 0}, {0x10, 0x83}, {0x10, 3, 0}} {
		s := doipTestActive(t)
		_, err := s.consume(0, doipTestWire(0x8001, append([]byte{0x0e, 0x80, 0x40, 0x10}, data...)...))
		require.Error(t, err)
		require.Zero(t, s.outstanding())
	}
	w := doipTestWire(5, 0x0e, 0x80, 0, 0, 0, 0, 0)
	for cut := 0; cut < len(w); cut++ {
		require.NotEqual(t, ProbeAccept, probeDoIP(w[:cut], 1024).Verdict)
	}
	w[1] ^= 1
	require.Equal(t, ProbeReject, probeDoIP(w, 1024).Verdict)
}

func TestDoIPRoutingDenialAndResponseMatch(t *testing.T) {
	for _, code := range []byte{0, 1, 2, 3, 4, 5, 6, 7, 0x11} {
		s := &binDoIP{clientDir: 0, maxPending: 2}
		_, err := s.consume(0, doipTestWire(5, 0x0e, 0x80, 0, 0, 0, 0, 0))
		require.NoError(t, err)
		_, err = s.consume(0, doipTestWire(6, 0x0e, 0x80, 0x40, 0x10, code, 0, 0, 0, 0))
		require.Error(t, err)
		require.Equal(t, 1, s.outstanding())
		_, err = s.consume(1, doipTestWire(6, 0x0e, 0x81, 0x40, 0x10, code, 0, 0, 0, 0))
		require.Error(t, err)
		require.Equal(t, 1, s.outstanding())
		fields, err := s.consume(1, doipTestWire(6, 0x0e, 0x80, 0x40, 0x10, code, 0, 0, 0, 0))
		require.NoError(t, err)
		require.False(t, s.active)
		require.Equal(t, int(code), fields["Response Code"])
		require.Zero(t, s.outstanding())
		_, err = s.consume(0, doipTestWire(0x8001, 0x0e, 0x80, 0x40, 0x10, 0x10, 3))
		require.Error(t, err)
	}
}

func TestDoIPDuplicateAcknowledgementAndNACK(t *testing.T) {
	s := doipTestActive(t)
	_, err := s.consume(0, doipTestWire(0x8001, 0x0e, 0x80, 0x40, 0x10, 0x10, 3))
	require.NoError(t, err)
	ack := doipTestWire(0x8002, 0x40, 0x10, 0x0e, 0x80, 0, 0x10, 3)
	first, err := s.consume(1, ack)
	require.NoError(t, err)
	_, err = s.consume(1, ack)
	require.Error(t, err)
	require.Equal(t, 1, s.outstanding())
	_, err = s.consume(1, doipTestWire(0x8001, 0x40, 0x10, 0x0e, 0x80, 0x7f, 0x10, 0x78))
	require.NoError(t, err)
	_, err = s.consume(1, doipTestWire(0x8001, 0x40, 0x10, 0x0e, 0x80, 0x7f, 0x10, 0x78))
	require.NoError(t, err)
	fields, err := s.consume(1, doipTestWire(0x8003, 0x40, 0x10, 0x0e, 0x80, 6))
	require.NoError(t, err)
	require.Equal(t, "Diagnostic Message NACK", fields["Packet Name"])
	require.Zero(t, s.outstanding())
	_, err = s.consume(0, doipTestWire(0x8001, 0x0e, 0x80, 0x40, 0x10, 0x10, 3))
	require.NoError(t, err)
	require.Equal(t, true, first["Diagnostic Acknowledged"], "previous event is an owned snapshot")
	_, err = s.consume(1, doipTestWire(0x8001, 0x40, 0x10, 0x0e, 0x80, 0x50, 3, 0, 50, 1, 0xf4))
	require.NoError(t, err)
}
