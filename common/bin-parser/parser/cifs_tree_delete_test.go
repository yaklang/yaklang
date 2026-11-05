package parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// Generated protocol fixtures, not copies of an external PCAP. The four normal
// bodies mirror the independently frozen 23..26 source layouts and strings;
// header PID high/security bytes deliberately exercise additional widths.
func cifsFileCommandFixture(tree, response bool, password []byte, text, service string) []byte {
	fixed, command, wc := 41, byte(6), byte(1)
	if response {
		fixed, wc = 39, 0
	}
	if tree {
		fixed, command, wc = 47, 0x75, 4
		if response {
			fixed, wc = 45, 3
		}
	}
	w := make([]byte, fixed)
	copy(w[4:8], []byte{0xff, 'S', 'M', 'B'})
	w[8], w[13], w[36] = command, 8, wc
	if response {
		w[13] |= 0x80
	}
	binary.LittleEndian.PutUint16(w[14:16], 0xc803)
	binary.LittleEndian.PutUint16(w[16:18], 0x1234)
	for i := 18; i < 26; i++ {
		w[i] = byte(i)
	}
	binary.LittleEndian.PutUint16(w[30:32], 32719)
	binary.LittleEndian.PutUint16(w[32:34], 2048)
	binary.LittleEndian.PutUint16(w[34:36], 5)
	if response || !tree {
		binary.LittleEndian.PutUint16(w[28:30], 2048)
	}
	if tree {
		binary.LittleEndian.PutUint16(w[34:36], 4)
		w[37] = 0xff
		if response {
			binary.LittleEndian.PutUint16(w[39:41], 54) // ignored terminal offset from source
			binary.LittleEndian.PutUint16(w[41:43], 1)
			w = append(w, []byte(service)...)
			w = append(w, 0)
			if len(w)%2 != 0 {
				w = append(w, 0xa5)
			}
			w = append(w, cifsFileCommandUTF16(text)...)
		} else {
			binary.LittleEndian.PutUint16(w[43:45], uint16(len(password)))
			w = append(w, password...)
			if len(w)%2 != 0 {
				w = append(w, 0)
			}
			w = append(w, cifsFileCommandUTF16(text)...)
			w = append(w, []byte(service)...)
			w = append(w, 0)
		}
	} else if !response {
		binary.LittleEndian.PutUint16(w[37:39], 0x16) // includes receiver-ignored directory bit
		w = append(w, 4)
		if len(w)%2 != 0 {
			w = append(w, 0xa5)
		}
		w = append(w, cifsFileCommandUTF16(text)...)
	}
	cifsFileCommandFixLengths(w, fixed)
	return w
}

func cifsFileCommandUTF16(s string) []byte {
	var out []byte
	for _, u := range utf16.Encode([]rune(s)) {
		out = binary.LittleEndian.AppendUint16(out, u)
	}
	return append(out, 0, 0)
}

func cifsFileCommandFixLengths(w []byte, fixed int) {
	n := len(w) - 4
	w[1], w[2], w[3] = byte(n>>16), byte(n>>8), byte(n)
	binary.LittleEndian.PutUint16(w[fixed-2:fixed], uint16(len(w)-fixed))
}

func cifsFileCommandEntry(tree, response bool) string {
	prefix := "CIFSDirectTCPDelete"
	if tree {
		prefix = "CIFSDirectTCPTreeConnectAndX"
	}
	if response {
		return prefix + "Response"
	}
	return prefix + "Request"
}

func cifsFileCommandProject(tree bool, n *base.Node, wire []byte) (map[string]any, error) {
	if tree {
		return ProjectCIFSTreeConnectAndXFields(n, wire)
	}
	return ProjectCIFSDeleteFields(n, wire)
}

func cifsFileCommandValidate(tree bool, wire []byte, response bool) error {
	if tree {
		return stream_parser.ValidateCIFSTreeConnectAndXProfile(wire, response)
	}
	return stream_parser.ValidateCIFSDeleteProfile(wire, response)
}

func cifsFileCommandParse(t *testing.T, tree, response bool, wire []byte) (*base.Node, map[string]any) {
	t.Helper()
	r := &structuredReader{Reader: bytes.NewReader(wire), bits: uint64(len(wire)) * 8}
	n, err := ParseBinary(r, "application-layer.cifs", cifsFileCommandEntry(tree, response))
	require.NoError(t, err)
	require.Zero(t, r.Len())
	f, err := cifsFileCommandProject(tree, n, wire)
	require.NoError(t, err)
	structured, err := ParseStructured(wire, "application-layer.cifs", cifsFileCommandEntry(tree, response))
	require.NoError(t, err)
	require.Equal(t, f, structured["fields"], "fast/fallback paths must preserve typed ranges and text")
	return n, f
}

