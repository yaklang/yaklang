package pcaputil

import (
	"bytes"
	"encoding/binary"
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
	require.NoError(t, ReplayPcapFile(path, WithOnProtocolMessage(func(event *ProtocolEvent) {
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
