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
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func mysqlFieldsTestPacket(seq byte, payload []byte) []byte {
	return append([]byte{byte(len(payload)), byte(len(payload) >> 8), byte(len(payload) >> 16), seq}, payload...)
}
func mysqlFieldsTestResult(maria bool, row []byte) []byte {
	count := []byte{1}
	if maria {
		count = append(count, 1)
	}
	column := []byte{3, 'd', 'e', 'f', 0, 0, 0, 1, 'x', 0}
	if maria {
		column = append(column, 0)
	}
	column = append(column, 12, 33, 0, 36, 0, 0, 0, 0xfd, 0, 0, 0x27, 0, 0)
	w := mysqlFieldsTestPacket(254, count)
	for i, p := range [][]byte{column, {0xfe, 0, 0, 2, 0}, row, {0xfe, 0, 0, 2, 0}} {
		w = append(w, mysqlFieldsTestPacket(byte(255+i), p)...)
	}
	return w
}
func mysqlFieldsTestFixtures() map[string][]byte {
	g := append([]byte{10}, []byte("8.0.example\x00")...)
	g = append(g, 0x78, 0x56, 0x34, 0x12)
	g = append(g, bytes.Repeat([]byte{'x'}, 8)...)
	g = append(g, 0, 1, 0x82, 45, 2, 0, 8, 0, 21)
	g = append(g, make([]byte, 10)...)
	g = append(g, bytes.Repeat([]byte{'y'}, 12)...)
	g = append(g, []byte("\x00mysql_native_password\x00")...)
	response := make([]byte, 32)
	binary.LittleEndian.PutUint32(response, 0x188200)
	binary.LittleEndian.PutUint32(response[4:], 1<<24)
	response[8] = 33
	response = append(response, []byte("name\x00\x00mysql_native_password\x00\x00")...)
	maria := append([]byte(nil), response...)
	maria[28] = 0x1d
	ssl := make([]byte, 32)
	binary.LittleEndian.PutUint32(ssl, 0xa00)
	ssl[8] = 45
	mssl := append([]byte(nil), ssl...)
	mssl[28] = 0x1d
	return map[string][]byte{
		"greeting":               mysqlFieldsTestPacket(0, g),
		"response41":             mysqlFieldsTestPacket(1, response),
		"mariadb-response41":     mysqlFieldsTestPacket(1, maria),
		"ssl-request":            mysqlFieldsTestPacket(1, ssl),
		"mariadb-ssl-request":    mysqlFieldsTestPacket(1, mssl),
		"command":                mysqlFieldsTestPacket(0, []byte{3, 's', 'e', 'l', 'e', 'c', 't', ' ', '1'}),
		"ok41":                   mysqlFieldsTestPacket(2, []byte{0, 0xfc, 0x34, 0x12, 0, 2, 0, 0, 0}),
		"ok41-session-track":     mysqlFieldsTestPacket(2, []byte{0, 0, 0, 2, 0x40, 0, 0, 0, 3, 1, 1, 'x'}),
		"error41":                mysqlFieldsTestPacket(1, []byte("\xff\x28\x04#HY000message")),
		"eof41":                  mysqlFieldsTestPacket(5, []byte{0xfe, 0, 0, 2, 0}),
		"text-resultset41":       mysqlFieldsTestResult(false, []byte{1, 'v'}),
		"mariadb-text-resultset": mysqlFieldsTestResult(true, []byte{0xfb}),
	}
}