func cifsFileCommandAssertRange(t *testing.T, owner map[string]any, key string, lo, hi uint64) {
	t.Helper()
	require.Equal(t, []any{lo, hi}, owner["Wire Field Ranges"].(map[string]any)[key], key)
}

// The actual literal leaves partition the complete PDU. Decoded string views
// intentionally overlap their own raw leaves, so exclude just those views.
func cifsFileCommandAssertCoverage(t *testing.T, f map[string]any, size int) {
	t.Helper()
	var spans [][2]uint64
	for _, name := range []string{"Direct TCP", "Header", "Body"} {
		owner := f[name].(map[string]any)
		for key, value := range owner["Wire Field Ranges"].(map[string]any) {
			if key == "Path" || key == "Service" || key == "Native File System" || key == "File Name" {
				continue
			}
			s := value.([]any)
			spans = append(spans, [2]uint64{s[0].(uint64), s[1].(uint64)})
		}
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i][0] == spans[j][0] {
			return spans[i][1] < spans[j][1]
		}
		return spans[i][0] < spans[j][0]
	})
	at := uint64(0)
	for _, s := range spans {
		require.Equal(t, at, s[0], "gap/overlap at %v", s)
		require.GreaterOrEqual(t, s[1], s[0])
		at = s[1]
	}
	require.Equal(t, uint64(size), at)
}

func TestCIFSTreeDeleteExplicitTypedSourceRanges(t *testing.T) {
	for _, tc := range []struct {
		tree, response bool
		text, service  string
		size           int
	}{
		{true, false, `\\192.168.114.129\TEST`, "?????", 100},
		{true, true, "NTFS", "A:", 58},
		{false, false, "scan-getinfo.dat", "", 76},
		{false, true, "", "", 39},
	} {
		t.Run(cifsFileCommandEntry(tc.tree, tc.response), func(t *testing.T) {
			w := cifsFileCommandFixture(tc.tree, tc.response, nil, tc.text, tc.service)
			require.Len(t, w, tc.size)
			n, f := cifsFileCommandParse(t, tc.tree, tc.response, w)
			body, header := f["Body"].(map[string]any), f["Header"].(map[string]any)
			require.Equal(t, uint16(0x1234), header["PID High"])
			require.Equal(t, uint16(32719), header["Process ID Low"])
			require.Equal(t, uint32(0), header["Status"])
			for _, key := range []string{"Authentication Verified", "Signature Verified", "Session Established"} {
				require.Equal(t, false, f[key])
			}
			require.Empty(t, f["Wire Field Ranges"], "derived metadata must not invent response/context ranges")
			cifsFileCommandAssertRange(t, header, "PID High", 16, 18)
			cifsFileCommandAssertRange(t, header, "Process ID Low", 30, 32)
			cifsFileCommandAssertCoverage(t, f, len(w))
			if tc.tree && !tc.response {
				require.Equal(t, []byte{}, body["Password"])
				require.Equal(t, []byte{0}, body["Alignment Pad"])
				require.Equal(t, tc.text, body["Path"])
				require.Equal(t, "?????", body["Service"])
				cifsFileCommandAssertRange(t, body, "Password", 47, 47)
				cifsFileCommandAssertRange(t, body, "Alignment Pad", 47, 48)
				cifsFileCommandAssertRange(t, body, "Path", 48, 92)
				cifsFileCommandAssertRange(t, body, "Path Raw Bytes", 48, 94)
				cifsFileCommandAssertRange(t, body, "Service", 94, 99)
			} else if tc.tree {
				require.Equal(t, uint16(54), body["AndX Offset"])
				require.Equal(t, true, f["AndX Offset Ignored"])
				require.Equal(t, "NTFS", body["Native File System"])
				cifsFileCommandAssertRange(t, body, "Service Raw Bytes", 45, 48)
				cifsFileCommandAssertRange(t, body, "Alignment Pad", 48, 48)
				cifsFileCommandAssertRange(t, body, "Native File System", 48, 56)
			} else if !tc.response {
				require.Equal(t, uint16(0x16), body["Search Attributes"])
				require.Equal(t, uint8(4), body["Buffer Format"])
				require.Equal(t, tc.text, body["File Name"])
				cifsFileCommandAssertRange(t, body, "File Name", 42, 74)
				cifsFileCommandAssertRange(t, body, "File Name Raw Bytes", 42, 76)
			} else {
				require.Equal(t, uint8(0), body["Word Count"])
				require.Equal(t, uint16(0), body["Byte Count"])
				require.NotContains(t, body, "File Name")
			}
			original := bytes.Clone(w)
			security := bytes.Clone(header["Security Features"].([]byte))
			// The first projection and the retained canonical Node own their bytes.
			w[18] ^= 1
			_, err := cifsFileCommandProject(tc.tree, n, w)
			require.Error(t, err, "a changed raw header cannot authorize old Node leaves")
			clear(w)
			require.Equal(t, security, header["Security Features"])
			delayed, err := cifsFileCommandProject(tc.tree, n, original)
			require.NoError(t, err)
			require.Equal(t, f, delayed)
		})
	}
}

