package pcaputil

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type doipDiscoveryControl struct {
	Name, File, SHA256 string
	Packets            int
	Steps              []struct {
		Dir    int
		Hex    string
		Source string `json:"src"`
		Dest   string `json:"dst"`
	}
	Messages []struct {
		Protocol, Transport string
		Raw                 string `json:"raw_hex"`
		Refs                []int  `json:"packet_refs"`
		Fields              map[string]any
	} `json:"expected_messages"`
	Error  *string `json:"expected_validation_error"`
	Budget *struct {
		Frame   int    `json:"MaxFrameBytes"`
		Decoded int    `json:"expected_decoded"`
		Error   string `json:"expected_error"`
	} `json:"public_resource_target"`
}

func doipDiscoveryControls(t *testing.T) []doipDiscoveryControl {
	t.Helper()
	w, err := trafficfixture.ReadFile("doip-discovery/controls.json")
	require.NoError(t, err)
	var doc struct{ Cases []doipDiscoveryControl }
	var rows struct{ Cases []json.RawMessage }
	require.NoError(t, json.Unmarshal(w, &doc))
	require.NoError(t, json.Unmarshal(w, &rows))
	require.Len(t, doc.Cases, 44)
	inventories, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for i, c := range doc.Cases {
		answerPath := "answers/" + c.Name + ".json"
		a, err := trafficfixture.ReadFile("doip-discovery/" + answerPath)
		require.NoError(t, err)
		require.JSONEq(t, string(rows.Cases[i]), string(a))
		bound := 0
		for _, inventory := range inventories {
			for _, v := range inventory.Cases {
				if v.Input.OriginalPath != "common/pcapx/pcaputil/doip-discovery/"+c.File {
					continue
				}
				require.Equal(t, c.SHA256, v.Input.SHA256)
				for _, e := range v.Expectations {
					var b struct {
						Answer  string `json:"answer_file"`
						SHA     string `json:"answer_sha256"`
						Packets int    `json:"packet_count"`
					}
					require.NoError(t, json.Unmarshal(e.PayloadConstraints, &b))
					if b.Answer != answerPath {
						continue
					}
					bound++
					require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(a)), b.SHA)
					require.Equal(t, c.Packets, b.Packets)
				}
			}
		}
		require.Equal(t, 1, bound, c.Name)
	}
	return doc.Cases
}

func doipDiscoveryWire(t *testing.T, s string) []byte {
	t.Helper()
	w, err := hex.DecodeString(s)
	require.NoError(t, err)
	return w
}

func doipDiscoveryPort(t *testing.T, c doipDiscoveryControl) uint16 {
	t.Helper()
	for _, ep := range []string{c.Steps[0].Source, c.Steps[0].Dest} {
		_, p, err := net.SplitHostPort(ep)
		require.NoError(t, err)
		if p == "13400" {
			return 13400
		}
	}
	_, p, err := net.SplitHostPort(c.Steps[0].Dest)
	require.NoError(t, err)
	if c.Steps[0].Dir == 1 {
		_, p, err = net.SplitHostPort(c.Steps[0].Source)
		require.NoError(t, err)
	}
	n, err := strconv.ParseUint(p, 10, 16)
	require.NoError(t, err)
	return uint16(n)
}

func doipDiscoveryAnswerAt(c doipDiscoveryControl, packet int) int {
	for i, m := range c.Messages {
		if len(m.Refs) == 1 && m.Refs[0] == packet {
			return i
		}
	}
	return -1
}

func TestDoIPDiscoverySealedCompleteByteOracle(t *testing.T) {
	for _, c := range doipDiscoveryControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			accepted := 0
			for i, st := range c.Steps {
				f, err := decodeDoIPDiscovery(doipDiscoveryWire(t, st.Hex), 1<<20, 4096)
				at := doipDiscoveryAnswerAt(c, i+1)
				if at < 0 {
					require.NotNil(t, c.Error)
					rocTypedError(t, *c.Error, err)
					require.Nil(t, f)
				} else {
					require.NoError(t, err)
					rocEqualFields(t, c.Messages[at].Fields, f)
					accepted++
				}
			}
			require.Equal(t, len(c.Messages), accepted)
		})
	}
}

