package stream_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func tdsFieldsTestHex(s string) []byte {
	b, e := hex.DecodeString(s)
	if e != nil {
		panic(e)
	}
	return b
}
func tdsFieldsTestPacket(kind byte, body []byte, chunk int) []byte {
	var wire []byte
	if chunk <= 0 {
		chunk = 65520
	}
	id := byte(255)
	for at := 0; at < len(body) || at == 0; {
		n := min(chunk, len(body)-at)
		h := []byte{kind, 4, 0, 0, 0x12, 0x34, id, 0}
		binary.BigEndian.PutUint16(h[2:4], uint16(n+8))
		if at+n == len(body) {
			h[1] = 1
		}
		wire = append(wire, h...)
		wire = append(wire, body[at:at+n]...)
		at += n
		id++
		if n == 0 {
			break
		}
	}
	return wire
}
func tdsFieldsTestHeader() []byte {
	return tdsFieldsTestHex("16000000120000000200080706050403020101000000")
}
func tdsFieldsTestFixtures() map[string][]byte {
	query := tdsFieldsTestHex("530045004c0045004300540020003100")
	rpc := tdsFieldsTestHex("ffff0a0000000000260404feffffff")
	// int4 column x, row -2, DONE and RETURNSTATUS -3; widths are profile-owned.
	response := func(v72 bool) []byte {
		b := []byte{0x81, 1, 0}
		if v72 {
			b = append(b, 0, 0)
		}
		b = append(b, tdsFieldsTestHex("0000010038017800d1fefffffffd1000c10001000000")...)
		if v72 {
			b = append(b, 0, 0, 0, 0)
		}
		return append(b, 0x79, 0xfd, 0xff, 0xff, 0xff)
	}
	return map[string][]byte{
		"batch71": tdsFieldsTestPacket(1, query, 3), "batch72": tdsFieldsTestPacket(1, append(tdsFieldsTestHeader(), query...), 7),
		"rpc71": tdsFieldsTestPacket(3, rpc, 5), "rpc72": tdsFieldsTestPacket(3, append(tdsFieldsTestHeader(), rpc...), 9),
		"response71": tdsFieldsTestPacket(4, response(false), 7), "response72": tdsFieldsTestPacket(4, response(true), 11),
	}
}

func TestTDSFieldsLayoutsPrefixesAndMutations(t *testing.T) {
	for profile, wire := range tdsFieldsTestFixtures() {
		t.Run(profile, func(t *testing.T) {
			fs, info, err := decodeTDSFields(wire, profile)
			require.NoError(t, err)
			tlsCertificateTestCoverage(t, fs, len(wire))
			require.Equal(t, true, info["Version Is Caller Supplied"])
			for cut := 0; cut < len(wire); cut++ {
				fs, info, err := decodeTDSFields(wire[:cut], profile)
				require.Error(t, err, "cut %d", cut)
				require.Nil(t, fs)
				require.Nil(t, info)
			}
			for at := range wire {
				for _, v := range []byte{0, 1, 127, 128, 255} {
					w := bytes.Clone(wire)
					w[at] = v
					fs, info, err := decodeTDSFields(w, profile)
					if err != nil {
						require.Nil(t, fs)
						require.Nil(t, info)
					} else {
						tlsCertificateTestCoverage(t, fs, len(w))
					}
				}
			}
		})
	}
}

