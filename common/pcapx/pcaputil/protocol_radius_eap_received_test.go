package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"io"
	"testing"
	"time"
)

func eapIndependentHex(t *testing.T, s string) []byte {
	t.Helper()
	b, e := hex.DecodeString(s)
	require.NoError(t, e)
	return b
}
func eapIndependentFields(t *testing.T, in map[string]any) map[string]any {
	t.Helper()
	out := cloneSession(in)
	for _, k := range []string{"Code", "Identifier", "Packet Length", "Padding Bytes", "Attribute Count", "User-Password Bytes"} {
		if n, ok := out[k].(float64); ok {
			out[k] = int(n)
		}
	}
	for _, k := range []string{"Authenticator", "Message-Authenticator", "Acct-Session-Id"} {
		if s, ok := out[k].(string); ok {
			out[k] = eapIndependentHex(t, s)
		}
	}
	if a, ok := out["Attributes"].([]any); ok {
		rows := make([]map[string]any, 0, len(a))
		for _, v := range a {
			m := v.(map[string]any)
			rows = append(rows, map[string]any{"Type": byte(m["Type"].(float64)), "Raw": eapIndependentHex(t, m["Raw"].(string))})
		}
		out["Attributes"] = rows
	}
	if a, ok := out["EAP Diagnostics"].([]any); ok {
		ss := make([]string, len(a))
		for i, v := range a {
			ss[i] = v.(string)
		}
		out["EAP Diagnostics"] = ss
	}
	if m, ok := out["EAP"].(map[string]any); ok {
		for _, k := range []string{"Code", "Identifier", "Type", "Value Size"} {
			if n, ok := m[k].(float64); ok {
				m[k] = byte(n)
			}
		}
		if n, ok := m["Length"].(float64); ok {
			m["Length"] = uint16(n)
		}
		for _, k := range []string{"Padding Bytes", "Fragments"} {
			if n, ok := m[k].(float64); ok {
				m[k] = int(n)
			}
		}
		for _, k := range []string{"Raw", "Type Data", "Value", "Name", "Prompt", "Identity", "Notification"} {
			if s, ok := m[k].(string); ok {
				m[k] = eapIndependentHex(t, s)
			}
		}
	}
	return out
}

