package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
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
