package stream_parser

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func mimeFieldsTestRanges(t *testing.T, wire []byte, value any) []byte {
	t.Helper()
	var got []byte
	last := 0
	for _, span := range value.([][2]int) {
		require.GreaterOrEqual(t, span[0], last)
		require.Greater(t, span[1], span[0])
		require.LessOrEqual(t, span[1], len(wire))
		got = append(got, wire[span[0]:span[1]]...)
		last = span[1]
	}
	return got
}

func TestMIMEMessageFieldsTransfersAndProvenance(t *testing.T) {
	for _, spec := range []struct {
		media, encoding, encoded, decoded string
		text                              bool
	}{
		{"text/plain", "7bit", "..first\r\nsecond\r\n", ".first\r\nsecond\r\n", true},
		{"text/plain; charset=utf-8", "quoted-printable", "caf=C3=A9=\r\n=20line=2E\r\n", "café line.\r\n", true},
		{"text/html; charset=utf-8", "base64", "PH%$RleHQ+\r\n", "<text>", true},
		{"text/plain; charset=iso-8859-1", "base64", "6Q==\r\n", "\xe9", false},
		{"text/plain; charset=utf-8", "base64", "/w==\r\n", "\xff", false},
		{"application/octet-stream", "base64", "AAEC/w==\r\n", "\x00\x01\x02\xff", false},
	} {
		wire := []byte("+OK\r\nContent-Type: " + spec.media + "\r\nContent-Transfer-Encoding: " + spec.encoding + "\r\nContent-Disposition: attachment;\r\n filename=example.txt\r\nX-Example: first\r\n\tfold\r\n\r\n" + spec.encoded + ".\r\n")
		info, err := decodeMIMEDotMessage(wire, 5)
		require.NoError(t, err)
		require.Equal(t, true, info["All Transfers Decoded"])
		require.Equal(t, false, info["Content Rendered"])
		require.Equal(t, false, info["External Content Retrieved"])
		parts := info["Parts"].([]map[string]any)
		require.Len(t, parts, 1)
		p := parts[0]
		require.Equal(t, []byte(spec.decoded), p["Decoded Body"])
		require.Equal(t, len(spec.decoded), info["Decoded Octets"])
		require.Equal(t, spec.text, p["Text Available"])
		require.Equal(t, "attachment", p["Disposition"])
		require.Equal(t, "example.txt", p["Disposition Parameters"].(map[string]string)["filename"])
		body := mimeFieldsTestRanges(t, wire, p["Body Encoded Relative Byte Ranges"])
		require.Equal(t, strings.TrimPrefix(spec.encoded, "."), string(body))
		for _, h := range p["Headers"].([]map[string]any) {
			require.Equal(t, h["Unfolded Value"], string(mimeFieldsTestRanges(t, wire, h["Value Encoded Relative Byte Ranges"])))
			require.True(t, bytes.HasPrefix(mimeFieldsTestRanges(t, wire, h["Encoded Relative Byte Ranges"]), []byte(h["Name"].(string)+":")))
		}
		if spec.text {
			require.Equal(t, spec.decoded, p["Decoded Text"])
		} else {
			require.NotContains(t, p, "Decoded Text")
		}
	}
}

