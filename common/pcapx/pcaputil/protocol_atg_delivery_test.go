package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"io"
	"math"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type atgDeliveryEvent struct {
	udpContextEvent
	Close     bool
	FloatBits map[string]string `json:"float_bits"`
}
type atgDeliveryCase struct {
	tftpFileCase
	Events []atgDeliveryEvent
}

func atgDeliveryControls(t *testing.T) []atgDeliveryCase {
	var d struct {
		Schema string
		Cases  []atgDeliveryCase
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "atg-delivery", "ATG_DELIVERY_TEST_ROOT", "controls.json"), &d))
	require.Equal(t, "owned-atg-delivery/v1", d.Schema)
	require.Len(t, d.Cases, 30)
	return d.Cases
}
func atgDeliveryCheck(t *testing.T, c atgDeliveryCase, events []*ProtocolEvent, deferred, captured bool) {
	t.Helper()
	require.Len(t, events, len(c.Events))
	ids := map[uint64]bool{}
	flows := map[int]uint64{}
	for i, w := range c.Events {
		e := events[i]
		require.Equal(t, w.Protocol, e.Protocol)
		require.Equal(t, w.Profile, e.Profile)
		require.Equal(t, "tcp", e.Transport)
		require.Equal(t, w.Direction, e.Direction)
		require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
		require.Equal(t, len(w.InputWire)/2, e.Length)
		require.Equal(t, w.Offset, e.Offset)
		require.True(t, time.Unix(w.Timestamp, 0).Equal(e.Timestamp))
		if w.Transaction == nil {
			require.Zero(t, e.TransactionID)
		} else {
			require.GreaterOrEqual(t, *w.Transaction, 0)
			require.Less(t, *w.Transaction, len(events))
			require.Equal(t, events[*w.Transaction].ID, e.TransactionID)
		}
		if w.ResponseTo == nil {
			require.Zero(t, e.ResponseTo)
		} else {
			require.GreaterOrEqual(t, *w.ResponseTo, 0)
			require.Less(t, *w.ResponseTo, len(events))
			require.Equal(t, events[*w.ResponseTo].ID, e.ResponseTo)
		}
		require.NotZero(t, e.ID)
		require.False(t, ids[e.ID])
		ids[e.ID] = true
		if w.FlowOwner < 0 {
			require.Zero(t, e.FlowID)
		} else {
			require.NotZero(t, e.FlowID)
			if id, ok := flows[w.FlowOwner]; ok {
				require.Equal(t, id, e.FlowID)
			} else {
				for _, id := range flows {
					require.NotEqual(t, id, e.FlowID)
				}
				flows[w.FlowOwner] = e.FlowID
			}
		}
		wire := wrapperWire(t, w.InputWire)
		dp := w.ServerPort
		if len(wire) >= 2 && (binary.BigEndian.Uint16(wire) == 1 || binary.BigEndian.Uint16(wire) == 2) {
			dp = w.InitialPort
		}
		src, dst := fmt.Sprintf("192.0.2.1:%d", w.ClientPort), fmt.Sprintf("192.0.2.2:%d", dp)
		if w.WireDirection == 1 {
			dst = fmt.Sprintf("192.0.2.2:%d", w.ServerPort)
		}
		if w.WireDirection == 1 {
			src, dst = dst, src
		}
		if captured {
			src = strings.ReplaceAll(strings.ReplaceAll(src, "192.0.2.1:", "192.0.2.10:"), "192.0.2.2:", "192.0.2.20:")
			dst = strings.ReplaceAll(strings.ReplaceAll(dst, "192.0.2.1:", "192.0.2.10:"), "192.0.2.2:", "192.0.2.20:")
		}
		require.Equal(t, src, e.Source)
		require.Equal(t, dst, e.Destination)
		require.Equal(t, CaptureDomain{Interface: w.Interface}, e.Domain)
		if captured {
			require.Len(t, e.SourceBytes.PacketRefs, len(w.Refs))
			for j, ref := range w.Refs {
				require.Equal(t, ref, e.SourceBytes.PacketRefs[j].Number)
				require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[j].Domain)
			}
		}
		status := "decoded"
		if deferred {
			status = "deferred"
		}
		switch w.Error {
		case "ResourceExceeded":
			status = "limited"
		case "MalformedMessage", "Desynchronized":
			status = "malformed"
		case "ContextRequired", "UnsupportedFeature", "UnsupportedVersion":
			status = "context-required"
		case "NeedMore":
			status = "incomplete"
		case "Unrecognized":
			status = "unrecognized"
		}
		if w.Status != "" {
			status = w.Status
		}
		require.Equal(t, status, e.Status)
		if w.Error != "" && w.Error != "Unrecognized" {
			require.Equal(t, w.Error, e.ExpertCode)
		}
		if w.Error == "" || w.Error == "Unrecognized" {
			require.Empty(t, e.Error)
		} else {
			require.NotEmpty(t, e.Error)
		}
		w.Session, _ = tftpFileResolve(w.Session, events).(map[string]any)
		w.Fields, _ = tftpFileResolve(w.Fields, events).(map[string]any)
		rocEqualFields(t, w.Session, e.Session)
		tftpFileQuery(t, e, w.tftpFileEvent)
		if w.QueryError == "" {
			f, err := e.GetFields()
			require.NoError(t, err)
			if v, ok := f["Tank Selector"]; ok {
				require.IsType(t, int(0), v)
			} else {
				require.Equal(t, "Command Rejected", f["Packet Name"])
				if f["Association"] == "ambiguous-response" {
					require.NotContains(t, f, "Request Tank Selector")
					require.NotContains(t, f, "Request Function")
					require.Equal(t, "unobserved", f["In Reply To"])
					require.NotEmpty(t, f["Candidate Requests"])
				} else {
					require.IsType(t, int(0), f["Request Tank Selector"])
				}
			}
			if _, ok := f["Observed Request ID"]; ok {
				require.IsType(t, uint64(0), f["Observed Request ID"])
			}
			if rows, ok := f["Tanks"].([]map[string]any); ok && f["Function"] == "i202" {
				for _, row := range rows {
					require.IsType(t, int(0), row["Delivery Count"])
					for _, d := range row["Deliveries"].([]map[string]any) {
						require.IsType(t, float64(0), d["Starting Volume"])
						require.IsType(t, float64(0), d["Ending Temp"])
					}
				}
			}
		}

		if len(w.FloatBits) > 0 {
			fields, err := e.GetFields()
			require.NoError(t, err)
			for key, hexBits := range w.FloatBits {
				parts := strings.SplitN(key, ":", 3)
				require.Len(t, parts, 3)
				ri, x := strconv.Atoi(parts[0])
				require.NoError(t, x)
				di, x := strconv.Atoi(parts[1])
				require.NoError(t, x)
				rows := fields["Tanks"].([]map[string]any)
				require.Less(t, ri, len(rows))
				ds := rows[ri]["Deliveries"].([]map[string]any)
				require.Less(t, di, len(ds))
				v, ok := ds[di][parts[2]].(float64)
				require.True(t, ok)
				bits, x := strconv.ParseUint(hexBits, 16, 64)
				require.NoError(t, x)
				require.Equal(t, bits, math.Float64bits(v), "exact IEEE754 conversion including signed zero/subnormal/maxfinite")
			}
		}
		for _, alias := range []string{"caller", "redis", "sip"} {
			tlsDTLSPoison(e, alias)
			tftpFileQuery(t, e, w.tftpFileEvent)
			if w.QueryError == "" {
				f, err := e.GetFields()
				require.NoError(t, err)
				switch w.Protocol {
				case "slmp":
					require.IsType(t, uint16(0), f["Command"])
					require.IsType(t, []byte{}, f["Loopback Data"])
				case "dlms":
					require.IsType(t, uint16(0), f["Frame Format"])
					require.IsType(t, uint8(0), f["Control"])
				case "dlms-wrapper":
					require.IsType(t, uint16(0), f["wrapper_version"])
					require.IsType(t, uint8(0), f["command"])
				}
			}

		}
	}
}

