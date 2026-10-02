package stream_parser

import (
	"bytes"
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

func pop3TestFixtures() map[string]string {
	return map[string]string{
		"command": "TOP 7 0\r\n", "status": "+OK example\r\n", "stat": "+OK 3 19191\r\n",
		"list": "+OK\r\n1 123\r\n2 456 extra\r\n.\r\n", "uidl": "+OK\r\n1 sample-id\r\n2 sample-id\r\n.\r\n",
		"capa":      "+OK\r\nTOP\r\nSASL PLAIN\r\nIMPLEMENTATION example\r\n.\r\n",
		"message":   "+OK\r\nSubject: example\r\n\tfolded\r\n\r\n..text\r\n.\r\n",
		"challenge": "+ YWJj\r\n", "client-continuation": "YWJj\r\n", "list-one": "+OK 1 123\r\n", "uidl-one": "+OK 1 sample-id\r\n",
	}
}

func TestPOP3FieldsLayouts(t *testing.T) {
	for profile, wire := range pop3TestFixtures() {
		t.Run(profile, func(t *testing.T) {
			f, info, err := decodePOP3Fields([]byte(wire), profile)
			require.NoError(t, err)
			tlsCertificateTestCoverage(t, f, len(wire))
			require.Equal(t, profile, info["Layout Context"])
			require.Equal(t, false, info["Session State Validated"])
			for cut := 0; cut < len(wire); cut++ {
				_, _, err := decodePOP3Fields([]byte(wire[:cut]), profile)
				require.Error(t, err)
			}
			_, _, err = decodePOP3Fields([]byte(wire+wire), profile)
			require.Error(t, err)
		})
	}
	for _, wire := range []string{"user sample", "pass a b c", "quit", "LIST", "UIDL", "RETR 1", "DELE 99", "STAT", "NOOP", "RSET", "CAPA", "STLS", "AUTH", "AUTH PLAIN", "AUTH PLAIN =", "AUTH PLAIN YWJj", "APOP sample 0123456789abcdef0123456789abcdef"} {
		f, info, err := decodePOP3Fields([]byte(wire+"\r\n"), "command")
		require.NoError(t, err)
		tlsCertificateTestCoverage(t, f, len(wire)+2)
		require.Equal(t, wire == "AUTH", info["Legacy AUTH Probe"])
		require.Equal(t, wire != "AUTH", info["Command Argument Layout Validated"])
	}
	for _, wire := range []string{".\r\n", "+OKAY\r\n", "-ERRx\r\n", "+ok\r\n", "+OK\tvalue\r\n", "+OK value\n", "+OK bad\x00\r\n"} {
		_, _, err := decodePOP3Fields([]byte(wire), "status")
		require.Error(t, err)
	}
	for _, wire := range []string{"QUIT x", "STAT ", "USER", "USER a b", "PASS", "RETR 0", "RETR -1", "TOP 1", "TOP 1 -1", "TOP 0 0", "UIDL ", "LIST 1 2", "APOP a abc", "AUTH ", "AUTH plain", "AUTH PLAIN ", "AUTH PLAIN =A", "AUTH PLAIN YQ=", "AUTH PLAIN YR==", "AUTH PLAIN YWJj extra", "AUTH AAAAAAAAAAAAAAAAAAAAA", "FOO"} {
		_, _, err := decodePOP3Fields([]byte(wire+"\r\n"), "command")
		require.Error(t, err, wire)
	}
	for _, profile := range []string{"status", "stat", "list-one", "uidl-one", "list", "uidl", "capa", "message"} {
		for _, wire := range []string{"-ERR\r\n", "-ERR [SYS/TEMP] example\r\n"} {
			f, info, err := decodePOP3Fields([]byte(wire), profile)
			require.NoError(t, err)
			require.Equal(t, false, info["Positive Status Observed"])
			tlsCertificateTestCoverage(t, f, len(wire))
		}
		_, _, err := decodePOP3Fields([]byte("-ERR\r\n.\r\n"), profile)
		require.Error(t, err)
	}
	for _, profile := range []string{"list", "uidl", "capa"} {
		f, info, err := decodePOP3Fields([]byte("+OK\r\n.\r\n"), profile)
		require.NoError(t, err)
		require.Equal(t, 0, info["Item Count"])
		tlsCertificateTestCoverage(t, f, 8)
	}
	for _, spec := range []struct{ p, w string }{
		{"stat", "+OK 0 0 extra\r\n"}, {"list-one", "+OK 0001 0 extra\r\n"}, {"uidl-one", "+OK 1 sample\r\n"}, {"challenge", "+ \r\n"}, {"client-continuation", "\r\n"}, {"client-continuation", "*\r\n"},
	} {
		f, _, err := decodePOP3Fields([]byte(spec.w), spec.p)
		require.NoError(t, err)
		tlsCertificateTestCoverage(t, f, len(spec.w))
	}
	for _, spec := range []struct{ p, w string }{
		{"stat", "+OK ready\r\n"}, {"stat", "+OK 1 18446744073709551616\r\n"}, {"list-one", "+OK 0 12\r\n"}, {"list", "+OK\r\n1  3\r\n.\r\n"}, {"uidl", "+OK\r\n1 sample extra\r\n.\r\n"}, {"uidl-one", "+OK 1 \r\n"}, {"capa", "+OK\r\nSASL  PLAIN\r\n.\r\n"}, {"capa", "+OK\r\n.X\r\n.\r\n"}, {"capa", "+OK\r\n\r\n.\r\n"}, {"challenge", "+\r\n"}, {"challenge", "+ *\r\n"}, {"client-continuation", "=\r\n"}, {"client-continuation", "YQ=\r\n"}, {"client-continuation", "YQ== \r\n"},
	} {
		_, _, err := decodePOP3Fields([]byte(spec.w), spec.p)
		require.Error(t, err, spec)
	}
}

func TestPOP3FieldsBridgeTransactions(t *testing.T) {
	var profiles []string
	for p := range pop3TestFixtures() {
		profiles = append(profiles, p)
	}
	testExactByteFieldsBridgeTransactions(t, profiles, func(p string) []byte { return []byte(pop3TestFixtures()[p]) }, parsePOP3Fields)
}

func TestPOP3FieldsLimits(t *testing.T) {
	for _, size := range []int{255, 256} {
		_, _, err := decodePOP3Fields([]byte("PASS "+strings.Repeat("a", size-7)+"\r\n"), "command")
		if size == 255 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	for _, size := range []int{512, 513} {
		_, _, err := decodePOP3Fields([]byte("+OK "+strings.Repeat("a", size-6)+"\r\n"), "status")
		if size == 512 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	for _, size := range []int{70, 71} {
		_, _, err := decodePOP3Fields([]byte("+OK 1 "+strings.Repeat("a", size)+"\r\n"), "uidl-one")
		if size == 70 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	for _, count := range []int{4096, 4097} {
		_, _, err := decodePOP3Fields([]byte("+OK\r\n"+strings.Repeat("1 0\r\n", count)+".\r\n"), "list")
		if count == 4096 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "list resource")
		}
	}
	for profile := range pop3TestFixtures() {
		limit := uint64(pop3FieldsLimit(profile)) * 8
		for _, bits := range []uint64{0, 1, 7, limit + 1, limit + 8} {
			n := giopBridgeInlineRoot(t, "Package:\n  Boundary: raw,1\n")
			n.Cfg.SetItem(CfgLength, bits)
			calls := 0
			err := parsePOP3Fields(n, func(*base.Node) (func(bool), error) { calls++; return nil, fmt.Errorf("must not read") }, profile)
			require.Error(t, err)
			require.Zero(t, calls)
		}
	}
}

func TestPOP3FieldsOriginalMessagesAndPrefixes(t *testing.T) {
	f, err := os.Open("../../testdata/protocol-corpus/captures/ndpi/ndpi-pop3.pcap")
	require.NoError(t, err)
	defer f.Close()
	r, err := pcapgo.NewNgReader(f, pcapgo.DefaultNgReaderOptions)
	require.NoError(t, err)
	groups := map[int]int{20: 20, 22: 20, 24: 20, 113: 113, 114: 113, 116: 113, 117: 113, 119: 113, 121: 121, 122: 121, 124: 121, 125: 121, 127: 121, 128: 121, 130: 121, 132: 132, 133: 132, 135: 132, 136: 132, 138: 132}
	messages := map[int][]byte{}
	frames, wireBytes, units := 0, 0, 0
	check := func(w []byte, p string) {
		f, _, err := decodePOP3Fields(w, p)
		require.NoError(t, err, "frame %d context %s", frames, p)
		tlsCertificateTestCoverage(t, f, len(w))
		for cut := 0; cut < len(w); cut++ {
			_, _, err := decodePOP3Fields(w[:cut], p)
			require.Error(t, err, "%s prefix %d", p, cut)
		}
		units++
	}
	for {
		frame, _, err := r.ReadPacketData()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		frames++
		tcp := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default).Layer(layers.LayerTypeTCP).(*layers.TCP)
		w := tcp.Payload
		wireBytes += len(w)
		if len(w) == 0 {
			continue
		}
		if group, ok := groups[frames]; ok {
			messages[group] = append(messages[group], w...)
			continue
		}
		profile := "status"
		if tcp.DstPort == 110 {
			profile = "command"
		}
		switch frames {
		case 56, 85, 104:
			profile = "client-continuation"
		case 55, 84, 103:
			profile = "challenge"
		case 14, 109:
			profile = "list"
		case 17, 111:
			profile = "uidl"
		case 39, 53, 69, 82, 101:
			profile = "capa"
		case 107:
			profile = "stat"
		}
		check(w, profile)
	}
	wantBodies := map[int][]struct {
		size int
		sha  string
	}{
		20:  {{303, "77e70c861ba0b2bd1f35952ff264d7e2987befd9e2e110e23593cb72fe7a297d"}},
		113: {{1543, "480b1066b1d71b7e331ff5e86d2ca43952d200438c866bb90d48aebb80506b42"}, {1915, "0ad0d0d0c04c846cf2d91d90d8be25fdf86c949184c341f7b357230412f43701"}},
		121: {{1203, "cb0b68b8c95a1c4893ecec3cfed900a0f121552784c2ab10f371fd9d8e771fd2"}, {3490, "e36b61c4be01d944a06e6f41b1daee659e6e6e8130835421f5919e63d890b6d2"}},
		132: {{1972, "bd6f11c970f2e4f747c4abf94babbaf0138ac319e7930e0e51097dad5d8cc342"}},
	}
	parts, decodedOctets, headerCount := 0, 0, 0
	for id, w := range messages {
		check(w, "message")
		_, info, err := decodePOP3Fields(w, "message")
		require.NoError(t, err)
		require.NotContains(t, info, "MIME Decoding Error", "message %d", id)
		require.Equal(t, true, info["MIME Decoded"])
		mime := info["MIME Fields"].(map[string]any)
		decodedOctets += mime["Decoded Octets"].(int)
		bodyIndex := 0
		for _, part := range mime["Parts"].([]map[string]any) {
			parts++
			headerCount += len(part["Headers"].([]map[string]any))
			if part["Body Transfer Decoded"] == true {
				want := wantBodies[id][bodyIndex]
				require.Equal(t, want.size, part["Decoded Octets"], "message %d body %d", id, bodyIndex)
				require.Equal(t, want.sha, part["Decoded SHA256"], "message %d body %d", id, bodyIndex)
				require.Equal(t, true, part["Text Available"])
				bodyIndex++
			}
		}
		require.Equal(t, len(wantBodies[id]), bodyIndex)
	}
	require.Equal(t, 8, parts)
	require.Equal(t, 10426, decodedOctets)
	require.Equal(t, 108, headerCount)
	require.Equal(t, 144, frames)
	require.Equal(t, 22700, wireBytes)
	require.Equal(t, 66, units)
	// Independent byte-count evidence: no padding from record 107 is treated
	// as part of its complete 13-byte STAT reply.
	require.Len(t, messages, 4)
	for _, w := range messages {
		require.True(t, bytes.HasSuffix(w, []byte("\r\n.\r\n")))
	}
}
