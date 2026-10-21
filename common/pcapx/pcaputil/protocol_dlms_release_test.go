package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func releaseWrapperControls(t *testing.T) []wrapperBlockControl {
	t.Helper()
	var b []byte
	var e error
	if root := os.Getenv("DLMS_RELEASE_TEST_ROOT"); root != "" {
		b, e = os.ReadFile(filepath.Join(root, "dlms-release-wrapper/controls.json"))
	} else {
		b, e = trafficfixture.ReadFile("dlms-release-wrapper/controls.json")
	}
	require.NoError(t, e)
	var m struct{ Cases []wrapperBlockControl }
	require.NoError(t, json.Unmarshal(b, &m))
	require.Len(t, m.Cases, 36)
	if os.Getenv("DLMS_RELEASE_TEST_ROOT") == "" {
		all, err := trafficfixture.AllExpectations()
		require.NoError(t, err)
		for _, c := range m.Cases {
			bound := 0
			for _, batch := range all {
				for _, v := range batch.Cases {
					if v.ID != "dlms-release-wrapper/"+c.Name {
						continue
					}
					require.Equal(t, c.SHA, v.Input.SHA256)
					require.Equal(t, c.Alias, v.Input.OriginalPath)
					for _, expect := range v.Expectations {
						var x struct {
							File string `json:"answer_file"`
							SHA  string `json:"answer_sha256"`
						}
						require.NoError(t, json.Unmarshal(expect.PayloadConstraints, &x))
						if x.File == "controls.json" {
							require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(b)), x.SHA)
							bound++
						}
					}
				}
			}
			require.Equal(t, 1, bound, c.Name)
		}
	}
	return m.Cases
}
func releaseHDLCControls(t *testing.T) []dlmsListControl {
	t.Helper()
	if root := os.Getenv("DLMS_RELEASE_TEST_ROOT"); root != "" {
		b, e := os.ReadFile(filepath.Join(root, "dlms-release-hdlc/controls.json"))
		require.NoError(t, e)
		var m struct {
			Schema string
			Cases  []dlmsListControl
		}
		require.NoError(t, json.Unmarshal(b, &m))
		require.Equal(t, "owned-dlms-release-hdlc/v1", m.Schema)
		require.Len(t, m.Cases, 36)
		return m.Cases
	}
	return dlmsNamedBlockControls(t, "dlms-release-hdlc", "owned-dlms-release-hdlc/v1", 36)
}
func TestDLMSReleaseWrapperSealedMatrix(t *testing.T) {
	wrapperBlocksReplay(t, releaseWrapperControls(t))
}
func TestDLMSReleaseWrapperSealedOwnership(t *testing.T) {
	wrapperBlocksOwnership(t, releaseWrapperControls(t))
}
func TestDLMSReleaseHDLCSealedMatrix(t *testing.T) { dlmsSealedMatrix(t, releaseHDLCControls(t)) }
func TestDLMSReleaseHDLCSealedOwnership(t *testing.T) {
	dlmsOwnershipAndChunks(t, releaseHDLCControls(t))
}

