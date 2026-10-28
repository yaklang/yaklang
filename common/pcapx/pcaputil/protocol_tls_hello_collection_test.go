package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func tlsHelloCollectionControls(t *testing.T) []tlsAlertCase {
	var x struct {
		Cases []struct {
			Name         string
			Wire         string `json:"input_hex"`
			SHA          string `json:"input_sha256"`
			Direction    int
			At           int64 `json:"timestamp_unix"`
			Offset       uint64
			Error        string
			Completeness string
			Fields       map[string]any `json:"correct_fields"`
			Late         *struct {
				Wire      string `json:"input_hex"`
				Direction int
				At        int64 `json:"timestamp_unix"`
				Error     string
			} `json:"late_after_refusal"`
		}
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "tls-hello-collection", "TLS_HELLO_COLLECTION_TEST_ROOT", "controls.json"), &x))
	require.Len(t, x.Cases, 2)
	out := make([]tlsAlertCase, 0, len(x.Cases))
	for _, v := range x.Cases {
		c := tlsAlertCase{Name: v.Name, Ports: [2]uint16{40000, 443}}
		c.Budget.Collection = 1
		c.Steps = []tlsAlertStep{{Wire: v.Wire, SHA: v.SHA, Direction: v.Direction, At: v.At, Offset: v.Offset, Fields: v.Fields, Error: v.Error, Completeness: v.Completeness}}
		n := uint64(len(wrapperWire(t, v.Wire)))
		c.Statistics = map[string]uint64{"Messages": 1, "InputBytes": n, "MessageBytes": n, "UnclassifiedBytes": 0, "LimitedBytes": 0, "Incomplete": 0, "Malformed": 0}
		if v.Late != nil {
			c.Probes = []tlsAlertProbe{{Wire: v.Late.Wire, Direction: v.Late.Direction, At: v.Late.At, Error: v.Late.Error}}
			n = uint64(len(wrapperWire(t, v.Late.Wire)))
			c.Statistics["InputBytes"] += n
			c.Statistics["UnclassifiedBytes"] = n
			c.Statistics["LimitedBytes"] = n
		}
		out = append(out, c)
	}
	return out
}
func TestTLSHelloCollectionNative(t *testing.T) {
	tlsRecordBudgetNative(t, tlsHelloCollectionControls(t))
}
func TestTLSHelloCollectionMatrix(t *testing.T) {
	tlsRecordBudgetMatrix(t, tlsHelloCollectionControls(t), "tls-hello-collection", "TLS_HELLO_COLLECTION")
}

// Both malformed vectors exceed cap1 but must retain structural error priority.
// They derive from the independent two-extension wire, without IUT-generated answers.
func TestTLSHelloCollectionFraming(t *testing.T) {
	c := tlsHelloCollectionControls(t)[0]
	for _, kind := range []string{"duplicate-type", "truncated-second-value"} {
		v := c
		v.Name = kind
		v.Steps = append([]tlsAlertStep(nil), c.Steps...)
		w := wrapperWire(t, c.Steps[0].Wire)
		// Final ALPN extension is type(2), length(2), then the 5-byte list.
		at := len(w) - 9
		if kind == "duplicate-type" {
			w[at], w[at+1] = 0, 0
		} else {
			w[at+2], w[at+3] = 0, 6
		}
		v.Steps[0].Wire = fmt.Sprintf("%x", w)
		v.Steps[0].SHA = fmt.Sprintf("%x", sha256.Sum256(w))
		v.Steps[0].Error = "MalformedMessage"
		v.Steps[0].Completeness = "malformed"
		v.Probes = []tlsAlertProbe{{Wire: v.Steps[0].Wire, Direction: 0, At: 101, Error: "FatalSessionError"}}
		// The discarded late record is unclassified; it is not a Resource refusal.
		n := uint64(len(w))
		v.Statistics = map[string]uint64{"Messages": 1, "InputBytes": n * 2, "MessageBytes": n, "UnclassifiedBytes": n, "LimitedBytes": 0, "Incomplete": 0, "Malformed": 1}
		tlsRecordBudgetNative(t, []tlsAlertCase{v})
	}
}