func TestMySQLFieldsLayoutsAndPrefixes(t *testing.T) {
	for p, w := range mysqlFieldsTestFixtures() {
		fs, info, err := decodeMySQLFields(w, p)
		require.NoError(t, err, p)
		tlsCertificateTestCoverage(t, fs, len(w))
		require.Equal(t, false, info["Session State Validated"])
		for cut := 0; cut < len(w); cut++ {
			_, _, err := decodeMySQLFields(w[:cut], p)
			require.Error(t, err, "%s prefix %d", p, cut)
		}
		_, _, err = decodeMySQLFields(append(append([]byte(nil), w...), 0), p)
		require.Error(t, err, p)
	}
	_, info, err := decodeMySQLFields(mysqlFieldsTestFixtures()["ok41"], "ok41")
	require.NoError(t, err)
	require.Equal(t, uint64(0x1234), info["Affected Rows"])
	_, info, err = decodeMySQLFields(mysqlFieldsTestResult(true, []byte{0xfb}), "mariadb-text-resultset")
	require.NoError(t, err)
	require.Equal(t, true, info["Rows"].([][]map[string]any)[0][0]["NULL"])
	_, info, err = decodeMySQLFields(mysqlFieldsTestResult(false, []byte{0}), "text-resultset41")
	require.NoError(t, err)
	require.Equal(t, false, info["Rows"].([][]map[string]any)[0][0]["NULL"])
	for _, p := range []string{"response41", "mariadb-response41"} {
		other := "response41"
		if p == other {
			other = "mariadb-response41"
		}
		// A MariaDB capability word must never be accepted as MySQL reserved zeros.
		if p == "mariadb-response41" {
			_, _, err = decodeMySQLFields(mysqlFieldsTestFixtures()[p], other)
			require.Error(t, err)
		}
	}
	for _, w := range [][]byte{{0xff}, {0xfb}, {0xfc, 1}, {0xfd, 1, 2}, {0xfe, 1, 2, 3, 4, 5, 6, 7}, {0xfe, 255, 255, 255, 255, 255, 255, 255, 255}} {
		r := &mysqlFieldsReader{wire: w, end: len(w)}
		r.sized("test", "raw", false)
		require.Error(t, r.err)
	}
	for _, tc := range []struct {
		w []byte
		n uint64
	}{{[]byte{250}, 250}, {[]byte{0xfc, 0x34, 0x12}, 0x1234}, {[]byte{0xfd, 0x56, 0x34, 0x12}, 0x123456}, {[]byte{0xfe, 8, 7, 6, 5, 4, 3, 2, 1}, 0x0102030405060708}} {
		r := &mysqlFieldsReader{wire: tc.w, end: len(tc.w)}
		n, null := r.length("n", false)
		require.NoError(t, r.err)
		require.False(t, null)
		require.Equal(t, tc.n, n)
		tlsCertificateTestCoverage(t, r.fields, len(tc.w))
	}
	for _, row := range [][]byte{{0xff}, {2, 'a'}, {0xfb, 0}, {0xfe, 255, 255, 255, 255, 255, 255, 255, 255}} {
		_, _, err = decodeMySQLFields(mysqlFieldsTestResult(false, row), "text-resultset41")
		require.Error(t, err)
	}
	w := mysqlFieldsTestResult(true, []byte{1, 'x'})
	w[5] = 0
	_, _, err = decodeMySQLFields(w, "mariadb-text-resultset")
	require.ErrorContains(t, err, "fresh metadata")
	w = mysqlFieldsTestResult(false, []byte{1, 'x'})
	w[8] = 17
	_, _, err = decodeMySQLFields(w, "text-resultset41")
	require.ErrorContains(t, err, "sequence")
}

