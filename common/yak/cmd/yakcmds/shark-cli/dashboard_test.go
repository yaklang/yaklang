package sharkcli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/mattn/go-runewidth"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/netutil/routewrapper"
)

func testRoute(name, gateway string, index, metric int, flags net.Flags) routewrapper.Route {
	_, network, _ := net.ParseCIDR("0.0.0.0/0")
	return routewrapper.Route{Destination: *network, Gateway: net.ParseIP(gateway), Metric: metric, Interface: &net.Interface{Name: name, Index: index, Flags: flags, HardwareAddr: net.HardwareAddr{0, 1, 2, 3, 4, 5}}}
}
func TestPhysicalDefaultRouteIgnoresVPNAndVirtualBridges(t *testing.T) {
	routes := []routewrapper.Route{
		testRoute("utun1024", "198.18.0.1", 1, 0, net.FlagUp|net.FlagPointToPoint),
		testRoute("bridge0", "192.168.4.1", 2, 0, net.FlagUp),
		testRoute("en0", "192.168.1.1", 3, 50, net.FlagUp),
		testRoute("en1", "192.168.3.1", 4, 10, net.FlagUp),
		testRoute("eth0", "192.168.5.1", 5, 0, 0),
	}
	chosen, err := choosePhysicalRoute(routes)
	require.NoError(t, err)
	require.Equal(t, "en1", chosen.Interface.Name)
	require.Equal(t, "192.168.3.1", chosen.Gateway.String())
	_, err = choosePhysicalRoute(routes[:2])
	require.ErrorContains(t, err, "explicit selection")
	for _, name := range []string{"tun0", "tap0", "wg0", "docker0", "veth0", "vEthernet (WSL)", "tailscale0"} {
		require.False(t, physicalInterface(*testRoute(name, "192.168.1.1", 1, 0, net.FlagUp).Interface), name)
	}
}
func TestLiveQueueKeepsNewestPackets(t *testing.T) {
	s := &captureSession{packets: make(chan *capturedPacket, 3)}
	for i := 1; i <= 100000; i++ {
		s.enqueueLatest(&capturedPacket{number: uint64(i)})
	}
	require.Equal(t, uint64(99997), s.skipped.Load())
	for _, n := range []uint64{99998, 99999, 100000} {
		require.Equal(t, n, (<-s.packets).number)
	}
}
func TestPinnedPacketSurvivesEvictionAndLiveDetailsAreImmediate(t *testing.T) {
	u := &tui{ring: newPacketRing(2), follow: true}
	p := dnsPacket(t)
	add := func(n uint64) {
		u.ring.add(&capturedPacket{number: n, data: p, link: layers.LinkTypeEthernet, ci: gopacket.CaptureInfo{Length: len(p), CaptureLength: len(p)}})
		u.refreshVisible()
		u.syncInspection(time.Now())
	}
	add(1)
	require.Equal(t, uint64(1), u.fields.number)
	require.NotEmpty(t, u.fields.hex)
	require.NotEmpty(t, u.fields.fields)
	u.key("\r", &captureSource{})
	require.False(t, u.follow)
	add(2)
	add(3)
	add(4)
	require.Equal(t, uint64(1), u.selected)
	require.Equal(t, uint64(1), u.inspect.number)
	require.Equal(t, uint64(1), u.fields.number)
	u.key(" ", &captureSource{})
	u.refreshVisible()
	u.syncInspection(time.Now())
	require.Equal(t, uint64(4), u.fields.number)
}
func TestRatesUseCounterDeltas(t *testing.T) {
	s := &captureSession{}
	var r trafficRate
	now := time.Now()
	r.reset(now, s)
	s.bytes.Store(4096)
	s.captured.Store(20)
	require.False(t, r.sample(now.Add(500*time.Millisecond), s))
	require.True(t, r.sample(now.Add(2*time.Second), s))
	require.Equal(t, 2048.0, r.bytesPerSecond)
	require.Equal(t, 10.0, r.packetsPerSecond)
	require.Equal(t, 2048.0, r.peak)
	require.True(t, r.sample(now.Add(3*time.Second), s))
	require.Zero(t, r.bytesPerSecond)
}
func TestDashboardFitsTerminalAndRedrawsOnlyChanges(t *testing.T) {
	ansi := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	for _, size := range [][2]int{{70, 24}, {80, 30}, {120, 30}, {160, 48}, {260, 76}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			u := &tui{ring: newPacketRing(3), follow: true, width: size[0], height: size[1], color: true}
			p := dnsPacket(t)
			u.ring.add(&capturedPacket{number: 1, data: p, ci: gopacket.CaptureInfo{Length: len(p)}, link: layers.LinkTypeEthernet})
			u.refreshVisible()
			u.syncInspection(time.Now())
			u.updateMix()
			s := &captureSession{}
			u.rate.history = []float64{0, 10, 40, 20, 80, 50, 30}
			for _, pane := range []int{0, 1, 2} {
				u.pane = pane
				frame := u.view(s, &captureSource{name: "en1", gateway: "192.168.3.1"}, captureConfig{})
				rows := strings.Split(frame, "\r\n")
				require.Len(t, rows, size[1])
				for _, row := range rows {
					require.Equal(t, size[0]-1, runewidth.StringWidth(ansi.ReplaceAllString(row, "")))
				}
				require.Contains(t, frame, "╭")
				require.Contains(t, frame, "192.168.3.1")
				var output bytes.Buffer
				writer := screenWriter{out: &output}
				require.NoError(t, writer.draw(frame, size[0], size[1]))
				require.Positive(t, output.Len())
				output.Reset()
				require.NoError(t, writer.draw(frame, size[0], size[1]))
				require.Zero(t, output.Len())
			}
		})
	}
}
func TestWorkerFrameToleratesDiagnosticNoise(t *testing.T) {
	data := dnsPacket(t)
	raw := &capturedPacket{number: 8, data: data, link: layers.LinkTypeEthernet, ci: gopacket.CaptureInfo{Length: len(data)}}
	input, err := json.Marshal(decodeRequest{Number: 8, Data: data, Length: len(data), Link: layers.LinkTypeEthernet})
	require.NoError(t, err)
	var output bytes.Buffer
	output.WriteString("unsolicited VM log\n")
	require.NoError(t, runDecodeWorker(bytes.NewReader(input), &output))
	result, err := decodeWorkerOutput(output.Bytes(), raw)
	require.NoError(t, err)
	require.Equal(t, uint64(8), result.number)
	require.NotEmpty(t, result.fields)
	raw.number = 9
	_, err = decodeWorkerOutput(output.Bytes(), raw)
	require.Error(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = decodeIsolated(ctx, raw)
	require.ErrorIs(t, err, context.Canceled)
}
