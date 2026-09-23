package pcaputil

import (
	"bytes"
	"encoding/binary"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func enipTestMessage(command uint16, session uint32, status uint32, body []byte) []byte {
	wire := make([]byte, enipHeaderSize+len(body))
	binary.LittleEndian.PutUint16(wire[0:2], command)
	binary.LittleEndian.PutUint16(wire[2:4], uint16(len(body)))
	binary.LittleEndian.PutUint32(wire[4:8], session)
	binary.LittleEndian.PutUint32(wire[8:12], status)
	copy(wire[enipHeaderSize:], body)
	return wire
}

func enipTestRegisterBody() []byte { return []byte{1, 0, 0, 0} }

func replayENIPTestStream(t *testing.T, client, server []byte) []*ProtocolEvent {
	t.Helper()
	steps := []tcpStep{{seq: 99, syn: true}, {seq: 199, syn: true, reverse: true}}
	if len(client) > 0 {
		steps = append(steps, tcpStep{seq: 100, data: string(client)})
	}
	if len(server) > 0 {
		steps = append(steps, tcpStep{seq: 200, data: string(server), reverse: true})
	}
	steps = append(steps, tcpStep{seq: 100 + uint32(len(client)), fin: true}, tcpStep{seq: 200 + uint32(len(server)), fin: true, reverse: true})
	return replayENIPTestSteps(t, steps)
}

func replayENIPTestSteps(t *testing.T, steps []tcpStep) []*ProtocolEvent {
	t.Helper()
	var events []*ProtocolEvent
	err := ReplayPcap(
		bytes.NewReader(binTestPcap(t, steps, 44818, false, true)),
		WithTCPReassemblyWorkers(1),
		WithOnProtocolMessage(func(event *ProtocolEvent) { events = append(events, event) }),
	)
	require.NoError(t, err)
	return events
}

func TestReplayPcapFileDecodesWinlabENIPCIPReadAndWrite(t *testing.T) {
	path := filepath.Join("..", "..", "bin-parser", "testdata", "winlab5013", "captures", "ics-03-enip-cip.pcapng")
	var events []*ProtocolEvent
	err := ReplayPcapFile(path, WithOnProtocolMessage(func(event *ProtocolEvent) {
		if event.Protocol == "enip" {
			events = append(events, event)
		}
	}))
	require.NoError(t, err)
	require.Len(t, events, 6, "RegisterSession and all SendRRData messages must be detected from the captured TCP stream")

	wantRoles := []string{"RegisterSession Request", "RegisterSession Response", "", "", "", ""}
	wantCommands := []uint16{enipCommandRegister, enipCommandRegister, enipCommandSendRRData, enipCommandSendRRData, enipCommandSendRRData, enipCommandSendRRData}
	for i, event := range events {
		require.Equal(t, "decoded", event.Status, "%s: %s", event.Summary, event.Error)
		require.Equal(t, "tcp", event.Transport)
		require.Equal(t, "ethernet-ip-cip-explicit", event.Profile)
		require.Equal(t, wantCommands[i], event.Fields["Command Code"])
		require.Equal(t, uint32(0), event.Fields["Status"])
		require.Equal(t, len(event.Raw), event.Length)
		if wantRoles[i] != "" {
			require.Equal(t, wantRoles[i], event.Fields["Role"])
		}
	}
	require.Equal(t, uint32(0), events[0].Fields["Session Handle"])
	for _, event := range events[1:] {
		require.Equal(t, uint32(0x5013), event.Fields["Session Handle"])
	}
	require.Equal(t, "Speed", events[2].Fields["Tag"])
	require.Equal(t, uint8(enipCIPReadTag), events[2].Fields["CIP Service Code"])
	require.Equal(t, uint16(1), events[2].Fields["Element Count"])
	require.Equal(t, "Speed", events[3].Fields["Tag"])
	require.Equal(t, "DINT", events[3].Fields["Data Type"])
	require.Equal(t, int32(1500), events[3].Fields["Read Value"])
	require.Equal(t, "Speed", events[4].Fields["Tag"])
	require.Equal(t, uint8(enipCIPWriteTag), events[4].Fields["CIP Service Code"])
	require.Equal(t, "DINT", events[4].Fields["Data Type"])
	require.Equal(t, int32(1510), events[4].Fields["Write Value"])
	require.Equal(t, "Speed", events[5].Fields["Tag"])
	require.Equal(t, int32(1510), events[5].Fields["Write Value"])
	require.Equal(t, true, events[5].Fields["Write Confirmed"])
	for _, event := range events {
		decoded, err := event.Decode()
		require.NoError(t, err)
		require.Equal(t, event.Fields, decoded["fields"])
	}
}

func TestReplayPcapRejectsENIPNearMatchesAndBadSessionBoundaries(t *testing.T) {
	validRequest := enipTestMessage(enipCommandRegister, 0, 0, enipTestRegisterBody())
	nearMatches := map[string][]byte{}
	wrongLength := append([]byte(nil), validRequest...)
	binary.LittleEndian.PutUint16(wrongLength[2:4], 5)
	nearMatches["declared-length"] = wrongLength
	wrongSession := append([]byte(nil), validRequest...)
	binary.LittleEndian.PutUint32(wrongSession[4:8], 1)
	nearMatches["request-session"] = wrongSession
	wrongStatus := append([]byte(nil), validRequest...)
	binary.LittleEndian.PutUint32(wrongStatus[8:12], 1)
	nearMatches["encapsulation-status"] = wrongStatus
	wrongOptions := append([]byte(nil), validRequest...)
	binary.LittleEndian.PutUint32(wrongOptions[20:24], 1)
	nearMatches["options"] = wrongOptions
	wrongVersion := enipTestMessage(enipCommandRegister, 0, 0, []byte{2, 0, 0, 0})
	nearMatches["protocol-version"] = wrongVersion
	nearMatches["truncated-register"] = validRequest[:len(validRequest)-1]

	for name, sample := range nearMatches {
		t.Run(name, func(t *testing.T) {
			for _, event := range replayENIPTestStream(t, sample, nil) {
				require.NotEqual(t, "enip", event.Protocol, "%s: %s", event.Status, event.Error)
			}
		})
	}

	t.Run("response-without-valid-session-handle", func(t *testing.T) {
		badResponse := enipTestMessage(enipCommandRegister, 0, 0, enipTestRegisterBody())
		events := replayENIPTestStream(t, validRequest, badResponse)
		var enipEvents []*ProtocolEvent
		for _, event := range events {
			if event.Protocol == "enip" {
				enipEvents = append(enipEvents, event)
			}
		}
		require.Len(t, enipEvents, 2)
		require.Equal(t, "decoded", enipEvents[0].Status)
		require.Equal(t, "context-required", enipEvents[1].Status)
		require.NotContains(t, enipEvents[1].Fields, "Session Handle")
	})
}

func TestReplayPcapMarksENIPMalformedSendRRDataCPF(t *testing.T) {
	request := enipTestMessage(enipCommandRegister, 0, 0, enipTestRegisterBody())
	response := enipTestMessage(enipCommandRegister, 0x5013, 0, enipTestRegisterBody())
	badCPF := make([]byte, 8)
	binary.LittleEndian.PutUint32(badCPF[0:4], 0)
	binary.LittleEndian.PutUint16(badCPF[4:6], 10)
	binary.LittleEndian.PutUint16(badCPF[6:8], 1)
	message := enipTestMessage(enipCommandSendRRData, 0x5013, 0, badCPF)
	steps := []tcpStep{
		{seq: 99, syn: true},
		{seq: 199, syn: true, reverse: true},
		{seq: 100, data: string(request)},
		{seq: 200, data: string(response), reverse: true},
		{seq: 100 + uint32(len(request)), data: string(message)},
		{seq: 100 + uint32(len(request)+len(message)), fin: true},
		{seq: 200 + uint32(len(response)), fin: true, reverse: true},
	}
	events := replayENIPTestSteps(t, steps)
	var enipEvents []*ProtocolEvent
	for _, event := range events {
		if event.Protocol == "enip" {
			enipEvents = append(enipEvents, event)
		}
	}
	require.Len(t, enipEvents, 3)
	require.Equal(t, "decoded", enipEvents[0].Status)
	require.Equal(t, "decoded", enipEvents[1].Status)
	require.Equal(t, "malformed", enipEvents[2].Status)
	require.Contains(t, enipEvents[2].Error, "exactly two CPF items")
}

func TestENIPProbeRequiresACompleteValidRegisterRequest(t *testing.T) {
	valid := enipTestMessage(enipCommandRegister, 0, 0, enipTestRegisterBody())
	require.Equal(t, ProbeAccept, probeENIP(valid).Verdict)
	for _, sample := range [][]byte{valid[:enipHeaderSize], valid[:len(valid)-1], []byte("not Ethernet/IP")} {
		require.Equal(t, ProbeReject, probeENIP(sample).Verdict)
	}
}
