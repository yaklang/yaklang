package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestUnrecognizedUDPCompleteDatagramSource(t *testing.T) {
	for _, size := range []int{0, 1, 15, 16, 63, 64, 65, 127, 512, 1200, 4096, 65507} {
		for _, public := range []bool{false, true} {
			t.Run(fmt.Sprintf("bytes%d/public%t", size, public), func(t *testing.T) {
				payload := bytes.Repeat([]byte{0x3f}, size)
				wire := carrierTestIPv4(carrierTestUDP(payload, size+8))
				events := carrierTestReplay(t, wire, len(wire), public)
				require.Len(t, events, 1)
				e := events[0]
				require.Empty(t, e.Protocol, "a complete datagram does not identify its application")
				require.Equal(t, "unrecognized", e.Status)
				require.Equal(t, "ProtocolNotRecognized", e.ExpertCode)
				require.Equal(t, "datagram", e.Completeness)
				require.Equal(t, size, e.Length)
				require.True(t, bytes.Equal(payload, e.Raw))
				if size != 0 {
					h := sha256.Sum256(payload)
					require.Equal(t, hex.EncodeToString(h[:]), e.SourceBytes.SHA256)
				} else {
					require.Empty(t, e.SourceBytes.SHA256, "no invented nonempty payload source")
				}
				require.Len(t, e.SourceBytes.PacketRefs, 1)
				require.Equal(t, uint64(1), e.SourceBytes.PacketRefs[0].Number)
				require.Zero(t, e.FlowID)
				require.Nil(t, e.Session)
				_, err := e.Decode()
				require.ErrorContains(t, err, "unrecognized", "carrier completeness cannot grant protocol decoding")
				for i := range wire {
					wire[i] = 0
				}
				require.True(t, bytes.Equal(payload, e.Raw))
			})
		}
	}
}

func TestUnrecognizedUDPCarrierRefusalsRemainBounded(t *testing.T) {
	for _, profile := range []string{"length-zero", "length-exceeds-IP", "capture-cut"} {
		t.Run(profile, func(t *testing.T) {
			body := carrierTestUDP(bytes.Repeat([]byte{0x3f}, 1200), 1208)
			if profile == "length-zero" {
				body[4], body[5] = 0, 0
			} else if profile == "length-exceeds-IP" {
				body[4], body[5] = 5, 0
			}
			wire := carrierTestIPv4(body)
			wireLength := len(wire)
			if profile == "capture-cut" {
				wire = wire[:len(wire)-100]
			}
			events := carrierTestReplay(t, wire, wireLength, false)
			require.Len(t, events, 1)
			e := events[0]
			require.Empty(t, e.Protocol)
			require.NotEqual(t, "unrecognized", e.Status)
			require.NotEqual(t, "datagram", e.Completeness)
			require.LessOrEqual(t, len(e.Raw), 64)
			require.NotEmpty(t, e.Error)
			require.Zero(t, e.FlowID)
		})
	}
}

// A borrowed decoder payload must not be retained even when callers mutate the
// datagram immediately after the callback. It remains unrecognized under edits.
func TestUnrecognizedUDPOwnershipAndImmutableAdmission(t *testing.T) {
	config := &CaptureConfig{}
	var event *ProtocolEvent
	require.NoError(t, WithBinParser(func(e *ProtocolEvent) { event = e })(config))
	require.NoError(t, config.prepareBinParser())
	payload := bytes.Repeat([]byte{0x3f}, 1200)
	wire := carrierTestIPv4(carrierTestUDP(payload, len(payload)+8))
	packet := gopacket.NewPacket(wire, layers.LayerTypeIPv4, gopacket.Default)
	udp := packet.Layer(layers.LayerTypeUDP).(*layers.UDP)
	config.binParser.datagram(packet, udp)
	require.NotNil(t, event)
	require.Equal(t, payload, event.Raw)
	for i := range udp.Payload {
		udp.Payload[i] = 0
	}
	require.Equal(t, payload, event.Raw)
	event.Protocol, event.Status = "dns", "decoded"
	event.Structured, event.Fields = map[string]any{"forged": true}, map[string]any{"forged": true}
	_, err := event.Decode()
	require.ErrorContains(t, err, "unrecognized")
}

