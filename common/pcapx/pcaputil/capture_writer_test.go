package pcaputil

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/pcap"
)

func TestCaptureWriterReplayRoundTrip(t *testing.T) {
	for _, ng := range []bool{false, true} {
		for _, workers := range []int{1, 2, 4} {
			t.Run(fmt.Sprintf("ng=%v/w%d", ng, workers), func(t *testing.T) {
				input := binTestPcap(t, []tcpStep{{syn: true, seq: 0}, {seq: 1, data: string(binMQTTConnect)}}, 1883, false, ng)
				var saved bytes.Buffer
				var stats TCPReassemblyStats
				events, analysis, err := binReplay(t, input, workers, WithCaptureWriter(&saved), WithTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s }))
				require.NoError(t, err)
				require.True(t, stats.CaptureAccountingAvailable)
				require.EqualValues(t, 2, stats.CapturedPackets)
				r, err := pcapgo.NewReader(bytes.NewReader(saved.Bytes()))
				require.NoError(t, err)
				var total uint64
				for i := 0; i < 2; i++ {
					raw, ci, err := r.ReadPacketData()
					require.NoError(t, err)
					total += uint64(len(raw))
					require.True(t, time.Unix(1700000000, int64(i)*1234).Equal(ci.Timestamp))
				}
				require.Equal(t, total, stats.CapturedBytes)
				_, _, err = r.ReadPacketData()
				require.ErrorIs(t, err, io.EOF)
				again, second, err := binReplay(t, saved.Bytes(), workers)
				require.NoError(t, err)
				require.Equal(t, analysis, second)
				for i := range events {
					require.True(t, events[i].Timestamp.Equal(again[i].Timestamp))
					events[i].Timestamp, again[i].Timestamp = time.Time{}, time.Time{}
					events[i].plan, again[i].plan = nil, nil
				}
				require.Equal(t, events, again)
			})
		}
	}
}

type failingCaptureWriter struct {
	left int
	err  error
}

func (w *failingCaptureWriter) Write(b []byte) (int, error) {
	if len(b) > w.left {
		n := w.left
		w.left = 0
		return n, w.err
	}
	w.left -= len(b)
	return len(b), nil
}
func TestCaptureWriterErrorsAndOptions(t *testing.T) {
	want := errors.New("storage full")
	w := &failingCaptureWriter{left: 24, err: want}
	input := binTestPcap(t, []tcpStep{{syn: true, seq: 0}, {seq: 1, data: string(binMQTTConnect)}}, 1883, false, false)
	var stats TCPReassemblyStats
	err := ReplayPcap(bytes.NewReader(input), WithCaptureWriter(w), WithTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s }))
	require.ErrorContains(t, err, want.Error()) // pcapgo wraps writer errors as text
	require.EqualValues(t, 1, stats.CapturedPackets, "read packet remains accounted when recording fails")
	var b bytes.Buffer
	r := &captureWriter{output: &b}
	require.NoError(t, r.init(layers.LinkTypeEthernet))
	require.ErrorContains(t, r.init(layers.LinkTypeNull), "link type changed")
	require.Error(t, r.init(layers.LinkTypeEthernet), "writer failures are sticky")
	for _, opt := range []CaptureOption{WithCaptureWriter(&b), WithCaptureBufferSize(1 << 20)} {
		require.ErrorContains(t, Start(WithEnableCache(true), opt), "exclusive")
		require.ErrorContains(t, Start(opt, WithEnableCache(true)), "exclusive")
	}
	require.Error(t, WithCaptureWriter(nil)(NewDefaultConfig()))
	for _, size := range []int{-1, 1, 257 << 20} {
		require.Error(t, WithCaptureBufferSize(size)(NewDefaultConfig()))
	}
	for _, size := range []int{0, 64 << 10, 32 << 20, 256 << 20} {
		require.NoError(t, WithCaptureBufferSize(size)(NewDefaultConfig()))
	}
	require.Error(t, ReplayPcap(bytes.NewReader(input), WithCaptureBufferSize(1<<20)))
	require.ErrorContains(t, Start(WithFile("unused.pcap"), WithCaptureBufferSize(1<<20)), "live devices")
}

