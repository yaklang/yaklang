package bin_parser_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	p "github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func dnsCompressionBoundaryWires() ([]byte, []byte, []int) {
	name := []byte{1, 'b', 3, 'm', 'v', 'p', 7, 'i', 'n', 'v', 'a', 'l', 'i', 'd', 0}
	q := make([]byte, 12)
	binary.BigEndian.PutUint16(q, 0x5013)
	binary.BigEndian.PutUint16(q[2:], 0x0100)
	binary.BigEndian.PutUint16(q[4:], 1)
	q = append(q, name...)
	q = append(q, 0, 255, 0, 1)
	r := bytes.Clone(q)
	binary.BigEndian.PutUint16(r[2:], 0x8180)
	binary.BigEndian.PutUint16(r[6:], 3)
	var starts []int
	for i, v := range [][]byte{{203, 0, 113, 7}, {0x20, 1, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 7}, {3, 'm', 'v', 'p', 0}} {
		starts = append(starts, len(r))
		r = append(r, 1, 'b', 0xc0, 14)
		typ := []uint16{1, 28, 16}[i]
		h := make([]byte, 10)
		binary.BigEndian.PutUint16(h, typ)
		binary.BigEndian.PutUint16(h[2:], 1)
		binary.BigEndian.PutUint32(h[4:], 60)
		binary.BigEndian.PutUint16(h[8:], uint16(len(v)))
		r = append(r, h...)
		r = append(r, v...)
	}
	return q, r, starts
}
func dnsCompressionChild(t *testing.T, n *base.NodeValue, names ...string) *base.NodeValue {
	t.Helper()
	for _, name := range names {
		require.NotNil(t, n)
		n = n.Child(name)
	}
	require.NotNil(t, n)
	return n
}
func TestDNSCompressionBoundaryAdjacentRecords(t *testing.T) {
	_, wire, starts := dnsCompressionBoundaryWires()
	root, err := parser.ParseBinary(bytes.NewReader(wire), "application-layer.dns", "DNS")
	require.NoError(t, err)
	v, err := root.Result()
	require.NoError(t, err)
	if v.Name != "DNS" {
		v = dnsCompressionChild(t, v, "DNS")
	}
	records := dnsCompressionChild(t, v, "Answers").Children()
	require.Len(t, records, 3)
	for i, rr := range records {
		labels := dnsCompressionChild(t, rr, "Name", "Labels").Children()
		require.Len(t, labels, 2)
		require.EqualValues(t, uint64(1), dnsCompressionChild(t, labels[0], "Count").Value)
		require.Equal(t, "b", dnsCompressionChild(t, labels[0], "Text").Value)
		require.EqualValues(t, uint64(0xc0), dnsCompressionChild(t, labels[1], "Count").Value)
		low := dnsCompressionChild(t, labels[1], "PointerLow")
		require.EqualValues(t, uint64(14), low.Value)
		require.Equal(t, [2]uint64{uint64(starts[i]+3) * 8, uint64(starts[i]+4) * 8}, stream_parser.GetNodeResultPos(low.Origin))
		typ := dnsCompressionChild(t, rr, "Type")
		require.EqualValues(t, uint64([]uint16{1, 28, 16}[i]), typ.Value)
		require.Equal(t, [2]uint64{uint64(starts[i]+4) * 8, uint64(starts[i]+6) * 8}, stream_parser.GetNodeResultPos(typ.Origin))
	}
	require.Equal(t, []byte{203, 0, 113, 7}, dnsCompressionChild(t, records[0], "DNSA", "Address").Value)
	require.Equal(t, wire[starts[1]+14:starts[1]+30], dnsCompressionChild(t, records[1], "DNSAAAA", "Address").Value)
	strings := dnsCompressionChild(t, records[2], "DNSTXT", "Strings").Children()
	require.Len(t, strings, 2)
	require.Equal(t, "mvp", dnsCompressionChild(t, strings[0], "Text").Value)
	require.EqualValues(t, uint64(0), dnsCompressionChild(t, strings[1], "Length").Value)
	require.EqualValues(t, uint64(len(wire)*8), root.Ctx.GetUint64("pointer"))
}
func TestDNSCompressionBoundaryMalformedName(t *testing.T) {
	q, wire, starts := dnsCompressionBoundaryWires()
	// One literal label followed by a missing pointer low octet. No RR header
	// follows, so the encoder cannot hide a short pointer behind another field.
	truncated := append(bytes.Clone(q), 1, 'b', 0xc0)
	binary.BigEndian.PutUint16(truncated[2:], 0x8180)
	binary.BigEndian.PutUint16(truncated[6:], 1)
	for _, bad := range [][]byte{truncated, append(bytes.Clone(wire[:starts[0]]), 0x40), append(bytes.Clone(wire[:starts[0]]), 0x80)} {
		binary.BigEndian.PutUint16(bad[6:], 1)
		_, err := parser.ParseBinary(bytes.NewReader(bad), "application-layer.dns", "DNS")
		require.Error(t, err)
		_, err = p.DecodeDNSMessage(bad, 100)
		require.Error(t, err)
	}
}
func TestDNSCompressionBoundaryPublicUDPAndTCP(t *testing.T) {
	query, response, _ := dnsCompressionBoundaryWires()
	for _, transport := range []string{"udp", "tcp"} {
		for _, chunk := range []int{0, 1, 7, 64} {
			if transport == "udp" && chunk != 0 {
				continue
			}
			t.Run(fmt.Sprintf("%s-chunk%d", transport, chunk), func(t *testing.T) {
				session, err := p.NewProtocolSessionWithOptions(p.DefaultParserBudget(), p.WithSessionTransport(transport), p.WithSessionPorts(49152, 53))
				require.NoError(t, err)
				var events []*p.ProtocolEvent
				for dir, raw := range [][]byte{query, response} {
					wire := bytes.Clone(raw)
					if transport == "tcp" {
						prefix := []byte{byte(len(wire) >> 8), byte(len(wire))}
						wire = append(prefix, wire...)
					}
					for off := 0; off < len(wire); {
						n := len(wire) - off
						if chunk > 0 {
							n = min(n, chunk)
						}
						result := session.Feed(dir, time.Unix(int64(dir+1), 0), wire[off:off+n])
						require.True(t, result.Err == nil || result.Err.Kind == p.ErrNeedMore, "%v", result.Err)
						events = append(events, result.Events...)
						off += n
					}
				}
				require.Len(t, events, 2)
				for _, e := range events {
					require.Equal(t, "dns", e.Protocol)
					require.Equal(t, "decoded", e.Status)
					fields, err := e.GetFields()
					require.NoError(t, err)
					require.NotEmpty(t, fields)
				}
				require.NotZero(t, events[1].ResponseTo)
				require.Equal(t, events[0].ID, events[1].ResponseTo)
				facts := events[1].Session["DNS"].(map[string]any)
				answers := facts["Answers"].([]map[string]any)
				require.Len(t, answers, 3)
				for _, a := range answers {
					require.Equal(t, "b.mvp.invalid", a["Name"])
				}
				require.Empty(t, session.Close("EOF"))
				require.Zero(t, session.Stats().BufferedBytes)
			})
		}
	}
}
