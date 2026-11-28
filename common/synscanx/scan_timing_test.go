package synscanx

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/netstackvm"
	"github.com/yaklang/yaklang/common/synscan"
)

func TestSynScanTimingSeparatesProbingAndResultDelivery(t *testing.T) {
	const delay = 40 * time.Millisecond
	for _, slowProbe := range []bool{true, false} {
		name := "slow_result_delivery"
		if slowProbe {
			name = "slow_probe"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			timing := &synScanTiming{}
			out := make(chan *synscan.SynScanResult, 1)
			if !slowProbe {
				out <- &synscan.SynScanResult{}
			}
			prober := &recordingProber{probe: func(context.Context, string) (netstackvm.TCPSegment, error) {
				if slowProbe {
					time.Sleep(delay)
				}
				return netstackvm.TCPSegment{RemoteIP: net.IPv4(192, 0, 2, 1), RemotePort: 1}, nil
			}}
			scanner := &Scannerx{
				ctx: ctx, halfOpen: prober, timing: timing,
				config: &SynxConfig{tcpProbeConcurrency: 1, tcpProbeTimeout: time.Second},
				openReporter: func(host net.IP, port int, proto string) {
					timing.sendResult(ctx, out, &synscan.SynScanResult{Host: host.String(), Port: port, Protocol: proto})
				},
			}
			targets := make(chan *SynxTarget, 1)
			targets <- &SynxTarget{Host: "192.0.2.1", Port: 1, Mode: TCP}
			close(targets)
			done := make(chan struct{})
			go func() {
				defer close(done)
				scanner.sendPacket(targets)
			}()

			var probeBeforeDelivery, probeEndBeforeDelivery int64
			if !slowProbe {
				require.Eventually(t, func() bool { return timing.resultSends.Load() == 1 }, time.Second, time.Millisecond)
				probeBeforeDelivery = timing.tcpProbeTime.Load()
				probeEndBeforeDelivery = timing.lastProbeEnd.UnixNano()
				require.Positive(t, probeEndBeforeDelivery)
				time.Sleep(delay)
				<-out
			}
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal("result delivery did not finish")
			}
			select {
			case got := <-out:
				require.Equal(t, &synscan.SynScanResult{Host: "192.0.2.1", Port: 1, Protocol: "tcp"}, got)
			default:
				t.Fatal("completed result is missing")
			}
			require.EqualValues(t, 1, timing.tcpProbes.Load())
			require.EqualValues(t, 1, timing.resultSends.Load())
			require.Zero(t, timing.resultSendCancel.Load())
			if slowProbe {
				require.GreaterOrEqual(t, time.Duration(timing.tcpProbeTime.Load()), delay)
			} else {
				require.Equal(t, probeBeforeDelivery, timing.tcpProbeTime.Load())
				require.Equal(t, probeEndBeforeDelivery, timing.lastProbeEnd.UnixNano())
				require.GreaterOrEqual(t, time.Duration(timing.resultSendTime.Load()), delay)
			}
			t.Logf("syn_probe=%s last_probe_finished_at_ns=%d result_send=%s", time.Duration(timing.tcpProbeTime.Load()), timing.lastProbeEnd.UnixNano(), time.Duration(timing.resultSendTime.Load()))
		})
	}
}
