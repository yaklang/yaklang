package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

type accountingStep struct {
	radiusControlStep
	UDP         map[string]any `json:"expected_udp_session"`
	Profile     string         `json:"expected_profile"`
	Association *string        `json:"association"`
	Outstanding *int           `json:"close_outstanding"`
}
type accountingControl struct {
	Name   string
	Steps  []accountingStep
	Ports  [2]uint16
	Budget struct{ MaxMessageBytes, MaxCollectionElements int }
}

func accountingControls(t *testing.T) []accountingControl {
	t.Helper()
	var x struct{ Cases []accountingControl }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "radius-accounting", "RADIUS_ACCOUNTING_TEST_ROOT", "controls.json"), &x))
	require.Len(t, x.Cases, 49)
	return x.Cases
}
func accountingWant(t *testing.T, c accountingControl, i int) map[string]any {
	t.Helper()
	w := c.Steps[i]
	if w.Error != "" {
		return nil
	}
	s := cloneSession(w.UDP)
	for _, k := range []string{"Code", "Identifier", "Attribute Count", "User-Password Bytes", "Packet Length", "Padding Bytes"} {
		if v, ok := s[k].(float64); ok {
			s[k] = int(v)
		}
	}
	for _, k := range []string{"Acct-Status-Type", "Acct-Delay-Time", "Acct-Input-Octets", "Acct-Output-Octets", "Acct-Authentic", "Acct-Session-Time", "Acct-Input-Packets", "Acct-Output-Packets", "Acct-Terminate-Cause", "Acct-Input-Gigawords", "Acct-Output-Gigawords"} {
		if v, ok := s[k].(float64); ok {
			require.GreaterOrEqual(t, v, float64(0))
			require.LessOrEqual(t, v, float64(0xffffffff))
			s[k] = uint32(v)
		}
	}
	for _, k := range []string{"Acct-Input-Octets64", "Acct-Output-Octets64"} {
		if v, ok := s[k].(string); ok {
			n, e := strconv.ParseUint(v, 10, 64)
			require.NoError(t, e)
			s[k] = n
		}
	}
	s["Authenticator"] = radiusWire(t, s["Authenticator"].(string))
	if v, ok := s["Acct-Session-Id"].(string); ok {
		s["Acct-Session-Id"] = radiusWire(t, v)
	}
	attrs := make([]map[string]any, 0)
	for _, v := range s["Attributes"].([]any) {
		a := v.(map[string]any)
		attrs = append(attrs, map[string]any{"Type": byte(a["Type"].(float64)), "Raw": radiusWire(t, a["Raw"].(string))})
	}
	s["Attributes"] = attrs
	return s
}
func accountingCapture(t *testing.T, c accountingControl) []byte {
	b := radiusControl{Name: c.Name, Ports: c.Ports}
	for _, s := range c.Steps {
		b.Steps = append(b.Steps, s.radiusControlStep)
	}
	return radiusControlCapture(t, b)
}
func accountingCheck(t *testing.T, c accountingControl, events []*ProtocolEvent, captured, deferred bool) {
	t.Helper()
	extra := 0
	for _, s := range c.Steps {
		if s.Outstanding != nil {
			extra = 1
		}
	}
	require.Len(t, events, len(c.Steps)+extra)
	ids := map[uint64]bool{}
	for i, w := range c.Steps {
		e := events[i]
		require.Equal(t, "radius", e.Protocol)
		require.Equal(t, w.Profile, e.Profile)
		require.NotZero(t, e.ID)
		require.False(t, ids[e.ID])
		ids[e.ID] = true
		require.NotZero(t, e.FlowID)
		require.Equal(t, events[0].FlowID, e.FlowID)
		require.Equal(t, w.Direction, e.Direction)
		require.Equal(t, "udp", e.Transport)
		require.Equal(t, CaptureDomain{}, e.Domain)
		require.True(t, time.Unix(w.At, 0).Equal(e.Timestamp))
		src, dst := fmt.Sprintf("192.0.2.1:%d", c.Ports[0]), fmt.Sprintf("192.0.2.2:%d", c.Ports[1])
		if w.Direction == 1 {
			src, dst = dst, src
		}
		require.Equal(t, src, e.Source)
		require.Equal(t, dst, e.Destination)
		status := "decoded"
		if deferred {
			status = "deferred"
		}
		if w.Error == "MalformedMessage" {
			status = "malformed"
		}
		if w.Error == "ResourceExceeded" {
			status = "limited"
		}
		require.Equal(t, status, e.Status)
		raw := radiusWire(t, w.Wire)
		preview := raw
		if c.Budget.MaxMessageBytes > 0 && len(raw) > c.Budget.MaxMessageBytes {
			preview = raw[:c.Budget.MaxMessageBytes]
			require.Equal(t, "limited", e.Completeness)
			require.Equal(t, "ResourceExceeded", e.ExpertCode)
		}
		require.Equal(t, preview, e.Raw)
		require.Equal(t, len(raw), e.Length)
		if w.Reply == nil {
			require.Zero(t, e.ResponseTo)
		} else {
			require.Equal(t, events[*w.Reply].ID, e.ResponseTo)
		}
		if w.Transaction == nil {
			require.Zero(t, e.TransactionID)
		} else {
			require.Equal(t, events[*w.Transaction].ID, e.TransactionID)
		}
		if captured {
			require.Len(t, e.SourceBytes.PacketRefs, 1)
			require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
			require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(preview)), e.SourceBytes.SHA256)
		}
		want := accountingWant(t, c, i)
		require.Equal(t, want, e.Session)
		f, err := e.GetFields()
		rocTypedError(t, w.Error, err)
		require.Equal(t, want, f)
		if w.Error == "" {
			tree, err := e.Decode()
			require.NoError(t, err)
			require.Equal(t, want, tree["session"])
			mutateRTPSQuery(tree)
			mutateRTPSQuery(f)
		} else {
			require.Nil(t, f)
			require.NotEmpty(t, e.Error)
		}
		tlsDTLSPoison(e, "http")
		f, err = e.GetFields()
		rocTypedError(t, w.Error, err)
		require.Equal(t, want, f)
	}
	if extra == 1 {
		e := events[len(c.Steps)]
		require.Equal(t, "radius", e.Protocol)
		require.Equal(t, "radius-accounting-observed", e.Profile)
		require.Equal(t, "incomplete", e.Status)
		require.Zero(t, e.ResponseTo)
		require.Zero(t, e.TransactionID)
		require.Equal(t, map[string]any{"Outstanding": 1}, e.Session)
		f, err := e.GetFields()
		require.Nil(t, f)
		rocTypedError(t, "NeedMore", err)
	}
}
func accountingBudget(c accountingControl) ParserBudget {
	b := DefaultParserBudget()
	if c.Budget.MaxMessageBytes > 0 {
		b.MaxMessageBytes = c.Budget.MaxMessageBytes
	}
	if c.Budget.MaxCollectionElements > 0 {
		b.MaxCollectionElements = c.Budget.MaxCollectionElements
	}
	return b
}
func TestRADIUSAccountingNative(t *testing.T) {
	for _, c := range accountingControls(t) {
		for _, d := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/d%t", c.Name, d), func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(accountingBudget(c), WithSessionTransport("udp"), WithSessionPorts(c.Ports[0], c.Ports[1]))
				require.NoError(t, err)
				defer s.Close("cleanup")
				s.(*captureSession).f.a.config.Deferred = d
				var events []*ProtocolEvent
				total, limited := 0, 0
				for _, w := range c.Steps {
					raw := radiusWire(t, w.Wire)
					n := len(raw)
					total += n
					if w.Error == "ResourceExceeded" {
						limited += n
					}
					o := s.Feed(w.Direction, time.Unix(w.At, 0), raw)
					clear(raw)
					require.Equal(t, n, o.Consumed)
					if w.Error == "" {
						require.Nil(t, o.Err)
					} else {
						rocTypedError(t, w.Error, o.Err)
					}
					require.Len(t, o.Events, 1)
					events = append(events, o.Events...)
				}
				events = append(events, s.Close("complete")...)
				require.Empty(t, s.Close("again"))
				st := s.Stats()
				require.Zero(t, st.BufferedBytes)
				require.EqualValues(t, total, st.InputBytes)
				require.EqualValues(t, total, st.MessageBytes)
				require.EqualValues(t, limited, st.LimitedBytes)
				accountingCheck(t, c, events, false, d)
			})
		}
	}
}
func TestRADIUSAccountingMatrix(t *testing.T) {
	for _, c := range accountingControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			raw := accountingCapture(t, c)
			if root := os.Getenv("RADIUS_ACCOUNTING_EXPORT"); root != "" {
				p := filepath.Join(root, "radius-accounting")
				require.NoError(t, os.MkdirAll(p, 0700))
				require.NoError(t, os.WriteFile(filepath.Join(p, c.Name+".pcap"), raw, 0600))
			} else {
				require.Equal(t, raw, finsReadBatch(t, "radius-accounting", "RADIUS_ACCOUNTING_TEST_ROOT", c.Name+".pcap"))
			}
			for _, workers := range []int{1, 2, 4} {
				for _, d := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/d%t/o%t", workers, d, observe), func(t *testing.T) {
							var events, originals []*ProtocolEvent
							var stats ProtocolStats
							seen := 0
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(d), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnProtocolMessage(func(e *ProtocolEvent) {
								snap := *e
								snap.Raw = bytes.Clone(e.Raw)
								snap.Session = cloneSession(e.Session)
								snap.SourceBytes.PacketRefs = append([]PacketReference(nil), e.SourceBytes.PacketRefs...)
								events = append(events, &snap)
								originals = append(originals, e)
								if observe {
									tlsDTLSPoison(e, "http")
									e.ID, e.FlowID = 0, 0
								}
							})}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
							}
							a, err := NewPacketAnalyzer(opts...)
							require.NoError(t, err)
							a.conf.binParser.budget = accountingBudget(c)
							if c.Budget.MaxMessageBytes > 0 {
								a.conf.binParser.config.MaxMessageBytes = c.Budget.MaxMessageBytes
							}
							r, err := NewCaptureReader(bytes.NewReader(raw))
							require.NoError(t, err)
							for {
								b, ci, err := r.ReadPacketData()
								if err == io.EOF {
									break
								}
								require.NoError(t, err)
								require.NoError(t, a.Feed(b, ci, r.LinkType()))
								clear(b)
							}
							require.NoError(t, a.Close())
							require.NoError(t, a.Close())
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
							if observe {
								require.Equal(t, len(c.Steps), seen)
							}
							accountingCheck(t, c, events, true, d)
							for i := range c.Steps {
								f, err := originals[i].GetFields()
								rocTypedError(t, c.Steps[i].Error, err)
								require.Equal(t, accountingWant(t, c, i), f)
							}
						})
					}
				}
			}
		})
	}
}
