package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func cassandraFieldsTestString(s string) []byte {
	b := []byte{byte(len(s) >> 8), byte(len(s))}
	return append(b, []byte(s)...)
}
func cassandraFieldsTestEnvelope(version, op byte, body []byte) []byte {
	w := append([]byte{version, 0, 0x12, 0x34, op, 0, 0, 0, 0}, body...)
	binary.BigEndian.PutUint32(w[5:], uint32(len(body)))
	return w
}

// Independent bit-at-a-time IEEE CRC, including Apache's four initial bytes.
func cassandraFieldsTestCRC(b []byte) uint32 {
	c := uint32(0xffffffff)
	for _, x := range append([]byte{0xfa, 0x2d, 0x55, 0xca}, b...) {
		c ^= uint32(x)
		for i := 0; i < 8; i++ {
			if c&1 != 0 {
				c = c>>1 ^ 0xedb88320
			} else {
				c >>= 1
			}
		}
	}
	return ^c
}
func cassandraFieldsTestInitiate(address []byte, flags uint32) []byte {
	w := []byte{0xca, 0x55, 0x2d, 0xfa, 0, 0, 0, 0, byte(len(address) + 2)}
	binary.BigEndian.PutUint32(w[4:], flags)
	w = append(w, address...)
	w = append(w, 0x23, 0x45)
	crc := cassandraFieldsTestCRC(w)
	return binary.BigEndian.AppendUint32(w, crc)
}
func cassandraFieldsTestFixtures() map[string][]byte {
	startup := []byte{0, 3}
	for _, pair := range [][2]string{{"CQL_VERSION", "3.4.9"}, {"NAME", "名🔍"}, {"NAME", ""}} {
		startup = append(startup, cassandraFieldsTestString(pair[0])...)
		startup = append(startup, cassandraFieldsTestString(pair[1])...)
	}
	supported := []byte{0, 2}
	supported = append(supported, cassandraFieldsTestString("X")...)
	supported = append(supported, 0, 0)
	supported = append(supported, cassandraFieldsTestString("X")...)
	supported = append(supported, 0, 2)
	supported = append(supported, cassandraFieldsTestString("α")...)
	supported = append(supported, cassandraFieldsTestString("")...)
	return map[string][]byte{
		"options4": cassandraFieldsTestEnvelope(4, 5, nil), "options5-initial": cassandraFieldsTestEnvelope(5, 5, nil),
		"startup4": cassandraFieldsTestEnvelope(4, 1, startup), "startup5-initial": cassandraFieldsTestEnvelope(5, 1, startup),
		"supported4": cassandraFieldsTestEnvelope(0x84, 6, supported), "supported5-initial": cassandraFieldsTestEnvelope(0x85, 6, supported),
		"internode-initiate-modern": cassandraFieldsTestInitiate([]byte{192, 0, 2, 73}, 0x0d0a0c02),
	}
}
func TestCassandraFieldsLayoutsPrefixesMutations(t *testing.T) {
	for profile, w := range cassandraFieldsTestFixtures() {
		t.Run(profile, func(t *testing.T) {
			fs, info, e := decodeCassandraFields(w, profile)
			require.NoError(t, e)
			tlsCertificateTestCoverage(t, fs, len(w))
			require.Equal(t, true, info["Context Is Caller Supplied"])
			for cut := 0; cut < len(w); cut++ {
				fs, info, e := decodeCassandraFields(w[:cut], profile)
				require.Error(t, e, "cut %d", cut)
				require.Nil(t, fs)
				require.Nil(t, info)
			}
			for at := range w {
				for _, v := range []byte{0, 1, 127, 128, 254, 255} {
					bad := bytes.Clone(w)
					bad[at] = v
					fs, info, e := decodeCassandraFields(bad, profile)
					if e != nil {
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
func TestCassandraFieldsMapsUTF8AndIsolation(t *testing.T) {
	fixtures := cassandraFieldsTestFixtures()
	_, m, e := decodeCassandraFields(fixtures["startup4"], "startup4")
	require.NoError(t, e)
	options := m["Options"].([]map[string]any)
	require.Len(t, options, 3)
	require.Equal(t, true, m["CQL Version Key Present"])
	require.Equal(t, "NAME", options[1]["Key"].(map[string]any)["Text"])
	require.Equal(t, "NAME", options[2]["Key"].(map[string]any)["Text"])
	require.Equal(t, "名🔍", options[1]["Value"].(map[string]any)["Text"])
	require.Equal(t, "", options[2]["Value"].(map[string]any)["Text"])
	original := bytes.Clone(fixtures["startup4"])
	options[1]["Value"].(map[string]any)["Bytes"].([]byte)[0] ^= 255
	require.Equal(t, original, fixtures["startup4"])
	_, m, e = decodeCassandraFields(fixtures["supported4"], "supported4")
	require.NoError(t, e)
	options = m["Options"].([]map[string]any)
	require.Len(t, options, 2)
	require.Empty(t, options[0]["Values"])
	require.Len(t, options[1]["Values"], 2)
	require.Equal(t, false, m["CQL Version Key Present"])
	for _, profile := range []string{"startup4", "supported4"} {
		v, op := byte(4), byte(1)
		if profile == "supported4" {
			v, op = 0x84, 6
		}
		fs, m, e := decodeCassandraFields(cassandraFieldsTestEnvelope(v, op, []byte{0, 0}), profile)
		require.NoError(t, e)
		tlsCertificateTestCoverage(t, fs, 11)
		require.Empty(t, m["Options"])
	}
	for _, s := range []string{string([]byte{0xc0, 0xaf}), string([]byte{0xff}), string([]byte{0xe2, 0x82})} {
		body := []byte{0, 1}
		body = append(body, cassandraFieldsTestString("K")...)
		body = append(body, cassandraFieldsTestString(s)...)
		fs, m, e := decodeCassandraFields(cassandraFieldsTestEnvelope(4, 1, body), "startup4")
		require.ErrorContains(t, e, "UTF-8")
		require.Nil(t, fs)
		require.Nil(t, m)
	}
	// A maximum-length UTF-8 value and an embedded NUL remain length-delimited.
	for _, s := range []string{string(bytes.Repeat([]byte{'x'}, 65535)), "a\x00b"} {
		body := []byte{0, 1}
		body = append(body, cassandraFieldsTestString("K")...)
		body = append(body, cassandraFieldsTestString(s)...)
		w := cassandraFieldsTestEnvelope(4, 1, body)
		fs, m, e := decodeCassandraFields(w, "startup4")
		require.NoError(t, e)
		tlsCertificateTestCoverage(t, fs, len(w))
		require.Equal(t, s, m["Options"].([]map[string]any)[0]["Value"].(map[string]any)["Text"])
	}
}
func TestCassandraFieldsInternodeFlagsEndpointAndCRC(t *testing.T) {
	for _, address := range [][]byte{{192, 0, 2, 91}, {0x20, 1, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}} {
		for _, low := range []uint32{0, 1, 2, 3, 4, 8, 12, 16, 24, 0xe1} {
			w := cassandraFieldsTestInitiate(address, 0x0d0a0c00|low)
			fs, m, e := decodeCassandraFields(w, "internode-initiate-modern")
			require.NoError(t, e)
			tlsCertificateTestCoverage(t, fs, len(w))
			require.Equal(t, true, m["CRC32 Verified"])
			require.Equal(t, uint64(0x2345), m["Endpoint Port"])
			require.Equal(t, uint64(10), m["Minimum Messaging Version"])
			require.Equal(t, low&8 != 0, m["Streaming Mode"])
			require.Equal(t, address, m["Endpoint Address"].(map[string]any)["Bytes"])
			require.Equal(t, uint64((low>>2)&1|(low>>3)&2), m["Framing ID"])
		}
	}
	for _, flags := range []uint32{0x0d0a0c14, 0x0b0a0c01, 0x0d0a0b01} {
		_, _, e := decodeCassandraFields(cassandraFieldsTestInitiate([]byte{192, 0, 2, 91}, flags), "internode-initiate-modern")
		require.Error(t, e)
	}
	w := cassandraFieldsTestFixtures()["internode-initiate-modern"]
	for at := range w {
		bad := bytes.Clone(w)
		bad[at] ^= 1
		_, _, e := decodeCassandraFields(bad, "internode-initiate-modern")
		require.Error(t, e, "byte %d", at)
	}
	// Recomputed CRC does not bless an unsupported address-length variant.
	w = append([]byte{0xca, 0x55, 0x2d, 0xfa, 0x0d, 0x0a, 0x0c, 1, 4}, []byte{192, 0, 2, 91}...)
	w = binary.BigEndian.AppendUint32(w, cassandraFieldsTestCRC(w))
	_, _, e := decodeCassandraFields(w, "internode-initiate-modern")
	require.ErrorContains(t, e, "explicit port")
}
func TestCassandraFieldsContextAndResourceBounds(t *testing.T) {
	fixtures := cassandraFieldsTestFixtures()
	for profile, w := range fixtures {
		for other := range fixtures {
			if profile != other {
				_, _, e := decodeCassandraFields(w, other)
				require.Error(t, e, "%s as %s", profile, other)
			}
		}
	}
	_, _, e := decodeCassandraFields(fixtures["options4"], "auto")
	require.Error(t, e)
	_, _, e = decodeCassandraFields(make([]byte, cassandraFieldsMaxBytes+1), "startup4")
	require.ErrorContains(t, e, "resource limit")
	for _, n := range []int{4096, 4097} {
		body := []byte{byte(n >> 8), byte(n)}
		body = append(body, bytes.Repeat([]byte{0, 0, 0, 0}, n)...)
		_, _, e := decodeCassandraFields(cassandraFieldsTestEnvelope(4, 1, body), "startup4")
		if n == 4096 {
			require.NoError(t, e)
		} else {
			require.ErrorContains(t, e, "resource limit")
		}
	}
	r := &cassandraFieldsReader{wire: make([]byte, 65537), end: 65537}
	for i := 0; i < 65536; i++ {
		r.take("Byte", "raw", 1)
	}
	require.NoError(t, r.err)
	r.take("Extra", "raw", 1)
	require.ErrorContains(t, r.err, "resource limit")
	for _, mutation := range []func([]byte){func(w []byte) { w[1] = 1 }, func(w []byte) { w[1] = 2 }, func(w []byte) { w[1] = 4 }, func(w []byte) { w[1] = 8 }, func(w []byte) { w[1] = 16 }, func(w []byte) { w[2] = 0x80 }, func(w []byte) { w[2] = 0xff }, func(w []byte) { w[5] = 0x80 }} {
		w := bytes.Clone(fixtures["startup4"])
		mutation(w)
		_, _, e := decodeCassandraFields(w, "startup4")
		require.Error(t, e)
	}
}
func TestCassandraFieldsOriginalPrefixes(t *testing.T) {
	f, e := os.Open("../../testdata/protocol-corpus/captures/ndpi/ndpi-cassandra.pcap")
	require.NoError(t, e)
	defer f.Close()
	r, e := pcapgo.NewReader(f)
	require.NoError(t, e)
	profiles := map[int]string{4: "options4", 6: "supported4", 8: "startup4", 12: "options5-initial", 14: "supported5-initial", 16: "startup5-initial", 20: "internode-initiate-modern"}
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
		fs, _, e := decodeCassandraFields(w, profile)
		require.NoError(t, e, "frame %d", frame)
		tlsCertificateTestCoverage(t, fs, len(w))
		count++
		total += len(w)
		for cut := 0; cut < len(w); cut++ {
			fs, m, e := decodeCassandraFields(w[:cut], profile)
			require.Error(t, e, "frame %d cut %d", frame, cut)
			require.Nil(t, fs)
			require.Nil(t, m)
		}
	}
	require.Equal(t, 7, count)
	require.Equal(t, 332, total)
}
func TestCassandraFieldsBridgeTransactions(t *testing.T) {
	fixtures := cassandraFieldsTestFixtures()
	var profiles []string
	for p := range fixtures {
		profiles = append(profiles, p)
	}
	testExactByteFieldsBridgeTransactions(t, profiles, func(p string) []byte { return bytes.Clone(fixtures[p]) }, parseCassandraFields)
	t.Log(fmt.Sprintf("%d profiles x 8 offsets x 4 transaction outcomes", len(profiles)))
}
