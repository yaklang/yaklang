package bin_parser

import (
	"bytes"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/binx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"gopkg.in/yaml.v2"
	"testing"
)

// TestParserAndGenerate 基础的解析与生成
func TestParserAndGenerate(t *testing.T) {
	result := `Ethernet:
  Destination: 3066d026811b
  Source: f84d8991af52
  Type: "0800"
Internet:
  Version: 4
  Header Length: 5
  Type of Service: "00"
  Total Length: 64
  Identification: "0000"
  Flags And Fragment Offset: "4000"
  Time to Live: "40"
  Protocol: "06"
  Header Checksum: 411f
  Source: c0a80316
  Destination: 5db8d822
TCP:
  Source Port: 57406
  Destination Port: 80
  Sequence Number: 6092a878
  Acknowledgement Number: "00000000"
  Header Length: 11
  Flags: "0002"
  Window: ffff
  Checksum: 230f
  Urgent Pointer: "0000"
  Options:
  - Kind: 2
    Length: 4
    Data: 05b4
  - Kind: 1
  - Kind: 3
    Length: 3
    Data: "06"
  - Kind: 1
  - Kind: 1
  - Kind: 8
    Length: 10
    Data: ae1982a000000000
  - Kind: 4
    Length: 2
  - Kind: 0
`
	data := `3066d026811bf84d8991af52080045000040000040004006411fc0a803165db8d822e03e00506092a87800000000b002ffff230f0000020405b4010303060101080aae1982a00000000004020000`
	ethernetData, err := codec.DecodeHex(data)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(ethernetData)
	res, err := ParseBinary(reader, "tcp")
	if err != nil {
		t.Fatal(err)
	}
	_ = res
	DumpNode(res)
	assert.Equal(t, result, SdumpNode(res))
	dataMap := make(map[string]any)
	err = yaml.Unmarshal([]byte(SdumpNode(res)), &dataMap)
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
		case []any:
			for i, v := range ret {
				v1, err := decodeHex(v)
				if err != nil {
					return nil, err
				}
				ret[i] = v1
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
	res1, err := GenerateBinary(dataMap1, "tcp")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, data, codec.EncodeToHex(res1.Bytes()))
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
func TestParseTCP(t *testing.T) {
	data := `3066d026811bf84d8991af52080045000040000040004006411fc0a803165db8d822e03e00506092a87800000000b002ffff230f0000020405b4010303060101080aae1982a00000000004020000`
	ethernetData, err := codec.DecodeHex(data)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(ethernetData)
	res, err := ParseBinary(reader, "tcp")
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(codec.EncodeToHex(res.Bytes()))
	assert.Equal(t, codec.EncodeToHex(ethernetData), codec.EncodeToHex(res.Bytes()))
}
func TestParseHTTP(t *testing.T) {
	raw := `POST / HTTP/1.1
Content-Type: application/json
Host: www.example.com

{"key": "value"}`
	data := lowhttp.FixHTTPRequest([]byte(raw))
	reader := bytes.NewReader(data)
	res, err := ParseBinary(reader, "http_request")
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(codec.EncodeToHex(res.Bytes))
	assert.Equal(t, codec.EncodeToHex(data), codec.EncodeToHex(res.Bytes))
}
