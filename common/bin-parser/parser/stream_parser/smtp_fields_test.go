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

func TestSMTPFieldsCommands(t *testing.T) {
	for _, line := range []string{
		"EHLO example.test", "HELO localhost", "ehlo [IPv6:2001:db8::1]", "EHLO [001.002.003.004]",
		"MAIL From:<sample@example.test>", "MAIL FROM:<>", "RCPT To:<Postmaster>",
		`MAIL FROM:<"a> @ \\"@example.test> SIZE=123 BODY=7BIT X-1`,
		"RCPT TO:<@a.test,@b.test:sample@example.test>", "RCPT TO:<a.b+q@[192.0.2.1]>",
		"RCPT TO:<x@[IPv6:::ffff:192.0.2.1]>", "RSET", "DATA", "QUIT", "NOOP", "HELP",
		`HELP "MAIL FROM"`, "VRFY sample", `EXPN "sample list"`, "NOOP sample", "QUIT \t", "EHLO example.test  \t",
	} {
		t.Run(line, func(t *testing.T) {
			wire := []byte(line + "\r\n")
			fields, info, err := decodeSMTPCommandFields(wire)
			require.NoError(t, err)
			tlsCertificateTestCoverage(t, fields, len(wire))
			require.False(t, info["Delivery Outcome Validated"].(bool))
			for cut := 0; cut < len(wire); cut++ {
				_, _, err := decodeSMTPCommandFields(wire[:cut])
				require.Error(t, err)
			}
			_, _, err = decodeSMTPCommandFields(append(bytes.Clone(wire), wire...))
			require.Error(t, err)
		})
	}
	for _, line := range []string{
		"", "EHLO", "EHLO ", "HELO [192.0.2.1]", "EHLO _bad.test", "EHLO a..test", "EHLO -a.test", "EHLO a-.test", "EHLO a.test.",
		"EHLO [256.0.0.1]", "EHLO [IPv6:192.0.2.1]", "EHLO [Other:value]", "EHLO [1.2.3]", "EHLO [1.2.3.4.5]",
		"MAIL FROM:<", "MAIL FROM:", "MAIL FROM:<@", "MAIL FROM:<@a.test>", "MAIL FROM:<@a.test:>", "MAIL FROM:<postmaster>",
		"RCPT TO:<>", "MAIL FROM:<a..b@example.test>", "MAIL FROM:<a@>", "MAIL FROM:<@example.test>",
		`MAIL FROM:<"x\"@example.test>`, "MAIL FROM:<a@example.test> X=", "MAIL FROM:<a@example.test> X=a=b", "MAIL FROM:<a@example.test> -X=1",
		"MAIL FROM:<a@example.test>  X=1", "MAIL FROM:<a@example.test>\tX=1", "MAIL FROM:<a@example.test>X=1",
		"MAIL FROM:<" + strings.Repeat("a", 65) + "@example.test>", "DATA x", "RSET x", "QUIT x", "QUITNOW", "NOOP a b", "VRFY", "EXPN", "HELO a\nb", "HELO a\rb", "HELO é", "HELO\texample.test",
		"AUTH PLAIN value", "STARTTLS",
	} {
		_, _, err := decodeSMTPCommandFields([]byte(line + "\r\n"))
		require.Error(t, err, "%q", line)
	}
}

func TestSMTPFieldsData(t *testing.T) {
	wire := []byte("Subject: example\r\n\tfolded\r\nX-Empty:\r\n\r\n..first\r\n...second\r\n\r\n.\r\n")
	f, info, err := decodeSMTPDataFields(wire)
	require.NoError(t, err)
	tlsCertificateTestCoverage(t, f, len(wire))
	require.Equal(t, 2, info["Header Count"])
	require.Equal(t, 1, info["Fold Count"])
	require.Equal(t, 3, info["Body Line Count"])
	require.Equal(t, 2, info["Transparency Dot Count"])
	require.Equal(t, true, info["Dot Transparency Canonical"])
	h := info["Headers"].([]map[string]any)
	require.Equal(t, " example\tfolded", h[0]["Unfolded Value"])
	require.Equal(t, "", h[1]["Unfolded Value"])
	require.Equal(t, [][2]int{{8, 16}, {18, 25}}, h[0]["Value Relative Byte Ranges"])
	for cut := 0; cut < len(wire); cut++ {
		_, _, err := decodeSMTPDataFields(wire[:cut])
		require.Error(t, err)
	}
	for _, valid := range []string{".\r\n", "\r\n.\r\n", "X: value\r\n.\r\n", "\r\n\x00\x7f\t\v\f\r\n.\r\n", "..X: value\r\n.\r\n", "X: a\r\n.\tfold\r\n\r\n.single\r\n.\r\n"} {
		f, info, err := decodeSMTPDataFields([]byte(valid))
		require.NoError(t, err)
		tlsCertificateTestCoverage(t, f, len(valid))
		if strings.Contains(valid, ".single") {
			require.Equal(t, false, info["Dot Transparency Canonical"])
			require.Equal(t, " a\tfold", info["Headers"].([]map[string]any)[0]["Unfolded Value"])
		}
	}
	for _, invalid := range []string{"", ".", ".\r", ".\r\nx", "X: a\r\n\r\nbody", "\tfold\r\n.\r\n", "X bad: value\r\n.\r\n", ": value\r\n.\r\n", "X: \x00\r\n.\r\n", "X: a\nb\r\n.\r\n", "X: a\rb\r\n.\r\n", "\r\né\r\n.\r\n"} {
		_, _, err := decodeSMTPDataFields([]byte(invalid))
		require.Error(t, err, "%q", invalid)
	}
}

