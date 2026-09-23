package pcaputil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMQTTSNMessageStructure(t *testing.T) {
	connect := append([]byte{0x0c, 0x04, 0x04, 0x01, 0x00, 0x3c}, []byte("winlab")...)
	fields, err := decodeMQTTSNMessage(connect, 0)
	require.NoError(t, err)
	require.Equal(t, "CONNECT", fields["Message Type"])
	require.Equal(t, "winlab", fields["Client ID"])
	require.Equal(t, uint16(60), fields["Duration"])

	publish := []byte{0x0b, 0x0c, 0x20, 0x00, 0x01, 0x00, 0x02, '2', '3', '.', '5'}
	fields, err = decodeMQTTSNMessage(publish, 0)
	require.NoError(t, err)
	require.Equal(t, "PUBLISH", fields["Message Type"])
	require.Equal(t, uint8(1), fields["QoS"])
	require.Equal(t, uint16(1), fields["Topic ID"])
	require.Equal(t, []byte("23.5"), fields["Data"])

	// Three-byte Length is an independent v1.2 encoding, not a second frame.
	extended := []byte{1, 0, 6, 0x18, 0, 60}
	fields, err = decodeMQTTSNMessage(extended, 0)
	require.NoError(t, err)
	require.Equal(t, uint16(60), fields["Duration"])

	for _, wire := range [][]byte{
		nil, {1, 0}, {1, 0, 6, 0x18, 0},
		{4, 1, 1},       // declared length mismatch
		{3, 3, 1},       // reserved message type
		{3, 0x01, 1, 1}, // trailing bytes after SEARCHGW
		{3, 0x05, 4},    // invalid CONNACK code
		{2, 0x04},       // truncated CONNECT
		{3, 0x18, 0},    // incomplete optional DISCONNECT duration
	} {
		require.Falsef(t, validMQTTSNMessage(wire), "%x", wire)
	}
}

func TestWinlab5013MQTTSNAllDatagrams(t *testing.T) {
	events := replayWinlab5013Protocols(t, "08-mqttsn.pcapng")
	require.Len(t, events, 9)
	kinds := []string{"SEARCHGW", "GWINFO", "CONNECT", "CONNACK", "REGISTER", "REGACK", "PUBLISH", "PUBACK", "DISCONNECT"}
	for i, event := range events {
		require.Equal(t, "mqtt-sn", event.Protocol, "message %d", i)
		require.Equal(t, "decoded", event.Status, "message %d: %s", i, event.Error)
		require.Equal(t, kinds[i], event.Fields["Message Type"], "message %d", i)
	}
	require.Equal(t, "winlab", events[2].Fields["Client ID"])
	require.Equal(t, "lab/temp", events[4].Fields["Topic Name"])
	require.Equal(t, uint16(1), events[6].Fields["Topic ID"])
	require.Equal(t, uint8(1), events[6].Fields["QoS"])
	require.Equal(t, []byte("23.5"), events[6].Fields["Data"])
}
