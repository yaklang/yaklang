package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func tnsFieldsTestPacket(kind byte, body []byte, wide bool) []byte {
	w := append([]byte{0, 0, 0, 0, kind, 0, 0, 0}, body...)
	if wide {
		binary.BigEndian.PutUint32(w, uint32(len(w)))
	} else {
		binary.BigEndian.PutUint16(w, uint16(len(w)))
	}
	return w
}
func tnsFieldsTestData(body []byte) []byte {
	return tnsFieldsTestPacket(6, append([]byte{0, 0}, body...), true)
}
func tnsFieldsTestFixtures() map[string][]byte {
	desc := []byte("(DESCRIPTION=(ADDRESS=(HOST=example.invalid)(PORT=1522))(LABEL=one)(LABEL=two))")
	connect := make([]byte, 70)
	copy(connect[8:], []byte{1, 59, 1, 44, 0x0c, 0x41, 0x20, 0, 0xff, 0xff, 0x7f, 8, 0, 0, 1, 0})
	binary.BigEndian.PutUint16(connect[24:], uint16(len(desc)))
	binary.BigEndian.PutUint16(connect[26:], 70)
	binary.BigEndian.PutUint32(connect[28:], 4096)
	connect[32], connect[33] = 0x41, 0x41
	binary.BigEndian.PutUint32(connect[58:], 16384)
	binary.BigEndian.PutUint32(connect[62:], 2097152)
	connect = tnsFieldsTestPacket(1, append(connect[8:], desc...), false)
	accept := make([]byte, 41)
	copy(accept[8:], []byte{1, 59, 0x0c, 0x41, 0, 0, 0, 0, 1, 0, 0, 0, 0, 41, 0x41, 0x41})
	binary.BigEndian.PutUint32(accept[32:], 16384)
	binary.BigEndian.PutUint32(accept[36:], 2097152)
	accept = tnsFieldsTestPacket(2, accept[8:], false)
	// Independent supervisor response with two array elements, not a capture copy.
	sns := []byte{0xde, 0xad, 0xbe, 0xef, 0, 0, 0x1e, 0, 0, 0, 0, 1, 0, 0, 4, 0, 3, 0, 0, 0, 0,
		0, 4, 0, 5, 0x1e, 0x10, 0x20, 0, 0, 2, 0, 6, 0, 31,
		0, 14, 0, 1, 0xde, 0xad, 0xbe, 0xef, 0, 3, 0, 0, 0, 2, 0, 4, 0, 3}
	binary.BigEndian.PutUint16(sns[4:], uint16(len(sns)))
	compile := make([]byte, 40)
	compile[0], compile[27], compile[37] = 6, 1, 2
	runtime := []byte{2, 1, 0, 1, 0x18, 0, 3}
	capabilities := append([]byte{40}, compile...)
	capabilities = append(capabilities, 7)
	capabilities = append(capabilities, runtime...)
	protocol := append([]byte{1, 6, 0}, []byte("Independent Platform\x00")...)
	protocol = append(protocol, []byte{0x69, 3, 1, 1, 0, 0x69, 3, 0x66, 3, 1, 0, 14}...)
	fdo := make([]byte, 14)
	fdo[9], fdo[10] = 7, 0xd0
	protocol = append(protocol, fdo...)
	protocol = append(protocol, capabilities...)
	tz := []byte{128, 0, 0, 0, 65, 90, 60, 128, 0, 0, 0, 0, 0, 0, 33}
	types := []byte{2, 0x69, 3, 0x69, 3, 2}
	types = append(types, capabilities...)
	types = append(types, tz...)
	types = append(types, 0xd0, 7)
	parameters := make([]byte, 3+48)
	copy(parameters, []byte{3, 118, 9})
	for _, off := range []int{3, 19, 35, 43} {
		binary.LittleEndian.PutUint64(parameters[off:], uint64(off+123))
	}
	binary.LittleEndian.PutUint32(parameters[11:], 30)
	binary.LittleEndian.PutUint32(parameters[15:], 0x1020304)
	binary.LittleEndian.PutUint32(parameters[27:], 2)
	parameters = append(parameters, 3, 'b', 'o', 'b')
	for _, value := range []string{"AB", ""} {
		parameters = append(parameters, 99, 0, 0, 0, 1, 'X', 99, 0, 0, 0, byte(len(value)))
		parameters = append(parameters, []byte(value)...)
		parameters = append(parameters, 4, 3, 2, 1)
	}
	return map[string][]byte{
		"connect315": connect, "accept315": accept, "resend16": tnsFieldsTestPacket(11, nil, false),
		"services32":             tnsFieldsTestData(sns),
		"protocol-request32":     tnsFieldsTestData(append([]byte{1, 6, 5, 0}, []byte("Another Platform\x00")...)),
		"protocol-response32":    tnsFieldsTestData(protocol),
		"types-request-native32": tnsFieldsTestData(types), "types-response-native32": tnsFieldsTestData(append([]byte{2}, tz...)),
		"parameters-native-le64-32": tnsFieldsTestData(parameters),
	}
}

