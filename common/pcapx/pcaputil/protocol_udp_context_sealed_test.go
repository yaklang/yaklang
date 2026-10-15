package pcaputil

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type udpContextEvent struct {
	tftpFileEvent
	ResponseTo  *int `json:"response_to"`
	Transaction *int `json:"transaction"`
}
type udpContextCase struct {
	tftpFileCase
	Events []udpContextEvent
}

func udpContextCheck(t *testing.T, c udpContextCase, events []*ProtocolEvent, deferred, captured bool) {
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

func TestUDPContextSealedMatrix(t *testing.T) {
	var d struct {
		Schema string
		Cases  []udpContextCase
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "udp-context", "UDP_CONTEXT_TEST_ROOT", "controls.json"), &d))
	require.Equal(t, "owned-udp-context/v1", d.Schema)
	require.Len(t, d.Cases, 27)
	for _, c := range d.Cases {
		t.Run(c.ID, func(t *testing.T) {
			raw := finsReadBatch(t, "udp-context", "UDP_CONTEXT_TEST_ROOT", c.Capture)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			flat := c.tftpFileCase
			for _, w := range c.Events {
				flat.Events = append(flat.Events, w.tftpFileEvent)
			}
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				events, stats := tftpFileReplay(t, flat, raw, workers, deferred, observe)
				udpContextCheck(t, c, events, deferred, true)
				require.Zero(t, stats.BufferedBytes)
				require.Zero(t, stats.CallbackPanics)
			})
		})
	}
}
