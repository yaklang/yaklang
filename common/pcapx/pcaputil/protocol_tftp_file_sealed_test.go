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
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type tftpFileEvent struct {
	InputWire                             string `json:"input_wire"`
	Raw, Protocol, Profile, Error, Status string
	QueryError                            string `json:"query_error"`
	WireDirection                         int    `json:"wire_direction"`
	Direction, Interface, Port            int
	Timestamp                             int64 `json:"timestamp_sec"`
	FlowOwner                             int   `json:"flow_owner"`
	ClientPort                            int   `json:"client_port"`
	InitialPort                           int   `json:"initial_port"`
	ServerPort                            int   `json:"server_port"`
	Offset                                uint64
	Refs                                  []uint64 `json:"packet_refs"`
	Fields, Session                       map[string]any
}
type tftpFileCase struct {
	ID, Transport, Capture, SHA256 string
	MaxMessage                     int  `json:"max_message_bytes"`
	MaxCollection                  int  `json:"max_collection"`
	FileEnabled                    bool `json:"file_enabled"`
	FileLimit                      int  `json:"file_limit"`
	MaxBuffered                    int  `json:"max_buffered_bytes"`
	Events                         []tftpFileEvent
}

func tftpFileControls(t *testing.T) []tftpFileCase {
	t.Helper()
	var d struct {
		Schema string
		Cases  []tftpFileCase
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "tftp-file", "TFTP_FILE_TEST_ROOT", "controls.json"), &d))
	require.Equal(t, "owned-tftp-file/v1", d.Schema)
	require.Len(t, d.Cases, 26)
	return d.Cases
}

func tftpFileQuery(t *testing.T, e *ProtocolEvent, w tftpFileEvent) {
	t.Helper()
	x := stunContextEvent{InputWire: w.InputWire, Fields: w.Fields, Session: w.Session, QueryError: w.QueryError}
	// This grammar has opaque binary DATA, not STUN attributes.
	x.InputWire = ""
	stunContextQuery(t, e, x)
	if w.QueryError == "" {
		wire := wrapperWire(t, w.InputWire)
		f, err := e.GetFields()
		require.NoError(t, err)
		if len(wire) > 4 && binary.BigEndian.Uint16(wire) == 3 {
			v, ok := f["File Data"].(string)
			require.True(t, ok)
			require.Equal(t, wire[4:], []byte(v))
		}
		out, err := e.Decode()
		require.NoError(t, err)
		if out["session"] != nil {
			v := out["session"].(map[string]any)
			if body, ok := v["File Bytes"]; ok {
				require.IsType(t, []byte{}, body)
				require.IsType(t, int(0), v["File Byte Count"])
				require.IsType(t, uint64(0), v["File Request ID"])
			}
		}
	}
}
func tftpFileResolve(v any, events []*ProtocolEvent) any {
	switch x := v.(type) {
	case string:
		if strings.HasPrefix(x, "@event:") {
			i, err := strconv.Atoi(strings.TrimPrefix(x, "@event:"))
			if err != nil || i < 0 || i >= len(events) {
				panic("invalid expected event link")
			}
			return events[i].ID
		}
		return x
	case map[string]any:
		if x == nil {
			return map[string]any(nil)
		}
		out := map[string]any{}
		for k, v := range x {
			out[k] = tftpFileResolve(v, events)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, v := range x {
			out[i] = tftpFileResolve(v, events)
		}
		return out
	default:
		return v
	}
}
func tftpFileCheck(t *testing.T, c tftpFileCase, events []*ProtocolEvent, deferred, captured bool) {
	t.Helper()
	require.Len(t, events, len(c.Events))
	ids := map[uint64]bool{}
	flows := map[int]uint64{}
	for i, w := range c.Events {
		e := events[i]
		require.Equal(t, w.Protocol, e.Protocol)
		require.Equal(t, w.Profile, e.Profile)
		require.Equal(t, "udp", e.Transport)
		require.Equal(t, w.Direction, e.Direction)
		require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
		require.Equal(t, len(w.InputWire)/2, e.Length)
		require.Equal(t, w.Offset, e.Offset)
		require.True(t, time.Unix(w.Timestamp, 0).Equal(e.Timestamp))
		require.Zero(t, e.TransactionID)
		require.Zero(t, e.ResponseTo)
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
		rocEqualFields(t, w.Session, e.Session)
		tftpFileQuery(t, e, w)
		for _, alias := range []string{"caller", "redis", "sip"} {
			tlsDTLSPoison(e, alias)
			tftpFileQuery(t, e, w)
		}
	}
}

// In observer-on configurations, edit events immediately during delivery, then
// validate the untouched snapshots and the originals' private query results.
func tftpFileReplay(t *testing.T, c tftpFileCase, raw []byte, workers int, deferred, observe bool) ([]*ProtocolEvent, ProtocolStats) {
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
		tftpFileQuery(t, e, w)
	}
	return snapshots, stats
}

func tftpFileSources(t *testing.T, events []*ProtocolEvent) {
	t.Helper()
	byID := map[uint64]*ProtocolEvent{}
	for _, e := range events {
		require.NotZero(t, e.ID)
		require.Nil(t, byID[e.ID])
		byID[e.ID] = e
	}
	for _, e := range events {
		body, ok := e.Session["File Bytes"].([]byte)
		if !ok {
			continue
		}
		req := byID[e.Session["File Request ID"].(uint64)]
		require.NotNil(t, req)
		require.Equal(t, req.FlowID, e.FlowID)
		require.Equal(t, req.Domain, e.Domain)
		require.Contains(t, []uint16{1, 2}, binary.BigEndian.Uint16(req.Raw))
		records := e.Session["File Sources"].([]map[string]any)
		offset := 0
		for _, rec := range records {
			id := rec["Event ID"].(uint64)
			src := byID[id]
			require.NotNil(t, src)
			require.Equal(t, e.FlowID, src.FlowID)
			require.Equal(t, e.Domain, src.Domain)
			require.Equal(t, uint16(3), binary.BigEndian.Uint16(src.Raw))
			require.Equal(t, 4, rec["Event Offset"])
			require.Equal(t, offset, rec["File Offset"])
			n := rec["Length"].(int)
			require.Equal(t, len(src.Raw)-4, n)
			require.LessOrEqual(t, offset+n, len(body))
			require.Equal(t, body[offset:offset+n], src.Raw[4:])
			nums := rec["Packet Numbers"].([]uint64)
			require.Len(t, nums, len(src.SourceBytes.PacketRefs))
			for i, ref := range src.SourceBytes.PacketRefs {
				require.Equal(t, ref.Number, nums[i])
			}
			offset += n
		}
		require.Equal(t, len(body), offset)
		require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(body)), e.Session["File SHA256"])
	}
}

func TestTFTPFileSealedMatrix(t *testing.T) {
	for _, c := range tftpFileControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw := finsReadBatch(t, "tftp-file", "TFTP_FILE_TEST_ROOT", c.Capture)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				events, stats := tftpFileReplay(t, c, raw, workers, deferred, observe)
				tftpFileSources(t, events)
				tftpFileCheck(t, c, events, deferred, true)
				require.Zero(t, stats.BufferedBytes)
				require.Zero(t, stats.CallbackPanics)
			})
		})
	}
}