func TestMIMEMessageFieldsMultipartAndLimits(t *testing.T) {
	wrap := func(boundary, child string) string {
		return "Content-Type: multipart/mixed; boundary=" + boundary + "\r\n\r\npreamble\r\n--" + boundary + " \t\r\n" + child + "\r\n--" + boundary + "--\t\r\nepilogue\r\n"
	}
	child := "Content-Type: text/plain\r\n\r\n..dot\r\n--near-not-a-delimiter\r\n"
	wire := []byte("+OK\r\n" + wrap("near", child) + ".\r\n")
	info, err := decodeMIMEDotMessage(wire, 5)
	require.NoError(t, err)
	parts := info["Parts"].([]map[string]any)
	require.Len(t, parts, 2)
	require.Equal(t, "0.1", parts[1]["Path"])
	require.Equal(t, ".dot\r\n--near-not-a-delimiter\r\n", parts[1]["Decoded Text"])
	require.Equal(t, "preamble\r\n", string(mimeFieldsTestRanges(t, wire, parts[0]["Preamble Encoded Relative Byte Ranges"])))
	require.Equal(t, "epilogue\r\n", string(mimeFieldsTestRanges(t, wire, parts[0]["Epilogue Encoded Relative Byte Ranges"])))
	delims := parts[0]["Boundary Encoded Relative Byte Ranges"].([][][2]int)
	require.Len(t, delims, 2)
	require.Equal(t, "--near \t\r\n", string(mimeFieldsTestRanges(t, wire, delims[0])))
	require.Equal(t, "\r\n--near--\t\r\n", string(mimeFieldsTestRanges(t, wire, delims[1])))
	for _, count := range []int{63, 64} {
		wire := []byte("Content-Type: multipart/mixed; boundary=x\r\n\r\n" + strings.Repeat("--x\r\n\r\nbody\r\n", count) + "--x--\r\n.\r\n")
		info, err := decodeMIMEDotMessage(wire, 0)
		if count == 63 {
			require.NoError(t, err)
			require.Equal(t, 64, info["Part Count"])
		} else {
			require.ErrorContains(t, err, "resource limit")
		}
	}
	for _, depth := range []int{8, 9} {
		w := "\r\nbody\r\n"
		for i := 0; i < depth; i++ {
			w = wrap(fmt.Sprintf("part%d", i), w)
		}
		_, err := decodeMIMEDotMessage([]byte(w+".\r\n"), 0)
		if depth == 8 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "resource limit")
		}
	}
	for _, count := range []int{1024, 1025} {
		_, err := decodeMIMEDotMessage([]byte(strings.Repeat("X: value\r\n", count)+"\r\nbody\r\n.\r\n"), 0)
		if count == 1024 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "header resource")
		}
	}
	for _, message := range []string{"Content-Type: message/rfc822\r\n\r\nSubject: inner\r\n\r\nbody\r\n", "Content-Type: multipart/digest; boundary=x\r\n\r\n--x\r\n\r\nSubject: inner\r\n\r\nbody\r\n\r\n--x--\r\n"} {
		info, err := decodeMIMEDotMessage([]byte(message+".\r\n"), 0)
		require.NoError(t, err)
		parts := info["Parts"].([]map[string]any)
		require.Equal(t, "body\r\n", parts[len(parts)-1]["Decoded Text"])
	}
	// The accounting guard also rejects the next decoded leaf before publishing
	// it when a caller's entity budget is already full.
	r := &mimeFieldsReader{wire: []byte("\r\nx"), origins: []int{0, 1, 2}, decoded: mimeFieldsMaxDecoded, complete: true}
	require.ErrorContains(t, r.entity(0, 3, 0, "0", "text/plain"), "decoded resource limit")
}

func TestMIMEMessageFieldsMalformedAndOpaque(t *testing.T) {
	for _, message := range []string{
		"Content-Type: plain\r\n\r\nbody\r\n", "Content-Type: text/plain\r\nContent-Type: text/html\r\n\r\nbody\r\n",
		"Content-Type: multipart/mixed\r\n\r\nbody\r\n", "Content-Type: multipart/mixed; boundary=x\r\n\r\n--x\r\n\r\nbody\r\n",
		"Content-Transfer-Encoding: base64\r\n\r\nA\r\n", "Content-Type: multipart/mixed; boundary=x\r\nContent-Transfer-Encoding: base64\r\n\r\nAAAA\r\n",
		"Content-Type: multipart/mixed; boundary=\"bad!\"\r\n\r\n--bad!--\r\n",
	} {
		w := []byte("+OK\r\n" + message + ".\r\n")
		_, err := decodeMIMEDotMessage(w, 5)
		require.Error(t, err)
		f, info, err := decodePOP3Fields(w, "message")
		require.NoError(t, err, "valid transport remains available for an uninterpreted message")
		tlsCertificateTestCoverage(t, f, len(w))
		require.Equal(t, false, info["MIME Decoded"])
		require.NotEmpty(t, info["MIME Decoding Error"])
		require.NotContains(t, info, "MIME Fields", "no partly published MIME tree")
	}
	w := []byte("Content-Transfer-Encoding: x-example\r\n\r\nopaque\r\n.\r\n")
	info, err := decodeMIMEDotMessage(w, 0)
	require.NoError(t, err)
	require.Equal(t, false, info["All Transfers Decoded"])
	p := info["Parts"].([]map[string]any)[0]
	require.Equal(t, false, p["Body Transfer Decoded"])
	require.NotContains(t, p, "Decoded Body")
	require.Equal(t, "opaque\r\n", string(mimeFieldsTestRanges(t, w, p["Body Encoded Relative Byte Ranges"])))
	for _, w := range [][]byte{[]byte("\r\n"), []byte(".\r\nextra"), []byte("X: missing newline"), bytes.Repeat([]byte{'x'}, 1<<20+1)} {
		_, err := decodeMIMEDotMessage(w, 0)
		require.Error(t, err)
	}
}
