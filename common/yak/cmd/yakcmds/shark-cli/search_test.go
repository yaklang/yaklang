package sharkcli

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/pcap"
)

func searchFixture(t *testing.T) *tui {
	u := &tui{ring: newPacketRing(20000), width: 180, height: 50, follow: true}
	packets := []*capturedPacket{
		{number: 1, data: dnsPacket(t), link: layers.LinkTypeEthernet},
		tcpPacket(t, 2, 1, "GET /search HTTP/1.1\r\nHost: Example.test\r\n\r\nHello World", false, false, false, false),
		{number: 3, data: udpPacket(t, 9000, []byte{'S', 'e', 'c', 'r', 'e', 't', 0, 0xff}), link: layers.LinkTypeEthernet},
		tcpPacket(t, 4, 1, "\x00\xff\xaa\x80", false, false, false, false),
	}
	for i, p := range packets {
		if i < 3 {
			copy(p.data[26:30], net.ParseIP([]string{"10.223.4.5", "10.223.4.6", "10.223.5.66"}[i]).To4())
		}
		p.ci = gopacket.CaptureInfo{CaptureLength: len(p.data), Length: len(p.data)}
		if i == 3 {
			binary.BigEndian.PutUint16(p.data[36:38], 8080)
			p.application, p.evidence = "HTTP", "port hint"
		}
		u.ring.add(p)
	}
	u.refreshVisible()
	return u
}
func visibleNumbers(u *tui) []uint64 {
	ids := []uint64{}
	for _, e := range u.visible {
		ids = append(ids, e.packet.number)
	}
	return ids
}
func TestSearchProtocolPartialExactCIDRPortsAndContent(t *testing.T) {
	u := searchFixture(t)
	for _, test := range []struct {
		query string
		want  []uint64
	}{
		{"10.223", []uint64{1, 2, 3}}, {"ip:10.223", []uint64{1, 2, 3}}, {"ip.str:223.4", []uint64{1, 2}},
		{"src=10.223.4.5", []uint64{1}}, {"ip=10.223", []uint64{}}, {"ip:10.223.4.0/24", []uint64{1, 2}},
		{"proto:http", []uint64{2}}, {"proto=HTTP", []uint64{2}}, {"proto:tcp !proto:http", []uint64{4}},
		{"proto:ipv4", []uint64{1, 2, 3, 4}}, {"proto:ethernet", []uint64{1, 2, 3, 4}},
		{"proto:udp ip:10.223", []uint64{1, 3}}, {"proto:dns OR proto:http", []uint64{1, 2}},
		{"proto:dns | proto:http ip=10.223.4.6", []uint64{1, 2}}, {"ip!=10.223.4.5", []uint64{2, 3, 4}},
		{"content:\"Hello World\"", []uint64{2}}, {"content:hello", []uint64{}}, {"text:hello", []uint64{2}},
		{"HELLO", []uint64{2}}, {"hex:\"53 65 63 72 65 74 00 ff\"", []uint64{3}}, {"hex:00ffaa80", []uint64{4}},
		{"proto:tcp port:8080,12011", []uint64{2, 4}}, {"dport:8999-9001", []uint64{3}}, {"sport:9000", []uint64{}},
		{"info:\"GET /search\"", []uint64{2}}, {"proto:http AND text:HOST", []uint64{2}}, {"", []uint64{1, 2, 3, 4}},
	} {
		t.Run(test.query, func(t *testing.T) {
			require.NoError(t, u.applySearch(test.query))
			u.refreshVisible()
			require.Equal(t, test.want, visibleNumbers(u))
		})
	}
	// Search intersects BPF; clearing one leaves the other intact.
	bpf, err := pcap.NewBPF(layers.LinkTypeEthernet, 65535, "udp")
	require.NoError(t, err)
	u.displayFilter = bpf
	require.NoError(t, u.applySearch("ip:10.223"))
	u.refreshVisible()
	require.Equal(t, []uint64{1, 3}, visibleNumbers(u))
	require.NoError(t, u.applySearch(""))
	u.refreshVisible()
	require.Equal(t, []uint64{1, 3}, visibleNumbers(u))
}

func TestSearchInvalidInputPreservesPreviousQuery(t *testing.T) {
	u := searchFixture(t)
	require.NoError(t, u.applySearch("proto:dns"))
	u.refreshVisible()
	for _, q := range []string{"ip:", "hex:0", "hex:zz", "text:\"unclosed", "port:90000", "port:80-20", "ip:10.1/999", "foo OR", "| foo", strings.Repeat("x ", 65)} {
		u.openSearch()
		u.input = q
		u.key("\r", nil)
		u.refreshVisible()
		require.Equal(t, "proto:dns", u.searchText, q)
		require.Equal(t, []uint64{1}, visibleNumbers(u), q)
		require.Contains(t, u.notice, "Search error:")
		require.Equal(t, "Search", u.editor)
	}
	u.key("\x1b", nil)
	require.Empty(t, u.editor)
	require.Equal(t, "proto:dns", u.searchText)
	// IPv6 and URL literals are not mistaken for unknown field names.
	q, err := compileSearch(`fe80::1 "http://example.test" content:"one | two"`)
	require.NoError(t, err)
	require.Len(t, q.groups, 1)
	require.Len(t, q.groups[0], 3)
	require.Equal(t, "one | two", string(q.groups[0][2].needle))
}