func TestDoIPDiscoverySealedDatagramMatrix(t *testing.T) {
	for _, c := range doipDiscoveryControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			capture, err := trafficfixture.ReadFile("doip-discovery/" + c.File)
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(capture)))
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				events, stats := discoveryReplay(t, capture, workers, deferred, observe, WithProtocolDecodeAs("udp", doipDiscoveryPort(t, c), "doip"))
				require.Len(t, events, len(c.Steps))
				require.EqualValues(t, len(c.Steps), stats.Messages)
				accepted := 0
				for i, e := range events {
					require.Equal(t, "doip", e.Protocol)
					require.Equal(t, doipDiscoveryProfile, e.Profile)
					require.Equal(t, "explicit-decode-as", e.Admission)
					require.Equal(t, "udp", e.Transport)
					require.Equal(t, c.Steps[i].Source, e.Source)
					require.Equal(t, c.Steps[i].Dest, e.Destination)
					require.Equal(t, doipDiscoveryWire(t, c.Steps[i].Hex), e.Raw)
					require.Len(t, e.SourceBytes.PacketRefs, 1)
					require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
					require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
					require.Zero(t, e.TransactionID)
					require.Zero(t, e.ResponseTo)
					if i > 0 {
						require.Greater(t, e.ID, events[i-1].ID)
					}
					f, err := e.GetFields()
					at := doipDiscoveryAnswerAt(c, i+1)
					if at < 0 {
						rocTypedError(t, *c.Error, err)
						require.Nil(t, f)
						require.Empty(t, e.Session)
						continue
					}
					require.NoError(t, err)
					rocEqualFields(t, c.Messages[at].Fields, f)
					require.Equal(t, "unassociated-discovery", e.Session["Association"])
					require.Equal(t, false, e.Session["Authentication Verified"])
					require.Equal(t, false, e.Session["Vehicle Identity Verified"])
					status := "decoded"
					if deferred {
						status = "deferred"
					}
					require.Equal(t, status, e.Status)
					require.Equal(t, "message", e.Completeness)
					// Public raw/fields/session/full projection cannot poison deferred
					// decoding or another returned ownership snapshot.
					f["kind"] = "changed"
					e.Session["Association"] = "changed"
					if e.Fields != nil {
						e.Fields["kind"] = "changed"
					}
					if e.Structured != nil {
						delete(e.Structured, "fields")
					}
					clear(e.Raw)
					f, err = e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, c.Messages[at].Fields, f)
					accepted++
				}
				require.Equal(t, len(c.Messages), accepted)
				if deferred {
					require.EqualValues(t, accepted, stats.Deferred)
				} else {
					require.EqualValues(t, accepted, stats.Decoded)
				}
			})
		})
	}
}

func TestDoIPDiscoveryAdmissionAndBoundary(t *testing.T) {
	for _, c := range doipDiscoveryControls(t) {
		if c.Name != "request-offport-context-required" && c.Name != "announcement-offport" && c.Name != "request-0001-version-ff" && c.Name != "announcement-no-sync" && c.Name != "two-frames-in-one-datagram" {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			p := doipDiscoveryPort(t, c)
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(41423, p))
			require.NoError(t, err)
			w := doipDiscoveryWire(t, c.Steps[0].Hex)
			probe := s.Probe(w)
			shouldAccept := c.Name != "request-offport-context-required" && c.Error == nil
			if shouldAccept {
				require.Equal(t, ProbeAccept, probe.Verdict)
				require.Equal(t, "doip", probe.Protocol)
			} else {
				require.NotEqual(t, ProbeAccept, probe.Verdict)
			}
			o := s.Feed(c.Steps[0].Dir, time.Unix(100, 0), w)
			clear(w)
			if shouldAccept {
				require.Nil(t, o.Err)
				require.Len(t, o.Events, 1)
				f, err := o.Events[0].GetFields()
				require.NoError(t, err)
				rocEqualFields(t, c.Messages[0].Fields, f)
			} else if c.Error != nil {
				require.NotNil(t, o.Err)
				require.Equal(t, ProtocolErrorKind(*c.Error), o.Err.Kind)
			} else {
				for _, e := range o.Events {
					require.NotEqual(t, "doip", e.Protocol)
				}
			}
			require.Empty(t, s.Close("discovery"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
			require.Equal(t, ErrFatalSessionError, s.Feed(0, time.Unix(101, 0), nil).Err.Kind)
		})
	}
}

