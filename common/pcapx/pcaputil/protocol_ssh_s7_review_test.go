package pcaputil

import (
	"bytes"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestS7COTPConnectionIsNotApplicationEvidence(t *testing.T) {
	cr, err := hex.DecodeString("0300001611e00000000100c0010ac1020100c2020102")
	require.NoError(t, err)
	require.NotEqual(t, ProbeAccept, probeS7(cr, 4096).Verdict)
	cc := append([]byte(nil), cr...)
	cc[5] = 0xd0
	require.NotEqual(t, ProbeAccept, probeS7(cc, 4096).Verdict)
	// Same COTP TSAP shape followed by session/presentation/MMS data.
	mms := append(append([]byte(nil), cr...), []byte{3, 0, 0, 11, 2, 0xf0, 0x80, 0x0d, 2, 1, 0}...)
	require.Equal(t, ProbeReject, probeS7(mms, 4096).Verdict)
}

func TestS7BufferedCOTPConfirmedByS7Data(t *testing.T) {
	cr, _ := hex.DecodeString("0300001611e00000000100c0010ac1020100c2020102")
	cc, _ := hex.DecodeString("0300001611d00001000100c0010ac1020100c2020102")
	request, _ := hex.DecodeString("0300001902f08032010000000100080000f0000001000101e0")
	response, _ := hex.DecodeString("0300001b02f080320300000001000800000000f0000001000101e0")
	for _, chunk := range []int{1, 7, 4096} {
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		var es []*ProtocolEvent
		for _, step := range []sessionStep{{0, cr}, {1, cc}, {0, request}, {1, response}} {
			w := step.wire
			for len(w) > 0 {
				n := min(len(w), chunk)
				r := s.Feed(step.dir, time.Unix(1, 0), w[:n])
				es = append(es, r.Events...)
				w = w[n:]
			}
		}
		es = append(es, s.Close("test")...)
		require.Zero(t, s.Stats().BufferedBytes)
		var ds []*ProtocolEvent
		for _, e := range es {
			t.Logf("chunk%d %s %s %v %s", chunk, e.Protocol, e.Status, e.Session, e.Error)
			if e.Protocol == "s7comm" && e.Status == "decoded" {
				ds = append(ds, e)
			}
			require.NotEqual(t, "malformed", e.Status, e.Error)
		}
		require.Len(t, ds, 4)
		require.Equal(t, "Connection Request", ds[0].Session["Packet Name"])
		require.Equal(t, "Connection Confirm", ds[1].Session["Packet Name"])
		require.Equal(t, uint16(480), ds[3].Session["PDU Length"])
		require.Equal(t, true, ds[3].Session["Matched"])
	}
}

func TestSSHServerPreIdentificationLines(t *testing.T) {
	steps := []tcpStep{{seq: 99, syn: true}, {seq: 199, syn: true, reverse: true}, {seq: 100, data: "SSH-2.0-control_client\r\n"}, {seq: 200, data: "Maintenance window\r\n" + "SSH-2.0-control_server\r\n", reverse: true}, {seq: 124, fin: true}, {seq: 244, fin: true, reverse: true}}
	var es []*ProtocolEvent
	var stats ProtocolStats
	require.NoError(t, ReplayPcap(bytes.NewReader(binTestPcap(t, steps, 50022, false, true)), WithTCPReassemblyWorkers(1), WithOnProtocolMessage(func(e *ProtocolEvent) { es = append(es, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
	require.Zero(t, stats.BufferedBytes)
	ident := 0
	pre := 0
	for _, e := range es {
		if e.Protocol == "ssh" && (e.Status == "decoded" || e.Status == "deferred") {
			if e.Session["Packet Name"] == "identification" {
				ident++
			}
			if e.Session["Packet Name"] == "pre-identification line" {
				pre++
			}
		}
		require.NotEqual(t, "malformed", e.Status, e.Error)
	}
	require.Equal(t, 2, ident)
	require.Equal(t, 1, pre)
}

func TestSSHPreIdentificationBoundsDirectionAndOwnership(t *testing.T) {
	line := []byte("维护窗口\r\n")
	s := &binSSH{client: 0, clientKnown: true}
	fields, err := s.consume(1, line)
	require.NoError(t, err)
	require.False(t, s.banner[1])
	line[0] = 0
	require.Equal(t, "维护窗口", fields["Text"])
	for i := 1; i < sshMaxPreLines; i++ {
		_, err = s.consume(1, []byte("notice\r\n"))
		require.NoError(t, err)
	}
	_, err = s.consume(1, []byte("extra\r\n"))
	require.Error(t, err)
	_, err = s.consume(1, []byte("SSH-2.0-control_server\r\n"))
	require.NoError(t, err)
	require.True(t, s.banner[1])
	_, err = s.consume(1, []byte("text after identification\r\n"))
	require.Error(t, err)
	for _, tc := range []struct {
		state binSSH
		dir   int
		raw   []byte
	}{
		{binSSH{client: 0, clientKnown: true}, 0, []byte("client preamble\r\n")},
		{binSSH{client: 0}, 1, []byte("unknown direction\r\n")},
		{binSSH{client: 0, clientKnown: true}, 1, []byte("binary\x00line\r\n")},
		{binSSH{client: 0, clientKnown: true}, 1, []byte("SSH-1.5-wrong\r\n")},
		{binSSH{client: 0, clientKnown: true}, 1, []byte("SSH-2.0-\r\n")},
	} {
		_, err = tc.state.consume(tc.dir, tc.raw)
		require.Error(t, err)
		require.False(t, tc.state.banner[tc.dir])
	}
	require.NotEqual(t, ProbeAccept, probeSSHServerPreamble([]byte("notice\r\n"), 4096).Verdict)
	require.Equal(t, ProbeAccept, probeSSHServerPreamble([]byte("notice\r\nSSH-2.0-server\r\n"), 4096).Verdict)
	require.Equal(t, ProbeReject, probeSSHServerPreamble([]byte("notice\r\nSSH-2.0-server\r\n"), 10).Verdict)
	for _, raw := range [][]byte{[]byte("SSH-2.0-\r\n"), []byte("SSH-2.0-bad-version\r\n"), []byte("SSH-2.0-bad\x00version\r\n"), append(append([]byte("SSH-2.0-"), bytes.Repeat([]byte{'a'}, 246)...), []byte("\r\n")...)} {
		_, _, err = sshIdentification(raw)
		require.Error(t, err)
	}
}
