package stream_parser

import (
	"bytes"
	"encoding/base64"
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
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func postgresqlFieldsTestTyped(typ byte, body []byte) []byte {
	w := make([]byte, 5)
	w[0] = typ
	binary.BigEndian.PutUint32(w[1:], uint32(4+len(body)))
	return append(w, body...)
}
func postgresqlFieldsTestUntyped(code uint32, body []byte) []byte {
	w := make([]byte, 8)
	binary.BigEndian.PutUint32(w, uint32(8+len(body)))
	binary.BigEndian.PutUint32(w[4:], code)
	return append(w, body...)
}
func postgresqlFieldsTestColumn() []byte {
	return postgresqlFieldsTestTyped('T', []byte{0, 1, 'x', 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 23, 0, 4, 255, 255, 255, 255, 0, 1})
}
func postgresqlFieldsTestFixtures() map[string][]byte {
	verifier := base64.StdEncoding.EncodeToString(make([]byte, 32))
	first := []byte("n,,n=,r=nonce")
	initial := append([]byte("SCRAM-SHA-256\x00"), 0, 0, 0, byte(len(first)))
	initial = append(initial, first...)
	backend := append(postgresqlFieldsTestColumn(), postgresqlFieldsTestTyped('D', []byte{0, 1, 0, 0, 0, 4, 0, 0, 0, 3})...)
	return map[string][]byte{
		"startup":     postgresqlFieldsTestUntyped(196608, []byte("user\x00example\x00database\x00demo\x00\x00")),
		"ssl-request": postgresqlFieldsTestUntyped(80877103, nil), "gss-request": postgresqlFieldsTestUntyped(80877104, nil), "cancel": postgresqlFieldsTestUntyped(80877102, make([]byte, 8)),
		"ssl-response": []byte("N"), "gss-response": []byte("G"),
		"frontend":            postgresqlFieldsTestTyped('P', []byte("statement\x00select $1\x00\x00\x01\x00\x00\x00\x17")),
		"backend":             postgresqlFieldsTestColumn(),
		"frontend-block":      append(postgresqlFieldsTestTyped('B', []byte{0, 0, 0, 1, 0, 1, 0, 1, 255, 255, 255, 255, 0, 1, 0, 1}), postgresqlFieldsTestTyped('S', nil)...),
		"backend-block":       backend,
		"backend-scram":       postgresqlFieldsTestTyped('R', append([]byte{0, 0, 0, 11}, []byte("r=nonce,s=c2FsdA==,i=4096")...)),
		"backend-scram-block": append(postgresqlFieldsTestTyped('R', append([]byte{0, 0, 0, 12}, []byte("v="+verifier)...)), postgresqlFieldsTestTyped('R', []byte{0, 0, 0, 0})...),
		"password":            postgresqlFieldsTestTyped('p', []byte("sample\x00")),
		"sasl-initial":        postgresqlFieldsTestTyped('p', initial),
		"sasl-response":       postgresqlFieldsTestTyped('p', []byte{0, 255, 1}),
		"sasl-scram-response": postgresqlFieldsTestTyped('p', []byte("c=biws,r=nonce,p="+verifier)),
	}
}
func TestPostgreSQLFieldsOriginalMessagesAndPrefixes(t *testing.T) {
	f, err := os.Open("../../testdata/protocol-corpus/captures/ndpi/ndpi-postgresql.pcap")
	require.NoError(t, err)
	defer f.Close()
	reader, err := pcapgo.NewReader(f)
	require.NoError(t, err)
	frames, appBytes, messages, typed, blocks := 0, 0, 0, 0, 0
	for {
		record, _, err := reader.ReadPacketData()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		frames++
		p := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
		tcp := p.Layer(layers.LayerTypeTCP).(*layers.TCP)
		w := tcp.Payload
		if len(w) == 0 {
			continue
		}
		appBytes += len(w)
		profile := "backend-block"
		if tcp.DstPort == 5432 {
			profile = "frontend-block"
		}
		switch frames {
		case 7, 9, 55, 80:
			profile = "startup"
		case 43, 66:
			profile = "gss-request"
		case 45, 68:
			profile = "gss-response"
		case 51, 74:
			profile = "ssl-request"
		case 53, 78:
			profile = "ssl-response"
		case 15, 16:
			profile = "password"
		case 83:
			profile = "sasl-initial"
		case 84, 87:
			profile = "backend-scram-block"
		case 86:
			profile = "sasl-scram-response"
		}
		fs, info, err := decodePostgreSQLFields(w, profile)
		require.NoError(t, err, "frame %d", frames)
		tlsCertificateTestCoverage(t, fs, len(w))
		check := func(unit []byte, profile string) {
			fs, _, err := decodePostgreSQLFields(unit, profile)
			require.NoError(t, err)
			tlsCertificateTestCoverage(t, fs, len(unit))
			for cut := 0; cut < len(unit); cut++ {
				_, _, err := decodePostgreSQLFields(unit[:cut], profile)
				require.Error(t, err, "frame %d %s prefix %d", frames, profile, cut)
			}
			messages++
		}
		if strings.HasSuffix(profile, "-block") {
			blocks++
			for _, m := range info["Messages"].([]map[string]any) {
				span := m["Relative Byte Range"].([2]int)
				check(w[span[0]:span[1]], strings.TrimSuffix(profile, "-block"))
				typed++
			}
		} else {
			check(w, profile)
			if profile == "password" || strings.HasPrefix(profile, "sasl-") {
				typed++
			}
		}
		if frames == 25 {
			v := info["Messages"].([]map[string]any)[6]["Values"].([]map[string]any)[0]
			require.Equal(t, int64(3), v["Decoded int4"])
		}
		if frames == 84 {
			attrs := info["Messages"].([]map[string]any)[0]["SCRAM Attributes"].([]map[string]any)
			require.Equal(t, uint64(4096), attrs[2]["Integer"])
		}
	}
	require.Equal(t, 88, frames)
	require.Equal(t, 2993, appBytes)
	require.Equal(t, 117, messages)
	require.Equal(t, 105, typed)
	require.Equal(t, 19, blocks)
}
func TestPostgreSQLFieldsFixturesAndByteMutations(t *testing.T) {
	for profile, wire := range postgresqlFieldsTestFixtures() {
		fs, _, err := decodePostgreSQLFields(wire, profile)
		require.NoError(t, err, profile)
		tlsCertificateTestCoverage(t, fs, len(wire))
		for at := range wire {
			for _, b := range []byte{0, 1, 0x7f, 0x80, 0xff} {
				w := append([]byte(nil), wire...)
				w[at] = b
				fs, info, err := decodePostgreSQLFields(w, profile)
				if err == nil {
					tlsCertificateTestCoverage(t, fs, len(w))
				} else {
					require.Nil(t, fs)
					require.Nil(t, info)
				}
			}
		}
	}
}
func TestPostgreSQLFieldsBridgeTransactions(t *testing.T) {
	fixtures := postgresqlFieldsTestFixtures()
	profiles := make([]string, 0, len(fixtures))
	for p := range fixtures {
		profiles = append(profiles, p)
	}
	testExactByteFieldsBridgeTransactions(t, profiles, func(p string) []byte { return bytes.Clone(fixtures[p]) }, parsePostgreSQLFields)
}
func TestPostgreSQLFieldsKnownLayouts(t *testing.T) {
	for _, tc := range []struct {
		profile string
		typ     byte
		body    []byte
	}{
		{"frontend", 'Q', []byte("select 1\x00")}, {"frontend", 'C', []byte("Sname\x00")}, {"frontend", 'D', []byte("P\x00")}, {"frontend", 'E', []byte{0, 0, 0, 0, 1}}, {"frontend", 'H', nil}, {"frontend", 'X', nil}, {"frontend", 'S', nil},
		{"frontend", 'd', []byte{0, 255}}, {"frontend", 'c', nil}, {"frontend", 'f', []byte("text\x00")},
		{"backend", 'K', make([]byte, 8)}, {"backend", 'Z', []byte("E")}, {"backend", '1', nil}, {"backend", '2', nil}, {"backend", '3', nil}, {"backend", 'n', nil}, {"backend", 'I', nil}, {"backend", 's', nil}, {"backend", 'C', []byte("SELECT 1\x00")},
		{"backend", 't', []byte{0, 1, 0, 0, 0, 23}}, {"backend", 'E', []byte("Mtext\x00SERROR\x00Zfuture\x00\x00")}, {"backend", 'N', []byte("Mnotice\x00\x00")}, {"backend", 'A', []byte{0, 0, 0, 1, 'c', 0, 'x', 0}},
		{"backend", 'G', []byte{0, 0, 1, 0, 0}}, {"backend", 'H', []byte{1, 0, 1, 0, 1}}, {"backend", 'W', []byte{1, 0, 0}}, {"backend", 'd', nil}, {"backend", 'c', nil}, {"backend", 'V', []byte{255, 255, 255, 255}},
		{"backend", 'D', []byte{0, 2, 255, 255, 255, 255, 0, 0, 0, 0}}, {"backend", 'T', []byte{0, 0}},
	} {
		w := postgresqlFieldsTestTyped(tc.typ, tc.body)
		fs, _, err := decodePostgreSQLFields(w, tc.profile)
		require.NoError(t, err, "%s %c", tc.profile, tc.typ)
		tlsCertificateTestCoverage(t, fs, len(w))
	}
	for _, code := range []byte{0, 2, 3, 5, 7, 8, 9, 10, 11, 12} {
		b := []byte{0, 0, 0, code}
		switch code {
		case 5:
			b = append(b, 0, 1, 2, 3)
		case 8, 11, 12:
			b = append(b, 0xff)
		case 10:
			b = append(b, []byte("SCRAM-SHA-256\x00SCRAM-SHA-256-PLUS\x00\x00")...)
		}
		w := postgresqlFieldsTestTyped('R', b)
		fs, _, err := decodePostgreSQLFields(w, "backend")
		require.NoError(t, err)
		tlsCertificateTestCoverage(t, fs, len(w))
	}
	for _, tc := range []struct {
		profile string
		typ     byte
		body    []byte
	}{
		{"frontend", 'p', []byte("name\x00")}, {"frontend", 'D', []byte("X\x00")}, {"frontend", 'E', []byte{0, 255, 255, 255, 255}}, {"frontend", 'S', []byte{0}},
		{"backend", 'Z', []byte("X")}, {"backend", 'D', []byte{0, 1, 255, 255, 255, 254}}, {"backend", 'D', []byte{0, 1, 0, 0, 0, 2, 1}}, {"backend", 'E', []byte("Mtext\x00")}, {"backend", 'E', []byte{0}}, {"backend", 'G', []byte{0, 0, 1, 0, 1}},
		{"backend", 'R', []byte{0, 0, 0, 5, 1, 2, 3}}, {"backend", 'R', []byte{0, 0, 0, 10, 0}}, {"backend", 'R', []byte{0, 0, 0, 10, 'X', 0}}, {"backend", 'R', []byte{0, 0, 0, 99}},
		{"sasl-initial", 'p', []byte("EXAMPLE\x00\xff\xff\xff\xfe")}, {"sasl-initial", 'p', []byte("EXAMPLE\x00\x00\x00\x00\x02x")},
	} {
		_, _, err := decodePostgreSQLFields(postgresqlFieldsTestTyped(tc.typ, tc.body), tc.profile)
		require.Error(t, err, "%s %c", tc.profile, tc.typ)
	}
	_, _, err := decodePostgreSQLFields(postgresqlFieldsTestUntyped(196608, []byte("database\x00demo\x00\x00")), "startup")
	require.ErrorContains(t, err, "user parameter")
	_, _, err = decodePostgreSQLFields(postgresqlFieldsTestUntyped(196610, []byte("user\x00example\x00\x00")), "startup")
	require.Error(t, err)
}
func TestPostgreSQLFieldsLocalRowContext(t *testing.T) {
	col := postgresqlFieldsTestColumn()
	row := postgresqlFieldsTestTyped('D', []byte{0, 1, 0, 0, 0, 4, 255, 255, 255, 255})
	for _, boundary := range [][]byte{nil, postgresqlFieldsTestTyped('C', []byte("SELECT 1\x00")), postgresqlFieldsTestTyped('Z', []byte("I")), postgresqlFieldsTestTyped('E', []byte("Mtext\x00\x00"))} {
		wire := append(bytes.Clone(col), boundary...)
		wire = append(wire, row...)
		_, info, err := decodePostgreSQLFields(wire, "backend-block")
		require.NoError(t, err)
		msgs := info["Messages"].([]map[string]any)
		m := msgs[len(msgs)-1]
		if boundary == nil {
			require.Equal(t, true, m["Row Metadata Applied"])
			require.Equal(t, int64(-1), m["Values"].([]map[string]any)[0]["Decoded int4"])
		} else {
			require.Equal(t, false, m["Row Metadata Applied"])
		}
	}
	_, info, err := decodePostgreSQLFields(row, "backend")
	require.NoError(t, err)
	require.Equal(t, false, info["Row Metadata Applied"])
	_, info, err = decodePostgreSQLFields(append(bytes.Clone(col), postgresqlFieldsTestTyped('D', []byte{0, 0})...), "backend-block")
	require.NoError(t, err)
	require.Equal(t, true, info["Messages"].([]map[string]any)[1]["Row Metadata Mismatch"])
}
func TestPostgreSQLFieldsLimits(t *testing.T) {
	for p := range postgresqlFieldsTestFixtures() {
		for _, bits := range []uint64{0, 1, 7, postgresqlFieldsMaxBytes*8 + 1, (postgresqlFieldsMaxBytes + 1) * 8} {
			n := giopBridgeInlineRoot(t, "Package:\n  Boundary: raw,1\n")
			n.Cfg.SetItem(CfgLength, bits)
			calls := 0
			err := parsePostgreSQLFields(n, func(*base.Node) (func(bool), error) { calls++; return nil, fmt.Errorf("must not read") }, p)
			require.Error(t, err)
			require.Zero(t, calls)
		}
	}
	for _, count := range []int{4096, 4097} {
		w := bytes.Repeat(postgresqlFieldsTestTyped('1', nil), count)
		fs, info, err := decodePostgreSQLFields(w, "backend-block")
		if count == 4096 {
			require.NoError(t, err)
			require.Equal(t, count, info["Message Count"])
			tlsCertificateTestCoverage(t, fs, len(w))
		} else {
			require.ErrorContains(t, err, "resource limit")
		}
		b := append([]byte{byte(count >> 8), byte(count)}, bytes.Repeat([]byte{255, 255, 255, 255}, count)...)
		w = postgresqlFieldsTestTyped('D', b)
		fs, info, err = decodePostgreSQLFields(w, "backend")
		if count == 4096 {
			require.NoError(t, err)
			require.Len(t, info["Values"], count)
			tlsCertificateTestCoverage(t, fs, len(w))
		} else {
			require.ErrorContains(t, err, "resource limit")
		}
	}
	for _, size := range []int{postgresqlFieldsMaxBytes, postgresqlFieldsMaxBytes + 1} {
		w := postgresqlFieldsTestTyped('d', bytes.Repeat([]byte{'x'}, size-5))
		fs, _, err := decodePostgreSQLFields(w, "frontend")
		if size == postgresqlFieldsMaxBytes {
			require.NoError(t, err)
			tlsCertificateTestCoverage(t, fs, len(w))
		} else {
			require.Error(t, err)
		}
	}
}

func TestPostgreSQLFieldsNestedResourceAndFormatLimits(t *testing.T) {
	// Eight leaves per minimal column, plus three per RowDescription header.
	// Two descriptions (4096 and 4095 columns) plus CopyDone reach 65536.
	var w []byte
	for _, count := range []int{4096, 4095} {
		body := []byte{byte(count >> 8), byte(count)}
		body = append(body, make([]byte, count*19)...)
		w = append(w, postgresqlFieldsTestTyped('T', body)...)
	}
	w = append(w, postgresqlFieldsTestTyped('c', nil)...)
	fs, _, err := decodePostgreSQLFields(w, "backend-block")
	require.NoError(t, err)
	tlsCertificateTestCoverage(t, fs, len(w))
	w = append(w, postgresqlFieldsTestTyped('c', nil)...)
	fs, info, err := decodePostgreSQLFields(w, "backend-block")
	require.ErrorContains(t, err, "field resource limit")
	require.Nil(t, fs)
	require.Nil(t, info)
	for _, formatCount := range []int{0, 1, 2, 3} {
		body := []byte{0, 0, 0, byte(formatCount)} // portal, statement, format count
		for i := 0; i < formatCount; i++ {
			body = append(body, 0, 1)
		}
		body = append(body, 0, 2, 255, 255, 255, 255, 0, 0, 0, 0, 0, 0) // two values, no result formats
		fs, info, err := decodePostgreSQLFields(postgresqlFieldsTestTyped('B', body), "frontend")
		if formatCount == 3 {
			require.ErrorContains(t, err, "format count mismatch")
			require.Nil(t, fs)
			require.Nil(t, info)
		} else {
			require.NoError(t, err)
			want := uint64(0)
			if formatCount > 0 {
				want = 1
			}
			for _, v := range info["Parameters"].([]map[string]any) {
				require.Equal(t, want, v["Format Code"])
			}
		}
	}
}