func TestMySQLFieldsOriginalPackets(t *testing.T) {
	specs := []struct {
		path    string
		ng      bool
		records int
	}{
		{"ndpi/ndpi-mysql.pcapng", true, 41},
		{"generated-local/gen-mariadb.pcap", false, 4},
		{"generated-pr5023/pr5023-gen-mariadb.pcap", false, 4},
	}
	units := 0
	for _, spec := range specs {
		f, err := os.Open("../../testdata/protocol-corpus/captures/" + spec.path)
		require.NoError(t, err)
		var read func() ([]byte, gopacket.CaptureInfo, error)
		if spec.ng {
			r, e := pcapgo.NewNgReader(f, pcapgo.DefaultNgReaderOptions)
			require.NoError(t, e)
			read = r.ReadPacketData
		} else {
			r, e := pcapgo.NewReader(f)
			require.NoError(t, e)
			read = r.ReadPacketData
		}
		frames := 0
		for {
			frame, _, err := read()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			frames++
			tcp := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default).Layer(layers.LayerTypeTCP).(*layers.TCP)
			w := tcp.Payload
			if len(w) == 0 {
				continue
			}
			profile := "greeting"
			if spec.ng {
				switch frames {
				case 4, 19:
				case 6:
					profile = "mariadb-response41"
				case 8:
					profile = "ok41-session-track"
				case 9, 12:
					profile = "command"
				case 10:
					profile = "mariadb-text-resultset"
				case 21:
					profile = "ssl-request"
				default:
					continue
				}
			}
			fs, info, err := decodeMySQLFields(w, profile)
			require.NoError(t, err, "%s frame %d", spec.path, frames)
			tlsCertificateTestCoverage(t, fs, len(w))
			units++
			for cut := 0; cut < len(w); cut++ {
				_, _, err = decodeMySQLFields(w[:cut], profile)
				require.Error(t, err, "%s frame %d prefix %d", spec.path, frames, cut)
			}
			if frames == 10 && spec.ng {
				require.Equal(t, []byte("Ubuntu 22.04"), info["Rows"].([][]map[string]any)[0][0]["Value"])
			}
		}
		require.Equal(t, spec.records, frames)
		require.NoError(t, f.Close())
	}
	require.Equal(t, 10, units)
}

func TestMySQLFieldsBridgeTransactions(t *testing.T) {
	fixtures := mysqlFieldsTestFixtures()
	profiles := make([]string, 0, len(fixtures))
	for p := range fixtures {
		profiles = append(profiles, p)
	}
	testExactByteFieldsBridgeTransactions(t, profiles, func(p string) []byte { return append([]byte(nil), fixtures[p]...) }, parseMySQLFields)
}
func TestMySQLFieldsLimitsAndIsolation(t *testing.T) {
	for p, w := range mysqlFieldsTestFixtures() {
		for _, bits := range []uint64{0, 1, 7, mysqlFieldsMaxBytes*8 + 1, (mysqlFieldsMaxBytes + 1) * 8} {
			n := giopBridgeInlineRoot(t, "Package:\n  Boundary: raw,1\n")
			n.Cfg.SetItem(CfgLength, bits)
			calls := 0
			err := parseMySQLFields(n, func(*base.Node) (func(bool), error) { calls++; return nil, fmt.Errorf("must not read") }, p)
			require.Error(t, err)
			require.Zero(t, calls)
		}
		_, _, err := decodeMySQLFields(bytes.Repeat([]byte{0}, mysqlFieldsMaxBytes+1), p)
		require.Error(t, err)
		original := append([]byte(nil), w...)
		_, info, err := decodeMySQLFields(w, p)
		require.NoError(t, err)
		if p == "text-resultset41" {
			info["Rows"].([][]map[string]any)[0][0]["Value"].([]byte)[0] = 'z'
			require.Equal(t, original, w)
		}
	}
	for _, count := range []int{4096, 4097} {
		// Explicit count is bounded before allocating any column list.
		w := mysqlFieldsTestPacket(1, []byte{0xfc, byte(count), byte(count >> 8)})
		_, _, err := decodeMySQLFields(w, "text-resultset41")
		require.Error(t, err)
		if count == 4097 {
			require.ErrorContains(t, err, "resource limit")
		}
	}
}