func TestTNSFieldsLayoutsPrefixesMutations(t *testing.T) {
	for profile, wire := range tnsFieldsTestFixtures() {
		t.Run(profile, func(t *testing.T) {
			fields, info, err := decodeTNSFields(wire, profile)
			require.NoError(t, err)
			tlsCertificateTestCoverage(t, fields, len(wire))
			require.Equal(t, true, info["Context Is Caller Supplied"])
			for cut := 0; cut < len(wire); cut++ {
				fields, info, err := decodeTNSFields(wire[:cut], profile)
				require.Error(t, err, "cut %d", cut)
				require.Nil(t, fields)
				require.Nil(t, info)
			}
			for at := range wire {
				for _, v := range []byte{0, 1, 127, 128, 254, 255} {
					w := bytes.Clone(wire)
					w[at] = v
					fields, info, err := decodeTNSFields(w, profile)
					if err != nil {
						require.Nil(t, fields)
						require.Nil(t, info)
					} else {
						tlsCertificateTestCoverage(t, fields, len(w))
					}
				}
			}
		})
	}
}

func TestTNSFieldsIndependentValues(t *testing.T) {
	fixtures := tnsFieldsTestFixtures()
	_, m, e := decodeTNSFields(fixtures["connect315"], "connect315")
	require.NoError(t, e)
	require.Equal(t, uint64(16384), m["SDU32"])
	require.Equal(t, uint64(2097152), m["TDU32"])
	desc := m["Connect Descriptor"].(map[string]any)["Children"].([]map[string]any)
	require.Len(t, desc, 3)
	require.Equal(t, "LABEL", desc[1]["Key"])
	require.Equal(t, "LABEL", desc[2]["Key"])
	require.Equal(t, []byte("two"), desc[2]["Value"].(map[string]any)["Bytes"])
	_, m, e = decodeTNSFields(fixtures["services32"], "services32")
	require.NoError(t, e)
	parts := m["Services"].([]map[string]any)[0]["Subpackets"].([]map[string]any)
	require.Equal(t, []uint64{4, 3}, parts[2]["Values"])
	_, m, e = decodeTNSFields(fixtures["protocol-response32"], "protocol-response32")
	require.NoError(t, e)
	require.Equal(t, uint64(873), m["Character Set"])
	require.Equal(t, uint64(2000), m["National Character Set"])
	require.Len(t, m["Character Conversions"], 1)
	for _, profile := range []string{"types-request-native32", "types-response-native32"} {
		_, m, e = decodeTNSFields(fixtures[profile], profile)
		require.NoError(t, e)
		require.Equal(t, int64(19800), m["Time Zone Offset Seconds"])
		require.Equal(t, uint64(33), m["Time Zone Data Version"])
	}
	_, m, e = decodeTNSFields(fixtures["parameters-native-le64-32"], "parameters-native-le64-32")
	require.NoError(t, e)
	pairs := m["Parameters"].([]map[string]any)
	require.Len(t, pairs, 2)
	require.Equal(t, []byte("bob"), m["User"].(map[string]any)["Bytes"])
	require.Equal(t, uint64(99), pairs[0]["Value Conversion Capacity"])
	require.Equal(t, []byte("AB"), pairs[0]["Value"].(map[string]any)["Bytes"])
	require.Empty(t, pairs[1]["Value"].(map[string]any)["Bytes"])
	require.Equal(t, uint64(0x01020304), pairs[1]["Flags"])
	original := bytes.Clone(fixtures["parameters-native-le64-32"])
	pairs[0]["Value"].(map[string]any)["Bytes"].([]byte)[0] ^= 255
	require.Equal(t, original, fixtures["parameters-native-le64-32"])
}