func TestCIFSTreeDeleteAlignmentEncodingAndIgnoredFields(t *testing.T) {
	for _, response := range []bool{false, true} {
		for _, password := range [][]byte{{}, {0}, {1, 2}, {1, 0, 3}} {
			for _, service := range []string{"A:", "IPC", "COMM", "LPT1:", "OTHER", ""} {
				for _, offset := range []uint16{0, 1, 54, 65535} {
					w := cifsFileCommandFixture(true, response, password, "A🔒", service)
					binary.LittleEndian.PutUint16(w[14:16], binary.LittleEndian.Uint16(w[14:16])&^0x0800)
					binary.LittleEndian.PutUint16(w[39:41], offset)
					w[38] = 255 // receiver-ignored AndXReserved
					_, f := cifsFileCommandParse(t, true, response, w)
					b := f["Body"].(map[string]any)
					require.Equal(t, service, b["Service"])
					require.Equal(t, offset, b["AndX Offset"])
					require.Equal(t, uint8(255), b["AndX Reserved"])
					if !response {
						require.Equal(t, password, b["Password"])
					}
					cifsFileCommandAssertCoverage(t, f, len(w))
				}
			}
		}
	}
	for _, tree := range []bool{false, true} {
		w := cifsFileCommandFixture(tree, false, nil, "", "")
		_, f := cifsFileCommandParse(t, tree, false, w)
		key := "File Name"
		if tree {
			key = "Path"
		}
		require.Equal(t, "", f["Body"].(map[string]any)[key])
		cifsFileCommandAssertCoverage(t, f, len(w))
	}
	// A successful DELETE WC0 response has no SMB_STRING or Unicode constraint.
	w := cifsFileCommandFixture(false, true, nil, "", "")
	binary.LittleEndian.PutUint16(w[14:16], 0)
	_, _ = cifsFileCommandParse(t, false, true, w)
	w = cifsFileCommandFixture(false, false, nil, "A", "")
	binary.LittleEndian.PutUint16(w[14:16], 0x8000)
	_, _ = cifsFileCommandParse(t, false, false, w)
	w = cifsFileCommandFixture(true, true, nil, "", "")
	_, f := cifsFileCommandParse(t, true, true, w)
	require.Equal(t, "", f["Body"].(map[string]any)["Native File System"])
}

func TestCIFSTreeDeleteLiteralByteCountMaximum(t *testing.T) {
	for _, tc := range []struct {
		tree, response bool
		text, service  string
		maximum, fixed int
	}{
		{true, false, "", strings.Repeat("A", 65531), stream_parser.CIFSTreeConnectAndXRequestMaxBytes, 47},
		{true, true, "", strings.Repeat("A", 65531), stream_parser.CIFSTreeConnectAndXResponseMaxBytes, 45},
		{false, false, strings.Repeat("A", 32766), "", stream_parser.CIFSDeleteRequestMaxBytes, 41},
		{false, true, "", "", stream_parser.CIFSDeleteResponseMaxBytes, 39},
	} {
		t.Run(cifsFileCommandEntry(tc.tree, tc.response), func(t *testing.T) {
			w := cifsFileCommandFixture(tc.tree, tc.response, nil, tc.text, tc.service)
			require.Len(t, w, tc.maximum)
			require.NoError(t, cifsFileCommandValidate(tc.tree, w, tc.response))
			_, f := cifsFileCommandParse(t, tc.tree, tc.response, w)
			cifsFileCommandAssertCoverage(t, f, len(w))
			bad := append(bytes.Clone(w), 0)
			cifsFileCommandFixLengths(bad, tc.fixed)
			require.Error(t, cifsFileCommandValidate(tc.tree, bad, tc.response), "ByteCount must not wrap into a larger accepted record")
		})
	}
}