func atgDeliveryReplay(t *testing.T, c tftpFileCase, raw []byte, workers int, deferred, observe bool) ([]*ProtocolEvent, ProtocolStats) {
	t.Helper()
	var snapshots, originals []*ProtocolEvent
	var stats ProtocolStats
	var seen atomic.Int64
	opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnProtocolMessage(func(e *ProtocolEvent) {
		snap := *e
		snap.Raw = bytes.Clone(e.Raw)
		snap.Session = cloneSession(e.Session)
		snap.Fields = cloneSession(e.Fields)
		snap.SourceBytes.PacketRefs = append([]PacketReference(nil), e.SourceBytes.PacketRefs...)
		snapshots = append(snapshots, &snap)
		originals = append(originals, e)
		if observe {
			tlsDTLSPoison(e, "redis")
			e.ID = 0
			e.FlowID = 0
			e.Timestamp = time.Unix(999999, 0)
			e.Direction = 99
			e.Status = "decoded"
			e.Error = ""
			if len(e.SourceBytes.PacketRefs) > 0 {
				e.SourceBytes.PacketRefs[0].Number = 999999
			}
		}
	})}
	if c.FileEnabled {
		opts = append(opts, WithProtocolTFTPFileAssembly(69, c.FileLimit))
	}
	if c.MaxMessage > 0 || c.MaxBuffered > 0 {
		m, b := c.MaxMessage, c.MaxBuffered
		if m == 0 {
			m = 1 << 20
		}
		if b == 0 {
			b = 1 << 20
		}
		opts = append(opts, WithProtocolBudget(m, b))
	}
	if observe {
		opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
	}
	a, err := NewPacketAnalyzer(opts...)
	require.NoError(t, err)
	if c.MaxCollection > 0 {
		a.conf.binParser.budget.MaxCollectionElements = c.MaxCollection
	}
	r, err := NewCaptureReader(bytes.NewReader(raw))
	require.NoError(t, err)
	packets := int64(0)
	for {
		b, ci, x := r.ReadPacketData()
		if x == io.EOF {
			break
		}
		require.NoError(t, x)
		require.NoError(t, a.Feed(b, ci, r.LinkType()))
		clear(b)
		packets++
	}
	require.NoError(t, a.Close())
	require.NoError(t, a.Close())
	require.Zero(t, stats.BufferedBytes)
	require.Zero(t, stats.CallbackPanics)
	if observe {
		require.Equal(t, packets, seen.Load())
	}
	require.Len(t, originals, len(c.Events))
	for i, e := range originals {
		w := c.Events[i]
		w.Session, _ = tftpFileResolve(w.Session, snapshots).(map[string]any)
		w.Fields, _ = tftpFileResolve(w.Fields, snapshots).(map[string]any)
		tftpFileQuery(t, e, w)
	}
	return snapshots, stats
}

