package bin_parser

import (
	"bytes"
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/binx"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"gopkg.in/yaml.v2"
	"testing"
	"time"
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
	_ = res1
	//assert.Equal(t, data, codec.EncodeToHex(res1.Bytes()))
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
	_ = res
	//spew.Dump(codec.EncodeToHex(res.Bytes()))
	//assert.Equal(t, codec.EncodeToHex(ethernetData), codec.EncodeToHex(res.Bytes()))
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
	_ = res
	//spew.Dump(codec.EncodeToHex(res.Bytes))
	//assert.Equal(t, codec.EncodeToHex(data), codec.EncodeToHex(res.Bytes))
}
func TestParseInternetProtocol(t *testing.T) {
	//log.SetLevel(log.DebugLevel)
	data := `f84d8991af523066d026811b080045a0020157be400031067af13b6e757bc0a8031601bbc3fdd3ff495aa37ef941801800546e5500000101080a82afe3966446750475ce0aac1dd76b44acfe1bd4a69821093ea24b33baba4b12a86b57279dfa9480b4684c7760ffd7295a383dce2d4b08569f69cb7bd8e236f93769c5ce36971cba0d3f15b365a0ec7412bdb3ade8de9ea1ecd3bfa9e0a5916d835912562f13a67e7973a1a389d5e1a58cce2dac8acf621665cdd9eea8b64008b57c50f937827aa40b3466ece997571f8a673e81bc3b35d32a480c160303012c0c00012803001d2061031d69d77218abde65c791d03963d124ba399f72b6c6bf2168b3becde1326108040100976db7b321691aafe470d0dec2b20bc222e19507d96e58d52b8e757a0d35d0efdb25bc30207476f0d7600a55f046e6249a575cf20e4f924cbd876470dd54c8d241f2b4565228c3ec75e819c6a92744c5f6fd85a063f828f806f0dc001bb48250cff7a96a06a083ce1779d5dff9a3902a0979bbd175005046bd16e0d7fe014e562a0385c43dee8811ae3a3a381473beff51ce951215025a8c36a20463552887028cd94d6e6fdaaed02c3c44e05fe28e271e4045fcac82d8ef5c72f3283e778192421909a9a5c8e1e2345d4dc439837cafe18768a78846cdcbe5f09c1bd01c1cc67b9a88a195d3ab7991c3b96df45bf73cadcc32522207053b99f43d2ab845b4da16030300040e000000`
	payload, err := codec.DecodeHex(data)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(payload)
	res, err := ParseBinary(reader, "application-layer.tls")
	if err != nil {
		t.Fatal(err)
	}
	DumpNode(res)
}
func TestTLSSession(t *testing.T) {
	data := `16030100fd010000f9030379fd1d601a4710f618dd7354ff98d638017afcec683102320d34abaa54359380204fdec12241340d59cd98b4e7dfc2c6f6d594180e1fb95c6c1e2307b1da00abbf0026c02bc02fc02cc030cca9cca8c009c013c00ac014009c009d002f0035c012000a1301130213030100008a000500050100000000000a000a0008001d001700180019000b00020100000d001a0018080404030807080508060401050106010503060302010203ff010001000010000b000908687474702f312e3100120000002b0009080304030303020301003300260024001d00203e8c72a8d10a674c47bccb04b043cdce74e9fe3b44de3025a44fa2f093618862160303007a020000760303e2cad785acd6b2fb94f3b6b9ef1673b76db389fadd23e6edfd5354048c9ef3dc204fdec12241340d59cd98b4e7dfc2c6f6d594180e1fb95c6c1e2307b1da00abbf130100002e002b0002030400330024001d002015d77e937e5f1376b7c3e48d6d45443a227d439706bb5c3a3da412c39e17917914030300010117030300171fff0e6cda94f9c35ddb9d35767a4a11e0f11a5ac6af591703030370096010dc5b172904f9a02f60aea1d6f6f4fd0e0ae974414b04169ab9693e3cdee925252678b945c690cd2444b40d9b84a9bb3aec06c659e4f54991bf6df561360412aca03568fa1f8ca32b62652cf7e4e21c42572893e83574fc5906249b10d53ce5beb99087c9e3462265eef5055eafbf1372885069eea9be28c7d7e23c935bbd810af3de0610199c8af38853b2c94720ef1f1cd89b5e31860058f661605d3318a6238a027027ece4cc88f4fff0d096fb0a1d9804edb217a40156ec346417ceb56e5ee36352e2a98e0f522d6eebfd3d2cf1fc9300a25460e6691680167240878b4d265c6672bbc92023cedd361a875660f16cc06389bea2e1183a40c8912d5a37d22eff407e6b28751953f29956808f378e88f0c9c9b2fcff7c0fa54cb655586ccfb197fcae7099981a8d8f5aef90815b58f2626c27a416e1ee627453a983039ba37003aa45f22b94873885c1f7bb5bb16b043214cd752338b42daa62a646098b9ad0670021c868bdb7e6124f581c1c611f9e9cd898a04a8a8964ca1fb9f472315e91899d56a00e5ef968f52ec53f710e18717f6a44bf6b5aec91761c3037e50f7aa2e5594d03030e2660446ab9ee7e353775e233ba68183abd5dd426134c7664a5e6727107333d456cc4949ec4163e9329a066996d0ea151d2b87b40db8dd843fe2f157897a4f6a53c7141a5716a8e9bd89a88dbb5e93893c423ea6f38681fd6e0725634c24e7ea8264b488b209cb68a1be7c41345c12ca98d36dec0614312802ac93303e73ebd8df2fadd8c0766a8ca537881abc0bd3a58f84a5a6ef9b217ec920af5a28dbd7e73b95fd41242a648e74df44612f1b6dd48676c355c338f1bfccb4ba98cd003d22098599c9847a0cf0e941f96d9f54ff7f6082016d181ac47566eda7645eae264d1e4736315a05d5f319d143393423de7fa4e7ae0afda80c62c96fc3373c5ace00cae3f059697a1c8273ab97c1aa8b37946326217db884e32396b84ecb8c0bcbe8426c1b7226562cbf8d3f07ab6f22d4232c118e23c2f8896fdcff285ddd9dd7ca8a88073f365945177f919a14dd57a83789b01641a02d29bc8f572ca23b45b2eb75c2d8ce198ca6c1350516021ea9d45194c6e6137cb526cc666b4f5605a6894cec7048945184df2f611c1a80b0009e93868b3ca0ed55d5d57e8ca3a495d23e2bff84eb71122fe08eebd9737814c393deea364624be45940bb6e13be7c4f11d94ceca77ca9096dad17030301198e95550e802f52761754acfa6668caee51a796d97dc03f968efeaaaa906a2cf853e14bc677c940dc6ecf49d7d8fe55f1562bfd46ad5bdc5f56086152fd7c8ed77694505b8959041a1daaed1867e14e74a7d55965cd300e8d3295ac5b0a1370e267252cb0c42894d54944592aaf02744f5449934fa46cf7b19353d1af2a904937b618da67286b8e8bc9d3b0480e933447f9cee4014528ef34ecf9282dea9014e7d1a58ccc28e02e17549fb7ce3e047b56969dc95ac1cd9e8d74ff707743d23e3eb843521cd27cdc7ae2bba84b43cb821ced27b6c1dac5ca9117ce427ffb905e84a6bd1220d1db223a905913f0179b161be3e881011a590090e1911c75d8fb59a2b2f8113f3b1bc4daffa83c703415eebc8200210037a272bb5a17030300355e19196552b8615f986c8ea69efda113b08c20a69289bf3b218816d7c709640f0429ac28999c499639f22474a63310760bac666239140303000101170303003553a7ea2b544498dc2c02be3a77839645513ee9f5e6f2eba783e80f1d64c4e40c59c4d931caa7aa5b0b7f624c91faa4104e7356035b1703030047d03f2b0417080e282366abca13dc861c26d12880c483874129700b6dddf73f0c106413572bbbe877a56035615b5d8bf6ace73a96aac4638abcaf8f1e9018510faaf5b641880e6617030300315380b458d112f9d178f44e268377719f0addfdd29461562313e0b39d9c720c682b5ed16ca4306c26433e0f06493286012f1703030013338063e857c0efa52bc1b6573a682732dbcd3a`
	payload, err := codec.DecodeHex(data)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(payload)
	res, err := ParseBinary(reader, "application-layer.tls")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Children) != 12 {
		t.Fatal("tls session parse failed")
	}
	DumpNode(res)
	dict1 := NodeToMap(res)
	res, err = GenerateBinary(dict1, "application-layer.tls")
	if err != nil {
		t.Fatal(err)
	}
	dict2 := NodeToMap(res)
	dict1Bytes, err := utils.MarshalIdempotent(dict1)
	if err != nil {
		t.Fatal(err)
	}
	dict2Bytes, err := utils.MarshalIdempotent(dict2)
	if err != nil {
		t.Fatal(err)
	}
	println(string(dict1Bytes))
	println(string(dict2Bytes))
	assert.Equal(t, string(dict1Bytes), string(dict2Bytes))
}
func TestReassembled(t *testing.T) {
	host, port := utils.DebugMockHTTPServerWithContextWithAddress(context.Background(), "127.0.0.1:9099", true, false, false, false, func(i []byte) []byte {
		return []byte("HTTP/1.1 200 OK\r\n\r\nHello, world!")
	})
	payload := []byte{}
	go func() {
		err := pcaputil.Start(
			pcaputil.WithDebug(false),
			pcaputil.WithDevice("lo0"),
			pcaputil.WithBPFFilter("host "+host+" and port "+fmt.Sprintf("%v", port)),
			pcaputil.WithOutput("/tmp/output.pcap"),
			pcaputil.WithOnTrafficFlowOnDataFrameReassembled(func(flow *pcaputil.TrafficFlow, conn *pcaputil.TrafficConnection, frame *pcaputil.TrafficFrame) {
				payload = append(payload, frame.Payload...)
			}),
		)
		if err != nil {
			t.Error(err)
		}
	}()
	time.Sleep(1 * time.Second)
	rsp, err := lowhttp.HTTP(lowhttp.WithRequest(`GET / HTTP/1.1
Host: www.example.com
Accept: */*
`), lowhttp.WithHttps(true), lowhttp.WithHost(host), lowhttp.WithPort(port))
	if err != nil {
		t.Error(err)
	}
	spew.Dump(string(rsp.RawPacket))
	time.Sleep(time.Second * 1)
	spew.Dump(codec.EncodeToHex(payload))
	reader := bytes.NewReader(payload)
	res, err := ParseBinary(reader, "application-layer.tls")
	if err != nil {
		t.Fatal(err)
	}
	DumpNode(res)
}
func TestNegotiateMessage(t *testing.T) {
	data := `4e544c4d535350000100000035820860000000000000000000000000000000000000000000000000`
	payload, err := codec.DecodeHex(data)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(payload)
	res, err := ParseBinary(reader, "application-layer.ntlm", "NegotiateMessage")
	if err != nil {
		t.Fatal(err)
	}
	DumpNode(res)
	mapData := map[string]any{
		"Signature":         "NTLMSSP\x00",
		"MessageType":       16777216,
		"NegotiateFlags":    897714272,
		"DomainNameFields":  "\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0000",
		"WorkstationFields": "\u0000\u0000\u0000\u0000\u0000\u0000\u0000\u0000",
	}
	res, err = GenerateBinary(mapData, "application-layer.ntlm", "NegotiateMessage")
	if err != nil {
		t.Fatal(err)
	}
	DumpNode(res)
	assert.Equal(t, "4e544c4d53535000010000003582086000000000000000000000000000000000", codec.EncodeToHex(NodeToBytes(res)))
}