// Parent may adopt this helper using existing corpus ZIP extraction/read helper;
// the historical rawpcap must remain referenced by SHA, notduplicated innewZIP.
func radiusEAPAssertReceived(t *testing.T, events, originals []*ProtocolEvent, stats ProtocolStats, deferred bool) {
	t.Helper()
	control := finsReadBatch(t, "radius-eap", "RADIUS_EAP_TEST_ROOT", "controls.json")
	var data struct {
		Received struct {
			SHA    string `json:"sha256"`
			Frames []struct {
				Wire        string         `json:"input_hex"`
				Fields      map[string]any `json:"fields_hexbytes"`
				Error       string         `json:"error"`
				Direction   int            `json:"direction"`
				Transaction *int           `json:"transaction_step"`
			} `json:"frames"`
		} `json:"received_capture"`
	}
	require.NoError(t, json.Unmarshal(control, &data))
	fact := finsReadBatch(t, "radius-eap", "RADIUS_EAP_TEST_ROOT", "real-carrier.json")
	var facts struct {
		Frames []struct {
			Seconds     int64  `json:"timestamp_unix_seconds"`
			Nanoseconds int64  `json:"timestamp_nanoseconds"`
			Source      string `json:"ipv4_source"`
			Destination string `json:"ipv4_destination"`
			SP          int    `json:"source_port"`
			DP          int    `json:"destination_port"`
		}
	}
	require.NoError(t, json.Unmarshal(fact, &facts))
	require.Len(t, events, 4)
	require.Len(t, originals, 4)
	require.Zero(t, stats.BufferedBytes)
	require.Zero(t, stats.CallbackPanics)
	require.EqualValues(t, 4, stats.Messages)
	require.EqualValues(t, 519, stats.MessageBytes)
	require.EqualValues(t, 2, stats.Malformed)
	if deferred {
		require.EqualValues(t, 2, stats.Deferred)
	} else {
		require.EqualValues(t, 2, stats.Decoded)
	}
	seen := map[uint64]bool{}
	for i, v := range events {
		w := data.Received.Frames[i]
		f := facts.Frames[i]
		require.NotZero(t, v.ID)
		require.False(t, seen[v.ID])
		seen[v.ID] = true
		require.NotZero(t, v.FlowID)
		require.Equal(t, events[0].FlowID, v.FlowID)
		require.Equal(t, "radius", v.Protocol)
		require.Equal(t, "radius-eap-observed", v.Profile)
		require.Equal(t, "udp", v.Transport)
		require.Equal(t, CaptureDomain{}, v.Domain)
		// UDP's canonical endpoint orientation is independent of request role:
		// the textual endpoint 10.0.0.100 sorts before 10.0.0.1:1645.
		expectedDirection := 0
		if fmt.Sprintf("%s:%d", f.Source, f.SP) > fmt.Sprintf("%s:%d", f.Destination, f.DP) {
			expectedDirection = 1
		}
		require.Equal(t, expectedDirection, v.Direction)
		require.True(t, time.Unix(f.Seconds, f.Nanoseconds).Equal(v.Timestamp))
		require.Equal(t, fmt.Sprintf("%s:%d", f.Source, f.SP), v.Source)
		require.Equal(t, fmt.Sprintf("%s:%d", f.Destination, f.DP), v.Destination)
		wire := eapIndependentHex(t, w.Wire)
		require.Equal(t, wire, v.Raw)
		require.Equal(t, len(wire), v.Length)
		require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(wire)), v.SourceBytes.SHA256)
		require.Equal(t, "captured", v.SourceBytes.Kind)
		require.Equal(t, []PacketReference{{Number: uint64(i + 1), Domain: CaptureDomain{}}}, v.SourceBytes.PacketRefs)
		require.Zero(t, v.ResponseTo)
		if w.Transaction == nil {
			require.Zero(t, v.TransactionID)
		} else {
			require.Equal(t, events[*w.Transaction].ID, v.TransactionID)
		}
		want := eapIndependentFields(t, w.Fields)
		require.Equal(t, want, v.Session)
		require.Equal(t, "not-verified", v.Session["Authentication"])
		status := "decoded"
		if deferred {
			status = "deferred"
		}
		complete := "message"
		if w.Error != "" {
			status = "malformed"
			complete = "malformed"
		}
		require.Equal(t, status, v.Status)
		require.Equal(t, complete, v.Completeness)
		o := originals[i]
		fields, err := o.GetFields()
		rocTypedError(t, w.Error, err)
		if w.Error == "" {
			require.Equal(t, want, fields)
			tree, e := o.Decode()
			require.NoError(t, e)
			require.Equal(t, want, tree["fields"])
			mutateRTPSQuery(fields)
			mutateRTPSQuery(tree)
		} else {
			require.Nil(t, fields)
			tree, e := o.Decode()
			require.Nil(t, tree)
			rocTypedError(t, w.Error, e)
		}
		tlsDTLSPoison(o, "http")
		fields, err = o.GetFields()
		rocTypedError(t, w.Error, err)
		if w.Error == "" {
			require.Equal(t, want, fields)
		} else {
			require.Nil(t, fields)
		}
	}
}

