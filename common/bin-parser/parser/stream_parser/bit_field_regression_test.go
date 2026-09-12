package stream_parser_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// Vary both source alignment and numeric width. Zero-only header fixtures
// cannot detect an extra shift at the final partial octet.
func TestNumericBitFieldsAtEveryWidthAndAlignment(t *testing.T) {
	data := []byte{0xad, 0x73, 0xe1, 0x59, 0xc6, 0x82, 0x3b, 0xf5, 0x96}
	for _, endian := range []string{"big", "little"} {
		for prefix := 0; prefix < 8; prefix++ {
			for width := 1; width <= 64; width++ {
				t.Run(fmt.Sprintf("%s/offset-%d/width-%d", endian, prefix, width), func(t *testing.T) {
					rule := "endian: " + endian + "\nPackage:\n  Message:\n"
					if prefix > 0 {
						rule += fmt.Sprintf("    Prefix: uint8,%dbit\n", prefix)
					}
					rule += fmt.Sprintf("    Value: uint64,%dbit\n", width)
					padding := (8 - (prefix+width)%8) % 8
					if padding > 0 {
						rule += fmt.Sprintf("    Padding: uint8,%dbit\n", padding)
					}
					root, _, reader := parseInlineRule(t, rule, data[:(prefix+width+7)/8])
					require.Zero(t, reader.Len())
					node := base.GetNodeByPath(root, "@Message.Value")
					require.NotNil(t, node)
					result, err := node.Result()
					require.NoError(t, err)
					var expected uint64
					if endian == "big" {
						for i := 0; i < width; i++ {
							bit := prefix + i
							expected = expected<<1 | uint64(data[bit/8]>>(7-bit%8)&1)
						}
					} else {
						for offset := 0; offset < width; offset += 8 {
							count := 8
							if width-offset < count {
								count = width - offset
							}
							var octet uint64
							for i := 0; i < count; i++ {
								bit := prefix + offset + i
								octet = octet<<1 | uint64(data[bit/8]>>(7-bit%8)&1)
							}
							expected |= octet << offset
						}
					}
					require.Equal(t, expected, result.Value)
				})
			}
		}
	}
}

func TestRawPartialOctetRepresentationIsPreserved(t *testing.T) {
	root, _, _ := parseInlineRule(t, "Package:\n  Message:\n    Prefix: uint8,2bit\n    Value: raw,14bit\n", []byte{0x81, 0x23})
	node := base.GetNodeByPath(root, "@Message.Value")
	require.NotNil(t, node)
	result, err := node.Result()
	require.NoError(t, err)
	require.Equal(t, []byte{0x04, 0x23}, result.Value)
}
