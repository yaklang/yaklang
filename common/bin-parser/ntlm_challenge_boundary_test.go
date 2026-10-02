package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

func TestNTLMChallengeVersionAndPayloadBoundaries(t *testing.T) {
	named := ntlmsspChallengeTarget("DOMAIN")
	withoutVersion := append(bytes.Clone(named[:48]), named[56:]...)
	binary.LittleEndian.PutUint32(withoutVersion[20:], binary.LittleEndian.Uint32(named[20:])&^0x02000000)
	binary.LittleEndian.PutUint32(withoutVersion[16:], 48)
	binary.LittleEndian.PutUint32(withoutVersion[44:], uint32(len(withoutVersion)))
	gap := append(append(bytes.Clone(named[:56]), 1, 2, 3, 4), named[56:]...)
	binary.LittleEndian.PutUint32(gap[16:], 60)
	infoAfter := append(bytes.Clone(named), 0, 0, 0, 0) // MsvAvEOL
	binary.LittleEndian.PutUint32(infoAfter[20:], binary.LittleEndian.Uint32(named[20:])|0x00800000)
	binary.LittleEndian.PutUint16(infoAfter[40:], 4)
	binary.LittleEndian.PutUint16(infoAfter[42:], 4)
	binary.LittleEndian.PutUint32(infoAfter[44:], uint32(len(named)))
	infoBefore := append(append(bytes.Clone(infoAfter[:56]), 0, 0, 0, 0), named[56:]...)
	binary.LittleEndian.PutUint32(infoBefore[16:], 60)
	binary.LittleEndian.PutUint32(infoBefore[44:], 56)
	oemName := append(bytes.Clone(named[:56]), 0, 'L', 'A', 'B')
	binary.LittleEndian.PutUint32(oemName[20:], binary.LittleEndian.Uint32(named[20:])&^1)
	binary.LittleEndian.PutUint16(oemName[12:], 3)
	binary.LittleEndian.PutUint16(oemName[14:], 3)
	binary.LittleEndian.PutUint32(oemName[16:], 57)

	for _, tc := range []struct {
		name    string
		input   []byte
		target  []byte
		version bool
	}{
		{"version-only", ntlmsspChallenge([]byte{1, 2, 3, 4, 5, 6, 7, 8}), nil, true},
		{"version-and-name", named, utf16LE("DOMAIN"), true},
		{"version-flag-clear", withoutVersion, utf16LE("DOMAIN"), false},
		{"padding-before-name", gap, utf16LE("DOMAIN"), true},
		{"info-after-name", infoAfter, utf16LE("DOMAIN"), true},
		{"info-before-name", infoBefore, utf16LE("DOMAIN"), true},
		{"oem-name-allows-odd-offset-and-length", oemName, []byte("LAB"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := bytes.NewReader(append(bytes.Clone(tc.input), 0xde, 0xad))
			node, err := parser.ParseBinary(stream, "application-layer.ntlm", "NTLMSSP")
			require.NoError(t, err)
			parsed, err := node.Result()
			require.NoError(t, err)
			require.Equal(t, 2, stream.Len())
			if tc.version {
				require.Equal(t, uint64(10), uintVal(t, mustChild(t, parsed, "Version", "ProductMajorVersion")))
				require.Equal(t, uint64(19041), uintVal(t, mustChild(t, parsed, "Version", "ProductBuild")))
			} else {
				require.Nil(t, parsed.Child("Version"))
			}
			if tc.target != nil {
				require.Equal(t, tc.target, bytesVal(t, parsed.Child("Target Name")))
			}
			protocolCorpusRequireBoundedRuleParse(t, tc.input, "application-layer.ntlm", "NTLMSSP")
			// NLMP permits padding after the referenced payload within an explicit
			// message boundary; a stream's unframed suffix is not that padding.
			protocolCorpusRequireBoundedRuleParse(t, append(bytes.Clone(tc.input), 0, 1, 2), "application-layer.ntlm", "NTLMSSP")
			for cut := 0; cut < len(tc.input); cut++ {
				t.Run(fmt.Sprintf("short-%d", cut), func(t *testing.T) {
					_, err := parser.ParseBinary(bytes.NewReader(tc.input[:cut]), "application-layer.ntlm", "NTLMSSP")
					require.Error(t, err)
					_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(tc.input[:cut]), "application-layer.ntlm", "NTLMSSP")
					require.Error(t, err)
				})
			}
		})
	}
}

