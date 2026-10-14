package pcaputil

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func gbtTestWire(src, dst uint16, bc byte, bn, ack uint16, body []byte) []byte {
	p := []byte{0xe0, bc, byte(bn >> 8), byte(bn), byte(ack >> 8), byte(ack), byte(len(body))}
	p = append(p, body...)
	w := make([]byte, 8)
	binary.BigEndian.PutUint16(w, 1)
	binary.BigEndian.PutUint16(w[2:], src)
	binary.BigEndian.PutUint16(w[4:], dst)
	binary.BigEndian.PutUint16(w[6:], uint16(len(p)))
	return append(w, p...)
}
func TestDLMSGBTExistingAPIMultiblock(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
	require.NoError(t, err)
	defer s.Close("test")
	req := []byte{0xc0, 1, 0xc1, 0, 1, 0, 0, 1, 0, 0, 0xff, 2, 0}
	var events []*ProtocolEvent
	steps := []struct {
		dir  int
		wire []byte
	}{{0, gbtTestWire(1, 16, 1, 1, 0, req[:6])}, {1, gbtTestWire(16, 1, 1, 1, 1, nil)}, {0, gbtTestWire(1, 16, 0x81, 2, 1, req[6:])}, {1, gbtTestWire(16, 1, 0x81, 2, 2, []byte{0xc4, 1, 0xc1, 0, 0x12, 1, 0})}}
	for _, x := range steps {
		out := s.Feed(x.dir, time.Unix(1, 0), x.wire)
		require.Nil(t, out.Err)
		require.Len(t, out.Events, 1)
		events = append(events, out.Events[0])
		f, e := out.Events[0].GetFields()
		require.NoError(t, e)
		require.Equal(t, "GeneralBlockTransfer", f["kind"])
	}
	require.Equal(t, events[2].ID, events[3].ResponseTo)
	require.Equal(t, events[2].ID, events[3].TransactionID)
	require.Zero(t, events[0].ResponseTo)
	fields, e := events[3].GetFields()
	require.NoError(t, e)
	require.Equal(t, "observed-response", events[3].Session["Association"])
	inner := fields["reassembled_apdu"].(map[string]any)
	require.Equal(t, "GetResponseNormal", inner["kind"])
	require.Equal(t, uint64(256), inner["data"].(map[string]any)["value"])
	require.Empty(t, s.Close("test"))
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestDLMSGBTSequenceConflictAndWindowBoundaries(t *testing.T) {
	req := []byte{0xc0, 1, 0xc1, 0, 1, 0, 0, 1, 0, 0, 0xff, 2, 0}
	for _, window := range []byte{1, 2, 3, 63} {
		t.Run(fmt.Sprint(window), func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("test")
			a := s.Feed(0, time.Unix(1, 0), gbtTestWire(1, 16, window|64, 1, 0, req[:6]))
			require.Nil(t, a.Err)
			require.Len(t, a.Events, 1)
			require.Equal(t, "dlms-wrapper-v1-gbt", a.Events[0].Profile)
			b := s.Feed(0, time.Unix(1, 0), gbtTestWire(1, 16, window|128, 2, 0, req[6:]))
			require.Nil(t, b.Err)
			require.Len(t, b.Events, 1)
			require.Equal(t, "observed-request", b.Events[0].Session["Association"])
			require.Equal(t, b.Events[0].ID, b.Events[0].TransactionID)
		})
	}
	for _, c := range []struct {
		name      string
		mid, last []byte
		want      string
	}{
		{"duplicate", gbtTestWire(1, 16, 1, 1, 0, req[:6]), gbtTestWire(1, 16, 129, 2, 0, req[6:]), "observed-request"},
		{"conflict", gbtTestWire(1, 16, 1, 1, 0, []byte{0xc0, 1, 0xc2, 0, 1, 0}), gbtTestWire(1, 16, 129, 2, 0, req[6:]), "ambiguous-gbt-transfer"},
		{"gap", gbtTestWire(1, 16, 1, 3, 0, req[6:]), gbtTestWire(1, 16, 129, 2, 0, req[6:]), "ambiguous-gbt-transfer"},
		{"ack-ahead", gbtTestWire(1, 16, 1, 2, 9, req[6:]), gbtTestWire(1, 16, 129, 3, 0, req[6:]), "ambiguous-gbt-transfer"},
		{"zero-window", gbtTestWire(1, 16, 0, 2, 0, req[6:]), gbtTestWire(1, 16, 129, 3, 0, req[6:]), "ambiguous-gbt-transfer"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("test")
			for _, w := range [][]byte{gbtTestWire(1, 16, 1, 1, 0, req[:6]), c.mid} {
				r := s.Feed(0, time.Unix(1, 0), w)
				require.Nil(t, r.Err)
				require.Len(t, r.Events, 1)
				require.Zero(t, r.Events[0].ResponseTo)
			}
			r := s.Feed(0, time.Unix(1, 0), c.last)
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			require.Equal(t, c.want, r.Events[0].Session["Association"])
			if c.want != "observed-request" {
				require.Zero(t, r.Events[0].TransactionID)
				f, e := r.Events[0].GetFields()
				require.NoError(t, e)
				require.NotContains(t, f, "reassembled_apdu")
			}
			s.Close("test")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDLMSGBTNativeOwnershipAndIncompleteClose(t *testing.T) {
	for _, chunk := range []int{1, 7, 65535} {
		t.Run(fmt.Sprint(chunk), func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("test")
			req := []byte{0xc0, 1, 0xc1, 0, 1, 0, 0, 1, 0, 0, 0xff, 2, 0}
			first := gbtTestWire(1, 16, 1, 1, 0, req[:6])
			last := gbtTestWire(1, 16, 129, 2, 0, req[6:])
			var events []*ProtocolEvent
			for _, wire := range [][]byte{first, last} {
				for off := 0; off < len(wire); off += chunk {
					r := s.Feed(0, time.Unix(1, 0), wire[off:min(off+chunk, len(wire))])
					if r.Err != nil {
						require.Equal(t, ErrNeedMore, r.Err.Kind)
						require.Empty(t, r.Events)
					}
					events = append(events, r.Events...)
				}
			}
			require.Len(t, events, 2)
			require.Equal(t, events[1].ID, events[1].TransactionID)
			f, e := events[1].GetFields()
			require.NoError(t, e)
			inner := f["reassembled_apdu"].(map[string]any)
			require.Equal(t, "GetRequestNormal", inner["kind"])
			clear(f)
			clear(events[1].Session)
			clear(events[1].Raw)
			clear(first)
			clear(last)
			f, e = events[1].GetFields()
			require.NoError(t, e)
			require.Equal(t, "GetRequestNormal", f["reassembled_apdu"].(map[string]any)["kind"])
			closeEvents := s.Close("test")
			require.NotEmpty(t, closeEvents)
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
func TestDLMSGBTWireRefusalAndBudget(t *testing.T) {
	cases := []struct {
		name   string
		wire   []byte
		budget ParserBudget
		want   ProtocolErrorKind
	}{
		{"count-truncated", append(gbtTestWire(1, 16, 129, 1, 0, nil)[:14], 0x82), DefaultParserBudget(), ErrMalformedMessage},
		{"data-short", func() []byte { w := gbtTestWire(1, 16, 129, 1, 0, []byte{0xc0}); w[14] = 2; return w }(), DefaultParserBudget(), ErrMalformedMessage},
		{"trailing", func() []byte { w := gbtTestWire(1, 16, 129, 1, 0, []byte{0xc0}); w[14] = 0; return w }(), DefaultParserBudget(), ErrMalformedMessage},
		{"unselected-inner", gbtTestWire(1, 16, 129, 1, 0, []byte{0xdb, 1, 2}), DefaultParserBudget(), ErrUnsupportedFeature},
		{"depth", gbtTestWire(1, 16, 1, 1, 0, []byte{0xc0}), func() ParserBudget { b := DefaultParserBudget(); b.MaxRecursionDepth = 3; return b }(), ErrResourceExceeded},
		{"bytes", gbtTestWire(1, 16, 1, 1, 0, []byte{0xc0}), func() ParserBudget {
			b := DefaultParserBudget()
			b.MaxMessageBytes = 1024
			b.MaxBufferedBytes = 6000
			return b
		}(), ErrResourceExceeded},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, e := NewProtocolSessionWithOptions(c.budget, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, e)
			defer s.Close("test")
			r := s.Feed(0, time.Unix(1, 0), c.wire)
			require.NotNil(t, r.Err)
			require.Equal(t, c.want, r.Err.Kind)
			require.Len(t, r.Events, 1)
			require.Zero(t, r.Events[0].TransactionID)
			require.Zero(t, r.Events[0].ResponseTo)
			require.Nil(t, r.Events[0].Session)
			f, e := r.Events[0].GetFields()
			require.Nil(t, f)
			rocTypedError(t, string(c.want), e)
			s.Close("test")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDLMSGBTLogicalAddressIsolationAndIdleRetirement(t *testing.T) {
	req := []byte{0xc0, 1, 0xc1, 0, 1, 0, 0, 1, 0, 0, 0xff, 2, 0}
	for _, idle := range []bool{false, true} {
		t.Run(fmt.Sprint(idle), func(t *testing.T) {
			s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, e)
			defer s.Close("test")
			first := gbtTestWire(1, 16, 0x42, 1, 0, req[:6])
			a := s.Feed(0, time.Unix(1, 0), first)
			require.Nil(t, a.Err)
			require.Len(t, a.Events, 1)
			clear(first)
			f, e := a.Events[0].GetFields()
			require.NoError(t, e)
			clear(f)
			clear(a.Events[0].Session)
			clear(a.Events[0].Raw)
			if !idle {
				alien := s.Feed(0, time.Unix(2, 0), gbtTestWire(2, 16, 0x42, 1, 0, []byte{0xc0, 1, 0xc2, 0, 1, 0}))
				require.Nil(t, alien.Err)
				require.Len(t, alien.Events, 1)
			}
			ts := time.Unix(3, 0)
			if idle {
				ts = time.Unix(32, 0)
			}
			b := s.Feed(0, ts, gbtTestWire(1, 16, 0x82, 2, 0, req[6:]))
			require.Nil(t, b.Err)
			require.Len(t, b.Events, 1)
			want := "observed-request"
			if idle {
				want = "ambiguous-conversation"
				require.Zero(t, b.Events[0].TransactionID)
				require.Zero(t, b.Events[0].ResponseTo)
			} else {
				f, e = b.Events[0].GetFields()
				require.NoError(t, e)
				require.Equal(t, byte(1), f["reassembled_apdu"].(map[string]any)["invoke_id"])
			}
			require.Equal(t, want, b.Events[0].Session["Association"])
			s.Close("test")
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func gbtTestControls(t *testing.T) []wrapperBlockControl {
	t.Helper()
	var raw []byte
	var err error
	if root := os.Getenv("DLMS_GBT_TEST_ROOT"); root != "" {
		raw, err = os.ReadFile(filepath.Join(root, "controls.json"))
	} else {
		raw, err = trafficfixture.ReadFile("dlms-gbt/controls.json")
	}
	require.NoError(t, err)
	var m struct{ Cases []wrapperBlockControl }
	require.NoError(t, json.Unmarshal(raw, &m))
	require.Len(t, m.Cases, 24)
	for _, c := range m.Cases {
		require.Equal(t, c.SHA, fmt.Sprintf("%x", sha256.Sum256(wrapperStructuredInput(t, c.Alias))))
	}
	return m.Cases
}
func TestDLMSGBTSealedMatrix(t *testing.T) {
	wrapperBlocksReplayConfigured(t, gbtTestControls(t), func(c wrapperBlockControl) []CaptureOption {
		if strings.HasPrefix(c.Name, "projection-resource-") {
			return []CaptureOption{WithProtocolBudget(1024, 18000)}
		}
		return nil
	})
}
func TestDLMSGBTSealedNativeOwnership(t *testing.T) {
	wrapperBlocksOwnershipConfigured(t, gbtTestControls(t), func(c wrapperBlockControl, b *ParserBudget) {
		if strings.HasPrefix(c.Name, "projection-resource-") {
			b.MaxMessageBytes = 1024
			b.MaxBufferedBytes = 18000
		}
	})
}

func TestDLMSGBTAmbiguousPeerCannotOwnAcknowledgement(t *testing.T) {
	s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
	require.NoError(t, e)
	defer s.Close("test")
	steps := []struct {
		dir  int
		wire []byte
	}{{0, gbtTestWire(1, 16, 0x42, 1, 0, []byte{0xc0, 1, 0xc1})}, {1, gbtTestWire(16, 1, 0x81, 1, 1, nil)}, {0, gbtTestWire(1, 16, 0x42, 1, 0, []byte{0xc0, 1, 0xc2})}, {1, gbtTestWire(16, 1, 0x81, 2, 1, nil)}}
	for i, x := range steps {
		r := s.Feed(x.dir, time.Unix(1, 0), x.wire)
		require.Nil(t, r.Err)
		require.Len(t, r.Events, 1)
		if i == 3 {
			require.Zero(t, r.Events[0].ResponseTo)
			require.Zero(t, r.Events[0].TransactionID)
			f, e := r.Events[0].GetFields()
			require.NoError(t, e)
			require.NotContains(t, f, "acknowledged_block_event")
			require.Equal(t, "unverified-gbt-acknowledgement", r.Events[0].Session["Association"])
		}
	}
	s.Close("test")
	require.Zero(t, s.Stats().BufferedBytes)
}
func TestDLMSGBTProjectionRefusalDoesNotCommitInvoke(t *testing.T) {
	b := DefaultParserBudget()
	b.MaxMessageBytes = 1024
	b.MaxBufferedBytes = 18000
	s, e := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
	require.NoError(t, e)
	defer s.Close("test")
	req := []byte{0xc0, 1, 0xc1, 0, 1, 0, 0, 1, 0, 0, 0xff, 2, 0}
	a := s.Feed(0, time.Unix(1, 0), gbtTestWire(1, 16, 0x42, 1, 0, req[:6]))
	require.Nil(t, a.Err)
	require.Len(t, a.Events, 1)
	r := s.Feed(0, time.Unix(1, 0), gbtTestWire(1, 16, 0x82, 2, 0, req[6:]))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrResourceExceeded, r.Err.Kind)
	require.Len(t, r.Events, 1)
	require.Nil(t, r.Events[0].Session)
	require.Zero(t, r.Events[0].TransactionID)
	wire := make([]byte, 8)
	binary.BigEndian.PutUint16(wire, 1)
	binary.BigEndian.PutUint16(wire[2:], 16)
	binary.BigEndian.PutUint16(wire[4:], 1)
	binary.BigEndian.PutUint16(wire[6:], 7)
	wire = append(wire, []byte{0xc4, 1, 0xc1, 0, 0x12, 1, 0}...)
	late := s.Feed(1, time.Unix(1, 0), wire)
	require.Nil(t, late.Err)
	require.Len(t, late.Events, 1)
	require.Zero(t, late.Events[0].ResponseTo)
	require.Zero(t, late.Events[0].TransactionID)
	require.Equal(t, "ambiguous-conversation", late.Events[0].Session["Association"])
	require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
	s.Close("test")
	require.Zero(t, s.Stats().BufferedBytes)
}