func TestRADIUSEAPReceivedCaptureIndependent(t *testing.T) {
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			for _, observe := range []bool{false, true} {
				t.Run(fmt.Sprintf("w%d/d%t/o%t", workers, deferred, observe), func(t *testing.T) {
					raw := binCorpusBytes(t, "tcpdump/tcpdump-radius.pcap")
					require.Len(t, raw, 775)
					require.Equal(t, "9cd173ba614345281e4b66927f3b17b357519540bbc0feedbc15dbc7ba8ad921", fmt.Sprintf("%x", sha256.Sum256(raw)))
					seen := 0
					var events, originals []*ProtocolEvent
					var stats ProtocolStats
					opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnProtocolMessage(func(v *ProtocolEvent) {
						snap := *v
						snap.Raw = bytes.Clone(v.Raw)
						snap.Session = cloneSession(v.Session)
						snap.SourceBytes.PacketRefs = append([]PacketReference(nil), v.SourceBytes.PacketRefs...)
						events = append(events, &snap)
						originals = append(originals, v)
						if observe {
							tlsDTLSPoison(v, "http")
							v.ID, v.FlowID = 0, 0
						}
					})}
					if observe {
						opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
					}
					a, e := NewPacketAnalyzer(opts...)
					require.NoError(t, e)
					r, e := NewCaptureReader(bytes.NewReader(raw))
					require.NoError(t, e)
					for {
						b, ci, e := r.ReadPacketData()
						if e == io.EOF {
							break
						}
						require.NoError(t, e)
						require.Zero(t, ci.InterfaceIndex)
						require.NoError(t, a.Feed(b, ci, r.LinkType()))
						clear(b)
					}
					require.NoError(t, a.Close())
					require.NoError(t, a.Close())

					if observe {
						require.Equal(t, 4, seen)
					}
					radiusEAPAssertReceived(t, events, originals, stats, deferred)
				})
			}
		}
	}
}

// Existing API oldfailure: exact complete firstrecord is resource-refused; a
// differentID nextrecordmust remain usable without arbitrary byte resync.
func TestRADIUSEAPLegacyAttributeRefusalRecovery(t *testing.T) {
	b := DefaultParserBudget()
	b.MaxCollectionElements = 3
	s, e := NewProtocolSession(b)
	require.NoError(t, e)
	defer s.Close("cleanup")
	first := radiusPkt(1, 7, append(append(append(radiusAttr(1, []byte("alice")), radiusAttr(4, []byte{192, 0, 2, 9})...), radiusAttr(79, []byte{2, 0, 0, 10, 1, 'a', 'l', 'i', 'c', 'e'})...), radiusAttr(80, make([]byte, 16))...))
	neighbor := radiusPkt(1, 8, append(radiusAttr(1, []byte("neighbor")), radiusAttr(4, []byte{192, 0, 2, 9})...))
	late := radiusPkt(2, 7, nil)
	answer := radiusPkt(2, 8, nil)
	out := s.Feed(0, time.Unix(100, 0), first)
	require.NotNil(t, out.Err)
	require.Equal(t, ErrResourceExceeded, out.Err.Kind)
	require.Equal(t, len(first), out.Consumed)
	require.Len(t, out.Events, 1)
	v := out.Events[0]
	require.Equal(t, "radius-access-observed", v.Profile)
	require.Equal(t, "limited", v.Status)
	require.Equal(t, "limited", v.Completeness)
	require.Equal(t, first, v.Raw)
	require.Nil(t, v.Session)
	require.Zero(t, v.TransactionID)
	require.Zero(t, v.ResponseTo)
	out = s.Feed(0, time.Unix(101, 0), neighbor)
	require.Nil(t, out.Err, "fullrecord refusal mustnotstop unrelated next Identifier")
	require.Len(t, out.Events, 1)
	q := out.Events[0]
	owner := q.ID
	require.NotZero(t, owner)
	require.Equal(t, owner, q.TransactionID)
	require.Zero(t, q.ResponseTo)
	require.Equal(t, "neighbor", q.Session["User-Name"])
	require.Equal(t, neighbor, q.Raw)
	out = s.Feed(1, time.Unix(102, 0), late)
	require.Nil(t, out.Err)
	require.Len(t, out.Events, 1)
	require.Equal(t, late, out.Events[0].Raw)
	require.Zero(t, out.Events[0].TransactionID)
	require.Zero(t, out.Events[0].ResponseTo)
	require.Equal(t, "ambiguous-identifier", out.Events[0].Session["Association"])
	out = s.Feed(1, time.Unix(103, 0), answer)
	require.Nil(t, out.Err)
	require.Len(t, out.Events, 1)
	require.Equal(t, answer, out.Events[0].Raw)
	require.Equal(t, owner, out.Events[0].TransactionID)
	require.Equal(t, owner, out.Events[0].ResponseTo)
	require.Equal(t, "Access-Request", out.Events[0].Session["In Reply To"])
	require.Empty(t, s.Close("first"))
	require.Empty(t, s.Close("second"))
	require.Zero(t, s.Stats().BufferedBytes)
}
