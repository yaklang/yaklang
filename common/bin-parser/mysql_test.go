package bin_parser

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

// MySQL HandshakeV10: https://dev.mysql.com/doc/dev/mysql-server/latest/page_protocol_connection_phase_packets_protocol_handshake_v10.html
func mysqlHandshakeV10(version string, connID uint32) []byte {
	payload := []byte{0x0a}
	payload = append(payload, version...)
	payload = append(payload, 0x00)
	id := make([]byte, 4)
	binary.LittleEndian.PutUint32(id, connID)
	payload = append(payload, id...)
	payload = append(payload, 1, 2, 3, 4, 5, 6, 7, 8) // auth-plugin-data-part-1
	payload = append(payload, 0x00)                   // filler
	payload = append(payload, 0xff, 0xf7)             // cap low
	payload = append(payload, 33)                     // charset utf8
	payload = append(payload, 0x02, 0x00)             // status
	payload = append(payload, 0x08, 0x00)             // cap high
	payload = append(payload, 21)                     // auth data len
	payload = append(payload, make([]byte, 10)...)    // reserved
	payload = append(payload, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 0)
	payload = append(payload, []byte("mysql_native_password")...)
	payload = append(payload, 0x00)
	return mysqlPacket(0, payload)
}

func mysqlPacket(seq byte, payload []byte) []byte {
	h := make([]byte, 4)
	h[0] = byte(len(payload))
	h[1] = byte(len(payload) >> 8)
	h[2] = byte(len(payload) >> 16)
	h[3] = seq
	return append(h, payload...)
}

func TestMySQLHandshakeV10(t *testing.T) {
	raw := mysqlHandshakeV10("5.7.29", 11)
	pkt := parseRule(t, raw, "application-layer.mysql", "MySQLPacket")
	require.Equal(t, uint64(len(raw)-4), uintVal(t, pkt.Child("Payload Length")))
	require.Equal(t, uint64(0), uintVal(t, pkt.Child("Sequence ID")))
	payload := mustChild(t, pkt, "Payload")
	require.Equal(t, uint64(0x0a), uintVal(t, payload.Child("First")))
	hs := mustChild(t, payload, "Handshake")
	require.Equal(t, "5.7.29", strVal(t, hs.Child("Server Version")))
	require.Equal(t, uint64(11), uintVal(t, hs.Child("Connection ID")))
	require.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8}, bytesVal(t, hs.Child("Auth Plugin Data 1")))
	require.Equal(t, uint64(33), uintVal(t, hs.Child("Character Set")))
	require.Equal(t, []byte{9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 0}, bytesVal(t, hs.Child("Auth Plugin Data 2")))
	require.Equal(t, "mysql_native_password", strVal(t, hs.Child("Auth Plugin Name")))

	eth := parseEthernet(t, ipv4TCPFrame(t, 3306, 50000, raw))
	wired := mustChild(t, eth, "IP", "TCP", "MySQL")
	require.Equal(t, "5.7.29", strVal(t, mustChild(t, wired, "Payload", "Handshake", "Server Version")))
}

func TestMySQLQueryCommand(t *testing.T) {
	raw := mysqlPacket(0, append([]byte{0x03}, []byte("SELECT 1")...))
	pkt := parseRule(t, raw, "application-layer.mysql", "MySQLPacket")
	payload := mustChild(t, pkt, "Payload")
	require.Equal(t, uint64(0x03), uintVal(t, payload.Child("First")))
	require.Equal(t, []byte("SELECT 1"), bytesVal(t, payload.Child("Query")))
}

func TestMySQLERRPacket(t *testing.T) {
	// ERR: 0xff, error code 1045 (0x0415 LE), rest
	raw := mysqlPacket(1, []byte{0xff, 0x15, 0x04, '#', '2', '8', '0', '0', '0', 'A', 'c', 'c', 'e', 's', 's', ' ', 'd', 'e', 'n', 'i', 'e', 'd'})
	pkt := parseRule(t, raw, "application-layer.mysql", "MySQLPacket")
	errp := mustChild(t, pkt, "Payload", "ERR")
	require.Equal(t, uint64(1045), uintVal(t, errp.Child("Error Code")))
}