func TestNTLM(t *testing.T) {
	//data := `TlRMTVNTUAABAAAAB4IIAAAAAAAgAAAAAAAAACAAAAA=`
	//payload, err := codec.DecodeBase64(data)
	//if err != nil {
	//	t.Fatal(err)
	//}
	//reader := bytes.NewReader(payload)
	//res, err := ParseBinary(reader, "application-layer.ntlm", "NegotiateMessage")
	//if err != nil {
	//	t.Fatal(err)
	//}
	//DumpNode(res)
	//data := `TlRMTVNTUAACAAAAHgAeADgAAAAFgooC7IuMmq8CVtsAAAAAAAAAAJgAmABWAAAACgA5OAAAAA9pAFoANwB3ADQAbgAxAGkAbwB1AG0ANgA0ADUAWgACAB4AaQBaADcAdwA0AG4AMQBpAG8AdQBtADYANAA1AFoAAQAeAGkAWgA3AHcANABuADEAaQBvAHUAbQA2ADQANQBaAAQAHgBpAFoANwB3ADQAbgAxAGkAbwB1AG0ANgA0ADUAWgADAB4AaQBaADcAdwA0AG4AMQBpAG8AdQBtADYANAA1AFoABwAIAMVIWn0iJNoBAAAAAA==`
	//payload, err := codec.DecodeBase64(data)
	//if err != nil {
	//	t.Fatal(err)
	//}
	//reader := bytes.NewReader(payload)
	//res, err := ParseBinary(reader, "application-layer.ntlm", "ChallengeMessage")
	//if err != nil {
	//	t.Fatal(err)
	//}
	//DumpNode(res)
	//byts := NodeToBytes(res)
	//assert.Equal(t, "TlRMTVNTUAACAAAAHgAeADgAAAAFgooC7IuMmq8CVtsAAAAAAAAAAJgAmABWAAAACgA5OAAAAA8=", codec.EncodeBase64(byts))
	data := `TlRMTVNTUAADAAAAGAAYAFgAAAAWARYBcAAAAAAAAACGAQAACAAIAIYBAAAyADIAjgEAAAAAAABYAAAABYIIAAAAAAAAAAAAqZz37sqK4x+vfueoTCWV7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAPECqzZKz5av1OJCyan6JsABAQAAAAAAAMVIWn0iJNoBNSvbAr/4/dAAAAAAAgAeAGkAWgA3AHcANABuADEAaQBvAHUAbQA2ADQANQBaAAEAHgBpAFoANwB3ADQAbgAxAGkAbwB1AG0ANgA0ADUAWgAEAB4AaQBaADcAdwA0AG4AMQBpAG8AdQBtADYANAA1AFoAAwAeAGkAWgA3AHcANABuADEAaQBvAHUAbQA2ADQANQBaAAcACADFSFp9IiTaAQYABAACAAAACgAQAAAAAAAAAAAAAAAAAAAAAAAJAC4ASABUAFQAUAAvADQANwAuADEAMgAwAC4ANAA0AC4AMgAxADkAOgA4ADAAOAA3AAAAAAAAAAAAdABlAHMAdAB6ADMAcgAwAG4AZQBkAGUATQBhAGMAQgBvAG8AawAtAFAAcgBvAC4AbABvAGMAYQBsAA==`
	payload, err := codec.DecodeBase64(data)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(payload)
	res, err := ParseBinary(reader, "application-layer.ntlm", "AuthenticationMessage")
	if err != nil {
		t.Fatal(err)
	}
	DumpNode(res)
	byts := NodeToBytes(res)
	assert.Equal(t, "TlRMTVNTUAADAAAAGAAYAFgAAAAWARYBcAAAAAAAAACGAQAACAAIAIYBAAAyADIAjgEAAAAAAABYAAAA", codec.EncodeBase64(byts))
}