func TestSMTPFieldsBridgeTransactions(t *testing.T) {
	testExactByteFieldsBridgeTransactions(t, []string{"command", "data"}, func(profile string) []byte {
		if profile == "data" {
			return []byte("Subject: example\r\n\tfolded\r\n\r\n..text\r\n.\r\n")
		}
		return []byte("MAIL FROM:<sample@example.test> SIZE=123\r\n")
	}, func(n *base.Node, process func(*base.Node) (func(bool), error), profile string) error {
		return parseSMTPFields(n, process, profile == "data")
	})
}

func TestSMTPFieldsLimits(t *testing.T) {
	for _, size := range []int{511, 512, 513} {
		w := []byte("NOOP " + strings.Repeat("a", size-7) + "\r\n")
		_, _, err := decodeSMTPCommandFields(w)
		if size <= 512 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	for _, size := range []int{64, 65} {
		_, _, err := decodeSMTPCommandFields([]byte("MAIL FROM:<" + strings.Repeat("a", size) + "@example.test>\r\n"))
		if size == 64 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	for _, size := range []int{256, 257} {
		_, _, err := decodeSMTPCommandFields([]byte("MAIL FROM:<a@" + strings.Repeat("d", size-4) + ">\r\n"))
		if size == 256 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	for _, size := range []int{255, 256} {
		_, _, err := decodeSMTPCommandFields([]byte("EHLO " + strings.Repeat("d", size) + "\r\n"))
		if size == 255 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	for _, size := range []int{998, 999} {
		for _, prefix := range []string{"", ".."} {
			w := []byte("\r\n" + prefix + strings.Repeat("a", size-len(prefix)/2) + "\r\n.\r\n")
			_, _, err := decodeSMTPDataFields(w)
			if size == 998 {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		}
	}
	for _, count := range []int{1024, 1025} {
		w := []byte(strings.Repeat("X:\r\n", count) + "\r\n.\r\n")
		_, _, err := decodeSMTPDataFields(w)
		if count == 1024 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "header resource")
		}
	}
	for _, count := range []int{8192, 8193} {
		w := []byte(strings.Repeat("\r\n", count) + ".\r\n")
		_, _, err := decodeSMTPDataFields(w)
		if count == 8192 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "line resource")
		}
	}
	for _, size := range []int{smtpFieldsMaxBytes, smtpFieldsMaxBytes + 1} {
		w := []byte("\r\n")
		for size-len(w)-3 > 1000 {
			w = append(w, []byte(strings.Repeat("a", 998)+"\r\n")...)
		}
		w = append(w, []byte(strings.Repeat("a", size-len(w)-5)+"\r\n.\r\n")...)
		require.Len(t, w, size)
		f, _, err := decodeSMTPDataFields(w)
		if size == smtpFieldsMaxBytes {
			require.NoError(t, err)
			tlsCertificateTestCoverage(t, f, size)
		} else {
			require.Error(t, err)
		}
	}
	for _, data := range []bool{false, true} {
		limit := uint64(512 * 8)
		if data {
			limit = smtpFieldsMaxBytes * 8
		}
		for _, bits := range []uint64{0, 1, 7, limit + 1, limit + 8} {
			n := giopBridgeInlineRoot(t, "Package:\n  Boundary: raw,1\n")
			n.Cfg.SetItem(CfgLength, bits)
			calls := 0
			err := parseSMTPFields(n, func(*base.Node) (func(bool), error) { calls++; return nil, fmt.Errorf("must not read") }, data)
			require.Error(t, err)
			require.Zero(t, calls)
		}
	}
}

func TestSMTPFieldsOriginalPrefixes(t *testing.T) {
	f, err := os.Open("../../testdata/protocol-corpus/captures/ndpi/ndpi-smtp.pcap")
	require.NoError(t, err)
	defer f.Close()
	r, err := pcapgo.NewReader(f)
	require.NoError(t, err)
	var data []byte
	frames, commands, replies := 0, 0, 0
	for {
		frame, _, err := r.ReadPacketData()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		frames++
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
		tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
		w := tcp.Payload
		if len(w) == 0 {
			continue
		}
		if frames >= 76 && frames <= 88 {
			data = append(data, w...)
			continue
		}
		if tcp.SrcPort == 25 {
			replies++
			_, _, err := decodeSMTPReply(w)
			require.NoError(t, err)
			for cut := 0; cut < len(w); cut++ {
				_, _, err := decodeSMTPReply(w[:cut])
				require.Error(t, err)
			}
		} else {
			commands++
			fields, _, err := decodeSMTPCommandFields(w)
			require.NoError(t, err)
			tlsCertificateTestCoverage(t, fields, len(w))
			for cut := 0; cut < len(w); cut++ {
				_, _, err := decodeSMTPCommandFields(w[:cut])
				require.Error(t, err)
			}
		}
	}
	require.Equal(t, 95, frames)
	require.Equal(t, 35, commands)
	require.Equal(t, 37, replies)
	require.Len(t, data, 15267)
	fields, info, err := decodeSMTPDataFields(data)
	require.NoError(t, err)
	tlsCertificateTestCoverage(t, fields, len(data))
	require.Equal(t, 5, info["Header Count"])
	require.Equal(t, 35, info["Fold Count"])
	for cut := 0; cut < len(data); cut++ {
		_, _, err := decodeSMTPDataFields(data[:cut])
		require.Error(t, err, "original DATA prefix %d", cut)
	}
}
