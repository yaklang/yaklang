package pcaputil

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type knxExtendedControl struct {
	Name, File, SHA256 string
	Packets            int
	Steps              []struct {
		Dir    int
		Hex    string
		Source string `json:"src"`
		Dest   string `json:"dst"`
	}
	Messages []struct {
		Raw    string `json:"raw_hex"`
		Refs   []int  `json:"packet_refs"`
		Fields map[string]any
	} `json:"expected_messages"`
	Error  *string `json:"expected_validation_error"`
	Budget *struct {
		Frame      int    `json:"MaxFrameBytes"`
		Collection int    `json:"MaxCollectionLength"`
		Decoded    int    `json:"decoded_messages"`
		Error      string `json:"error"`
	} `json:"public_resource_target"`
}

func knxExtendedControls(t *testing.T) []knxExtendedControl {
	t.Helper()
	b, err := trafficfixture.ReadFile("knx-extended/controls.json")
	require.NoError(t, err)
	var doc struct{ Cases []knxExtendedControl }
	var rows struct{ Cases []json.RawMessage }
	require.NoError(t, json.Unmarshal(b, &doc))
	require.NoError(t, json.Unmarshal(b, &rows))
	require.Len(t, doc.Cases, 52)
	inv, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for i, c := range doc.Cases {
		path := "answers/" + c.Name + ".json"
		a, err := trafficfixture.ReadFile("knx-extended/" + path)
		require.NoError(t, err)
		require.JSONEq(t, string(rows.Cases[i]), string(a))
		bound := 0
		for _, d := range inv {
			for _, v := range d.Cases {
				if v.Input.OriginalPath != "common/pcapx/pcaputil/knx-extended/"+c.File {
					continue
				}
				require.Equal(t, c.SHA256, v.Input.SHA256)
				for _, e := range v.Expectations {
					var binding struct {
						Answer  string `json:"answer_file"`
						SHA     string `json:"answer_sha256"`
						Packets int    `json:"packet_count"`
					}
					require.NoError(t, json.Unmarshal(e.PayloadConstraints, &binding))
					if binding.Answer != path {
						continue
					}
					bound++
					require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(a)), binding.SHA)
					require.Equal(t, c.Packets, binding.Packets)
				}
			}
		}
		require.Equal(t, 1, bound, c.Name)
	}
	return doc.Cases
}
func knxExtendedAnswer(c knxExtendedControl, packet int) int {
	for i, m := range c.Messages {
		if len(m.Refs) == 1 && m.Refs[0] == packet {
			return i
		}
	}
	return -1
}
func knxExtendedPort(t *testing.T, c knxExtendedControl) uint16 {
	t.Helper()
	for _, e := range []string{c.Steps[0].Source, c.Steps[0].Dest} {
		_, p, err := net.SplitHostPort(e)
		require.NoError(t, err)
		if p == "3671" {
			return 3671
		}
	}
	return 34567
}
func TestKNXExtendedSealedByteOracle(t *testing.T) {
	for _, c := range knxExtendedControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			n := 0
			for i, st := range c.Steps {
				f, err := decodeKNXSearch(doipDiscoveryWire(t, st.Hex), 4096)
				at := knxExtendedAnswer(c, i+1)
				if at < 0 {
					require.NotNil(t, c.Error)
					rocTypedError(t, *c.Error, err)
					require.Nil(t, f)
				} else {
					require.NoError(t, err)
					rocEqualFields(t, c.Messages[at].Fields, f)
					n++
				}
			}
			require.Equal(t, len(c.Messages), n)
		})
	}
}
func TestKNXExtendedSealedDatagramMatrix(t *testing.T) {
	for _, c := range knxExtendedControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			raw, err := trafficfixture.ReadFile("knx-extended/" + c.File)
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			discoveryMatrix(t, func(t *testing.T, w int, d, o bool) {
				events, stats := discoveryReplay(t, raw, w, d, o, WithProtocolDecodeAs("udp", knxExtendedPort(t, c), "knx"))
				require.Len(t, events, len(c.Steps))
				require.EqualValues(t, len(c.Steps), stats.Messages)
				n := 0
				for i, e := range events {
					wire := doipDiscoveryWire(t, c.Steps[i].Hex)
					require.Equal(t, "knx", e.Protocol)
					profile := "knx-basic-search"
					if len(wire) >= 4 && (binary.BigEndian.Uint16(wire[2:4]) == 0x20b || binary.BigEndian.Uint16(wire[2:4]) == 0x20c) {
						profile = "knx-extended-search"
					}
					require.Equal(t, profile, e.Profile)
					require.Equal(t, "explicit-decode-as", e.Admission)
					require.Equal(t, "udp", e.Transport)
					require.Equal(t, c.Steps[i].Source, e.Source)
					require.Equal(t, c.Steps[i].Dest, e.Destination)
					require.Equal(t, wire, e.Raw)
					require.Len(t, e.SourceBytes.PacketRefs, 1)
					require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
					require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
					require.Zero(t, e.FlowID)
					require.Zero(t, e.TransactionID)
					require.Zero(t, e.ResponseTo)
					if i > 0 {
						require.Greater(t, e.ID, events[i-1].ID)
					}
					f, err := e.GetFields()
					at := knxExtendedAnswer(c, i+1)
					if at < 0 {
						rocTypedError(t, *c.Error, err)
						require.Nil(t, f)
						require.Empty(t, e.Session)
						continue
					}
					require.NoError(t, err)
					rocEqualFields(t, c.Messages[at].Fields, f)
					require.Empty(t, e.Error)
					require.Equal(t, "message", e.Completeness)
					status := "decoded"
					if d {
						status = "deferred"
					}
					require.Equal(t, status, e.Status)
					n++
					f["kind"] = "caller change"
					e.Session["kind"] = "caller change"
					if e.Fields != nil {
						e.Fields["kind"] = "caller change"
					}
					if e.Structured != nil {
						delete(e.Structured, "fields")
					}
					clear(e.Raw)
					f, err = e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, c.Messages[at].Fields, f)
				}
				require.Equal(t, len(c.Messages), n)
				if d {
					require.EqualValues(t, n, stats.Deferred)
				} else {
					require.EqualValues(t, n, stats.Decoded)
				}
			})
		})
	}
}
func TestKNXExtendedPublicAdmissionAndClose(t *testing.T) {
	for _, c := range knxExtendedControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			p := knxExtendedPort(t, c)
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41343, p))
			require.NoError(t, err)
			for i, st := range c.Steps {
				w := doipDiscoveryWire(t, st.Hex)
				at := knxExtendedAnswer(c, i+1)
				probe := s.Probe(w)
				if p == 3671 && at >= 0 {
					require.Equal(t, ProbeAccept, probe.Verdict)
					require.Equal(t, "knx", probe.Protocol)
				} else {
					require.NotEqual(t, ProbeAccept, probe.Verdict)
				}
				r := s.Feed(st.Dir, time.Unix(100+int64(i), 0), w)
				clear(w)
				if p != 3671 || len(st.Hex) < 2 || st.Hex[:2] != "06" {
					for _, e := range r.Events {
						require.NotEqual(t, "knx", e.Protocol)
					}
					continue
				}
				require.Len(t, r.Events, 1)
				f, err := r.Events[0].GetFields()
				if at >= 0 {
					require.Nil(t, r.Err)
					require.NoError(t, err)
					rocEqualFields(t, c.Messages[at].Fields, f)
					require.Equal(t, st.Dir, r.Events[0].Direction)
				} else {
					require.NotNil(t, r.Err)
					require.Equal(t, ProtocolErrorKind(*c.Error), r.Err.Kind)
					require.Nil(t, f)
				}
			}
			require.Empty(t, s.Close("observation"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
			require.Equal(t, ErrFatalSessionError, s.Feed(0, time.Unix(110, 0), nil).Err.Kind)
		})
	}
}
func TestKNXExtendedResourceIsolation(t *testing.T) {
	cs := knxExtendedControls(t)
	var positive knxExtendedControl
	for _, c := range cs {
		if c.Name == "request-all-four-types" {
			positive = c
		}
		if c.Budget == nil {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxFrameBytes = c.Budget.Frame
			b.MaxCollectionElements = c.Budget.Collection
			b.MaxMessageBytes = 64
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(41343, 3671))
			require.NoError(t, err)
			w := doipDiscoveryWire(t, c.Steps[0].Hex)
			r := s.Feed(c.Steps[0].Dir, time.Unix(100, 0), w)
			require.Len(t, r.Events, 1)
			if c.Budget.Error != "" {
				require.NotNil(t, r.Err)
				require.Equal(t, ProtocolErrorKind(c.Budget.Error), r.Err.Kind)
				require.Equal(t, "limited", r.Events[0].Status)
				require.Empty(t, r.Events[0].Session)
			} else {
				require.Nil(t, r.Err)
				f, err := r.Events[0].GetFields()
				require.NoError(t, err)
				rocEqualFields(t, c.Messages[0].Fields, f)
			}
			require.EqualValues(t, c.Budget.Decoded, s.Stats().Decoded)
			require.Empty(t, s.Close("budget"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	require.NotEmpty(t, positive.Name)
	wire := doipDiscoveryWire(t, positive.Steps[0].Hex)
	for _, x := range []struct {
		Name   string
		Budget ParserBudget
		Pass   bool
	}{
		{"projection-exact", ParserBudget{MaxMessageBytes: 64, MaxFrameBytes: 64, MaxBufferedBytes: 4096 + 128*len(wire)}, true},
		{"projection-under", ParserBudget{MaxMessageBytes: 64, MaxFrameBytes: 64, MaxBufferedBytes: 4096 + 128*len(wire) - 1}, false},
		{"selector-depth", ParserBudget{MaxRecursionDepth: 3}, false},
		{"selector-map", ParserBudget{MaxCollectionElements: 8}, false},
	} {
		t.Run(x.Name, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(x.Budget, WithSessionTransport("udp"), WithSessionPorts(41343, 3671))
			require.NoError(t, err)
			probe := s.Probe(wire)
			require.Zero(t, s.Stats().BufferedBytes)
			r := s.Feed(0, time.Unix(100, 0), wire)
			require.Len(t, r.Events, 1)
			if x.Pass {
				require.Equal(t, ProbeAccept, probe.Verdict)
				require.Nil(t, r.Err)
				f, err := r.Events[0].GetFields()
				require.NoError(t, err)
				rocEqualFields(t, positive.Messages[0].Fields, f)
			} else {
				require.Equal(t, ProbeReject, probe.Verdict)
				require.Equal(t, ErrResourceExceeded, r.Err.Kind)
				require.Equal(t, "limited", r.Events[0].Status)
				require.Empty(t, r.Events[0].Session)
			}
			require.Empty(t, s.Close("resource"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	// This profile consumes whole datagrams, including both selected service types.
	require.EqualValues(t, 0x20b, binary.BigEndian.Uint16(wire[2:4]))
}