func TestTNSFieldsOriginalMessagePrefixes(t *testing.T) {
	f, e := os.Open("../../testdata/protocol-corpus/captures/ndpi/ndpi-oracle.pcapng")
	require.NoError(t, e)
	defer f.Close()
	r, e := pcapgo.NewNgReader(f, pcapgo.DefaultNgReaderOptions)
	require.NoError(t, e)
	profiles := map[int]string{4: "connect315", 6: "resend16", 8: "connect315", 10: "accept315", 11: "services32", 13: "services32", 14: "protocol-request32", 16: "protocol-response32", 17: "types-request-native32", 19: "types-response-native32", 20: "parameters-native-le64-32"}
	count, total := 0, 0
	for frame := 1; ; frame++ {
		b, _, e := r.ReadPacketData()
		if e == io.EOF {
			require.Equal(t, 21, frame)
			break
		}
		require.NoError(t, e)
		p := gopacket.NewPacket(b, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, p.ErrorLayer())
		w := p.Layer(layers.LayerTypeTCP).(*layers.TCP).Payload
		if len(w) == 0 {
			require.Empty(t, profiles[frame])
			continue
		}
		profile := profiles[frame]
		require.NotEmpty(t, profile)
		fields, _, e := decodeTNSFields(w, profile)
		require.NoError(t, e, "frame %d", frame)
		tlsCertificateTestCoverage(t, fields, len(w))
		count++
		total += len(w)
		for cut := 0; cut < len(w); cut++ {
			fields, info, e := decodeTNSFields(w[:cut], profile)
			require.Error(t, e, "frame %d cut %d", frame, cut)
			require.Nil(t, fields)
			require.Nil(t, info)
		}
	}
	require.Equal(t, 11, count)
	require.Equal(t, 1382, total)
}

func TestTNSFieldsBoundsAndExplicitContext(t *testing.T) {
	fixtures := tnsFieldsTestFixtures()
	for profile, w := range fixtures {
		for other := range fixtures {
			if profile != other {
				_, _, e := decodeTNSFields(w, other)
				require.Error(t, e, "%s as %s", profile, other)
			}
		}
	}
	for _, profile := range []string{"auto", "", "parameters-universal"} {
		_, _, e := decodeTNSFields(fixtures["resend16"], profile)
		require.Error(t, e)
	}
	_, _, e := decodeTNSFields(make([]byte, tnsFieldsMaxBytes+1), "services32")
	require.ErrorContains(t, e, "resource limit")
	for _, depth := range []int{33, 34} {
		desc := []byte(strings.Repeat("(X=", depth) + "a" + strings.Repeat(")", depth))
		w := append(bytes.Clone(fixtures["connect315"][:70]), desc...)
		binary.BigEndian.PutUint16(w, uint16(len(w)))
		binary.BigEndian.PutUint16(w[24:], uint16(len(desc)))
		_, _, e := decodeTNSFields(w, "connect315")
		if depth == 33 {
			require.NoError(t, e)
		} else {
			require.ErrorContains(t, e, "nesting")
		}
	}
	for _, n := range []int{4096, 4097} {
		sns := make([]byte, 13)
		binary.BigEndian.PutUint32(sns, 0xdeadbeef)
		binary.BigEndian.PutUint16(sns[10:], uint16(n))
		sns = append(sns, bytes.Repeat([]byte{0, 8, 0, 0, 0, 0, 0, 0}, n)...)
		binary.BigEndian.PutUint16(sns[4:], uint16(len(sns)))
		_, _, e := decodeTNSFields(tnsFieldsTestData(sns), "services32")
		if n == 4096 {
			require.NoError(t, e)
		} else {
			require.ErrorContains(t, e, "resource limit")
		}
	}
	r := &tnsFieldsReader{wire: make([]byte, 65537), end: 65537}
	for i := 0; i < 65536; i++ {
		r.take("Byte", "raw", 1)
	}
	require.NoError(t, r.err)
	r.take("Extra", "raw", 1)
	require.ErrorContains(t, r.err, "resource limit")
	// The packet length is correct, but the *inner* service length is not.
	bad := bytes.Clone(fixtures["services32"])
	bad[15]--
	_, _, e = decodeTNSFields(bad, "services32")
	require.ErrorContains(t, e, "services length")
	// CLR capacity is not an exact length, but cannot be below actual bytes.
	bad = bytes.Clone(fixtures["parameters-native-le64-32"])
	binary.LittleEndian.PutUint32(bad[21:], 2)
	_, _, e = decodeTNSFields(bad, "parameters-native-le64-32")
	require.ErrorContains(t, e, "conversion capacity")
	// A 32-bit frame must not silently parse as the historical 16-bit header.
	_, _, e = decodeTNSFields(fixtures["services32"], "connect315")
	require.Error(t, e)
}

func TestTNSFieldsBridgeTransactions(t *testing.T) {
	fixtures := tnsFieldsTestFixtures()
	var profiles []string
	for profile := range fixtures {
		profiles = append(profiles, profile)
	}
	testExactByteFieldsBridgeTransactions(t, profiles, func(profile string) []byte { return bytes.Clone(fixtures[profile]) }, parseTNSFields)
	t.Log(fmt.Sprintf("%d profiles x 8 offsets x 4 transaction outcomes", len(profiles)))
}
