package stream_parser

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func imapSASLFixtures() map[string]string {
	return map[string]string{"response": "AGR1bW15AHNlY3JldA==\r\n", "challenge": "+ PGF1ZGl0QGV4YW1wbGU+\r\n", "initial-value": "="}
}

func TestIMAPSASLExactValuesAndOwnership(t *testing.T) {
	for profile, value := range imapSASLFixtures() {
		wire := []byte(value)
		fields, info, err := decodeIMAPSASL(wire, profile)
		require.NoError(t, err)
		tlsCertificateTestCoverage(t, fields, len(wire))
		require.Equal(t, false, info["Authentication Verified"])
		require.Equal(t, false, info["Decoded Values Are Wire Spans"])
		decoder := StructuredDecoderForProgram(fmt.Sprintf("err = parseIMAPSASL(%q)\nif err != nil { panic(err) }\n", profile))
		require.NotNil(t, decoder)
		_, metadata, err := decoder(wire)
		require.NoError(t, err)
		require.Equal(t, info, metadata)
		encoded := bytes.Clone(info["Encoded Value"].([]byte))
		clear(wire)
		require.Equal(t, encoded, info["Encoded Value"])
		if profile != "initial-value" {
			for cut := 0; cut < len(value); cut++ {
				_, _, err := decodeIMAPSASL([]byte(value[:cut]), profile)
				require.Error(t, err, "%s prefix %d", profile, cut)
			}
		}
	}
	for _, tc := range []struct {
		wire, profile   string
		canceled, empty bool
	}{
		{"*\r\n", "response", true, false}, {"=", "initial-value", false, true}, {"\r\n", "response", false, false}, {"+ \r\n", "challenge", false, false},
	} {
		fields, info, err := decodeIMAPSASL([]byte(tc.wire), tc.profile)
		require.NoError(t, err)
		tlsCertificateTestCoverage(t, fields, len(tc.wire))
		require.Equal(t, tc.canceled, info["Canceled"])
		require.Equal(t, tc.empty, info["Empty Initial Response"])
	}
	_, info, err := decodeIMAPSASL([]byte("Zh==\r\n"), "response")
	require.NoError(t, err)
	require.Equal(t, []byte("f"), info["Decoded Payload"])
	require.Equal(t, false, info["Base64 Canonical"])
	require.NotEmpty(t, info["Conformance Warning"])
}

func TestIMAPSASLRejectsInvalidValuesAndLimits(t *testing.T) {
	for _, value := range []string{"=", "A", "AA=", "AA=A", "AAAA=", "A===", "AAAA AAAA", "AAAA\r\nAAAA", "\x00AAA", "****", "AAAA\r"} {
		fields, info, err := decodeIMAPSASL([]byte(value+"\r\n"), "response")
		require.Error(t, err, "%q", value)
		require.Nil(t, fields)
		require.Nil(t, info)
	}
	for _, profile := range []string{"response", "challenge", "initial-value"} {
		testExactByteFieldsBridgeTransactions(t, []string{profile}, func(p string) []byte { return []byte(imapSASLFixtures()[p]) }, parseIMAPSASL)
		for _, bits := range []uint64{0, 1, 7, IMAPSASLMaxBytes*8 + 8} {
			n := giopBridgeInlineRoot(t, "Package:\n  Boundary: raw,1\n")
			n.Cfg.SetItem(CfgLength, bits)
			calls := 0
			err := parseIMAPSASL(n, func(*base.Node) (func(bool), error) { calls++; return nil, fmt.Errorf("must not read") }, profile)
			require.Error(t, err)
			require.Zero(t, calls)
		}
	}
	_, _, err := decodeIMAPSASL([]byte(strings.Repeat("A", IMAPSASLMaxBytes)+"\r\n"), "response")
	require.ErrorContains(t, err, "bound")
}

func FuzzIMAPSASLTransportValue(f *testing.F) {
	for _, wire := range imapSASLFixtures() {
		f.Add([]byte(wire), uint8(0))
	}
	f.Add([]byte("*\r\n"), uint8(0))
	f.Fuzz(func(t *testing.T, wire []byte, choice uint8) {
		if len(wire) > IMAPSASLMaxBytes+1 {
			return
		}
		profile := []string{"response", "challenge", "initial-value"}[choice%3]
		fields, info, err := decodeIMAPSASL(wire, profile)
		if err != nil {
			require.Nil(t, fields)
			require.Nil(t, info)
			return
		}
		tlsCertificateTestCoverage(t, fields, len(wire))
		require.Equal(t, false, info["Authentication Verified"])
		if info["Canceled"] != true && info["Empty Initial Response"] != true {
			decoded, err := base64.StdEncoding.DecodeString(string(info["Encoded Value"].([]byte)))
			require.NoError(t, err)
			require.Equal(t, decoded, info["Decoded Payload"])
		}
	})
}
