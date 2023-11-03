package bin_parser

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/binx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"gopkg.in/yaml.v2"
	"testing"
)

// TestParserAndGenerate 基础的解析与生成
func TestParserAndGenerate(t *testing.T) {
	result := `package:
  Ethernet:
    Destination: "3066d026811b"
    Source: "f84d8991af52"
    Type: "0800"
  Internet:
    Version And Header Length: "45"
    Type of Service: "00"
    Total Length: 64
    Identification: "0000"
    Flags And Fragment Offset: "4000"
    Time to Live: "40"
    Protocol: "06"
    Header Checksum: "411f"
    Source: "c0a80316"
    Destination: "5db8d822"
  TCP:
    Source Port: 57406
    Destination Port: 80
    Sequence Number: "6092a878"
    Acknowledgement Number: "00000000"
    Data Offset And Reserved: "b0"
    Flags: "02"
    Window: "ffff"
    Checksum: "230f"
    Urgent Pointer: "0000"
    Options: "020405b4010303060101080aae1982a00000000004020000"
`
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
	assert.Equal(t, result, SdumpBinResult(res))
	dataMap := make(map[string]any)
	err = yaml.Unmarshal([]byte(result), &dataMap)
	if err != nil {
		t.Fatal(err)
	}
	var decodeHex func(d any) (any, error)
	decodeHex = func(d any) (any, error) {
		switch ret := d.(type) {
		case map[any]any:
			return decodeHex(utils.InterfaceToMapInterface(ret))
		case map[string]any:
			for k, v := range ret {
				v1, err := decodeHex(v)
				if err != nil {
					return nil, err
				}
				ret[k] = v1
			}
			return ret, nil
		case string:
			res, err := codec.DecodeHex(ret)
			if err != nil {
				return nil, err
			}
			return res, nil
		default:
			return ret, nil
		}
	}
	dataMap1, err := decodeHex(dataMap)
	if err != nil {
		t.Fatal(err)
	}
	dataMap2 := dataMap1.(map[string]any)
	res1, err := Generate(utils.InterfaceToMapInterface(dataMap2["package"]), "tcp")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, data, codec.EncodeToHex(res1))
}

// TestEndianness test generate endianness
func TestEndianness(t *testing.T) {
	res, err := generate(map[string]any{
		"a": 2,
	}, yaml.MapSlice{
		yaml.MapItem{
			Key:   "a",
			Value: "int,10",
		},
	}, []ConfigFunc{
		WithEndian(binx.LittleEndianByteOrder),
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "02000000000000000000", codec.EncodeToHex(res.GetBytes()))

	res, err = generate(map[string]any{
		"a": 2,
	}, yaml.MapSlice{
		yaml.MapItem{
			Key:   "a",
			Value: "int,10",
		},
	}, []ConfigFunc{
		WithEndian(binx.BigEndianByteOrder),
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "00000000000000000002", codec.EncodeToHex(res.GetBytes()))
}