func TestTDSFieldsValuesPLPAndPhysicalSpans(t *testing.T) {
	// Independent type/value encodings, with NULL distinct from an empty value.
	types := []struct {
		wire string
		key  string
		want any
	}{
		{"260101ff", "Unsigned Integer", uint64(255)}, {"260202feff", "Signed Integer", int64(-2)},
		{"260404feffffff", "Signed Integer", int64(-2)}, {"2608080000000000000080", "Signed Integer", int64(-9223372036854775808)},
		{"2410100123456789abcdef0123456789abcdef", "GUID", "67452301-ab89-efcd-0123-456789abcdef"},
		{"68010101", "Boolean", true}, {"6f0808feffffff00000000", "Days", int64(-2)}, {"6f040401000200", "Minutes", uint16(2)},
		{"e700000904d000340000", "Text", ""}, {"e702000904d00034ffff", "Null", true}, {"1f", "Null", true},
		{"e702000904d00034020000d8", "Unicode Text Valid", false},
		// PLP declared total 8; a surrogate pair is split between odd chunks.
		{"e7ffff0904d0003408000000000000000300000041003d05000000d800de420000000000", "Text", "A😀B"},
		{"a5fffffeffffffffffffff01000000ff00000000", "Bytes", []byte{255}},
		{"e7ffff0904d00034ffffffffffffffff", "Null", true},
	}
	for _, tc := range types {
		body := append(tdsFieldsTestHeader(), tdsFieldsTestHex("ffff0a0000000000")...)
		body = append(body, tdsFieldsTestHex(tc.wire)...)
		for _, chunk := range []int{1, 3, 13, 65520} {
			wire := tdsFieldsTestPacket(3, body, chunk)
			fs, info, err := decodeTDSFields(wire, "rpc72")
			require.NoError(t, err, "%s chunk %d", tc.wire, chunk)
			tlsCertificateTestCoverage(t, fs, len(wire))
			value := info["RPC Requests"].([]map[string]any)[0]["Parameters"].([]map[string]any)[0]["Value"].(map[string]any)
			require.Equal(t, tc.want, value[tc.key], tc.wire)
			if tc.key == "Unicode Text Valid" {
				require.NotContains(t, value, "Text")
			}
			var recovered []byte
			for _, span := range value["Data Wire Byte Ranges"].([][2]int) {
				recovered = append(recovered, wire[span[0]:span[1]]...)
			}
			require.True(t, bytes.Equal(recovered, value["Bytes"].([]byte)))
			original := bytes.Clone(wire)
			if len(value["Bytes"].([]byte)) > 0 {
				value["Bytes"].([]byte)[0] ^= 255
			}
			require.Equal(t, original, wire)
		}
	}
	// PLP totals, chunk envelopes, invalid numeric widths and Unicode lengths.
	for _, bad := range []string{
		"260300", "2604020000", "24100f000000000000000000000000000000", "68010102", "6f04040000a005", "6f080800000000ffffffff",
		"e701000904d000340000", "e702000904d00034010041", "af401f0904d00034411f", "e7ffff0904d00034010000000000000000000000",
		"e7ffff0904d00034feffffffffffffff050000000000", "e7ffff0904d000340000000000000000", "62", "efFFFF0904d00034",
	} {
		body := append(tdsFieldsTestHeader(), tdsFieldsTestHex("ffff0a0000000000")...)
		body = append(body, tdsFieldsTestHex(bad)...)
		fs, info, err := decodeTDSFields(tdsFieldsTestPacket(3, body, 0), "rpc72")
		require.Error(t, err, bad)
		require.Nil(t, fs)
		require.Nil(t, info)
	}
}

func TestTDSFieldsContextAndResourceBounds(t *testing.T) {
	for _, tc := range []struct {
		profile, body string
		want          any
	}{
		{"response71", "fd1000000000000080", int64(-2147483648)},
		{"response72", "fd10000000ffffffffffffffff", uint64(18446744073709551615)},
	} {
		_, info, err := decodeTDSFields(tdsFieldsTestPacket(4, tdsFieldsTestHex(tc.body), 0), tc.profile)
		require.NoError(t, err)
		require.Equal(t, tc.want, info["Response Tokens"].([]map[string]any)[0]["Row Count"])
	}
	for _, body := range []string{"d100000000", "81ffffd100000000", "8101000000010038017800fd0000000000000000d100000000", "ff000000000000000000000000ac"} {
		_, _, err := decodeTDSFields(tdsFieldsTestPacket(4, tdsFieldsTestHex(body), 0), "response71")
		require.Error(t, err)
	}
	_, _, err := decodeTDSFields(tdsFieldsTestPacket(4, tdsFieldsTestHex("fd0000d50000000000"), 0), "response72")
	require.Error(t, err)
	for _, profile := range []string{"", "response", "rpc73"} {
		_, _, err := decodeTDSFields(nil, profile)
		require.Error(t, err)
	}
	// Byte bound including all packet headers, with no oversized TDS packet.
	for _, size := range []int{tdsFieldsMaxBytes, tdsFieldsMaxBytes + 1} {
		wire := tdsFieldsTestPacket(1, bytes.Repeat([]byte{0}, size-17*8), 65520)
		require.Len(t, wire, size)
		fs, _, err := decodeTDSFields(wire, "batch71")
		if size == tdsFieldsMaxBytes {
			require.NoError(t, err)
			tlsCertificateTestCoverage(t, fs, len(wire))
		} else {
			require.Error(t, err)
		}
	}
	for _, count := range []int{4096, 4097} {
		for _, category := range []string{"packets", "values", "tokens", "columns", "chunks"} {
			var body []byte
			profile, kind, chunk := "rpc71", byte(3), 65520
			switch category {
			case "packets":
				profile, kind, chunk = "batch71", 1, 2
				body = bytes.Repeat([]byte{0, 0}, count)
			case "values":
				body = append(tdsFieldsTestHex("ffff0a000000"), bytes.Repeat([]byte{0, 0, 0x1f}, count)...)
			case "tokens":
				profile, kind = "response71", 4
				body = bytes.Repeat([]byte{0x79, 0, 0, 0, 0}, count)
			case "columns":
				profile, kind = "response71", 4
				body = []byte{0x81, byte(count), byte(count >> 8)}
				body = append(body, bytes.Repeat([]byte{0, 0, 0, 0, 0x38, 0}, count)...)
			case "chunks":
				profile = "rpc72"
				body = append(tdsFieldsTestHeader(), tdsFieldsTestHex("ffff0a0000000000a5fffffeffffffffffffff")...)
				body = append(body, bytes.Repeat([]byte{1, 0, 0, 0, 0}, count)...)
				body = append(body, 0, 0, 0, 0)
			}
			wire := tdsFieldsTestPacket(kind, body, chunk)
			fs, info, err := decodeTDSFields(wire, profile)
			if count == 4096 {
				require.NoError(t, err, category)
				tlsCertificateTestCoverage(t, fs, len(wire))
			} else {
				require.ErrorContains(t, err, "resource limit", category)
				require.Nil(t, fs)
				require.Nil(t, info)
			}
		}
	}
	// Count leaves before publication, including the projected framing leaves.
	r := &tdsFieldsReader{wire: make([]byte, 65537), end: 65537}
	for i := 0; i < 65536; i++ {
		r.take("byte", "raw", 1)
	}
	require.NoError(t, r.err)
	r.take("extra", "raw", 1)
	require.ErrorContains(t, r.err, "resource limit")
	for _, count := range []int{4096, 4097} {
		body := bytes.Repeat(tdsFieldsTestHex("ffff0a00000080"), count-1)
		body = append(body, tdsFieldsTestHex("ffff0a000000")...)
		_, _, err := decodeTDSFields(tdsFieldsTestPacket(3, body, 0), "rpc71")
		if count == 4096 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "resource limit")
		}
	}
	columnBlock := append([]byte{0x81, 0, 16}, bytes.Repeat([]byte{0, 0, 0, 0, 0x38, 0}, 4096)...)
	projectedLimitBody := append(bytes.Repeat(columnBlock, 2), bytes.Repeat([]byte{0x79, 0, 0, 0, 0}, 500)...)
	_, _, err = decodeTDSFields(tdsFieldsTestPacket(4, projectedLimitBody, 13), "response71")
	require.ErrorContains(t, err, "projected field resource limit")
	_, _, err = decodeTDSFields(tdsFieldsTestPacket(4, bytes.Repeat(columnBlock, 4), 65520), "response71")
	require.ErrorContains(t, err, "field resource limit")
}

