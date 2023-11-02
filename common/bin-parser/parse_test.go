package bin_parser

import (
	"bytes"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"testing"
)

func TestParser(t *testing.T) {
	data := `3066d026811bf84d8991af52080045000040000040004006411fc0a803165db8d822e03e00506092a87800000000b002ffff230f0000020405b4010303060101080aae1982a00000000004020000`
	ethernetData, err := codec.DecodeHex(data)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(ethernetData)
	res, err := Parse(reader, "tcp")
	if err != nil {
		t.Fatal(err)
	}
	DumpBinResult(res)
}
func TestGenerator(t *testing.T) {
	res, err := Generate(map[string]any{
		"Ethernet": map[string]any{
			"Destination": []byte{0x30, 0x66, 0xd0, 0x26, 0x81, 0x1b},
			"Source":      []byte{0xf8, 0x4d, 0x89, 0x91, 0xaf, 0x52},
			"Type":        []byte{0x08, 0x00},
		},
		"Internet": map[string]any{
			"Version And Header Length": []byte{0x45},
			"Type of Service":           []byte{0x00},
			"Total Length":              []byte{0x00, 0x40},
			"Identification":            []byte{0x00, 0x00},
			"Flags And Fragment Offset": []byte{0x40, 0x00},
			"Time to Live":              []byte{0x40},
			"Protocol":                  []byte{0x06},
			"Header Checksum":           []byte{0x11, 0xfc},
			"Source":                    []byte{0x0a, 0x80, 0x31, 0x65},
			"Destination":               []byte{0xdb, 0x8d, 0x82, 0x2e},
		},

		"TCP": map[string]any{
			"Source Port":              []byte{0x03, 0xe0},
			"Destination Port":         []byte{0x05, 0x06},
			"Sequence Number":          []byte{0x92, 0xa8, 0x78, 0x00},
			"Acknowledgement Number":   []byte{0x00, 0x00, 0x00, 0x00},
			"Data Offset And Reserved": []byte{0xb0},
			"Flags":                    []byte{0x02},
			"Window":                   []byte{0xff, 0xff},
			"Checksum":                 []byte{0x23, 0x0f},
			"Urgent Pointer":           []byte{0x00, 0x00},
			"Options":                  []byte{0x02, 0x04, 0x05, byte(0xb4), 0x01, 0x03, 0x03, 0x06, 0x01, 0x01, 0x08, 0x0a, 0xae, 0x19, 0x82, 0xa0, 0x00, 0x00, 0x00, 0x00, 0x40, 0x20, 0x00},
		},
	}, "tcp")
	if err != nil {
		t.Fatal(err)
	}
	hexRes := codec.EncodeToHex(res)
	println(hexRes)
}
