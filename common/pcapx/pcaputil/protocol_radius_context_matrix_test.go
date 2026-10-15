package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type radiusControlStep struct {
	Direction   int
	At          int64          `json:"timestamp_unix"`
	Wire        string         `json:"input_hex"`
	Error       string         `json:"error"`
	Reply       *int           `json:"response_to_step"`
	Transaction *int           `json:"transaction_step"`
	Fields      map[string]any `json:"canonical_fields_hexbytes"`
}
type radiusControl struct {
	Name   string
	Steps  []radiusControlStep
	Ports  [2]uint16
	Budget struct{ MaxCollectionElements int }
}

func radiusControls(t *testing.T) []radiusControl {
	t.Helper()
	var v struct{ Cases []radiusControl }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "radius-context", "RADIUS_CONTEXT_TEST_ROOT", "controls.json"), &v))
	require.Len(t, v.Cases, 29)
	return v.Cases
}
func radiusWire(t *testing.T, s string) []byte {
	t.Helper()
	b, e := hex.DecodeString(s)
	require.NoError(t, e)
	return b
}
func radiusWant(t *testing.T, c radiusControl, i int, udp bool) map[string]any {
	t.Helper()
	w := c.Steps[i]
	if w.Error != "" {
		return nil
	}
	s := cloneSession(w.Fields)
	for _, key := range []string{"Code", "Identifier", "Attribute Count", "User-Password Bytes", "Packet Length", "Padding Bytes"} {
		if v, ok := s[key].(float64); ok {
			s[key] = int(v)
		}
	}
	s["Authenticator"] = radiusWire(t, s["Authenticator"].(string))
	raw := s["Attributes"].([]any)
	attrs := make([]map[string]any, 0, len(raw))
	for _, a := range raw {
		m := a.(map[string]any)
		attrs = append(attrs, map[string]any{"Type": byte(m["Type"].(float64)), "Raw": radiusWire(t, m["Raw"].(string))})
	}
	s["Attributes"] = attrs
	if udp {
		s["Observation Scope"] = "conversation"
	}
	if w.Reply != nil {
		s["In Reply To"] = "Access-Request"
	} else if w.Transaction == nil {
		diag := "ambiguous-identifier"
		if c.Name == "orphan-reply" || c.Name == "orphan-then-reuse" && i == 0 || c.Name == "wrong-direction" && i == 1 {
			diag = "missing-request"
		}
		s["Association"] = diag
	}
	return s
}
func radiusControlCapture(t *testing.T, c radiusControl) []byte {
	t.Helper()
	var out bytes.Buffer
	w := pcapgo.NewWriter(&out)
	require.NoError(t, w.WriteFileHeader(65535, layers.LinkTypeEthernet))
	for _, s := range c.Steps {
		src, dst := net.IPv4(192, 0, 2, 1), net.IPv4(192, 0, 2, 2)
		sp, dp := c.Ports[0], c.Ports[1]
		if s.Direction == 1 {
			src, dst = dst, src
			sp, dp = dp, sp
		}
		eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
		ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: src, DstIP: dst}
		udp := &layers.UDP{SrcPort: layers.UDPPort(sp), DstPort: layers.UDPPort(dp)}
		require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
		b := gopacket.NewSerializeBuffer()
		require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, udp, gopacket.Payload(radiusWire(t, s.Wire))))
		require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(s.At, 0), CaptureLength: len(b.Bytes()), Length: len(b.Bytes())}, b.Bytes()))
	}
	return out.Bytes()
}
func radiusCheck(t *testing.T, c radiusControl, events []*ProtocolEvent, udp, captured, deferred bool) {
	t.Helper()
	extra := 0
	if c.Name == "unknown-attribute" || c.Name == "password-128" {
		extra = 1
	}
	require.Len(t, events, len(c.Steps)+extra)
	ids := map[uint64]bool{}
	for i, w := range c.Steps {
		e := events[i]
		require.Equal(t, "radius", e.Protocol)
		require.Equal(t, "radius-access-observed", e.Profile)
		require.NotZero(t, e.ID)
		require.False(t, ids[e.ID])
		ids[e.ID] = true
		require.Equal(t, w.Direction, e.Direction)
		require.Equal(t, "udp", e.Transport)
		src, dst := fmt.Sprintf("192.0.2.1:%d", c.Ports[0]), fmt.Sprintf("192.0.2.2:%d", c.Ports[1])
		if w.Direction == 1 {
			src, dst = dst, src
		}
		require.Equal(t, src, e.Source)
		require.Equal(t, dst, e.Destination)
		require.NotZero(t, e.FlowID)
		require.Equal(t, events[0].FlowID, e.FlowID)
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

		wire := radiusWire(t, w.Wire)
		require.Equal(t, wire, e.Raw)
		require.Equal(t, len(wire), e.Length)
		require.True(t, time.Unix(w.At, 0).Equal(e.Timestamp))
		require.Equal(t, CaptureDomain{}, e.Domain)
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
			require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(wire)), e.SourceBytes.SHA256)
		}
		want := radiusWant(t, c, i, udp)
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
	if extra != 0 {
		e := events[len(c.Steps)]
		require.Equal(t, "radius", e.Protocol)
		require.Equal(t, "incomplete", e.Status)
		require.Equal(t, map[string]any{"Outstanding": 1}, e.Session)
		f, err := e.GetFields()
		rocTypedError(t, "NeedMore", err)
		require.Nil(t, f)
		require.Zero(t, e.TransactionID)
		require.Zero(t, e.ResponseTo)
	}
}
func radiusReplay(t *testing.T, c radiusControl, raw []byte, workers int, deferred, observe bool) ([]*ProtocolEvent, []*ProtocolEvent, ProtocolStats) {
	t.Helper()
	var snapshots, originals []*ProtocolEvent
	var stats ProtocolStats
	seen := 0
	opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnProtocolMessage(func(e *ProtocolEvent) {
		snap := *e
		snap.Raw = bytes.Clone(e.Raw)
		snap.Session = cloneSession(e.Session)
		snap.SourceBytes.PacketRefs = append([]PacketReference(nil), e.SourceBytes.PacketRefs...)
		snapshots = append(snapshots, &snap)
		originals = append(originals, e)
		if observe {
			tlsDTLSPoison(e, "http")
			e.ID = 0
			e.FlowID = 0
		}
	})}
	if observe {
		opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
	}
	a, err := NewPacketAnalyzer(opts...)
	require.NoError(t, err)
	if c.Budget.MaxCollectionElements > 0 {
		a.conf.binParser.budget.MaxCollectionElements = c.Budget.MaxCollectionElements
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
	return snapshots, originals, stats
}
func TestRADIUSContextMatrix(t *testing.T) {
	for _, c := range radiusControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			raw := radiusControlCapture(t, c)
			if root := os.Getenv("RADIUS_CONTEXT_EXPORT"); root != "" {
				p := filepath.Join(root, "radius-context")
				require.NoError(t, os.MkdirAll(p, 0700))
				require.NoError(t, os.WriteFile(filepath.Join(p, c.Name+".pcap"), raw, 0600))
			} else {
				require.Equal(t, raw, finsReadBatch(t, "radius-context", "RADIUS_CONTEXT_TEST_ROOT", c.Name+".pcap"))
			}
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/d%t/o%t", workers, deferred, observe), func(t *testing.T) {
							events, originals, _ := radiusReplay(t, c, raw, workers, deferred, observe)
							radiusCheck(t, c, events, true, true, deferred)
							for i, e := range originals {
								want := radiusWant(t, c, min(i, len(c.Steps)-1), true)
								if i >= len(c.Steps) {
									continue
								}
								f, err := e.GetFields()
								rocTypedError(t, c.Steps[i].Error, err)
								require.Equal(t, want, f)
							}
						})
					}
				}
			}
		})
	}
}
func TestRADIUSContextNative(t *testing.T) {
	for _, c := range radiusControls(t) {
		for _, d := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/d%t", c.Name, d), func(t *testing.T) {
				b := DefaultParserBudget()
				if c.Budget.MaxCollectionElements > 0 {
					b.MaxCollectionElements = c.Budget.MaxCollectionElements
				}
				s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(c.Ports[0], c.Ports[1]))
				require.NoError(t, err)
				defer s.Close("cleanup")
				s.(*captureSession).f.a.config.Deferred = d
				var events []*ProtocolEvent
				for _, w := range c.Steps {
					raw := radiusWire(t, w.Wire)
					o := s.Feed(w.Direction, time.Unix(w.At, 0), raw)
					clear(raw)
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
				require.Zero(t, s.Stats().BufferedBytes)
				radiusCheck(t, c, events, true, false, d)
			})
		}
	}
}
