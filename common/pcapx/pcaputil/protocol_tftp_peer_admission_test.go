package pcaputil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTFTPRefusedPeerBudgetRecoveryAndGenerationRetirement(t *testing.T) {
	for _, tc := range []struct {
		name              string
		exceed, byteLimit bool
	}{
		{"unique-peer-cap-allows-new-generation", false, false},
		{"count-refusal-quarantines-future-generation", true, false},
		{"byte-refusal-quarantines-future-generation", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewDefaultConfig()
			require.NoError(t, WithProtocolBudget(64, 900)(c))
			require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
			require.NoError(t, c.prepareBinParser())
			a := c.binParser
			a.budget.MaxCollectionElements = 2
			wantRefusal, wantKind := "limited", ErrResourceExceeded
			if tc.byteLimit {
				a.budget.MaxCollectionElements = 128
				// Capture-memory reservations preserve the library's context
				// refusal wrapper and its nested ResourceExceeded cause.
				wantRefusal = "context-required"
			}
			defer func() {
				a.closeUDPSessions()
				require.Zero(t, a.stats().BufferedBytes)
			}()
			feed := func(src, dst string, wire []byte) *ProtocolEvent {
				e := &ProtocolEvent{Source: src, Destination: dst, Transport: "udp", Length: len(wire), Timestamp: time.Unix(100, 0)}
				require.True(t, a.decodeTFTPDatagram(e, wire))
				return e
			}
			client, service := "192.0.2.1:4500", "192.0.2.2:69"
			first, second, third := "192.0.2.2:50000", "192.0.2.2:50001", "192.0.2.2:50002"
			request := feed(client, service, []byte("\x00\x01old.txt\x00octet\x00blksize\x00512\x00"))
			require.Empty(t, request.Error)
			partial := []byte("\x00\x06blksize\x00")
			for _, peer := range []string{first, second} {
				e := feed(peer, client, partial)
				require.Equal(t, "malformed", e.Status)
				require.Nil(t, e.Session)
			}
			charged := a.buffered.Load()
			require.Equal(t, "malformed", feed(first, client, partial).Status)
			require.Equal(t, charged, a.buffered.Load(), "repeated candidate evidence is not charged twice")
			if tc.exceed {
				e := feed(third, client, partial)
				require.Equal(t, wantRefusal, e.Status)
				require.Equal(t, wantKind, e.sessionError.Kind)
				require.Contains(t, e.Error, string(ErrResourceExceeded))
				require.Equal(t, charged, a.buffered.Load())
			}
			beforeID, beforeClock := a.ids.Load(), a.udpSessions.clock
			alien := &ProtocolEvent{Transport: "udp", Source: client, Destination: "192.0.2.2:1812", Timestamp: time.Unix(999999, 0)}
			require.False(t, a.decodeTFTPDatagram(alien, []byte{1, 7, 0, 20}))
			require.Zero(t, alien.ID)
			require.Equal(t, beforeID, a.ids.Load())
			require.Equal(t, beforeClock, a.udpSessions.clock)
			require.Equal(t, charged, a.buffered.Load())
			complete := feed(first, client, []byte("\x00\x06blksize\x00512\x00"))
			require.Empty(t, complete.Error, "a refused candidate is not retired in its current generation")
			require.Equal(t, first, complete.Session["Server Transfer ID"])
			require.Empty(t, feed(client, first, []byte{0, 4, 0, 0}).Error)
			require.Empty(t, feed(first, client, []byte{0, 3, 0, 1, 'o', 'k'}).Error)
			require.Equal(t, true, feed(client, first, []byte{0, 4, 0, 1}).Session["Transfer Complete"])
			next := feed(client, service, []byte("\x00\x01new.txt\x00octet\x00"))
			require.Empty(t, next.Error, "the current server was already counted as one of two unique peers")
			require.NotEqual(t, request.FlowID, next.FlowID)
			require.Empty(t, next.Session["Server Transfer ID"])
			require.Equal(t, "context-required", feed(first, client, []byte{0, 3, 0, 1, 'x'}).Status)
			fresh := feed(third, client, []byte{0, 3, 0, 1, 'x'})
			if tc.exceed {
				// The generation remains unverified, and this additional peer
				// still cannot fit the count/byte budget for refusal evidence.
				require.Equal(t, wantRefusal, fresh.Status)
				require.Equal(t, wantKind, fresh.sessionError.Kind)
				require.Contains(t, fresh.Error, string(ErrResourceExceeded))
				require.Nil(t, fresh.Session)
			} else {
				require.Empty(t, fresh.Error)
				require.Equal(t, third, fresh.Session["Server Transfer ID"])
			}
		})
	}
}
