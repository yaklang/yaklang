package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func bacnetTestPacket(function uint8, npduAPDU []byte) []byte {
	wire := make([]byte, 4+len(npduAPDU))
	wire[0], wire[1] = 0x81, function
	binary.BigEndian.PutUint16(wire[2:4], uint16(len(wire)))
	copy(wire[4:], npduAPDU)
	return wire
}

func TestReplayPcapFileRecognizesBACnetIP(t *testing.T) {
	path := filepath.Join("..", "..", "bin-parser", "testdata", "winlab5013", "captures", "ics-02-bacnet.pcapng")
	var events []*ProtocolEvent
	require.NoError(t, replayFixtureFile(t, path, WithOnProtocolMessage(func(event *ProtocolEvent) {
		if event.Protocol == "bacnet" {
			events = append(events, event)
		}
	})))
	require.Len(t, events, 6, "all six BACnet/IP datagrams must be recognized on the default capture path")

	wantFunctions := []uint8{0x0b, 0x0b, 0x0a, 0x0a, 0x0a, 0x0a}
	wantPDU := []string{"Unconfirmed-Request", "Unconfirmed-Request", "Confirmed-Request", "ComplexACK", "Confirmed-Request", "SimpleACK"}
	wantService := []uint8{8, 0, 12, 12, 15, 15}
	wantServiceName := []string{"Who-Is", "I-Am", "ReadProperty", "ReadProperty", "WriteProperty", "WriteProperty"}
	for i, event := range events {
		require.Equal(t, "decoded", event.Status, "%s: %s", event.Protocol, event.Error)
		require.Equal(t, "udp", event.Transport)
		require.Equal(t, "bacnet-ip-envelope", event.Profile)
		require.Equal(t, "wire-and-port-hint", event.Admission)
		require.Equal(t, "application-layer.extended_protocols", event.Rule)
		require.Equal(t, "BACnetIP", event.Entry)
		require.Equal(t, "captured", event.SourceBytes.Kind)
		require.Len(t, event.SourceBytes.PacketRefs, 1)
		require.Equal(t, event.Length, len(event.Raw), "Raw must retain the complete BVLC message")
		require.Equal(t, uint8(0x81), event.Fields["BVLC Type"])
		require.Equal(t, uint16(event.Length), event.Fields["BVLC Length"])
		require.Equal(t, wantFunctions[i], event.Fields["BVLC Function"])
		require.Equal(t, uint8(0x01), event.Fields["NPDU Version"])
		require.Equal(t, event.Raw[5], event.Fields["NPDU Control"])
		require.Equal(t, wantPDU[i], event.Fields["APDU Type"])
		require.Equal(t, wantService[i], event.Fields["Service Choice"])
		require.Equal(t, wantServiceName[i], event.Fields["Service"])
		require.Equal(t, event.Raw, event.Fields["Complete BVLC PDU"])
		require.Equal(t, event.Raw[6:], event.Fields["APDU"])
		require.Equal(t, wantService[i], event.Session["Service Choice"])
	}
}

