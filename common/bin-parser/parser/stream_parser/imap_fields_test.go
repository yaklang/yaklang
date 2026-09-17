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

func imapFieldsTestFixtures() map[string]string {
	return map[string]string{
		"command":        "A01 UID FETCH 1:* (FLAGS RFC822.SIZE INTERNALDATE BODY.PEEK[HEADER])\r\n",
		"response":       "* 1 FETCH (UID 10 FLAGS (\\Recent) BODY[HEADER] {18}\r\nSubject: value\r\n\r\n)\r\n",
		"response-block": "* LIST () \"/\" \"Example Folder\"\r\nA02 OK completed\r\n",
	}
}

func TestIMAPFieldsOriginalMessagesAndPrefixes(t *testing.T) {
	f, err := os.Open("../../testdata/protocol-corpus/captures/ndpi/ndpi-imap.pcap")
	require.NoError(t, err)
	defer f.Close()
	r, err := pcapgo.NewReader(f)
	require.NoError(t, err)
	frames, appBytes, commands, responses := 0, 0, 0, 0
	var fetch []byte
	check := func(w []byte, profile string) map[string]any {
		fs, info, err := decodeIMAPFields(w, profile)
		require.NoError(t, err, "frame %d %s", frames, profile)
		tlsCertificateTestCoverage(t, fs, len(w))
		if profile == "response-block" {
			for _, m := range info["Responses"].([]map[string]any) {
				span := m["Relative Byte Range"].([2]int)
				unit := w[span[0]:span[1]]
				fs, _, err := decodeIMAPFields(unit, "response")
				require.NoError(t, err)
				tlsCertificateTestCoverage(t, fs, len(unit))
				for cut := 0; cut < len(unit); cut++ {
					_, _, err := decodeIMAPFields(unit[:cut], "response")
					require.Error(t, err, "response prefix %d", cut)
				}
				responses++
			}
		} else {
			for cut := 0; cut < len(w); cut++ {
				_, _, err := decodeIMAPFields(w[:cut], profile)
				require.Error(t, err, "command prefix %d", cut)
			}
			commands++
		}
		return info
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
		appBytes += len(w)
		if len(w) == 0 {
			continue
		}
		if frames == 30 || frames == 32 {
			fetch = append(fetch, w...)
			continue
		}
		profile := "response-block"
		if tcp.DstPort == 143 {
			profile = "command"
		}
		check(w, profile)
	}
	info := check(fetch, "response-block")
	require.Equal(t, 1, info["Literal Count"])
	m := info["Responses"].([]map[string]any)[0]
	attrs := m["Attributes"].([]map[string]any)
	require.Len(t, attrs, 5)
	require.Equal(t, "HEADER", attrs[4]["Section"])
	require.Equal(t, 666, attrs[4]["Octets"])
	require.Equal(t, 18, attrs[4]["Header Count"])
	require.Equal(t, true, attrs[4]["Header Layout Parsed"])
	require.Equal(t, false, attrs[4]["MIME Decoded"])
	require.Equal(t, 33, frames)
	require.Equal(t, 1580, appBytes)
	require.Equal(t, 6, commands)
	require.Equal(t, 19, responses)
}

