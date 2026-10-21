package pcaputil

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func c37TwoControlPMUs() []byte {
	one := c37ControlCFG(1, 7, 0)
	body := append([]byte(nil), one[14:len(one)-4]...)
	binary.BigEndian.PutUint16(body[4:], 2)
	body = append(body, one[20:len(one)-4]...)
	body = append(body, 0, 50)
	return c37frame(3, 1, 7, body)
}

func TestC37118CollectionRefusalInvalidatesContext(t *testing.T) {
	for _, transport := range []string{"tcp", "udp"} {
		t.Run(transport, func(t *testing.T) {
			budget := DefaultParserBudget()
			budget.MaxCollectionElements = 1
			s, err := NewProtocolSessionWithOptions(budget, WithSessionTransport(transport), WithSessionPorts(39300, 28400))
			require.NoError(t, err)
			defer s.Close("test")
			cfg := c37ControlCFG(1, 7, 0)
			data := c37frame(0, 1, 7, []byte{0, 0, 0, 5, 255, 254})
			for i, wire := range [][]byte{cfg, data} {
				r := s.Feed(0, time.Unix(int64(i+1), 0), wire)
				require.Nil(t, r.Err)
				require.Len(t, r.Events, 1)
			}
			r := s.Feed(0, time.Unix(3, 0), c37TwoControlPMUs())
			require.NotNil(t, r.Err)
			require.Equal(t, ErrResourceExceeded, r.Err.Kind)
			r = s.Feed(0, time.Unix(4, 0), data)
			require.NotNil(t, r.Err, "a missed CFG-2 replacement must not leave the old measurement layout usable")
			require.Equal(t, ErrContextRequired, r.Err.Kind)
			require.Len(t, r.Events, 1)
			require.NotContains(t, r.Events[0].Session, "Data")
			for i, wire := range [][]byte{cfg, data} {
				r = s.Feed(0, time.Unix(int64(i+5), 0), wire)
				require.Nil(t, r.Err, "a subsequently observed affordable configuration recovers")
				require.Len(t, r.Events, 1)
			}
			s.Close("test")
			s.Close("again")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestC37118SessionQueriesOwnAcceptedFields(t *testing.T) {
	for _, transport := range []string{"tcp", "udp"} {
		t.Run(transport, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(39300, 28400))
			require.NoError(t, err)
			defer s.Close("test")
			for i, wire := range [][]byte{c37ControlCFG(1, 7, 0), c37frame(0, 1, 7, []byte{0, 0, 0, 5, 255, 254})} {
				r := s.Feed(0, time.Unix(int64(i+1), 0), wire)
				require.Nil(t, r.Err)
				require.Len(t, r.Events, 1)
				e := r.Events[0]
				want, err := e.GetFields()
				require.NoError(t, err)
				require.EqualValues(t, 7, want["ID Code"])
				require.EqualValues(t, 1, want["Version"])
				for _, edit := range []func(){func() { e.Session["ID Code"] = 999999 }, func() { e.Session = map[string]any{"forged": true} }, func() { e.Session = nil }} {
					edit()
					got, err := e.Decode()
					require.NoError(t, err)
					require.Equal(t, want, got["fields"])
					require.Equal(t, want, got["session"], "public Session mutation cannot alter a later accepted query")
					clear(got["session"].(map[string]any))
					again, err := e.Decode()
					require.NoError(t, err)
					require.Equal(t, want, again["session"])
				}
			}
		})
	}
}

func TestC37118CaptureDomainClockIsolation(t *testing.T) {
	var events []*ProtocolEvent
	var stats ProtocolStats
	a, err := NewPacketAnalyzer(WithProtocolDecodeAs("udp", 53, "c37118"), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }))
	require.NoError(t, err)
	defer a.Close()
	feed := func(iface int, second int64, wire []byte) *ProtocolEvent {
		raw := m1UDP(t, wire)
		// This helper uses port 53; explicit decode-as on that endpoint keeps
		// the public capture-domain test independent of port heuristics.
		require.NoError(t, a.Feed(raw, gopacket.CaptureInfo{Timestamp: time.Unix(second, 0), CaptureLength: len(raw), Length: len(raw), InterfaceIndex: iface}, layers.LinkTypeRaw))
		require.NotEmpty(t, events)
		return events[len(events)-1]
	}
	cfg, data := c37ControlCFG(1, 7, 0), c37frame(0, 1, 7, []byte{0, 0, 0, 5, 255, 254})
	first := feed(0, 1, cfg)
	require.Empty(t, first.Error)
	other := feed(1, 100, cfg)
	require.Empty(t, other.Error)
	got := feed(0, 2, data)
	require.Empty(t, got.Error, "an unrelated capture domain's future timestamp cannot expire this publisher's CFG-2")
	require.Equal(t, first.FlowID, got.FlowID)
	require.NotEqual(t, other.FlowID, got.FlowID)
	require.Contains(t, got.Session, "Data")
	got = feed(1, 101, data)
	require.Empty(t, got.Error)
	require.Equal(t, other.FlowID, got.FlowID)
	got = feed(0, 33, data)
	require.NotEmpty(t, got.Error)
	require.Equal(t, ErrContextRequired, got.sessionError.Kind)
	require.Zero(t, got.FlowID)
	require.Empty(t, feed(0, 34, cfg).Error)
	require.Empty(t, feed(0, 35, data).Error)
	require.NoError(t, a.Close())
	require.NoError(t, a.Close())
	require.Zero(t, stats.BufferedBytes)
	// Seven packets above, one independently classified event for each.
	require.Len(t, events, 7)
}