func TestReplayPcapRejectsBACnetPortAndNearSignatureFrames(t *testing.T) {
	valid := bacnetTestPacket(0x0b, []byte{0x01, 0x00, 0x10, 0x08})
	badType := append([]byte(nil), valid...)
	badType[0] = 0x82
	badFunction := append([]byte(nil), valid...)
	badFunction[1] = 0xff
	badLength := append([]byte(nil), valid...)
	binary.BigEndian.PutUint16(badLength[2:4], uint16(len(badLength)+1))
	badNPDU := append([]byte(nil), valid...)
	badNPDU[4] = 0x02
	missingAPDU := bacnetTestPacket(0x0b, []byte{0x01, 0x00})
	truncatedConfirmed := bacnetTestPacket(0x0a, []byte{0x01, 0x04, 0x00, 0x05, 0x01})
	truncatedDestination := bacnetTestPacket(0x0b, []byte{0x01, 0x80, 0x00})
	randomOnBACnetPort := []byte("not BACnet even though UDP uses 47808")

	nearMatches := [][]byte{badType, badFunction, badLength, badNPDU, missingAPDU, truncatedConfirmed, truncatedDestination, randomOnBACnetPort}
	steps := make([]sessionStep, 0, len(nearMatches))
	for _, wire := range nearMatches {
		steps = append(steps, sessionStep{dir: 0, wire: wire})
	}
	var events []*ProtocolEvent
	err := ReplayPcap(bytes.NewReader(sessionDatagramPCAP(t, steps, layers.UDPPort(bacnetIPv4UDPPort))), WithOnProtocolMessage(func(event *ProtocolEvent) {
		events = append(events, event)
	}))
	require.NoError(t, err)
	require.Len(t, events, len(nearMatches))
	for _, event := range events {
		require.NotEqual(t, "bacnet", event.Protocol, "%s: %s", event.Status, event.Error)
		require.NotEqual(t, "decoded", event.Status)
	}
}
func TestBACnetNPDUCaptureBoundary(t *testing.T) {
	for _, ctrl := range []byte{0, 1, 2, 3, 4, 5, 6, 7} {
		t.Run(fmt.Sprintf("priority-%d", ctrl), func(t *testing.T) {
			apdu := []byte{0x10, 8}
			fn := byte(0x0b)
			service := byte(8)
			name := "Who-Is"
			typ := "Unconfirmed-Request"
			if ctrl&4 != 0 {
				apdu = []byte{0, 5, 36, 12, 12, 0, 128, 0, 1, 25, 85}
				fn = 10
				service = 12
				name = "ReadProperty"
				typ = "Confirmed-Request"
			}
			wire := bacnetTestPacket(fn, append([]byte{1, ctrl}, apdu...))
			var all []*ProtocolEvent
			require.NoError(t, ReplayPcap(bytes.NewReader(sessionDatagramPCAP(t, []sessionStep{{dir: 0, wire: wire}}, layers.UDPPort(bacnetIPv4UDPPort))), WithOnProtocolMessage(func(e *ProtocolEvent) { all = append(all, e) })))
			require.Len(t, all, 1)
			e := all[0]
			require.Equal(t, "bacnet", e.Protocol)
			require.Equal(t, "decoded", e.Status)
			require.Equal(t, wire, e.Raw)
			require.Equal(t, "bacnet-ip-envelope", e.Profile)
			require.Equal(t, "udp", e.Transport)
			require.Equal(t, ctrl, e.Fields["NPDU Control"])
			require.Equal(t, ctrl, e.Session["NPDU Control"])
			require.Equal(t, service, e.Fields["Service Choice"])
			require.Equal(t, name, e.Session["Service"])
			require.Equal(t, typ, e.Session["APDU Type"])
			require.Equal(t, wire[6:], e.Fields["APDU"])
			require.Equal(t, wire, e.Fields["Complete BVLC PDU"])
			require.Zero(t, e.ResponseTo)
			require.Zero(t, e.TransactionID)
		})
	}
	for name, npdu := range map[string][]byte{"source-address-trap": {1, 8, 0, 5, 1, 12, 16, 8}, "reserved4": {1, 16, 16, 8}, "reserved6": {1, 64, 16, 8}} {
		t.Run(name, func(t *testing.T) {
			wire := bacnetTestPacket(0x0b, npdu)
			var all []*ProtocolEvent
			require.NoError(t, ReplayPcap(bytes.NewReader(sessionDatagramPCAP(t, []sessionStep{{dir: 0, wire: wire}}, layers.UDPPort(bacnetIPv4UDPPort))), WithOnProtocolMessage(func(e *ProtocolEvent) { all = append(all, e) })))
			require.Len(t, all, 1)
			require.NotEqual(t, "bacnet", all[0].Protocol)
			require.NotEqual(t, "decoded", all[0].Status)
		})
	}
}
func TestBACnetAcceptedQueryOwnership(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprint(deferred), func(t *testing.T) {
			wire := bacnetTestPacket(11, []byte{1, 0, 16, 8})
			var all []*ProtocolEvent
			require.NoError(t, ReplayPcap(bytes.NewReader(sessionDatagramPCAP(t, []sessionStep{{dir: 0, wire: wire}}, 47808)), WithBinParserConfig(BinParserConfig{Deferred: deferred}), WithOnProtocolMessage(func(e *ProtocolEvent) { all = append(all, e) })))
			require.Len(t, all, 1)
			e := all[0]
			want := map[string]any{"BVLC Type": uint8(129), "BVLC Function": uint8(11), "BVLC Length": uint16(8), "NPDU Version": uint8(1), "NPDU Control": uint8(0), "APDU": []byte{16, 8}, "APDU Type": "Unconfirmed-Request", "Service Choice": uint8(8), "Service": "Who-Is", "Complete BVLC PDU": bytes.Clone(wire)}
			session := map[string]any{"BVLC Function": uint8(11), "NPDU Control": uint8(0), "APDU": []byte{16, 8}, "APDU Type": "Unconfirmed-Request", "Service Choice": uint8(8), "Service": "Who-Is"}
			fields, err := e.GetFields()
			require.NoError(t, err)
			require.Equal(t, want, fields)
			require.Equal(t, session, e.Session)
			fields["APDU"].([]byte)[0] = 255
			fields["Service"] = "CALLER"
			e.Fields = map[string]any{"CALLER": true}
			e.Structured = map[string]any{"fields": map[string]any{"CALLER": true}, "metadata": "CALLER"}
			e.Metadata = "CALLER"
			e.Session = map[string]any{"CALLER": true}
			clear(e.Raw)
			got, err := e.GetFields()
			require.NoError(t, err)
			require.Equal(t, want, got)
			result, err := e.Decode()
			require.NoError(t, err)
			require.Equal(t, want, protocolFields(result))
			require.Equal(t, map[string]any{"fields": want, "metadata": nil, "session": session}, result)
		})
	}
}
func TestUDPUnrecognizedCapturedQueryAuthority(t *testing.T) {
	wire := bacnetTestPacket(11, []byte{2, 0, 16, 8})
	var all []*ProtocolEvent
	require.NoError(t, ReplayPcap(bytes.NewReader(sessionDatagramPCAP(t, []sessionStep{{dir: 0, wire: wire}}, 47808)), WithOnProtocolMessage(func(e *ProtocolEvent) { all = append(all, e) })))
	require.Len(t, all, 1)
	e := all[0]
	require.Equal(t, "unrecognized", e.Status)
	require.Empty(t, e.Protocol)
	first, err := e.GetFields()
	require.Nil(t, first)
	require.EqualError(t, err, "protocol parser: event is unrecognized, not an exact message")
	e.Fields = map[string]any{"CALLER": true}
	e.Structured = map[string]any{"fields": e.Fields}
	e.Session = map[string]any{"CALLER": true}
	e.Status = "decoded"
	e.Protocol = "bacnet"
	e.Profile = "bacnet-ip-envelope"
	clear(e.Raw)
	fields, err := e.GetFields()
	require.Nil(t, fields)
	require.EqualError(t, err, "protocol parser: event is unrecognized, not an exact message")
	result, err := e.Decode()
	require.Nil(t, result)
	require.EqualError(t, err, "protocol parser: event is unrecognized, not an exact message")
}

func TestBACnetManualQueryCacheCompatibility(t *testing.T) {
	for _, protocol := range []string{"", "bacnet"} {
		e := &ProtocolEvent{Protocol: protocol, Transport: "udp", Status: "unrecognized", Fields: map[string]any{"Caller": true}}
		fields, err := e.GetFields()
		require.NoError(t, err)
		require.Equal(t, e.Fields, fields)
	}
}
