package bin_parser

import (
	"encoding/binary"
	"testing"
)

func TestDHCPFailPaths(t *testing.T) {
	parseMustFail(t, nil, "application-layer.dhcp", "DHCP")
	parseMustFail(t, []byte{3}, "application-layer.dhcp", "DHCP")
	parseMustFail(t, []byte{1, 1}, "application-layer.dhcp", "DHCP")
	badCookie := make([]byte, 240)
	badCookie[0] = 1
	badCookie[1] = 1
	badCookie[2] = 6
	binary.BigEndian.PutUint32(badCookie[236:], 0x00000000)
	parseMustFail(t, badCookie, "application-layer.dhcp", "DHCP")
}

func TestMySQLFailPaths(t *testing.T) {
	parseMustFail(t, nil, "application-layer.mysql", "MySQLPacket")
	parseMustFail(t, []byte{0x01, 0x00, 0x00}, "application-layer.mysql", "MySQLPacket")
	// Payload length 8 but no bytes follow.
	parseMustFail(t, []byte{0x08, 0x00, 0x00, 0x00}, "application-layer.mysql", "MySQLPacket")
}

func TestRedisFailPaths(t *testing.T) {
	parseMustFail(t, nil, "application-layer.redis", "Redis")
	parseMustFail(t, []byte{'X'}, "application-layer.redis", "Redis")
	parseMustFail(t, []byte{'X', '\r', '\n'}, "application-layer.redis", "Redis")
}

func TestSOCKS5FailPaths(t *testing.T) {
	parseMustFail(t, nil, "application-layer.socks5", "ClientNegotiation")
	parseMustFail(t, []byte{0x05}, "application-layer.socks5", "ClientNegotiation")
	parseMustFail(t, []byte{0x05, 0x01}, "application-layer.socks5", "Request")
}

func TestHTTPFailPaths(t *testing.T) {
	parseMustFail(t, nil, "application-layer.http", "HTTP")
	parseMustFail(t, []byte{'G'}, "application-layer.http", "HTTP")
	parseMustFail(t, []byte("GET /"), "application-layer.http", "HTTP")
}

func TestDNSFailPaths(t *testing.T) {
	parseMustFail(t, nil, "application-layer.dns", "DNS")
	parseMustFail(t, []byte{0x00, 0x01, 0x01, 0x00}, "application-layer.dns", "DNS")
	parseMustFail(t, []byte{0x00}, "application-layer.dns", "DNS")
}

func TestSNMPFailPaths(t *testing.T) {
	parseMustFail(t, nil, "application-layer.snmp", "SNMP")
	parseMustFail(t, []byte{0x31, 0x03, 0x02, 0x01, 0x00}, "application-layer.snmp", "SNMP")
	parseMustFail(t, []byte{0x30, 0x20}, "application-layer.snmp", "SNMP")
}

func TestNBSSFailPaths(t *testing.T) {
	parseMustFail(t, nil, "application-layer.nbss", "NBSS")
	parseMustFail(t, []byte{0x99, 0x00, 0x00, 0x04}, "application-layer.nbss", "NBSS")
	parseMustFail(t, []byte{0x00, 0x00, 0x00, 0x10, 0x00}, "application-layer.nbss", "NBSS")
}

func TestVLANFailPaths(t *testing.T) {
	parseMustFail(t, nil, "ieee_802_1q", "IEEE 802.1Q")
	parseMustFail(t, []byte{0x00}, "ieee_802_1q", "IEEE 802.1Q")
	parseMustFail(t, []byte{0x00, 0x64}, "ieee_802_1q", "IEEE 802.1Q")
}
