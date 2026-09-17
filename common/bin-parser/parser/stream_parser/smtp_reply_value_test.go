package stream_parser

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSMTPReplyValueGrammar(t *testing.T) {
	for _, wire := range []string{"220 ready\r\n", "220\r\n", "220 \r\n", "250-\r\n250\r\n", "250-First line\r\n250-Second line\r\n250-234 Text beginning with numbers\r\n250 The last line\r\n", "550 \t\r\n"} {
		lines, info, err := decodeSMTPReply([]byte(wire))
		require.NoError(t, err, wire)
		require.Equal(t, len(wire), lines[len(lines)-1].End)
		require.Equal(t, len(lines), info["Line Count"])
		require.Equal(t, false, info["Sender Conformance Validated"])
		last := lines[len(lines)-1]
		require.Equal(t, last.Separator == ' ' && last.TextStart == last.TextEnd, info["Receiver Empty Final Text"])
		at := 0
		for _, line := range lines {
			require.Equal(t, at, line.Start)
			require.LessOrEqual(t, line.TextStart, line.TextEnd)
			require.Equal(t, "\r\n", wire[line.End-2:line.End])
			at = line.End
		}
		for cut := 0; cut < len(wire); cut++ {
			_, _, err := decodeSMTPReply([]byte(wire[:cut]))
			require.Error(t, err, "prefix %d of %q", cut, wire)
		}
	}
	for a := 0; a <= 9; a++ {
		for b := 0; b <= 9; b++ {
			for c := 0; c <= 9; c++ {
				_, _, err := decodeSMTPReply([]byte(fmt.Sprintf("%d%d%d\r\n", a, b, c)))
				require.Equal(t, a < 2 || a > 5 || b > 5, err != nil)
			}
		}
	}
	for value := 0; value < 256; value++ {
		_, _, err := decodeSMTPReply([]byte{'2', '5', '0', ' ', byte(value), '\r', '\n'})
		require.Equal(t, value != 9 && (value < 32 || value > 126), err != nil, "text octet %d", value)
	}
	for _, wire := range []string{"250-x\r\n251 y\r\n", "250 x\r\n250 y\r\n", "250-x\r\n", "250-x\r\n250-y\r\n", "250\tx\r\n", "250x\r\n", "250 x\n", "250 \rX\r\n", "250 x\r\ny", "250- x\r\nnot a reply\r\n"} {
		lines, info, err := decodeSMTPReply([]byte(wire))
		require.Error(t, err, wire)
		require.Nil(t, lines)
		require.Nil(t, info)
	}
}

func TestSMTPReplyValueLimits(t *testing.T) {
	line := append([]byte("250-"), bytes.Repeat([]byte{'x'}, 506)...)
	line = append(line, '\r', '\n')
	require.Len(t, line, 512)
	maximum := bytes.Repeat(line, smtpReplyMaxLines)
	maximum[len(maximum)-512+3] = ' '
	lines, _, err := decodeSMTPReply(maximum)
	require.NoError(t, err)
	require.Len(t, lines, smtpReplyMaxLines)
	_, _, err = decodeSMTPReply(append(maximum, 'x'))
	require.ErrorContains(t, err, "reply size outside")
	tooMany := append(bytes.Repeat([]byte("250-\r\n"), smtpReplyMaxLines), []byte("250\r\n")...)
	_, _, err = decodeSMTPReply(tooMany)
	require.ErrorContains(t, err, "1024 line")
	_, _, err = decodeSMTPReply(append(append([]byte("250 "), bytes.Repeat([]byte{'x'}, 507)...), '\r', '\n'))
	require.ErrorContains(t, err, "512 octets")
}
