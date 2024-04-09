package bin_parser

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"testing"
)

func TestCHAPMessage(t *testing.T) {
	data := `01030022105c36e2c2ee83c339e9799344e9ec85d348695065722e6174742e6e6574`
	payload, err := codec.DecodeHex(data)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(payload)
	res, err := parser.ParseBinary(reader, "challenge_handshake_authentication_protocol")
	if err != nil {
		t.Fatal(err)
	}
	DumpNode(res)
	mapData := map[string]any{
		"Code":       1,
		"Identifier": 3,
		"Length":     34,
		"Data": map[string]any{
			"Value Size": 16,
			"Value":      "\x5c\x36\xe2\xc2\xee\x83\xc3\x39\xe9\x79\x93\x44\xe9\xec\x85\xd3",
			"Name":       "\x48\x69\x50\x65\x72\x2e\x61\x74\x74\x2e\x6e\x65\x74",
		},
	}
	res, err = parser.GenerateBinary(mapData, "challenge_handshake_authentication_protocol", "CHAP")
	if err != nil {
		t.Fatal(err)
	}
	DumpNode(res)
	assert.Equal(t, "01030022105c36e2c2ee83c339e9799344e9ec85d348695065722e6174742e6e6574", codec.EncodeToHex(NodeToBytes(res)))
}

func TestPAPMessage(t *testing.T) {
	data := `0100000e04697869610469786961`
	payload, err := codec.DecodeHex(data)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(payload)
	res, err := parser.ParseBinary(reader, "password_authentication_protocol")
	if err != nil {
		t.Fatal(err)
	}
	DumpNode(res)
	mapData := map[string]any{
		"Code":       1,
		"Identifier": 0,
		"Length":     14,
		"Request": map[string]any{
			"Peer ID Length":  4,
			"Peer ID":         "ixia",
			"Password Length": 4,
			"Password":        "ixia",
		},
	}
	res, err = parser.GenerateBinary(mapData, "password_authentication_protocol", "PAP")
	if err != nil {
		t.Fatal(err)
	}
	DumpNode(res)
	assert.Equal(t, "0100000e04697869610469786961", codec.EncodeToHex(NodeToBytes(res)))
}

func TestPPPMessage(t *testing.T) {
	data := `ff03c0230100000e04697869610469786961`
	payload, err := codec.DecodeHex(data)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(payload)
	res, err := parser.ParseBinary(reader, "ppp")
	if err != nil {
		t.Fatal(err)
	}
	DumpNode(res)
	mapData := map[string]any{
		"Address":  0xff,
		"Control":  0x03,
		"Protocol": 0xc023,
		//"Information": map[string]any{
		"PAP": "\x01\x00\x00\x0e\x04\x69\x78\x69\x61\x04\x69\x78\x69\x61",
		//
		//},
	}
	res, err = parser.GenerateBinary(mapData, "ppp", "PPP")
	if err != nil {
		t.Fatal(err)
	}
	DumpNode(res)
	assert.Equal(t, "ff03c0230100000e04697869610469786961", codec.EncodeToHex(NodeToBytes(res)))
}

func TestGRE_PPP_Message(t *testing.T) {
	data := `3081880b0012000100000001ffffffffff03c0210101000e0304c02305060f3f117c`
	payload, err := codec.DecodeHex(data)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(payload)
	res, err := parser.ParseBinary(reader, "generic_routing_encapsulation")
	if err != nil {
		t.Fatal(err)
	}
	DumpNode(res)
	LCPDate, _ := codec.DecodeHex("0100000a05060a94c166")

	mapData := map[string]any{
		"Flags And Version": 0x3081,
		"Protocol Type":     0x880b,
		"Payload Length":    14,
		"Call ID":           24,
		"Number":            0,
		"Payload": map[string]any{
			"PPP": map[string]any{
				"Address":  0xff,
				"Control":  0x03,
				"Protocol": 0xc021,
				"LCP":      LCPDate,
			},
		},
	}
	res, err = parser.GenerateBinary(mapData, "generic_routing_encapsulation", "GRE")
	if err != nil {
		t.Fatal(err)
	}
	DumpNode(res)
	assert.Equal(t, "3081880b000e001800000000ff03c0210100000a05060a94c166", codec.EncodeToHex(NodeToBytes(res)))
}

func TestLCPMessage(t *testing.T) {
	data := `01010024010405ea0206000000000305c223050506dfc53f2f07020802110405ea130300`
	payload, err := codec.DecodeHex(data)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(payload)
	res, err := parser.ParseBinary(reader, "link_control_protocol")
	if err != nil {
		t.Fatal(err)
	}
	DumpNode(res)
	mapData := map[string]any{
		"Code":       1,
		"Identifier": 1,
		"Length":     36,
		"Options": []map[string]any{
			{
				"Type":   1,
				"Length": 4,
				"Data":   "\x05\xea",
			}, {
				"Type":   2,
				"Length": 6,
				"Data":   "\x00\x00\x00\x00",
			}, {
				"Type":   3,
				"Length": 5,
				"Data":   "\xc2\x23\x05",
			},
			{
				"Type":   5,
				"Length": 6,
				"Data":   "\xdf\xc5\x3f\x2f",
			},
			{
				"Type":   7,
				"Length": 2,
				"Data":   "",
			},
			{
				"Type":   8,
				"Length": 2,
				"Data":   "",
			}, {
				"Type":   17,
				"Length": 4,
				"Data":   "\x05\xea",
			}, {
				"Type":   19,
				"Length": 3,
				"Data":   "\x00",
			},
		},
	}
	res, err = parser.GenerateBinary(mapData, "link_control_protocol", "LCP")
	if err != nil {
		t.Fatal(err)
	}
	DumpNode(res)
	assert.Equal(t, "01010024010405ea0206000000000305c223050506dfc53f2f07020802110405ea130300", codec.EncodeToHex(NodeToBytes(res)))
}