func unrecognizedUDPBudgetEvent(t testing.TB, payload []byte) (*ProtocolEvent, BinParserStats) {
	t.Helper()
	config := &CaptureConfig{}
	var event *ProtocolEvent
	count := 0
	require.NoError(t, WithBinParserConfig(BinParserConfig{MaxMessageBytes: 128, MaxBufferedBytes: 128, ProbeBytes: 16, OnEvent: func(e *ProtocolEvent) { event = e; count++ }})(config))
	require.NoError(t, config.prepareBinParser())
	wire := carrierTestIPv4(carrierTestUDP(payload, len(payload)+8))
	packet := gopacket.NewPacket(wire, layers.LayerTypeIPv4, gopacket.Default)
	config.binParser.datagram(packet, packet.Layer(layers.LayerTypeUDP).(*layers.UDP))
	require.Equal(t, 1, count)
	require.NotNil(t, event)
	return event, config.binParser.stats()
}

func TestUnrecognizedUDPMessageBudgetPrecedesFullPayloadCopy(t *testing.T) {
	for _, size := range []int{16, 17, 64, 65, 127, 128, 129, 1200} {
		t.Run(fmt.Sprintf("bytes%d", size), func(t *testing.T) {
			payload := bytes.Repeat([]byte{0x3f}, size)
			event, stats := unrecognizedUDPBudgetEvent(t, payload)
			require.Equal(t, uint64(size), stats.InputBytes)
			require.Zero(t, stats.Messages)
			require.Zero(t, stats.MessageBytes)
			require.Zero(t, stats.Flows)
			require.Zero(t, stats.BufferedBytes)
			require.Empty(t, event.Protocol)
			require.Equal(t, size, event.Length)
			if size <= 128 {
				require.Equal(t, payload, event.Raw)
				require.Equal(t, "datagram", event.Completeness)
				require.Equal(t, "unrecognized", event.Status)
				require.Equal(t, uint64(1), stats.Unknown)
				require.Equal(t, uint64(size), stats.UnclassifiedBytes)
				require.Zero(t, stats.LimitedBytes)
			} else {
				require.Equal(t, payload[:16], event.Raw)
				require.Equal(t, "limited", event.Status)
				require.Equal(t, "limited", event.Completeness)
				require.Equal(t, uint64(size), stats.LimitedBytes)
				require.Zero(t, stats.Unknown)
				require.Zero(t, stats.UnclassifiedBytes)
			}
		})
	}
}

func FuzzUnrecognizedUDPDatagramSourceBudget(f *testing.F) {
	for _, n := range []int{0, 47, 48, 49, 112, 113} {
		f.Add(bytes.Repeat([]byte{0xa5}, n))
	}
	f.Fuzz(func(t *testing.T, tail []byte) {
		if len(tail) > 128 {
			return
		}
		// Keep the transport payload outside the admitted application profiles
		// while varying every preserved byte after the opaque common prefix.
		payload := append(bytes.Repeat([]byte{0x3f}, 16), tail...)
		event, stats := unrecognizedUDPBudgetEvent(t, payload)
		require.Empty(t, event.Protocol)
		require.Zero(t, stats.Messages)
		require.Zero(t, stats.Flows)
		require.Zero(t, stats.BufferedBytes)
		require.Equal(t, len(payload), event.Length)
		if len(payload) <= 128 {
			require.Equal(t, payload, event.Raw)
			require.Equal(t, "datagram", event.Completeness)
			require.Equal(t, "unrecognized", event.Status)
			_, err := event.Decode()
			require.ErrorContains(t, err, "unrecognized")
		} else {
			require.Equal(t, payload[:16], event.Raw)
			require.Equal(t, "limited", event.Status)
		}
	})
}
