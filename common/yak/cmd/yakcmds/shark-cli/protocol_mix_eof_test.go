package sharkcli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func mixFixturePacket(u *tui, number uint64, protocol string) {
	u.ring.add(&capturedPacket{number: number})
	u.ring.at(u.ring.size - 1).summary = packetSummary{Number: number, Protocol: protocol}
}

func TestProtocolMixAtEOFUsesCurrentPhysicalWindow(t *testing.T) {
	for _, profile := range []string{"final-batch", "late-observation", "filtered-pinned", "empty"} {
		t.Run(profile, func(t *testing.T) {
			now := time.Unix(1000, 0)
			s := &captureSession{}
			u := &tui{ring: newPacketRing(256), finished: true}
			u.rate.reset(now, s)
			for n := uint64(1); n <= 128; n++ {
				mixFixturePacket(u, n, "TCP")
			}
			u.updateMix()
			require.Equal(t, []protocolShare{{"TCP", 128}}, u.mix)
			want := []protocolShare{{"DNS", 128}}
			switch profile {
			case "final-batch", "filtered-pinned":
				for n := uint64(129); n <= 256; n++ {
					mixFixturePacket(u, n, "DNS")
				}
				if profile == "filtered-pinned" {
					u.searchText = "proto:TCP"
					u.visible = []*packetEntry{u.ring.at(0)}
					u.frozen = []packetEntry{*u.ring.at(0)}
				}
			case "late-observation":
				version := u.ring.version
				for n := 0; n < 128; n++ {
					u.ring.at(n).summary.Protocol = "DNS"
				}
				require.Equal(t, version, u.ring.version)
			case "empty":
				u.ring = newPacketRing(1)
				want = nil
			}
			u.refreshProtocolMix(now.Add(100*time.Millisecond), s)
			require.Equal(t, want, u.mix)
			if profile == "empty" {
				require.Zero(t, u.mixTotal)
			} else {
				require.Equal(t, 128, u.mixTotal)
			}
			require.Empty(t, u.rate.history, "EOF refresh is not a synthetic traffic sample")
			require.Zero(t, s.captured.Load())
			require.Zero(t, s.bytes.Load())
		})
	}
}

func TestProtocolMixLiveRetainsRateInterval(t *testing.T) {
	now := time.Unix(1000, 0)
	s := &captureSession{}
	u := &tui{ring: newPacketRing(2)}
	u.rate.reset(now, s)
	mixFixturePacket(u, 1, "TCP")
	u.updateMix()
	mixFixturePacket(u, 2, "DNS")
	u.refreshProtocolMix(now.Add(100*time.Millisecond), s)
	require.Equal(t, []protocolShare{{"TCP", 1}}, u.mix)
	u.refreshProtocolMix(now.Add(time.Second), s)
	require.Equal(t, []protocolShare{{"DNS", 1}, {"TCP", 1}}, u.mix)
	require.Equal(t, 2, u.mixTotal)
	require.Len(t, u.rate.history, 1)
}
