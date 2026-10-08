package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"

	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/internal/trafficfixture"
)

// Seeds are complete bidirectional envelopes from unmodified independent
// open62541 sockets. Fuzz mutations alter record directions, framing, versions,
// endpoint strings, security policies and bodies; no raw packet writer is used.
func FuzzOPCUAStateSequence(f *testing.F) {
	root := "../../bin-parser/testdata/protocol-native/industrial/"
	for _, name := range []string{"open62541-none.pcap", "open62541-reverse-none.pcap", "open62541-rsa-sign.pcap", "open62541-reverse-ecc-encrypt.pcap"} {
		raw, err := trafficfixture.ReadFile(root + name)
		require.NoError(f, err)
		flows := map[uint64][]byte{}
		err = ReplayPcap(bytes.NewReader(raw), WithOnProtocolMessage(func(e *ProtocolEvent) {
			if e.Protocol != "opcua" || len(e.Raw) > 65535 {
				return
			}
			// Keep transport/security transitions and reads; the large discovery
			// address-space response is covered by the native replay test.
			if e.Session["Message Type"] == "MSG" && e.Session["Service Name"] != "ReadRequest" && e.Session["Service Name"] != "ReadResponse" && e.Session["Semantic Status"] != "security-context-required" {
				return
			}
			header := []byte{byte(e.Direction), 0, 0}
			binary.LittleEndian.PutUint16(header[1:], uint16(len(e.Raw)))
			flows[e.FlowID] = append(flows[e.FlowID], header...)
			flows[e.FlowID] = append(flows[e.FlowID], e.Raw...)
		}))
		require.NoError(f, err)
		for _, seed := range flows {
			for _, split := range []byte{0, 1, 7, 255} {
				f.Add(split, seed)
			}
			// The two unused direction-header bits inject lifecycle actions
			// between native records, while the original seeds remain unchanged.
			if len(seed) >= 3 {
				next := 3 + int(binary.LittleEndian.Uint16(seed[1:]))
				if next+3 <= len(seed) {
					for _, action := range []byte{0x80, 0xc0} {
						closed := bytes.Clone(seed)
						closed[next] |= action
						f.Add(byte(7), closed)
					}
				}
			}
		}
	}
	f.Fuzz(func(t *testing.T, split byte, tape []byte) {
		if len(tape) > 65536 {
			return
		}
		before := sha256.Sum256(tape)
		budget := ParserBudget{MaxFrameBytes: 32768, MaxMessageBytes: 32768, MaxBufferedBytes: 262144, MaxCollectionElements: 32, MaxRecursionDepth: 16, ProbeBytes: 64}
		session, err := NewProtocolSession(budget)
		require.NoError(t, err)
		internal := session.(*captureSession)
		internal.f.a.config.Deferred = split&4 != 0
		var events []*ProtocolEvent
		var snapshots [][32]byte
		reverseDir := -1
		reverseEndpoint := ""
		helloVersions := map[int]uint32{}
		observedControl := map[string]bool{}
		transportSeen := false
		closedDuringSequence := false
		checkEvents := func(batch []*ProtocolEvent) {
			for _, event := range batch {
				encoded, err := json.Marshal([]any{event.Raw, event.Session})
				require.NoError(t, err)
				events = append(events, event)
				snapshots = append(snapshots, sha256.Sum256(encoded))
				if event.Protocol != "opcua" || event.Error != "" {
					continue
				}
				out := event.Session
				if out["Semantic Status"] == "security-context-required" {
					require.Equal(t, false, out["Content Decoded"])
					require.NotContains(t, out, "Service Type ID")
					require.NotContains(t, out, "Service Name")
				}
				kind, _ := out["Message Type"].(string)
				if kind == "RHE" || kind == "HEL" || kind == "ACK" {
					require.False(t, observedControl[kind], "transport handshake cannot restart")
					observedControl[kind] = true
				}
				if kind == "RHE" {
					require.False(t, transportSeen, "ReverseHello must precede established transport")
				} else {
					transportSeen = true
				}
				switch kind {
				case "RHE":
					if reverseDir != -1 {
						require.Equal(t, reverseDir, event.Direction)
					}
					reverseDir = event.Direction
					reverseEndpoint, _ = out["Endpoint URL"].(string)
				case "HEL":
					if reverseDir != -1 {
						require.NotEqual(t, reverseDir, event.Direction)
						require.Equal(t, reverseEndpoint, out["Endpoint URL"])
					}
					helloVersions[event.Direction] = out["Protocol Version"].(uint32)
				case "ACK":
					if reverseDir != -1 {
						require.Equal(t, reverseDir, event.Direction)
					}
					if requested, ok := helloVersions[1-event.Direction]; ok {
						require.LessOrEqual(t, out["Protocol Version"].(uint32), requested)
					}
				case "OPN":
					if out["Security Policy URI"] != "http://opcfoundation.org/UA/SecurityPolicy#None" {
						require.Equal(t, "security-context-required", out["Semantic Status"])
					}
				}
			}
		}
		for at, records := 0, 0; at+3 <= len(tape) && records < 64; records++ {
			direction := int(tape[at] & 1)
			if tape[at]&0x80 != 0 && !closedDuringSequence {
				reason := "FIN"
				if tape[at]&0x40 != 0 {
					reason = "idle timeout"
				}
				checkEvents(session.Close(reason))
				closedDuringSequence = true
				require.Zero(t, session.Stats().BufferedBytes)
				require.Empty(t, internal.events)
			}
			size := int(binary.LittleEndian.Uint16(tape[at+1:]))
			at += 3
			if size > len(tape)-at {
				size = len(tape) - at
			}
			wire := tape[at : at+size]
			at += size
			for len(wire) > 0 {
				n := len(wire)
				if split != 0 {
					n = min(n, int(split))
				}
				result := session.Feed(direction, time.Unix(1, 0), wire[:n])
				if closedDuringSequence {
					require.Equal(t, "closed", result.State)
					require.Empty(t, result.Events)
					require.Zero(t, session.Stats().BufferedBytes)
				} else {
					checkEvents(result.Events)
				}
				require.LessOrEqual(t, session.Stats().BufferedBytes, int64(budget.MaxBufferedBytes))
				wire = wire[n:]
			}
		}
		checkEvents(session.Close("FIN"))
		require.Empty(t, session.Close("FIN"))
		require.Zero(t, session.Stats().BufferedBytes)
		require.LessOrEqual(t, session.Stats().PeakBufferedBytes, int64(budget.MaxBufferedBytes))
		require.Nil(t, internal.f.a.err.Load(), "mutation must not reach panic recovery")
		closed := session.Feed(0, time.Unix(2, 0), []byte("HELF"))
		require.Equal(t, "closed", closed.State)
		require.Empty(t, closed.Events)
		for i, event := range events {
			encoded, err := json.Marshal([]any{event.Raw, event.Session})
			require.NoError(t, err)
			require.Equal(t, snapshots[i], sha256.Sum256(encoded), "published event changed after later input or close")
		}
		require.Equal(t, before, sha256.Sum256(tape), "Feed changed borrowed input")
	})
}