func TestTDSFieldsOriginalMessagePrefixes(t *testing.T) {
	f, err := os.Open("../../testdata/protocol-corpus/captures/ndpi/ndpi-mssql.pcap")
	require.NoError(t, err)
	defer f.Close()
	r, err := pcapgo.NewReader(f)
	require.NoError(t, err)
	var long []byte
	count := 0
	for frame := 1; ; frame++ {
		b, _, err := r.ReadPacketData()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		p := gopacket.NewPacket(b, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, p.ErrorLayer())
		wire := p.Layer(layers.LayerTypeTCP).(*layers.TCP).Payload
		if len(wire) == 0 {
			continue
		}
		if frame >= 26 && frame <= 32 {
			long = append(long, wire...)
			if frame < 32 {
				continue
			}
			wire = long
		}
		version := "72"
		if frame == 5 || frame == 6 || frame == 8 || frame >= 35 && frame <= 37 {
			version = "71"
		}
		profile := map[byte]string{1: "batch", 3: "rpc", 4: "response"}[wire[0]] + version
		fields, _, err := decodeTDSFields(wire, profile)
		require.NoError(t, err)
		tlsCertificateTestCoverage(t, fields, len(wire))
		count++
		for cut := 0; cut < len(wire); cut++ {
			fields, info, err := decodeTDSFields(wire[:cut], profile)
			require.Error(t, err, "frame %d prefix %d", frame, cut)
			require.Nil(t, fields)
			require.Nil(t, info)
		}
	}
	require.Equal(t, 29, count)
}

func TestTDSFieldsBridgeTransactions(t *testing.T) {
	fixtures := tdsFieldsTestFixtures()
	profiles := []string{"batch71", "batch72", "rpc71", "rpc72", "response71", "response72"}
	testExactByteFieldsBridgeTransactions(t, profiles, func(profile string) []byte { return bytes.Clone(fixtures[profile]) }, parseTDSFields)
}

func TestTDSFieldsMixedEndianBridgeRejectsBeforePublication(t *testing.T) {
	// The helper injects FF for its invalid case. Here decoding succeeds, but
	// the staged tree rejects an invalid override and rolls back identically.
	testExactByteFieldsBridgeTransactions(t, []string{"mixed"}, func(string) []byte { return []byte{1, 2, 3, 4} }, func(n *base.Node, process func(*base.Node) (func(bool), error), profile string) error {
		return parseCertificateFieldTree(n, process, func(w []byte) ([]tlsCertificateField, map[string]any, error) {
			order := "little"
			if w[0] == 255 {
				order = "invalid"
			}
			return []tlsCertificateField{{Name: "Big", Type: "uint16", Start: 0, End: 2}, {Name: "Little", Type: "uint16", Endian: order, Start: 2, End: 4}}, map[string]any{"Profile": fmt.Sprint(profile)}, nil
		}, profile)
	})
}
