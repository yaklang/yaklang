package pcaputil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Pin every claimed application label and status on the actual ReplayPcapFile
// path. One valid event must not hide another protocol's false positive or a
// same-protocol malformed/incomplete event. The original ClickHouse lab capture
// is the one exception: its server Hello omits the required packet type, so a
// strict decoder reports malformed rather than inventing a server Hello.
func TestWinlab5013AutomaticRecognitionMatrix(t *testing.T) {
	expected := map[string]map[string]int{
		"01-socks5.pcapng":         {"socks5:decoded": 4, "http:decoded": 2},
		"02-finger.pcapng":         {"finger:decoded": 4},
		"03-whois.pcapng":          {"whois:decoded": 4},
		"04-gopher.pcapng":         {"gopher:decoded": 4},
		"05-dict.pcapng":           {"dict:decoded": 10},
		"06-bjnp.pcapng":           {"bjnp:decoded": 8},
		"07-zookeeper.pcapng":      {"zookeeper:decoded": 10},
		"08-mqttsn.pcapng":         {"mqtt-sn:decoded": 9},
		"09-turn.pcapng":           {"turn:decoded": 10},
		"10-caldav.pcapng":         {"http:decoded": 8},
		"11-carddav.pcapng":        {"http:decoded": 6},
		"12-scgi.pcapng":           {"scgi:decoded": 4},
		"13-hessian2.pcapng":       {"http:decoded": 4},
		"14-msgpack-rpc.pcapng":    {"msgpack-rpc:decoded": 5},
		"15-clickhouse.pcapng":     {"clickhouse:decoded": 2, "clickhouse:malformed": 1},
		"16-bittorrent-dht.pcapng": {"bittorrent-dht:decoded": 8},
		"17-ms-bits.pcapng":        {"http:decoded": 8},
		"18-consul.pcapng":         {"http:decoded": 8},
		"19-gearman.pcapng":        {"gearman:decoded": 11},
		"20-beanstalkd.pcapng":     {"beanstalkd:decoded": 6},
		"blue-01-stratum.pcapng":   {"stratum:decoded": 7, "dns:decoded": 2},
		"blue-02-beacon.pcapng":    {"http:decoded": 2},
		"ctf-01-dns.pcapng":        {"dns:decoded": 8},
		"ctf-01-ftp.pcapng":        {"ftp:decoded": 14},
		"ics-01-modbus.pcapng":     {"modbus:decoded": 8},
		"ics-02-bacnet.pcapng":     {"bacnet:decoded": 6},
		"ics-03-enip-cip.pcapng":   {"enip:decoded": 6},
		"power-01-iec104.pcapng":   {"iec104:decoded": 7},
		"power-02-goose.pcapng":    {"goose:decoded": 2},
		"power-03-c37118.pcapng":   {"c37118:decoded": 3},
	}
	require.Len(t, expected, 30)
	dir := filepath.Join("..", "..", "bin-parser", "testdata", "winlab5013", "captures")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, len(expected))
	for _, entry := range entries {
		entry := entry
		t.Run(entry.Name(), func(t *testing.T) {
			want, ok := expected[entry.Name()]
			require.True(t, ok, "untracked capture")
			got := make(map[string]int)
			for _, event := range replayWinlab5013Protocols(t, entry.Name()) {
				if event.Protocol == "" {
					continue
				}
				got[event.Protocol+":"+event.Status]++
			}
			require.Equal(t, want, got, "wrong protocol claim, extra status, or dropped message")
		})
	}
}