func TestIMAPFieldsLayouts(t *testing.T) {
	for _, w := range []string{"a FETCH 1 BODY[1.]\r\n", "a FETCH 1 BODY[1HEADER]\r\n", "a FETCH 1 BODY[1.0]\r\n"} {
		_, _, err := decodeIMAPFields([]byte(w), "command")
		require.Error(t, err, w)
	}
	for _, w := range []string{"* 01 FETCH (UID 1)\r\n", "* 01 EXPUNGE\r\n"} {
		_, _, err := decodeIMAPFields([]byte(w), "response")
		require.Error(t, err, w)
	}
	for p, w := range imapFieldsTestFixtures() {
		f, _, err := decodeIMAPFields([]byte(w), p)
		require.NoError(t, err, p)
		tlsCertificateTestCoverage(t, f, len(w))
	}
	for _, w := range []string{
		"a NOOP\r\n", "a login {3}\r\none {3}\r\ntwo\r\n", "a LIST \"\" *\r\n", "a LSUB \"\" \"a\\\"b\\\\c\"\r\n", "a SELECT INBOX\r\n", "a EXAMINE NIL\r\n", "a COPY 4:2,*,5 a]b\r\n",
		"a UID COPY 1:* INBOX\r\n", "a fetch 4294967295 (BODY.PEEK[1.2.HEADER.FIELDS.NOT (From To)]<0.100> UID)\r\n", "a FETCH 1 BODY[]\r\n", "a FETCH 1 FAST\r\n", "a FETCH 1 BODY[1.MIME]\r\n",
	} {
		fs, _, err := decodeIMAPFields([]byte(w), "command")
		require.NoError(t, err, w)
		tlsCertificateTestCoverage(t, fs, len(w))
	}
	for _, w := range []string{
		"* 0 EXISTS\r\n", "* 0 RECENT\r\n", "* 1 EXPUNGE\r\n", "* SEARCH\r\n", "* SEARCH 1 2\r\n", "* FLAGS ()\r\n", "* LIST (\\Noselect) NIL INBOX\r\n", "* CAPABILITY AUTH=PLAIN IMAP4rev1\r\n",
		"* OK [PERMANENTFLAGS (\\Seen \\*)] done\r\n", "a OK [UIDNEXT 2] done\r\n", "a NO [X-EXAMPLE value] explanation\r\n", "+ ready\r\n", "* PREAUTH ready\r\n", "* BYE done\r\n",
		"* 1 FETCH (BODY[] NIL RFC822.SIZE 0)\r\n", "* 1 FETCH (BODY[] {4}\r\nx\r\ny)\r\n", "* 1 FETCH (INTERNALDATE \" 1-Jan-2026 00:00:00 +0000\")\r\n", "* 1 FETCH (BODY[HEADER]<0> \"partial\")\r\n",
	} {
		fs, _, err := decodeIMAPFields([]byte(w), "response")
		require.NoError(t, err, w)
		tlsCertificateTestCoverage(t, fs, len(w))
	}
	for _, w := range []string{
		"* 1 FETCH (UID 0)\r\n", "* 0 FETCH (UID 1)\r\n", "* 1 FETCH ()\r\n", "* 1 FETCH (BODY.PEEK[] NIL)\r\n", "* 1 FETCH (BODY[] {1}\r\n\x00)\r\n", "* 1 FETCH (BODY[] {3+}\r\nabc)\r\n", "* 1 FETCH (BODY[] \"bad\\x\")\r\n",
		"* 1 FETCH (BODY[] NILx)\r\n", "a BYE done\r\n", "a CAPABILITY IMAP4rev1\r\n", "* CAPABILITY IDLE\r\n", "* LIST () {1}\r\n/ INBOX\r\n", "* LIST () \"xx\" INBOX\r\n", "* 4294967296 EXISTS\r\n", "* 1 FETCH (INTERNALDATE \"31-Feb-2026 00:00:00 +0000\")\r\n",
	} {
		_, _, err := decodeIMAPFields([]byte(w), "response")
		require.Error(t, err, w)
	}
	for _, w := range []string{
		"a+ NOOP\r\n", "* NOOP\r\n", "a NOOP extra\r\n", "a SELECT\r\n", "a LOGIN one\r\n", "a UID NOOP\r\n", "a FETCH 0 FLAGS\r\n", "a FETCH 01 FLAGS\r\n", "a FETCH 1: FLAGS\r\n", "a FETCH 1, FLAGS\r\n", "a FETCH 1 ()\r\n", "a FETCH 1 (FAST)\r\n", "a FETCH 1 BODY.PEEK\r\n", "a FETCH 1 BODY[MIME]\r\n", "a FETCH 1 BODY[]<0.0>\r\n", "a FETCH 1 BODY[]<0>\r\n",
	} {
		_, _, err := decodeIMAPFields([]byte(w), "command")
		require.Error(t, err, w)
	}
	_, info, err := decodeIMAPFields([]byte("* OK [PERMANENTFLAGS ()]\r\n"), "response")
	require.NoError(t, err)
	require.Equal(t, true, info["Empty Response Text Observed"])
	require.Equal(t, false, info["Response Text Layout Validated"])
	_, info, err = decodeIMAPFields([]byte("* 1 FETCH (BODY[HEADER]<0> \"partial\")\r\n"), "response")
	require.NoError(t, err)
	require.NotContains(t, info["Attributes"].([]map[string]any)[0], "Headers")
}

func TestIMAPFieldsBridgeTransactions(t *testing.T) {
	testExactByteFieldsBridgeTransactions(t, []string{"command", "response", "response-block"}, func(p string) []byte { return []byte(imapFieldsTestFixtures()[p]) }, parseIMAPFields)
}

func TestIMAPFieldsLimits(t *testing.T) {
	_, _, err := decodeIMAPFields([]byte("a SELECT \""+strings.Repeat("\\\"", imapFieldsMaxLeaves)+"\"\r\n"), "command")
	require.ErrorContains(t, err, "field resource limit")
	for _, p := range []string{"command", "response", "response-block"} {
		for _, bits := range []uint64{0, 1, 7, imapFieldsMaxBytes*8 + 1, (imapFieldsMaxBytes + 1) * 8} {
			n := giopBridgeInlineRoot(t, "Package:\n  Boundary: raw,1\n")
			n.Cfg.SetItem(CfgLength, bits)
			calls := 0
			err := parseIMAPFields(n, func(*base.Node) (func(bool), error) { calls++; return nil, fmt.Errorf("must not read") }, p)
			require.Error(t, err)
			require.Zero(t, calls)
		}
	}
	for _, count := range []int{4096, 4097} {
		w := []byte("* SEARCH" + strings.Repeat(" 1", count) + "\r\n")
		_, _, err := decodeIMAPFields(w, "response")
		if count == 4096 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "resource limit")
		}
		w = []byte(strings.Repeat("* 0 EXISTS\r\n", count))
		_, _, err = decodeIMAPFields(w, "response-block")
		if count == 4096 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "resource limit")
		}
	}
	for _, size := range []int{imapFieldsMaxBytes, imapFieldsMaxBytes + 1} {
		w := append([]byte("* OK "), bytes.Repeat([]byte{'x'}, size-7)...)
		w = append(w, '\r', '\n')
		_, _, err := decodeIMAPFields(w, "response")
		if size == imapFieldsMaxBytes {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}
