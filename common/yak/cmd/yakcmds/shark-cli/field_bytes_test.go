package sharkcli

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func opaqueIPv6Packet() *capturedPacket {
	data := make([]byte, 90)
	copy(data, []byte{0x26, 0x2b, 0x7c, 0x1a, 0xeb, 0x87, 0x80, 0xc0, 0x1e, 0x48, 0xe8, 0x79, 0x86, 0xdd})
	data[14], data[20], data[21] = 0x60, 0, 1 // IPv6, Hop-by-Hop, hop limit.
	binary.BigEndian.PutUint16(data[18:20], 36)
	copy(data[22:38], net.ParseIP("fe80::390f:2788:b159:92d3").To16())
	copy(data[38:54], net.ParseIP("ff02::16").To16())
	copy(data[54:], []byte{58, 0, 5, 2, 0, 0, 1, 0, 143, 0, 0x39, 0x18})
	return &capturedPacket{number: 1, data: data, link: layers.LinkTypeEthernet, ci: gopacket.CaptureInfo{Length: len(data), CaptureLength: len(data)}}
}

func findField(t *testing.T, d packetDetail, name string) field {
	t.Helper()
	for _, f := range d.fields {
		if strings.HasPrefix(f.text, name+":") {
			return f
		}
	}
	t.Fatalf("missing %s in %+v", name, d.fields)
	return field{}
}

func TestOpaqueIPv6FieldHexAndPacketOffsetsSurviveWorker(t *testing.T) {
	raw := opaqueIPv6Packet()
	// Keep this an opaque payload even when the branch learns new IPv6
	// extension/ICMPv6 dissectors. 253 is reserved for experimentation.
	raw.data[20] = 253
	req, err := json.Marshal(decodeRequest{Number: raw.number, Data: raw.data, Length: raw.ci.Length, Link: raw.link})
	require.NoError(t, err)
	var result bytes.Buffer
	require.NoError(t, runDecodeWorker(bytes.NewReader(req), &result))
	d, err := decodeWorkerOutput(result.Bytes(), raw)
	require.NoError(t, err)
	f := findField(t, d, "Next Protocol Data")
	require.Contains(t, f.text, "36 bytes · HEX 3a 00 05 02 00 00 01 00 8f 00 39 18")
	require.NotContains(t, f.text, "�")
	require.True(t, f.hasRange)
	require.Equal(t, 54, f.start)
	require.Equal(t, 90, f.end)
	src := findField(t, d, "Source") // First occurrence is Ethernet source.
	require.Equal(t, 6, src.start)
	require.Equal(t, 12, src.end)
	version := findField(t, d, "Version")
	require.True(t, version.hasRange)
	require.Equal(t, 14, version.start)
	require.Equal(t, 15, version.end, "a four-bit field highlights its containing byte")
	// The same IPv6 bytes after a loopback header must map into the actual frame.
	loop := *raw
	loop.link = layers.LinkTypeNull
	loop.data = append([]byte{30, 0, 0, 0}, raw.data[14:]...)
	loop.data[10] = 0 // IPv6 Hop-by-Hop extension precedes the opaque payload.
	loop.data[44] = 253
	loop.ci.Length = len(loop.data)
	f = findField(t, detail(&loop), "Next Protocol Data")
	require.True(t, f.hasRange)
	require.Equal(t, 52, f.start)
	require.Equal(t, 80, f.end)
}

func TestFieldValuesDistinguishBinaryFromReadableText(t *testing.T) {
	for _, v := range []any{[]byte{0, 255, 65, 27}, string([]byte{0, 255, 65, 27})} {
		s := formatFieldValue("value", v)
		require.Contains(t, s, "4 bytes · HEX 00 ff 41 1b |..A.|")
		require.NotContains(t, s, "\x1b")
	}
	require.Equal(t, "example.com", formatFieldValue("Host", "example.com"))
	require.Equal(t, "你好", formatFieldValue("text", "你好"))
	require.Contains(t, formatFieldValue("Next Protocol Data", "hello"), "HEX 68 65 6c 6c 6f |hello|")
	require.Less(t, len(formatFieldValue("Data", bytes.Repeat([]byte{255}, 65535))), 250)
}

func TestFieldAndByteSelectionHighlightBothHexAndASCII(t *testing.T) {
	u := browsingFixture(t)
	u.freezeList()
	longInspector(u)
	u.fields.fields = []field{
		{text: "frame", hasRange: true, end: len(u.inspect.data)},
		{text: "section", hasRange: true, start: 2400, end: 2600},
		{text: "payload", hasRange: true, start: 2444, end: 2500},
		{text: "generated"},
	}
	r := u.viewport(1)
	u.mouse(mouseEvent{action: mousePress, button: 0, x: r.x + 3, y: r.y + 2})
	require.Equal(t, 2444, u.hexStart)
	require.Equal(t, 2500, u.hexEnd)
	br := u.viewport(2)
	perRow := bytesPerRow(br.w - 2)
	require.Equal(t, 2444/perRow, u.hexTop)
	c := newCanvas(u.width, u.height)
	u.drawBytes(c, br)
	i := 2444 % perRow
	hexX := br.x + 8 + i*3 + i/8
	asciiX := br.x + 9 + perRow*3 + (perRow-1)/8 + i
	require.Equal(t, inkSelected, c.cells[br.y*c.width+hexX].style)
	require.Equal(t, inkSelected, c.cells[br.y*c.width+asciiX].style)
	// Clicking either representation chooses the narrowest containing field.
	for _, x := range []int{hexX, asciiX} {
		u.fieldSelection = 0
		u.mouse(mouseEvent{action: mousePress, button: 0, x: x, y: br.y})
		require.Equal(t, 2, u.fieldSelection)
		require.Equal(t, 2, u.pane)
	}
	top := u.hexTop
	u.mouse(mouseEvent{action: mouseWheel, x: r.x + 3, y: r.y + 1, delta: 3})
	require.Equal(t, top, u.hexTop, "wheel scroll does not seek the other pane")
	u.pane = 1
	u.fieldSelection = 2
	u.key("j", nil)
	require.Zero(t, u.hexEnd, "generated fields must not retain another field's highlight")
	// Narrow terminals can open a field's bytes with Enter.
	u.resize(85, 32)
	u.fieldSelection = 2
	u.key("\r", nil)
	require.Equal(t, 2, u.pane)
	require.Equal(t, 2444, u.hexStart)
	u.follow = true
	u.selected = 0
	u.refreshVisible()
	u.inspect = nil
	u.syncInspection(time.Now())
	require.Zero(t, u.hexEnd, "new packet clears stale byte selections")
}