// A legitimate wire ClientHello can exceed the observer policy. Its refusal must
// own the entire unprocessed datagram and must not poison an independent CID.
func TestTLSHelloCollectionQUICNestedBudget(t *testing.T) {
	b := DefaultParserBudget()
	b.MaxCollectionElements = 2
	s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 14443))
	require.NoError(t, err)
	defer s.Close("cleanup")
	cli := rfc9001ClientInitial()
	wire := bytes.Repeat(cli, 4)
	input := bytes.Clone(wire)
	o := s.Feed(0, time.Unix(100, 0), input)
	clear(input)
	rocTypedError(t, "ResourceExceeded", o.Err)
	require.False(t, o.NeedMore)
	require.Equal(t, len(wire), o.Consumed)
	require.Len(t, o.Events, 1)
	e := o.Events[0]
	require.Equal(t, "quic", e.Protocol)
	require.Equal(t, "quic-v1-native", e.Profile)
	require.Equal(t, "udp", e.Transport)
	require.Equal(t, wire, e.Raw)
	require.Equal(t, len(wire), e.Length)
	require.NotZero(t, e.ID)
	require.Zero(t, e.FlowID)
	require.Zero(t, e.TransactionID)
	require.Zero(t, e.ResponseTo)
	require.Zero(t, e.Direction)
	require.Zero(t, e.Offset)
	require.Equal(t, time.Unix(100, 0), e.Timestamp)
	require.Nil(t, e.Session)
	require.Equal(t, "limited", e.Status)
	tlsCertificateBudgetQuery(t, e, nil, "ResourceExceeded")
	for _, alias := range []string{"radius", "tls", "redis", "snmp", "caller"} {
		tlsDTLSPoison(e, alias)
		tlsCertificateBudgetQuery(t, e, nil, "ResourceExceeded")
	}
	fresh, want, fields := quicCoalescingBudgetInitial(t)
	input = bytes.Clone(fresh)
	n := s.Feed(0, time.Unix(101, 0), input)
	clear(input)
	require.Nil(t, n.Err)
	require.Len(t, n.Events, 1)
	good := n.Events[0]
	require.NotZero(t, good.FlowID)
	require.NotEqual(t, e.ID, good.ID)
	require.Equal(t, fresh, good.Raw)
	require.Equal(t, len(fresh), good.Length)
	require.Zero(t, good.TransactionID)
	require.Zero(t, good.ResponseTo)
	require.Zero(t, good.Direction)
	require.Equal(t, time.Unix(101, 0), good.Timestamp)
	rocEqualFields(t, want, good.Session)
	got, err := good.GetFields()
	require.NoError(t, err)
	rocEqualFields(t, fields, got)
	require.Empty(t, s.Close("done"))
	require.Empty(t, s.Close("again"))
	tlsAlertCheckStats(t, s.Stats(), map[string]uint64{"Messages": 2, "InputBytes": 6000, "MessageBytes": 6000, "LimitedBytes": 4800, "UnclassifiedBytes": 0, "Incomplete": 0, "Malformed": 0})
}

func TestTLSHelloCollectionQUICOwnedNormal(t *testing.T) {
	for _, d := range []bool{false, true} {
		t.Run(fmt.Sprintf("d%t", d), func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxCollectionElements = 2
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 14443))
			require.NoError(t, err)
			defer s.Close("cleanup")
			s.(*captureSession).f.a.config.Deferred = d
			wire, session, fields := quicCoalescingBudgetInitial(t)
			input := bytes.Clone(wire)
			o := s.Feed(0, time.Unix(100, 0), input)
			clear(input)
			require.Nil(t, o.Err)
			require.Len(t, o.Events, 1)
			e := o.Events[0]
			require.Equal(t, wire, e.Raw)
			require.Equal(t, len(wire), e.Length)
			require.NotZero(t, e.ID)
			require.NotZero(t, e.FlowID)
			require.Zero(t, e.TransactionID)
			require.Zero(t, e.ResponseTo)
			require.Equal(t, "quic", e.Protocol)
			rocEqualFields(t, session, e.Session)
			got, err := e.GetFields()
			require.NoError(t, err)
			rocEqualFields(t, fields, got)
			decoded, err := e.Decode()
			require.NoError(t, err)
			rocEqualFields(t, session, decoded["session"].(map[string]any))
			mutateRTPSQuery(got)
			mutateRTPSQuery(decoded)
			for _, alias := range []string{"radius", "tls", "redis", "snmp", "caller"} {
				tlsDTLSPoison(e, alias)
				e.TransactionID, e.ResponseTo = 999999, 999999
				got, err = e.GetFields()
				require.NoError(t, err)
				rocEqualFields(t, fields, got)
				decoded, err = e.Decode()
				require.NoError(t, err)
				require.Len(t, decoded, 3)
				metadata, present := decoded["metadata"]
				require.True(t, present)
				require.Nil(t, metadata)
				rocEqualFields(t, fields, decoded["fields"].(map[string]any))
				rocEqualFields(t, session, decoded["session"].(map[string]any))
				mutateRTPSQuery(got)
				mutateRTPSQuery(decoded)
			}
			require.Empty(t, s.Close("done"))
			require.Empty(t, s.Close("again"))
			tlsAlertCheckStats(t, s.Stats(), map[string]uint64{"Messages": 1, "InputBytes": 1200, "MessageBytes": 1200, "LimitedBytes": 0, "UnclassifiedBytes": 0, "Incomplete": 0, "Malformed": 0})
		})
	}
}