func TestNTLMChallengeRejectsOriginalMissingVersionAndBadOffsets(t *testing.T) {
	// Exact pre-correction helper output: VERSION was set, but the 8-byte
	// Version was absent. The named fixture put its payload at offset 48.
	originalEmpty := mustHex(t, "4e544c4d53535000020000000000000000000000058208e2010203040506070800000000000000000000000000000000")
	originalNamed := mustHex(t, "4e544c4d53535000020000000c000c0030000000058208e201020304050607080000000000000000000000003c00000044004f004d00410049004e00")
	require.Len(t, originalEmpty, 48)
	require.Len(t, originalNamed, 60)
	require.Equal(t, ntlmsspChallenge([]byte{1, 2, 3, 4, 5, 6, 7, 8})[:48], originalEmpty)
	corrected := ntlmsspChallengeTarget("DOMAIN")
	reconstructed := append(bytes.Clone(corrected[:48]), corrected[56:]...)
	binary.LittleEndian.PutUint32(reconstructed[16:], 48)
	binary.LittleEndian.PutUint32(reconstructed[44:], 60)
	require.Equal(t, reconstructed, originalNamed)
	cases := map[string][]byte{"original-empty": originalEmpty, "original-named": originalNamed}
	for name, offset := range map[string]uint32{"name-in-version": 48, "name-in-header": 16, "name-past-end": 80, "name-offset-overflow": 0xffffffff} {
		input := ntlmsspChallengeTarget("DOMAIN")
		binary.LittleEndian.PutUint32(input[16:], offset)
		cases[name] = input
	}
	for name, offset := range map[string]uint32{"info-in-version": 48, "info-past-end": 80, "info-offset-overflow": 0xffffffff} {
		input := ntlmsspChallengeTarget("DOMAIN")
		binary.LittleEndian.PutUint32(input[20:], binary.LittleEndian.Uint32(input[20:])|0x00800000)
		binary.LittleEndian.PutUint16(input[40:], 4)
		binary.LittleEndian.PutUint32(input[44:], offset)
		cases[name] = input
	}
	oddLength := ntlmsspChallengeTarget("DOMAIN")
	binary.LittleEndian.PutUint16(oddLength[12:], 11)
	cases["unicode-odd-length"] = oddLength
	oddOffset := append(ntlmsspChallengeTarget("DOMAIN"), 0)
	binary.LittleEndian.PutUint32(oddOffset[16:], 57)
	cases["unicode-odd-offset"] = oddOffset
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parser.ParseBinary(bytes.NewReader(input), "application-layer.ntlm", "NTLMSSP")
			require.Error(t, err)
			_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(input), "application-layer.ntlm", "NTLMSSP")
			require.Error(t, err)
		})
	}
}

func TestNTLMChallengeIgnoresReceiveOnlyReservedValues(t *testing.T) {
	input := ntlmsspChallengeTarget("DOMAIN")
	// MS-NLMP requires MaxLength and Reserved to be ignored on receipt.
	// Version is diagnostic only; no version-number or reserved-bit allowlist.
	binary.LittleEndian.PutUint16(input[14:], 0)
	input[32], input[48], input[52], input[55] = 0xff, 0xfe, 0xff, 0xfe
	binary.LittleEndian.PutUint32(input[20:], binary.LittleEndian.Uint32(input[20:])|0x01000000)
	parsed := parseRule(t, input, "application-layer.ntlm", "NTLMSSP")
	require.Equal(t, utf16LE("DOMAIN"), bytesVal(t, parsed.Child("Target Name")))
	protocolCorpusRequireBoundedRuleParse(t, input, "application-layer.ntlm", "NTLMSSP")
	input = ntlmsspChallenge(nil)
	binary.LittleEndian.PutUint32(input[20:], binary.LittleEndian.Uint32(input[20:])&^0x00800004)
	// A descriptor without its enabling flag is not a pointer to follow.
	for _, region := range [][]byte{input[12:20], input[40:48]} {
		for i := range region {
			region[i] = 0xff
		}
	}
	parsed = parseRule(t, input, "application-layer.ntlm", "NTLMSSP")
	require.Nil(t, parsed.Child("Target Name"))
	protocolCorpusRequireBoundedRuleParse(t, input, "application-layer.ntlm", "NTLMSSP")
}