func TestMySQLFieldsNestedBoundsAndResources(t *testing.T) {
	fixtures := mysqlFieldsTestFixtures()
	// Rebuild outer lengths so these reach nested grammar checks, instead of
	// merely failing the classic packet length at the first four bytes.
	for _, tc := range []struct {
		profile string
		payload []byte
	}{
		{"command", []byte{1, 0}}, {"command", []byte{0x16}},
		{"eof41", []byte{0xfe, 0, 0, 2}},
		{"ok41", []byte{0, 0xfb, 0, 2, 0, 0, 0}},
		{"ok41-session-track", []byte{0, 0, 0, 2, 0x40, 0, 0}},
		{"ok41-session-track", []byte{0, 0, 0, 2, 0x40, 0, 0, 0, 2, 1, 1}},
		{"ok41-session-track", []byte{0, 0, 0, 2, 0, 0, 0, 2, 'x'}},
		{"error41", []byte("\xff\x28\x04!HY000text")},
		{"response41", fixtures["ssl-request"][4:]},
		{"response41", fixtures["response41"][4 : len(fixtures["response41"])-1]},
	} {
		_, _, err := decodeMySQLFields(mysqlFieldsTestPacket(0, tc.payload), tc.profile)
		require.Error(t, err, tc.profile)
	}
	// Reserved, capability and plugin terminators are checked independently.
	for _, off := range []int{13, 20, 31} {
		w := append([]byte(nil), fixtures["response41"]...)
		w[off] = 1
		_, _, err := decodeMySQLFields(w, "response41")
		require.Error(t, err)
	}
	w := append([]byte(nil), fixtures["ssl-request"]...)
	w[5] &^= 8
	_, _, err := decodeMySQLFields(w, "ssl-request")
	require.Error(t, err)
	w = append([]byte(nil), fixtures["greeting"]...)
	w[len(w)-1] = 1
	_, _, err = decodeMySQLFields(w, "greeting")
	require.Error(t, err)
	// A fresh empty resultset has zero rows, not a fabricated NULL row.
	w = mysqlFieldsTestResult(false, []byte{0xfb})
	w = append(w[:len(w)-14], mysqlFieldsTestPacket(1, []byte{0xfe, 0, 0, 2, 0})...)
	fs, info, err := decodeMySQLFields(w, "text-resultset41")
	require.NoError(t, err)
	tlsCertificateTestCoverage(t, fs, len(w))
	require.Empty(t, info["Rows"])
	for _, count := range []int{4096, 4097} {
		attrs := bytes.Repeat([]byte{1, 'k', 0}, count)
		p := append([]byte(nil), fixtures["response41"][4:len(fixtures["response41"])-1]...)
		p = append(p, 0xfc, byte(len(attrs)), byte(len(attrs)>>8))
		p = append(p, attrs...)
		fs, info, err := decodeMySQLFields(mysqlFieldsTestPacket(1, p), "response41")
		if count == 4096 {
			require.NoError(t, err)
			require.Len(t, info["Connection Attributes"], count)
			tlsCertificateTestCoverage(t, fs, len(p)+4)
		} else {
			require.ErrorContains(t, err, "resource limit")
		}
		base := mysqlFieldsTestResult(false, []byte{0})
		wire := append([]byte(nil), base[:len(base)-14]...)
		for i := 0; i < count; i++ {
			wire = append(wire, mysqlFieldsTestPacket(byte(i+1), []byte{0})...)
		}
		wire = append(wire, mysqlFieldsTestPacket(byte(count+1), []byte{0xfe, 0, 0, 2, 0})...)
		fs, info, err = decodeMySQLFields(wire, "text-resultset41")
		if count == 4096 {
			require.NoError(t, err)
			require.Equal(t, count, info["Row Count"])
			tlsCertificateTestCoverage(t, fs, len(wire))
		} else {
			require.ErrorContains(t, err, "resource limit")
		}
	}
	for _, size := range []int{mysqlFieldsMaxBytes, mysqlFieldsMaxBytes + 1} {
		p := append([]byte{3}, bytes.Repeat([]byte{'x'}, size-5)...)
		fs, _, err := decodeMySQLFields(mysqlFieldsTestPacket(0, p), "command")
		if size == mysqlFieldsMaxBytes {
			require.NoError(t, err)
			tlsCertificateTestCoverage(t, fs, size)
		} else {
			require.Error(t, err)
		}
	}
}

func TestMySQLFieldsByteMutationBoundaries(t *testing.T) {
	for profile, original := range mysqlFieldsTestFixtures() {
		for at := range original {
			for _, value := range []byte{0, 1, 0xfb, 0xfc, 0xfd, 0xfe, 0xff} {
				wire := append([]byte(nil), original...)
				wire[at] = value
				fields, info, err := decodeMySQLFields(wire, profile)
				if err == nil {
					tlsCertificateTestCoverage(t, fields, len(wire))
					require.Equal(t, false, info["Session State Validated"])
				} else {
					require.Nil(t, fields, "no partially published fields")
					require.Nil(t, info, "no partially published metadata")
				}
			}
		}
	}
}