func TestDLMSReleaseRetiresGetAndIsolatesEndpoints(t *testing.T) {
	base := wrapperFind(t, "wrapper-udp-complete-exchange")
	require.Len(t, base.CompleteFrames, 2)
	q, r := wrapperWire(t, base.CompleteFrames[0]), wrapperWire(t, base.CompleteFrames[1])
	src, dst := binary.BigEndian.Uint16(q[2:4]), binary.BigEndian.Uint16(q[4:6])
	release := acseTestWire(t, src, dst, "6203800100")
	for _, other := range []bool{false, true} {
		t.Run(fmt.Sprintf("independent-%t", other), func(t *testing.T) {
			s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, e)
			defer s.Close("test")
			input, reply := bytes.Clone(q), bytes.Clone(r)
			wantQ, wantR := cloneSession(base.Fields[0]), cloneSession(base.Fields[1])
			if other {
				binary.BigEndian.PutUint16(input[2:4], src+1)
				binary.BigEndian.PutUint16(reply[4:6], src+1)
				wantQ["source_wport"], wantR["destination_wport"] = uint16(src+1), uint16(src+1)
				wantQ["wrapper_header_hex"], wantR["wrapper_header_hex"] = fmt.Sprintf("%x", input[:8]), fmt.Sprintf("%x", reply[:8])
			}
			first := s.Feed(0, time.Unix(1, 0), input)
			require.Len(t, first.Events, 1)
			f, e := first.Events[0].GetFields()
			require.NoError(t, e)
			rocEqualFields(t, wantQ, f)
			closed := s.Feed(0, time.Unix(2, 0), bytes.Clone(release))
			require.Len(t, closed.Events, 1)
			_, e = closed.Events[0].GetFields()
			require.NoError(t, e)
			late := s.Feed(1, time.Unix(3, 0), reply)
			require.Len(t, late.Events, 1)
			f, e = late.Events[0].GetFields()
			require.NoError(t, e)
			rocEqualFields(t, wantR, f)
			if other {
				require.Equal(t, first.Events[0].ID, late.Events[0].ResponseTo)
			} else {
				require.Zero(t, late.Events[0].ResponseTo)
				require.Zero(t, late.Events[0].TransactionID)
				require.Equal(t, "retired-association-context", late.Events[0].Session["Association"])
			}
			ack := s.Feed(1, time.Unix(4, 0), acseTestWire(t, dst, src, "6303800100"))
			require.Len(t, ack.Events, 1)
			require.Equal(t, closed.Events[0].ID, ack.Events[0].ResponseTo)
			require.Empty(t, s.Close("done"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDLMSReleasePairsAndWireBoundaries(t *testing.T) {
	for _, carrier := range []string{"wrapper", "hdlc"} {
		t.Run(carrier, func(t *testing.T) {
			s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, e)
			defer s.Close("test")
			q, r := "00010010000100056203800100", "00010001001000056303800100"
			if carrier == "hdlc" {
				q = "7ea011032110a73fe6e6006203800100bd9b7e"
				r = "7ea011210330a5b8e6e70063038001002c0f7e"
			}
			out := s.Feed(0, time.Unix(1, 0), wrapperWire(t, q))
			require.Len(t, out.Events, 1)
			f, e := out.Events[0].GetFields()
			require.NoError(t, e)
			if carrier == "hdlc" {
				f = f["ACSE"].(map[string]any)
			}
			require.Equal(t, "RLRQ", f["kind"])
			require.Equal(t, byte(0), f["reason"])
			require.Equal(t, true, f["reason_present"])
			require.Equal(t, true, f["eligibility_retired"])
			resp := s.Feed(1, time.Unix(2, 0), wrapperWire(t, r))
			require.Len(t, resp.Events, 1)
			f, e = resp.Events[0].GetFields()
			require.NoError(t, e)
			require.Equal(t, out.Events[0].ID, resp.Events[0].ResponseTo)
			if carrier == "hdlc" {
				require.Zero(t, resp.Events[0].TransactionID)
				f = f["ACSE"].(map[string]any)
			} else {
				require.Equal(t, out.Events[0].ID, resp.Events[0].TransactionID)
			}
			require.Equal(t, "RLRE", f["kind"])
			require.Equal(t, true, f["release_observed"])
			late := s.Feed(1, time.Unix(3, 0), wrapperWire(t, r))
			require.Len(t, late.Events, 1)
			require.Zero(t, late.Events[0].ResponseTo)
			require.Zero(t, late.Events[0].TransactionID)
			require.Empty(t, s.Close("done"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	for _, p := range [][]byte{{0x62, 0}, {0x63, 0}} {
		m, e := decodeACSEAPDU(p, 4096)
		require.NoError(t, e)
		require.False(t, m.fields["reason_present"].(bool))
		require.NotContains(t, m.fields, "reason")
	}
	bad := wrapperWire(t, "630e800100be0f040e0800065f1f040000001004000007")
	_, e := decodeACSEAPDU(bad, 4096)
	rocTypedError(t, "MalformedMessage", e)
	for _, p := range [][]byte{{0x62, 3, 0x80, 1, 2}, {0x64, 3, 0x80, 1, 2}} {
		_, e := decodeACSEAPDU(p, 4096)
		rocTypedError(t, "UnsupportedFeature", e)
	}
	for _, p := range [][]byte{{0x64, 0}, {0x62, 6, 0x80, 1, 0, 0x80, 1, 0}, {0x62, 4, 0x80, 1, 0}} {
		_, e := decodeACSEAPDU(p, 4096)
		rocTypedError(t, "MalformedMessage", e)
	}
	for _, p := range [][]byte{{0x62, 3, 0x80, 1, 0}, {0x63, 3, 0x80, 1, 0}, {0x64, 3, 0x80, 1, 0}} {
		require.True(t, acseLifecycleEnvelopeHint(p))
		trailing := append(bytes.Clone(p), 0)
		require.False(t, acseLifecycleEnvelopeHint(trailing))
	}
}

func TestDLMSReleaseExistingAPIBaseline(t *testing.T) {
	for _, carrier := range []string{"wrapper", "hdlc"} {
		t.Run(carrier, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("baseline")
			wire := "00010010000100056203800100"
			if carrier == "hdlc" {
				wire = "7ea011032110a73fe6e6006203800100bd9b7e"
			}
			q := s.Feed(0, time.Unix(1, 0), wrapperWire(t, wire))
			require.Nil(t, q.Err, "selected fixed-reference release must be observed")
			require.Len(t, q.Events, 1)
			f, err := q.Events[0].GetFields()
			require.NoError(t, err)
			if carrier == "hdlc" {
				f = f["ACSE"].(map[string]any)
			}
			require.Equal(t, "RLRQ", f["kind"])
		})
	}
}

func TestDLMSReleaseCloseBudgetAndIdle(t *testing.T) {
	for _, carrier := range []string{"wrapper", "hdlc"} {
		for _, transport := range []string{"tcp", "udp"} {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("close/%s/%s/%t", carrier, transport, deferred), func(t *testing.T) {
					q := "00010010000100056203800100"
					profile := "dlms-wrapper-v1-acse-lifecycle"
					if carrier == "hdlc" {
						q = "7ea011032110a73fe6e6006203800100bd9b7e"
						profile = "dlms-hdlc-acse-lifecycle"
					}
					s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(40000, 4059))
					require.NoError(t, e)
					s.(*captureSession).f.a.config.Deferred = deferred
					out := s.Feed(0, time.Unix(1, 0), wrapperWire(t, q))
					require.Len(t, out.Events, 1)
					_, e = out.Events[0].GetFields()
					require.NoError(t, e)
					closed := s.Close("unfinished")
					require.Len(t, closed, 1)
					c := closed[0]
					require.Equal(t, profile, c.Profile)
					require.Equal(t, transport, c.Transport)
					require.Equal(t, out.Events[0].Source, c.Source)
					require.Equal(t, out.Events[0].Destination, c.Destination)
					require.Equal(t, out.Events[0].FlowID, c.FlowID)
					require.Equal(t, "incomplete", c.Status)
					require.Equal(t, "incomplete", c.Completeness)
					require.Zero(t, c.ResponseTo)
					require.Zero(t, c.TransactionID)
					f, e := c.GetFields()
					rocTypedError(t, "NeedMore", e)
					require.Nil(t, f)
					want := map[string]any{"Outstanding": float64(1)}
					if carrier == "wrapper" {
						want["source_wport"], want["destination_wport"] = float64(16), float64(1)
					}
					rocEqualFields(t, want, c.Session)
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
	for _, carrier := range []string{"wrapper", "hdlc"} {
		t.Run("refusal/"+carrier, func(t *testing.T) {
			q, r := "00010010000100056203800100", "00010001001000056303800100"
			if carrier == "hdlc" {
				q = "7ea011032110a73fe6e6006203800100bd9b7e"
				r = "7ea011210330a5b8e6e70063038001002c0f7e"
			}
			b := DefaultParserBudget()
			b.MaxMessageBytes, b.MaxFrameBytes, b.MaxBufferedBytes = 2049, 2049, 8192
			s, e := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, e)
			defer s.Close("test")
			first := s.Feed(0, time.Unix(1, 0), wrapperWire(t, q))
			require.Len(t, first.Events, 1)
			f, e := first.Events[0].GetFields()
			rocTypedError(t, "ResourceExceeded", e)
			require.Nil(t, f)
			require.Nil(t, first.Events[0].Session)
			require.Zero(t, first.Events[0].ResponseTo)
			require.Zero(t, first.Events[0].TransactionID)
			s.(*captureSession).f.a.budget.MaxBufferedBytes = 1 << 20
			fresh := s.Feed(0, time.Unix(2, 0), wrapperWire(t, q))
			require.Len(t, fresh.Events, 1)
			require.Zero(t, fresh.Events[0].TransactionID)
			late := s.Feed(1, time.Unix(3, 0), wrapperWire(t, r))
			require.Len(t, late.Events, 1)
			require.Zero(t, late.Events[0].ResponseTo)
			require.Zero(t, late.Events[0].TransactionID)
			require.Empty(t, s.Close("refusal"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
		t.Run("idle/"+carrier, func(t *testing.T) {
			q, r := "00010010000100056203800100", "00010001001000056303800100"
			if carrier == "hdlc" {
				q = "7ea011032110a73fe6e6006203800100bd9b7e"
				r = "7ea011210330a5b8e6e70063038001002c0f7e"
			}
			s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, e)
			out := s.Feed(0, time.Unix(1, 0), wrapperWire(t, q))
			require.Len(t, out.Events, 1)
			late := s.Feed(1, time.Unix(32, 0), wrapperWire(t, r))
			require.Len(t, late.Events, 1)
			require.Zero(t, late.Events[0].ResponseTo)
			require.Zero(t, late.Events[0].TransactionID)
			require.Empty(t, s.Close("idle"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDLMSReleaseHDLCFreshAssociationAndLateReply(t *testing.T) {
	s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
	require.NoError(t, e)
	defer s.Close("test")
	wires := []struct {
		dir int
		hex string
	}{{0, "7ea011032110a73fe6e6006203800100bd9b7e"}, {1, "7ea011210330a5b8e6e70063038001002c0f7e"}, {0, "7ea02b032132ebade6e600601da109060760857405080101be10040e01000000065f1f0400000010040081e17e"}}
	var events []*ProtocolEvent
	for _, w := range wires {
		out := s.Feed(w.dir, time.Unix(1, 0), wrapperWire(t, w.hex))
		require.Len(t, out.Events, 1)
		_, e = out.Events[0].GetFields()
		require.NoError(t, e)
		events = append(events, out.Events...)
	}
	late := s.Feed(1, time.Unix(2, 0), wrapperWire(t, wires[1].hex))
	require.Len(t, late.Events, 1)
	f, e := late.Events[0].GetFields()
	rocTypedError(t, "ContextRequired", e)
	require.Nil(t, f)
	require.Zero(t, late.Events[0].ResponseTo)
	aa := s.Feed(1, time.Unix(3, 0), wrapperWire(t, "7ea037210352783ce6e7006129a109060760857405080101a203020100a305a103020100be10040e0800065f1f04000000100400000786a17e"))
	require.Len(t, aa.Events, 1)
	_, e = aa.Events[0].GetFields()
	require.NoError(t, e)
	require.Equal(t, events[2].ID, aa.Events[0].ResponseTo)
	get := s.Feed(0, time.Unix(4, 0), wrapperWire(t, "7ea0190321545fdee6e600c001c1000f0000280000ff020091537e"))
	require.Len(t, get.Events, 1)
	_, e = get.Events[0].GetFields()
	require.NoError(t, e)
	r := s.Feed(1, time.Unix(5, 0), wrapperWire(t, "7ea0122103744899e6e700c401c100112a43c37e"))
	require.Len(t, r.Events, 1)
	f, e = r.Events[0].GetFields()
	require.NoError(t, e)
	require.EqualValues(t, 42, f["Data Value"])
	require.Equal(t, get.Events[0].ID, r.Events[0].ResponseTo)
	require.Empty(t, s.Close("done"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestDLMSReleaseHDLCBlocksAndFragmentsRemainRetired(t *testing.T) {
	for _, wire := range []string{"7ea0190321326fd8e6e600c001c1000f0000280000ff020091537e", "7ea013032132c104e6e600c002c10000000151be7e", "7ea8190321324f82e6e600c001c1000f0000280000ff020091537e"} {
		t.Run(wire, func(t *testing.T) {
			s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, e)
			defer s.Close("test")
			q := s.Feed(0, time.Unix(1, 0), wrapperWire(t, "7ea011032110a73fe6e6006203800100bd9b7e"))
			require.Len(t, q.Events, 1)
			r := s.Feed(1, time.Unix(2, 0), wrapperWire(t, "7ea011210330a5b8e6e70063038001002c0f7e"))
			require.Len(t, r.Events, 1)
			require.Equal(t, q.Events[0].ID, r.Events[0].ResponseTo)
			out := s.Feed(0, time.Unix(3, 0), wrapperWire(t, wire))
			require.Len(t, out.Events, 1)
			f, e := out.Events[0].GetFields()
			rocTypedError(t, "ContextRequired", e)
			require.Nil(t, f)
			require.Nil(t, out.Events[0].Session)
			require.Zero(t, out.Events[0].ResponseTo)
			require.Zero(t, out.Events[0].TransactionID)
			require.Empty(t, s.Close("done"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