func TestATGDeliverySealedMatrix(t *testing.T) {
	for _, c := range atgDeliveryControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw := finsReadBatch(t, "atg-delivery", "ATG_DELIVERY_TEST_ROOT", c.Capture)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			flat := c.tftpFileCase
			for _, w := range c.Events {
				flat.Events = append(flat.Events, w.tftpFileEvent)
			}
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				events, stats := atgDeliveryReplay(t, flat, raw, workers, deferred, observe)
				atgDeliveryCheck(t, c, events, deferred, true)
				require.Zero(t, stats.BufferedBytes)
				require.Zero(t, stats.CallbackPanics)
			})
		})
	}
}
func TestATGDeliveryNativeChunks(t *testing.T) {
	for _, c := range atgDeliveryControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			for _, chunk := range []int{0, 1, 2, 7, 64, 4096} {
				for _, deferred := range []bool{false, true} {
					t.Run(fmt.Sprintf("chunk%d/d%t", chunk, deferred), func(t *testing.T) {
						budget := DefaultParserBudget()
						if c.MaxCollection > 0 {
							budget.MaxCollectionElements = c.MaxCollection
						}
						s, err := NewProtocolSessionWithOptions(budget, WithSessionTransport("tcp"), WithSessionPorts(41707, 41001), WithSessionClientDirection(0))
						require.NoError(t, err)
						s.(*captureSession).f.a.config.Deferred = deferred

						var events, originals []*ProtocolEvent
						appendEvents := func(out []*ProtocolEvent) {
							for _, e := range out {
								snap := *e
								snap.Raw = bytes.Clone(e.Raw)
								snap.Fields = cloneSession(e.Fields)
								snap.Session = cloneSession(e.Session)
								snap.SourceBytes.PacketRefs = append([]PacketReference(nil), e.SourceBytes.PacketRefs...)
								events = append(events, &snap)
								originals = append(originals, e)
								tlsDTLSPoison(e, "redis")
								e.ID = 0
								e.FlowID = 0
							}
						}
						type feedStep struct {
							dir  int
							wire []byte
						}
						var groups []feedStep
						allowed := map[ProtocolErrorKind]bool{ErrNeedMore: true}
						for _, w := range c.Events {
							if w.Error != "" {
								allowed[ProtocolErrorKind(w.Error)] = true
							}
							if w.Close {
								continue
							}
							raw := wrapperWire(t, w.InputWire)
							if chunk == 0 && len(groups) > 0 && groups[len(groups)-1].dir == w.Direction {
								groups[len(groups)-1].wire = append(groups[len(groups)-1].wire, raw...)
							} else {
								groups = append(groups, feedStep{w.Direction, raw})
							}
						}
						for _, g := range groups {
							size := chunk
							if size == 0 {
								size = len(g.wire)
							}
							for at := 0; at < len(g.wire); at += size {
								input := bytes.Clone(g.wire[at:min(at+size, len(g.wire))])
								o := s.Feed(g.dir, time.Unix(1700000000, 0), input)
								clear(input)
								if o.Err != nil {
									require.True(t, allowed[o.Err.Kind], "unexpected Feed error %v", o.Err)
								}
								appendEvents(o.Events)
							}
						}
						appendEvents(s.Close("EOF"))
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
						require.Len(t, originals, len(c.Events))
						for i, e := range originals {
							w := c.Events[i]
							w.Fields, _ = tftpFileResolve(w.Fields, events).(map[string]any)
							w.Session, _ = tftpFileResolve(w.Session, events).(map[string]any)
							tftpFileQuery(t, e, w.tftpFileEvent)
						}

						atgDeliveryCheck(t, c, events, deferred, false)
					})
				}
			}
		})
	}
}
