package pcaputil

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/cli"
	yaklang "github.com/yaklang/yaklang/common/yak/antlr4yak"
)

func protocolAPICapture(t *testing.T) []byte {
	t.Helper()
	return binTestPcap(t, []tcpStep{{seq: 0, syn: true}, {seq: 1, data: string(binMQTTConnect) + string(binMQTTPublish(32))}}, 1883, false, false)
}

func TestProtocolAPIDefaultAndStatsOnly(t *testing.T) {
	c := NewDefaultConfig()
	require.NotNil(t, c.binParserConfig, "protocol capability is built in")
	require.NoError(t, c.prepareBinParser())
	require.Nil(t, c.binParser, "no consumer must not prepare plans or decode")
	require.False(t, c.reassemblyOptions.Stream, "raw-only/legacy captures retain their behavior")
	var stats ProtocolStats
	require.NoError(t, ReplayPcap(bytes.NewReader(protocolAPICapture(t)), WithOnProtocolStats(func(s ProtocolStats) { stats = s })))
	require.Equal(t, uint64(2), stats.Decoded)
}

func TestProtocolAPIFieldsAndUnsubscribe(t *testing.T) {
	for _, workers := range []int{1, 2, 4} {
		var events []*ProtocolEvent
		require.NoError(t, ReplayPcap(bytes.NewReader(protocolAPICapture(t)), WithTCPReassemblyWorkers(workers), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
		require.Len(t, events, 2)
		require.NotEmpty(t, events[1].Fields)
		events[1].Fields["owned"] = "retained"
		require.Equal(t, "retained", events[1].Structured["fields"].(map[string]any)["owned"])
		require.NotContains(t, events[0].Fields, "owned")
	}
	called := false
	require.NoError(t, ReplayPcap(bytes.NewReader(protocolAPICapture(t)), WithOnProtocolMessage(func(*ProtocolEvent) { called = true }), WithOnProtocolMessage(nil)))
	require.False(t, called)
}

func TestProtocolAPISerialConsumer(t *testing.T) {
	c := NewDefaultConfig()
	var count int // deliberately unguarded user state; the public subscription serializes it
	require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) { count++ })(c))
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 100; n++ {
				c.binParserConfig.OnEvent(&ProtocolEvent{})
			}
		}()
	}
	wg.Wait()
	require.Equal(t, 400, count)
}

func TestProtocolAPIHistoryOwnership(t *testing.T) {
	view, err := NewProtocolInspector()
	require.NoError(t, err)
	require.NoError(t, ReplayPcap(bytes.NewReader(protocolAPICapture(t)), WithOnProtocolMessage(view.OnEvent)))
	rows := view.Rows("mqtt", 0)
	require.Len(t, rows, 2)
	require.Nil(t, rows[1].Fields)
	detail, err := view.Details(rows[1].ID)
	require.NoError(t, err)
	require.NotEmpty(t, detail.Fields)
	detail.Fields["changed"] = true
	again, err := view.Details(rows[1].ID)
	require.NoError(t, err)
	require.NotContains(t, again.Fields, "changed")
}

func TestProtocolAPIDeferredFieldsAndOutputOwnership(t *testing.T) {
	var event *ProtocolEvent
	output := filepath.Join(t.TempDir(), "saved.pcap")
	require.NoError(t, ReplayPcap(bytes.NewReader(protocolAPICapture(t)), WithOutputFile(output), WithProtocolDeferred(true), WithOnProtocolMessage(func(e *ProtocolEvent) { event = e })))
	require.Nil(t, event.Fields)
	fields, err := event.GetFields()
	require.NoError(t, err)
	require.NotEmpty(t, fields)
	legacy, err := event.Decode()
	require.NoError(t, err)
	require.Equal(t, legacy["fields"], fields)
	before, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Error(t, ReplayPcap(bytes.NewReader(protocolAPICapture(t)), WithOutputFile(output)))
	after, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, before, after, "an existing recording must never be truncated")
	require.NoError(t, os.Rename(output, output+".closed"), "owned output must be closed before return")
	for _, options := range [][]CaptureOption{
		{WithOutputFile(output), WithCaptureWriter(&bytes.Buffer{})},
		{WithCaptureWriter(&bytes.Buffer{}), WithOutputFile(output)},
	} {
		require.Error(t, ReplayPcap(bytes.NewReader(protocolAPICapture(t)), options...))
	}
}

func TestProtocolAPICaptureContext(t *testing.T) {
	for _, seconds := range []float64{-1, math.NaN(), math.Inf(1), math.MaxFloat64} {
		_, _, err := CaptureContext(seconds)
		require.Error(t, err)
	}
	ctx, stop, err := CaptureContext(0)
	require.NoError(t, err)
	stop()
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	ctx, stop, err = CaptureContext(0.001)
	require.NoError(t, err)
	defer stop()
	select {
	case <-ctx.Done():
		require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	case <-time.After(time.Second):
		t.Fatal("capture deadline did not fire")
	}
}

func TestProtocolAPISharkScript(t *testing.T) {
	code, err := os.ReadFile(filepath.Join("..", "examples", "shark.yak"))
	require.NoError(t, err)
	input, output := filepath.Join(t.TempDir(), "input.pcap"), filepath.Join(t.TempDir(), "recorded.pcap")
	require.NoError(t, os.WriteFile(input, protocolAPICapture(t), 0600))
	for _, args := range [][]string{
		{"-r", input, "-Y", "mqtt", "-V", "-w", output},
		{"-r", input, "-c", "1"},
		{"-r", output, "-Y", "mqtt"},
	} {
		app := cli.NewCliApp()
		app.SetArgs(args)
		app.SetCliCheckCallback(func() { panic("invalid CLI arguments") })
		engine := yaklang.New()
		var text bytes.Buffer
		fields := 0
		exports := make(map[string]any, len(Exports))
		for name, value := range Exports {
			exports[name] = value
		}
		// File replay has no native devices. Exercise both live-capture display
		// branches too, without requiring a driver in the script regression.
		exports["pcap_onTCPReassemblyStats"] = func(callback func(TCPReassemblyStats)) CaptureOption {
			return WithTCPReassemblyStats(func(stats TCPReassemblyStats) {
				stats.Devices = []TCPDeviceCaptureStats{
					{Device: "test-loopback", Available: true, Dropped: 2},
					{Device: "test-unavailable", Error: "unavailable"},
				}
				callback(stats)
			})
		}
		engine.SetVars(map[string]any{
			"append": func(values []any, more ...any) []any { return append(values, more...) },
			"pcapx":  exports, "cli": cli.GetCliExportMapByCliApp(app),
			"printf":  func(format string, values ...any) { fmt.Fprintf(&text, format, values...) },
			"sprintf": fmt.Sprintf,
			"die":     func(value any) { panic(value) },
			"dump":    func(value any) { require.NotEmpty(t, value); fields++ },
		})
		require.NoError(t, engine.SafeEval(context.Background(), string(code)), "args=%v\n%s", args, text.String())
		require.Contains(t, text.String(), "decoded=")
		require.Contains(t, text.String(), "test-loopback: dropped=2 interfaceDropped=0")
		require.Contains(t, text.String(), "test-unavailable: 抓包丢包统计不可用 unavailable")
		if len(args) > 4 {
			require.Equal(t, 2, fields)
		}
		if len(args) == 4 && args[2] == "-c" {
			require.Equal(t, 1, bytes.Count(text.Bytes(), []byte("decoded         ")))
		}
	}
}