func TestCIFSTreeDeleteStrictProfileBoundaries(t *testing.T) {
	for _, tree := range []bool{false, true} {
		for _, response := range []bool{false, true} {
			w := cifsFileCommandFixture(tree, response, nil, "A", "A:")
			entry := cifsFileCommandEntry(tree, response)
			fixed := 41
			if response {
				fixed = 39
			}
			if tree {
				fixed = 47
				if response {
					fixed = 45
				}
			}
			t.Run(entry, func(t *testing.T) {
				for end := 0; end < len(w); end++ {
					require.Error(t, cifsFileCommandValidate(tree, w[:end], response), "cut %d", end)
					_, err := ParseStructured(w[:end], "application-layer.cifs", entry)
					require.Error(t, err, "cut %d", end)
				}
				mutations := map[string]func([]byte){
					"prefix":    func(b []byte) { b[0] = 1 },
					"length":    func(b []byte) { b[3]++ },
					"signature": func(b []byte) { b[4] = 0xfe },
					"command":   func(b []byte) { b[8] ^= 1 },
					"direction": func(b []byte) { b[13] ^= 0x80 },
					"wc":        func(b []byte) { b[36]++ },
					"bytecount": func(b []byte) { b[fixed-2]++ },
				}
				if !response || tree {
					mutations["unicode"] = func(b []byte) { b[15] &^= 0x80 }
				}
				if tree {
					mutations["active-andx"] = func(b []byte) { b[37] = 0x75 }
				}
				if response {
					mutations["error-status"] = func(b []byte) { b[9] = 1 }
				}
				if !tree && !response {
					mutations["buffer-format"] = func(b []byte) { b[41] = 3 }
				}
				if tree && !response {
					mutations["password-overrun"] = func(b []byte) { binary.LittleEndian.PutUint16(b[43:45], 65535) }
				}
				for name, mutate := range mutations {
					bad := bytes.Clone(w)
					mutate(bad)
					require.Error(t, cifsFileCommandValidate(tree, bad, response), name)
					_, err := ParseStructured(bad, "application-layer.cifs", entry)
					require.Error(t, err, name)
				}
				// Counted unknown tails remain unsupported, not silently consumed.
				bad := append(bytes.Clone(w), 0x91)
				cifsFileCommandFixLengths(bad, fixed)
				require.Error(t, cifsFileCommandValidate(tree, bad, response))
			})
		}
	}
}

func TestCIFSTreeDeleteStringFailuresAreSourceBound(t *testing.T) {
	for _, tc := range []struct {
		tree, response bool
		textAt         int
	}{{true, false, 48}, {true, true, 48}, {false, false, 42}} {
		w := cifsFileCommandFixture(tc.tree, tc.response, nil, "A", "A:")
		for _, u := range []uint16{0xd800, 0xdc00} {
			bad := bytes.Clone(w)
			binary.LittleEndian.PutUint16(bad[tc.textAt:tc.textAt+2], u)
			require.Error(t, cifsFileCommandValidate(tc.tree, bad, tc.response), "unpaired surrogate %x", u)
		}
		fixed := 41
		if tc.tree {
			fixed = 47
			if tc.response {
				fixed = 45
			}
		}
		// Drop the actual Unicode NUL code unit, retaining valid framing/BC.
		end := tc.textAt + 4
		bad := append(bytes.Clone(w[:end-2]), w[end:]...)
		cifsFileCommandFixLengths(bad, fixed)
		require.Error(t, cifsFileCommandValidate(tc.tree, bad, tc.response), fmt.Sprint(tc))
	}
	for _, response := range []bool{false, true} {
		w := cifsFileCommandFixture(true, response, nil, "A", "A:")
		at := len(w) - 3
		if response {
			at = 45
		}
		w[at] = 0xe9
		require.ErrorIs(t, cifsFileCommandValidate(true, w, response), stream_parser.ErrCIFSFileCommandUnsupported)
		w = cifsFileCommandFixture(true, response, nil, "A", "A:")
		w[37] = 0x06
		require.ErrorIs(t, cifsFileCommandValidate(true, w, response), stream_parser.ErrCIFSFileCommandUnsupported)
	}
}