func TestSearchStreamReassemblyRefreshesFrozenRowsAndRespectsGaps(t *testing.T) {
	s := newStreamStore(8, 1024)
	u := &tui{ring: newPacketRing(10), streams: s, follow: true}
	p := tcpPacket(t, 1, 100, "", false, true, false, false)
	s.add(p)
	u.ring.add(p)
	first := "GET /hello"
	p = tcpPacket(t, 2, 101, first, false, false, false, false)
	s.add(p)
	u.ring.add(p)
	u.refreshVisible()
	u.freezeList()
	require.NoError(t, u.applySearch(`stream:"hello-world" proto:http`))
	u.refreshVisible()
	require.Empty(t, u.visible)
	// The new segment is not in the frozen packet list, but its connection is.
	p = tcpPacket(t, 3, 101+uint32(len(first)), "-world HTTP/1.1\r\nHost: test\r\n\r\n", false, false, false, false)
	s.add(p)
	u.ring.add(p)
	u.refreshVisible()
	require.Equal(t, []uint64{1, 2}, visibleNumbers(u))
	require.NoError(t, u.applySearch(`content:"hello-world"`))
	u.refreshVisible()
	require.Empty(t, u.visible, "single-packet content must not claim a cross-segment match")
	for _, v := range []*streamSnapshot{
		{chunks: []streamChunk{{direction: 0, data: []byte("hello "), offset: 0}, {direction: 0, data: []byte("world"), offset: 7, gap: 1}}},
		{chunks: []streamChunk{{direction: 0, data: []byte("hello ")}, {direction: 1, data: []byte("world")}}},
		{prefix: [2][]byte{[]byte("hello "), []byte("world")}},
		{prefix: [2][]byte{[]byte("hello ")}, chunks: []streamChunk{{direction: 0, data: []byte("world"), offset: 100}}},
	} {
		require.False(t, streamContains(v, []byte("hello world")))
	}
	v := &streamSnapshot{chunks: []streamChunk{{data: []byte("hello ")}, {data: []byte("wo"), offset: 6}, {data: []byte("rld"), offset: 8}}}
	require.True(t, streamContains(v, []byte("HELLO world")))
}

func TestClickableSearchCacheResizeAndCacheRollover(t *testing.T) {
	u := searchFixture(t)
	input, clear, cache := searchControls(u.width - 1)
	u.mouse(mouseEvent{action: mousePress, button: 0, x: input.x + 3, y: input.y})
	require.Equal(t, "Search", u.editor)
	u.input = "proto:dns"
	u.key("\r", nil)
	u.refreshVisible()
	require.Equal(t, []uint64{1}, visibleNumbers(u))
	u.mouse(mouseEvent{action: mousePress, button: 0, x: clear.x + 2, y: clear.y})
	u.refreshVisible()
	require.Len(t, u.visible, 4)
	u.freezeList()
	u.mouse(mouseEvent{action: mousePress, button: 0, x: cache.x + 2, y: cache.y})
	require.Equal(t, "Cache", u.editor)
	u.input = "2"
	u.key("\r", nil)
	u.refreshVisible()
	require.Equal(t, 2, len(u.ring.entries))
	require.Equal(t, uint64(2), u.ring.evicted)
	require.Equal(t, []uint64{3, 4}, visibleNumbers(u))
	require.Len(t, u.frozen, 2)
	u.openCache()
	u.input = "2w"
	u.key("\r", nil)
	require.Equal(t, 20000, len(u.ring.entries))
	require.Equal(t, 2, u.ring.size)
	for _, invalid := range []string{"0", "100001", "99999999999999999999", "-1", "0w"} {
		require.Error(t, u.resizePacketCache(invalid))
		require.Equal(t, 20000, len(u.ring.entries))
	}
	// A reused ring slot cannot keep the previous packet's cached search result.
	u.resumeLive()
	require.NoError(t, u.resizePacketCache("1"))
	require.NoError(t, u.applySearch("content:Secret"))
	u.refreshVisible()
	require.Empty(t, u.visible)
	p := &capturedPacket{number: 9, data: udpPacket(t, 9999, []byte("Secret")), link: layers.LinkTypeEthernet}
	u.ring.add(p)
	u.refreshVisible()
	require.Equal(t, []uint64{9}, visibleNumbers(u))
	u.ring.add(&capturedPacket{number: 10, data: dnsPacket(t), link: layers.LinkTypeEthernet})
	u.refreshVisible()
	require.Empty(t, u.visible)
}

func BenchmarkSearch20000CachedPackets(b *testing.B) {
	u := &tui{ring: newPacketRing(20000), follow: true}
	for i := 0; i < 20000; i++ {
		u.ring.add(&capturedPacket{number: uint64(i + 1), data: []byte(fmt.Sprintf("payload %d Hello World", i)), link: layers.LinkTypeRaw})
	}
	_ = u.applySearch("text:hello")
	u.refreshVisible()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		u.filterDirty = true
		u.refreshVisible()
	}
}

func TestFoldMatcherRepeatedPrefixesAndBinaryValues(t *testing.T) {
	for _, pair := range [][2]string{
		{"aaaAaAaAb", "AAAAAb"}, {"abcabcabdabcd", "abcabd"}, {"\x00\xffHeLLo\x00", "\xffhello"},
		{strings.Repeat("a", 65536), strings.Repeat("a", 4095) + "b"},
		{"你好 World", "你好 world"}, {"aaa", "aaaa"}, {"", ""},
	} {
		require.Equal(t, strings.Contains(strings.ToLower(pair[0]), strings.ToLower(pair[1])), containsFold([]byte(pair[0]), []byte(pair[1])))
	}
}
