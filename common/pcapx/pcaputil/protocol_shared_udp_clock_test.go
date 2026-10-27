package pcaputil

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func sharedUDPClockParser(t *testing.T) *binParser {
	t.Helper()
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
	require.NoError(t, c.prepareBinParser())
	t.Cleanup(func() { c.binParser.closeUDPSessions(); require.Zero(t, c.binParser.buffered.Load()) })
	return c.binParser
}

func clockUDPEvent(domain CaptureDomain, at int64) *ProtocolEvent {
	return &ProtocolEvent{Source: "192.0.2.1:49000", Destination: "192.0.2.2:14443", Domain: domain, Transport: "udp", Timestamp: time.Unix(at, 0)}
}

func TestQUICIdleLeaseCannotBeRetiredByForeignUDPObservation(t *testing.T) {
	for _, operation := range []string{"foreign-domain-QUIC", "unknown-CID-same-domain", "CoAP-same-domain", "CoAP-foreign-domain"} {
		t.Run(operation, func(t *testing.T) {
			a := sharedUDPClockParser(t)
			domain := CaptureDomain{Section: 2, Interface: 3, Encapsulation: "/vlan:17"}
			wire := rfc9001ClientInitial()
			first, handled := a.decodeQUICDatagram(clockUDPEvent(domain, 100), wire)
			require.True(t, handled)
			require.Len(t, first, 1)
			require.Equal(t, true, first[0].Session["Authentication Verified"])
			var original *binUDPEntry
			for _, el := range a.udpSessions.entries {
				original = el.Value.(*binUDPEntry)
			}
			require.NotNil(t, original)
			foreign := clockUDPEvent(domain, 10000)
			if operation == "foreign-domain-QUIC" || operation == "CoAP-foreign-domain" {
				foreign.Domain.Interface++
			}
			if operation == "foreign-domain-QUIC" {
				_, handled = a.decodeQUICDatagram(foreign, wire)
				require.True(t, handled)
			} else if operation == "unknown-CID-same-domain" {
				_, handled = a.decodeQUICDatagram(foreign, append([]byte{0x40}, bytes.Repeat([]byte{0xa5}, 40)...))
				require.False(t, handled)
			} else {
				foreign.Source, foreign.Destination = "192.0.2.3:49001", "192.0.2.4:5683"
				require.True(t, a.decodeCoAPDatagram(foreign, []byte{0x40, 1, 0, 7, 0xb1, 'x'}))
			}
			require.Contains(t, a.udpSessions.entries, original.key, "a CoAP cleanup cannot delete another protocol's retained CID history")
			require.Same(t, original, a.udpSessions.entries[original.key].Value)
			require.False(t, original.flow.quic.retired, "foreign clocks cannot retire this captured connection")
			require.Equal(t, time.Unix(100, 0), original.touched)
			last, handled := a.decodeQUICDatagram(clockUDPEvent(domain, 101), wire)
			require.True(t, handled)
			require.Len(t, last, 1)
			require.Empty(t, last[0].Error)
			require.Equal(t, first[0].FlowID, last[0].FlowID)
			require.Equal(t, true, last[0].Session["Authentication Verified"])
			require.Equal(t, true, last[0].Session["Duplicate"])
			require.Equal(t, time.Unix(101, 0), original.touched, "the entry owns actual captured time, not the shared store watermark")
		})
	}
}

func TestCoAPIdleCleanupRespectsCaptureDomainAndActualTime(t *testing.T) {
	a := sharedUDPClockParser(t)
	domain := CaptureDomain{Interface: 3}
	first := clockUDPEvent(domain, 100)
	wire := []byte{0x40, 1, 0, 7, 0xb1, 'x'}
	require.True(t, a.decodeCoAPDatagram(first, wire))
	var original *binUDPEntry
	for _, el := range a.udpSessions.entries {
		original = el.Value.(*binUDPEntry)
	}
	foreign := clockUDPEvent(CaptureDomain{Interface: 4}, 10000)
	require.True(t, a.decodeCoAPDatagram(foreign, wire))
	require.Contains(t, a.udpSessions.entries, original.key)
	require.Same(t, original, a.udpSessions.entries[original.key].Value)
	for _, stamp := range []int64{101, 99} {
		request := clockUDPEvent(domain, stamp)
		require.True(t, a.decodeCoAPDatagram(request, wire))
		require.Equal(t, first.FlowID, request.FlowID)
		require.Equal(t, time.Unix(101, 0), original.touched, "late timestamps do not rewind or borrow another domain's time")
	}
}

func FuzzSharedUDPClockCannotEraseQUICIdentity(f *testing.F) {
	for _, seed := range [][]byte{{0, 1, 2, 3}, {0, 0, 0}, {3, 2, 1}, {255, 128, 1, 4}} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, operations []byte) {
		if len(operations) > 32 {
			return
		}
		a := sharedUDPClockParser(t)
		domain := CaptureDomain{Section: 1, Interface: 2}
		wire := rfc9001ClientInitial()
		first, handled := a.decodeQUICDatagram(clockUDPEvent(domain, 100), wire)
		require.True(t, handled)
		require.Len(t, first, 1)
		for i, operation := range operations {
			foreign := clockUDPEvent(CaptureDomain{Section: 1, Interface: 3 + int(operation&1)}, 10000+int64(i)*100)
			if operation&4 != 0 {
				foreign.Timestamp = time.Time{}
			}
			switch operation & 3 {
			case 0:
				a.decodeQUICDatagram(foreign, wire)
			case 1:
				foreign.Domain = domain
				a.decodeQUICDatagram(foreign, append([]byte{0x40}, bytes.Repeat([]byte{0xff}, 40)...))
			case 2:
				foreign.Source, foreign.Destination = "192.0.2.3:49001", "192.0.2.4:5683"
				a.decodeCoAPDatagram(foreign, []byte{0x40, 1, 0, 7, 0xb1, 'x'})
			case 3:
				foreign.Source, foreign.Destination = "192.0.2.5:49002", "192.0.2.6:69"
				a.decodeTFTPDatagram(foreign, []byte{0, 1, 'x', 0, 'o', 'c', 't', 'e', 't', 0})
			}
			observed, handled := a.decodeQUICDatagram(clockUDPEvent(domain, 101), wire)
			require.True(t, handled)
			require.Len(t, observed, 1)
			require.Equal(t, first[0].FlowID, observed[0].FlowID)
			require.Empty(t, observed[0].Error)
			require.Equal(t, true, observed[0].Session["Authentication Verified"])
			require.Equal(t, true, observed[0].Session["Duplicate"])
		}
	})
}