func TestDoIPDiscoveryResourceAndOwnership(t *testing.T) {
	for _, c := range doipDiscoveryControls(t) {
		if c.Budget == nil {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxFrameBytes = c.Budget.Frame
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(41423, 13400))
			require.NoError(t, err)
			w := doipDiscoveryWire(t, c.Steps[0].Hex)
			o := s.Feed(1, time.Unix(100, 0), w)
			require.Len(t, o.Events, 1)
			if c.Budget.Error != "" {
				require.NotNil(t, o.Err)
				require.Equal(t, ProtocolErrorKind(c.Budget.Error), o.Err.Kind)
				require.Equal(t, "limited", o.Events[0].Status)
				_, err := o.Events[0].GetFields()
				rocTypedError(t, c.Budget.Error, err)
			} else {
				require.Nil(t, o.Err)
				clear(w)
				f, err := o.Events[0].GetFields()
				require.NoError(t, err)
				rocEqualFields(t, c.Messages[0].Fields, f)
			}
			require.Empty(t, s.Close("budget"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	var positive doipDiscoveryControl
	for _, c := range doipDiscoveryControls(t) {
		if c.Name == "announcement-no-sync" {
			positive = c
		}
	}
	require.NotEmpty(t, positive.Name)
	w := doipDiscoveryWire(t, positive.Steps[0].Hex)
	for _, c := range []struct {
		Name   string
		Budget ParserBudget
		Pass   bool
	}{
		{"projection-exact", ParserBudget{MaxMessageBytes: 64, MaxFrameBytes: 64, MaxBufferedBytes: 4096 + 128*len(w)}, true},
		{"projection-under", ParserBudget{MaxMessageBytes: 64, MaxFrameBytes: 64, MaxBufferedBytes: 4096 + 128*len(w) - 1}, false},
		{"field-depth", ParserBudget{MaxRecursionDepth: 1}, false},
		{"field-map-limit", ParserBudget{MaxCollectionElements: 15}, false},
	} {
		t.Run(c.Name, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(c.Budget, WithSessionTransport("udp"), WithSessionPorts(41423, 13400))
			require.NoError(t, err)
			probe := s.Probe(w)
			require.Zero(t, s.Stats().BufferedBytes)
			o := s.Feed(1, time.Unix(100, 0), w)
			require.Len(t, o.Events, 1)
			if c.Pass {
				require.Equal(t, ProbeAccept, probe.Verdict)
				require.Nil(t, o.Err)
				f, err := o.Events[0].GetFields()
				require.NoError(t, err)
				rocEqualFields(t, positive.Messages[0].Fields, f)
			} else {
				require.Equal(t, ProbeReject, probe.Verdict)
				require.NotNil(t, o.Err)
				require.Equal(t, ErrResourceExceeded, o.Err.Kind)
				require.Equal(t, "limited", o.Events[0].Status)
				require.Empty(t, o.Events[0].Session)
				require.Zero(t, o.Events[0].TransactionID)
			}
			require.Zero(t, s.Stats().BufferedBytes)
			require.Empty(t, s.Close("resource"))
		})
	}
}
