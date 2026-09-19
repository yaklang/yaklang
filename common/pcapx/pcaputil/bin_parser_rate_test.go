package pcaputil

import (
	"bytes"
	"io"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/require"
)

type binPacedReader struct {
	reader io.Reader
	start  time.Time
	rate   float64
	bytes  int64
	late   time.Duration
}

func (r *binPacedReader) Read(p []byte) (int, error) {
	if len(p) > 32<<10 {
		p = p[:32<<10]
	}
	n, err := r.reader.Read(p)
	if n > 0 {
		r.bytes += int64(n)
		due := r.start.Add(time.Duration(float64(r.bytes) / r.rate * float64(time.Second)))
		if wait := time.Until(due); wait > 0 {
			time.Sleep(wait)
		} else if -wait > r.late {
			r.late = -wait
		}
	}
	return n, err
}

// An explicitly paced file-reader experiment, NOT a native NIC/drop test.
// Full fields plus bounded viewer retention are included; terminal/JSON aren't.
func TestBinParserPacedReplay(t *testing.T) {
	if os.Getenv("PCAPX_BIN_RATE") != "1" {
		t.Skip("set PCAPX_BIN_RATE=1 for a 20-second 150 Mbps replay")
	}
	wire, payload, want := binBenchmarkCapture(t, true)
	binPacedReplay(t, wire, payload, want)
}

func binPacedReplay(t *testing.T, wire []byte, payload int64, want uint64) {
	t.Helper()
	old := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(old)
	view, err := NewBinParserInspector(4096, 32<<20)
	require.NoError(t, err)
	proc, err := process.NewProcess(int32(os.Getpid()))
	require.NoError(t, err)
	cpu, err := proc.Times()
	require.NoError(t, err)
	start := time.Now()
	reader := &binPacedReader{start: start, rate: 150e6 / 8 * float64(len(wire)) / float64(payload)}
	batches := int((150e6 / 8 * 20) / float64(payload))
	var decoded, inputs uint64
	for batch := 0; batch < batches; batch++ {
		reader.reader = bytes.NewReader(wire)
		var stats BinParserStats
		err := ReplayPcap(reader, WithTCPReassemblyWorkers(1), WithBinParser(view.OnEvent), WithBinParserStats(func(s BinParserStats) { stats = s }))
		require.NoError(t, err)
		require.Equal(t, want, stats.Decoded)
		require.EqualValues(t, payload, stats.MessageBytes)
		require.EqualValues(t, payload, stats.InputBytes)
		require.Zero(t, stats.Unknown)
		require.Zero(t, stats.Malformed)
		require.Zero(t, stats.Incomplete)
		require.Zero(t, stats.BufferedBytes)
		decoded += stats.Decoded
		inputs += stats.InputBytes
	}
	elapsed := time.Since(start)
	end, err := proc.Times()
	require.NoError(t, err)
	t.Logf("batches=%d messages=%d payload_bytes=%d elapsed_s=%.6f payload_Mbps=%.3f cpu_s=%.6f max_reader_lateness_ms=%.3f history_evicted=%d", batches, decoded, inputs, elapsed.Seconds(), float64(inputs)*8/elapsed.Seconds()/1e6, end.User+end.System-cpu.User-cpu.System, float64(reader.late)/float64(time.Millisecond), view.Evicted())
}
