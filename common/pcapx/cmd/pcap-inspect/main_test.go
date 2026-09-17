package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInspectReplayModesAndRecording(t *testing.T) {
	input := filepath.Join("..", "..", "..", "bin-parser", "testdata", "protocol-corpus", "captures", "ndpi", "ndpi-mqtt.pcap")
	for _, mode := range []string{"full", "deferred"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			saved := filepath.Join(dir, "recording.pcap")
			metrics := filepath.Join(dir, "report.json")
			var out, diagnostics bytes.Buffer
			args := []string{"-read", input, "-" + mode, "-quiet", "-workers", "1", "-history", "128", "-detail", "1", "-write", saved, "-report", metrics}
			require.NoError(t, run(context.Background(), args, &out, &diagnostics))
			var first report
			data, err := os.ReadFile(metrics)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(data, &first))
			require.Equal(t, mode, first.Mode)
			require.NotZero(t, first.Analysis.Messages)
			require.Contains(t, out.String(), "message 1:")
			require.Contains(t, out.String(), "fields")
			if mode == "full" {
				require.NotZero(t, first.DecodedBytes)
				require.Equal(t, first.Analysis.Decoded, first.Analysis.Messages)
			} else {
				require.Zero(t, first.DecodedBytes)
				require.Zero(t, first.DecodedMbps)
				require.Equal(t, first.Analysis.Deferred, first.Analysis.Messages)
			}
			secondFile := filepath.Join(dir, "again.json")
			require.NoError(t, run(context.Background(), []string{"-read", saved, "-" + mode, "-history", "0", "-quiet", "-report", secondFile}, io.Discard, io.Discard))
			data, err = os.ReadFile(secondFile)
			require.NoError(t, err)
			var second report
			require.NoError(t, json.Unmarshal(data, &second))
			require.Equal(t, first.Analysis, second.Analysis)
			require.Equal(t, first.Reassembly.CapturedBytes, second.Reassembly.CapturedBytes)
			require.Equal(t, first.Reassembly.CapturedPackets, second.Reassembly.CapturedPackets)
			before, err := os.ReadFile(saved)
			require.NoError(t, err)
			require.ErrorContains(t, run(context.Background(), []string{"-read", input, "-write", saved}, io.Discard, io.Discard), "create recording")
			after, err := os.ReadFile(saved)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestInspectOptions(t *testing.T) {
	for _, args := range [][]string{
		{}, {"-read", "x", "-interface", "y"}, {"-read", "x", "-workers", "6"},
		{"-read", "x", "-capture-buffer-mib", "257"}, {"-read", "x", "-history", "0", "-follow"},
		{"-read", "x", "-interval", "1ms"}, {"-read", "x", "-duration", "-1s"},
	} {
		_, err := parseOptions(args, io.Discard)
		require.Error(t, err, "%v", args)
	}
	o, err := parseOptions([]string{"-read", "x"}, io.Discard)
	require.NoError(t, err)
	require.True(t, o.full)
	require.False(t, o.deferred)
	require.NoError(t, run(context.Background(), []string{"-help"}, io.Discard, io.Discard))
}

func TestInspectHTTP2AndMySQLSessions(t *testing.T) {
	for _, tc := range []struct {
		name, protocol, detail, field string
		count                         uint64
	}{
		{"http2-multiplex", "http2", "5", "Header Kind", 12},
		{"mysql-classic", "mysql", "8", "More Results", 14},
	} {
		for _, mode := range []string{"full", "deferred"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				input := filepath.Join("..", "..", "pcaputil", "testdata", "protocol-sessions", tc.name+".pcap")
				metrics := filepath.Join(t.TempDir(), "report.json")
				var out bytes.Buffer
				require.NoError(t, run(context.Background(), []string{"-read", input, "-" + mode, "-quiet", "-protocol", tc.protocol, "-detail", tc.detail, "-report", metrics}, &out, io.Discard))
				require.Contains(t, out.String(), `"session"`)
				require.Contains(t, out.String(), tc.field)
				data, err := os.ReadFile(metrics)
				require.NoError(t, err)
				var result report
				require.NoError(t, json.Unmarshal(data, &result))
				require.Equal(t, tc.count, result.Analysis.Messages)
				require.Zero(t, result.Analysis.Malformed)
				require.Zero(t, result.Analysis.ContextRequired)
				if mode == "full" {
					require.Equal(t, tc.count, result.Analysis.Decoded)
				} else {
					require.Equal(t, tc.count, result.Analysis.Deferred)
				}
			})
		}
	}
}

func TestInspectSchedulerIndependentOfWorkers(t *testing.T) {
	old := runtime.GOMAXPROCS(3)
	defer runtime.GOMAXPROCS(old)
	input := filepath.Join("..", "..", "pcaputil", "testdata", "protocol-sessions", "http2-multiplex.pcap")
	for _, procs := range []int{0, 2} {
		file := filepath.Join(t.TempDir(), "report.json")
		require.NoError(t, run(context.Background(), []string{"-read", input, "-quiet", "-workers", "1", "-gomaxprocs", strconv.Itoa(procs), "-report", file}, io.Discard, io.Discard))
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		var got report
		require.NoError(t, json.Unmarshal(data, &got))
		want := procs
		if want == 0 {
			want = 3
		}
		require.Equal(t, want, got.GOMAXPROCS)
		require.Equal(t, runtime.Version(), got.GoVersion)
		require.Equal(t, 3, runtime.GOMAXPROCS(0))
		require.Equal(t, 1, got.Workers)
		require.EqualValues(t, 12, got.Analysis.Decoded)
	}
	_, err := parseOptions([]string{"-read", input, "-gomaxprocs", "-1"}, io.Discard)
	require.Error(t, err)
}