func TestCaptureWriterEmptyPcapng(t *testing.T) {
	var input, output bytes.Buffer
	w, err := pcapgo.NewNgWriter(&input, layers.LinkTypeNull)
	require.NoError(t, err)
	require.NoError(t, w.Flush())
	require.NoError(t, ReplayPcap(&input, WithCaptureWriter(&output)))
	r, err := pcapgo.NewReader(&output)
	require.NoError(t, err)
	require.Equal(t, layers.LinkTypeNull, r.LinkType())
	_, _, err = r.ReadPacketData()
	require.ErrorIs(t, err, io.EOF)
}

func TestCaptureWriterNativeBPFFilter(t *testing.T) {
	if _, err := pcap.FindAllDevs(); err != nil {
		t.Skipf("native pcap unavailable: %v", err)
	}
	input := binTestPcap(t, []tcpStep{{syn: true, seq: 0}, {seq: 1, data: string(binMQTTConnect)}}, 1883, false, false)
	name := filepath.Join(t.TempDir(), "input.pcap")
	require.NoError(t, os.WriteFile(name, input, 0600))
	for _, public := range []bool{false, true} {
		var saved bytes.Buffer
		var stats TCPReassemblyStats
		opts := []CaptureOption{WithCaptureWriter(&saved), WithBPFFilter("tcp port 80"), WithTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s })}
		if public {
			opts = append(opts, WithEveryPacket(func(gopacket.Packet) { t.Error("filtered packet delivered") }))
		}
		require.NoError(t, OpenPcapFile(name, opts...))
		require.Zero(t, stats.CapturedPackets)
		r, err := pcapgo.NewReader(bytes.NewReader(saved.Bytes()))
		require.NoError(t, err)
		_, _, err = r.ReadPacketData()
		require.ErrorIs(t, err, io.EOF)
	}
}

func TestBinParserInspectorCursor(t *testing.T) {
	v, err := NewBinParserInspector(3, 8)
	require.NoError(t, err)
	for _, e := range []*BinParserEvent{
		{ID: 9, Protocol: "http", FlowID: 1, Raw: []byte("a")},
		{ID: 2, Protocol: "dns", FlowID: 2, Raw: []byte("b")},
		{ID: 5, Protocol: "http", FlowID: 1, Raw: []byte("c")},
	} {
		v.OnEvent(e)
	}
	rows, cursor, omitted := v.RowsAfter(0, 2, "", 0)
	require.EqualValues(t, 3, cursor)
	require.EqualValues(t, 1, omitted)
	require.Equal(t, []uint64{2, 5}, []uint64{rows[0].ID, rows[1].ID})
	rows[0].ID = 100
	require.Nil(t, rows[0].Raw)
	require.EqualValues(t, 2, v.Rows("dns", 0)[0].ID)
	rows, _, omitted = v.RowsAfter(0, 2, "http", 1)
	require.Len(t, rows, 2)
	require.Zero(t, omitted, "display filters are not omissions")
	v.OnEvent(&BinParserEvent{ID: 3, Protocol: "http", Raw: []byte("d")})
	v.OnEvent(&BinParserEvent{ID: 4, Raw: bytes.Repeat([]byte("x"), 9)})
	rows, next, omitted := v.RowsAfter(cursor, 10, "", 0)
	require.Len(t, rows, 1)
	require.EqualValues(t, 3, rows[0].ID)
	require.EqualValues(t, 5, next)
	require.EqualValues(t, 1, omitted, "oversized history exclusion advances cursor")
	rows, _, omitted = v.RowsAfter(0, 10, "", 0)
	require.Len(t, rows, 3)
	require.EqualValues(t, 2, omitted)
	rows, _, omitted = v.RowsAfter(next, 10, "", 0)
	require.Empty(t, rows)
	require.Zero(t, omitted)
}
