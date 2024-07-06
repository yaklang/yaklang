package bin_parser

import (
	"bytes"
	"errors"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	utils2 "github.com/yaklang/yaklang/common/bin-parser/utils"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"gopkg.in/yaml.v2"
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
	LCPDate, _ := codec.DecodeHex("0101000e0304c02305060f3f117c")

	mapData := map[string]any{
		"Flags And Version":     0x3081,
		"Protocol Type":         0x880b,
		"Payload Length":        18,
		"Call ID":               1,
		"Number":                0,
		"Sequence Number":       1,
		"Acknowledgment Number": 4294967295,
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
	assert.Equal(t, "3081880b0012000100000001ffffffffff03c0210101000e0304c02305060f3f117c", codec.EncodeToHex(NodeToBytes(res)))
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
		"Info": map[string]any{
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
		},
	}
	res, err = parser.GenerateBinary(mapData, "link_control_protocol", "LCP")
	if err != nil {
		t.Fatal(err)
	}
	DumpNode(res)
	assert.Equal(t, "01010024010405ea0206000000000305c223050506dfc53f2f07020802110405ea130300", codec.EncodeToHex(NodeToBytes(res)))
}

func TestBaseProtocol(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	type args struct {
		data   string
		rule   string
		expect string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "arp",
			args: args{
				data:   `ffffffffffff3066d0268abb080600010800060400013066d0268abbc0a80337000000000000c0a80363000000000000000000000000000000000000`,
				rule:   "ethernet",
				expect: arpExpect,
			},
		},
		{
			name: "icmp",
			args: args{
				data:   `3066d026811bf84d8991af520800450000546a110000400199a5c0a803166ef2444208009bfc47790000657fb59d00030e6708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f3031323334353637`,
				rule:   "ethernet",
				expect: icmpExpect,
			},
		},
		{
			name: "http request",
			args: args{
				data:   `3066d026811bf84d8991af5208004500010b00004000400662c6c0a8031677609c08cea0370db485ff921c6fdee680180808235200000101080a929a15075619e69f474554202f20485454502f312e310d0a486f73743a20373664643164383362372e69716979692e636f6d3a31343039330d0a557365722d4167656e743a206375726c2f372e34382e300d0a436f6e74656e742d4c656e6774683a2033320d0a436f6e6e656374696f6e3a20557067726164650d0a5365632d576562736f636b65742d4b65793a204f45692d724e56326c3443754264347a567a664c6a673d3d0d0a557067726164653a20776562736f636b65740d0a0d0a2acc9cc819e51ccf44bdee6f4e26f45f63038a6cfddf86a550a6ff9b5d1f875b`,
				rule:   "ethernet",
				expect: httpRequestExpect,
			},
		},
		{
			name: "tls",
			args: args{
				data:   `3066d026811bf84d8991af5208004500005c00004000400675ddc0a8031601000001c75f01bbc23157078c7aa68280180800ad6800000101080a858e40e3c784a921170303002339aa76173aee3468a1e8402150499a9585259f6f799c7895d7d40be6879f4b63cdec72`,
				rule:   "ethernet",
				expect: tlsExpect,
			},
		},
		{
			name: "dns request",
			args: args{
				data:   `3066d026811bf84d8991af52080045000055edc10000401134dec0a80316771d1d1dfa0d003500417b4514520100000100000000000011636f70696c6f742d74656c656d657472791167697468756275736572636f6e74656e7403636f6d0000010001`,
				rule:   "ethernet",
				expect: dnsExpect,
			},
		},
		{
			name: "dns response",
			args: args{
				data:   `f84d8991af523066d026811b080045000067b95000007511e46cdf050505c0a803160035d28b00530000bc35818000010002000000000b636c6f7564636f6e666967096a6574627261696e7303636f6d0000010001c00c000100010000001300043412ec15c00c00010001000000130004364dbb13`,
				rule:   "ethernet",
				expect: dnsResponseExpect,
			},
		},
		{
			name: "icmp v6",
			args: args{
				data:   `3333ffa35b78f84d8991af5286dd6000000000203afffe8000000000000000237c9bf9dd7b2dff0200000000000000000001ffa35b7887000c5200000000fe8000000000000014ae6f6a11a35b780101f84d8991af52`,
				rule:   "ethernet",
				expect: icmpV6Expect,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ethernetData, err := codec.DecodeHex(tt.args.data)
			if err != nil {
				t.Fatal(err)
			}
			ret, err := testParse(ethernetData, tt.args.rule)
			if err != nil {
				t.Fatal(err)
			}
			resMap, err := ret.Result()
			resYaml, err := DumpNodeValueYaml(resMap)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, tt.args.expect, string(resYaml))
		})
	}
}

func _TestT3(t *testing.T) {
	data := "000005be016501ffffffffffffffff000000690000ea60000000181abd8810f7e91853b7a14836659a8dd1c3c7b2441481ddb5027973720078720178720278700000000a000000030000000000000001007070707070700000000a000000030000000000000001007006fe010000aced00057372001d7765626c6f6769632e726a766d2e436c6173735461626c65456e7472792f52658157f4f9ed0c000078707200247765626c6f6769632e636f6d6d6f6e2e696e7465726e616c2e5061636b616765496e666fe6f723e7b8ae1ec90200084900056d616a6f724900056d696e6f7249000c726f6c6c696e67506174636849000b736572766963655061636b5a000e74656d706f7261727950617463684c0009696d706c5469746c657400124c6a6176612f6c616e672f537472696e673b4c000a696d706c56656e646f7271007e00034c000b696d706c56657273696f6e71007e000378707702000078fe010000aced00057372001d7765626c6f6769632e726a766d2e436c6173735461626c65456e7472792f52658157f4f9ed0c000078707200247765626c6f6769632e636f6d6d6f6e2e696e7465726e616c2e56657273696f6e496e666f972245516452463e0200035b00087061636b616765737400275b4c7765626c6f6769632f636f6d6d6f6e2f696e7465726e616c2f5061636b616765496e666f3b4c000e72656c6561736556657273696f6e7400124c6a6176612f6c616e672f537472696e673b5b001276657273696f6e496e666f417342797465737400025b42787200247765626c6f6769632e636f6d6d6f6e2e696e7465726e616c2e5061636b616765496e666fe6f723e7b8ae1ec90200084900056d616a6f724900056d696e6f7249000c726f6c6c696e67506174636849000b736572766963655061636b5a000e74656d706f7261727950617463684c0009696d706c5469746c6571007e00044c000a696d706c56656e646f7271007e00044c000b696d706c56657273696f6e71007e000478707702000078fe010000aced00057372001d7765626c6f6769632e726a766d2e436c6173735461626c65456e7472792f52658157f4f9ed0c000078707200217765626c6f6769632e636f6d6d6f6e2e696e7465726e616c2e50656572496e666f585474f39bc908f10200064900056d616a6f724900056d696e6f7249000c726f6c6c696e67506174636849000b736572766963655061636b5a000e74656d706f7261727950617463685b00087061636b616765737400275b4c7765626c6f6769632f636f6d6d6f6e2f696e7465726e616c2f5061636b616765496e666f3b787200247765626c6f6769632e636f6d6d6f6e2e696e7465726e616c2e56657273696f6e496e666f972245516452463e0200035b00087061636b6167657371007e00034c000e72656c6561736556657273696f6e7400124c6a6176612f6c616e672f537472696e673b5b001276657273696f6e496e666f417342797465737400025b42787200247765626c6f6769632e636f6d6d6f6e2e696e7465726e616c2e5061636b616765496e666fe6f723e7b8ae1ec90200084900056d616a6f724900056d696e6f7249000c726f6c6c696e67506174636849000b736572766963655061636b5a000e74656d706f7261727950617463684c0009696d706c5469746c6571007e00054c000a696d706c56656e646f7271007e00054c000b696d706c56657273696f6e71007e000578707702000078fe00fffe010000aced0005737200137765626c6f6769632e726a766d2e4a564d4944dc49c23ede121e2a0c00007870774d210000000000000000000e3137322e3234352e35372e313836000e3137322e3234352e35372e313836e76219fc0000000700001b59ffffffffffffffffffffffffffffffffffffffffffffffff78fe010000aced0005"
	pyaload, err := codec.DecodeHex(data)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(pyaload)
	res, err := parser.ParseBinary(reader, "application-layer.t3")
	if err != nil {
		t.Fatal(err)
	}
	r, err := res.Children[0].Result()
	if err != nil {
		t.Fatal(err)
	}
	//spew.Dump(r)
	res, err = parser.GenerateBinary(r, "application-layer.t3")
	resHex := codec.EncodeToHex(NodeToBytes(res))
	assert.Equal(t, data, resHex)
}
func _TestLdap(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	dataStr := "3084000005420201076084000005390201030400a3840000052e040a4753532d53504e45474f0482051e6082051a06062b0601050502a082050e3082050aa024302206092a864882f71201020206092a864886f712010202060a2b06010401823702020aa28204e0048204dc608204d806092a864886f71201020201006e8204c7308204c3a003020105a10302010ea20703050020000000a38203e0618203dc308203d8a003020105a1151b1357324b332e564d4e4554312e564d2e42415345a22f302da003020102a12630241b046c6461701b1c77326b332d3130312e77326b332e766d6e6574312e766d2e62617365a382038730820383a003020117a103020107a2820375048203716a61c886ba58d162113db4268f7743a17eb476183bc0c519addea76556a3701de34903e6bd3f3fdca0b01bbccb9a8693b23fa8d1985e14922e4ca19b05a90769845a5858515bba4af2d7e59bfa8634285a2e954fb518378b8d3f2744b9bbf8842b4807879ff28e55bfba4967e8c1d3b6c4e358a561c54abbc1cb7c97b6503fe59b7fee6423dffe66fe6dcb8af00e69c53d6b576f5506990438310fb7dd1468a32fd8e0deab40b15ecfd438568370140a1edafee701a4a4b4e7b3aaefdc4b1aff5868aefe5a36294d5dd687d5a6493143d3ade8031c98d28f6c7f3dcea41435132f675f26940d1f69e573e5ece6ed5a66111ff9f4b02a8ddd19086e5b9dc0adc86a0bc1230f1b715ffc4004dfc4a7d5f78a4dc31abf830ae6e3bfd21c87fa5196549e130f6a081bafcf4170ae201c78a3829a01dba578a2ef968f2ab6668d8114dfcc65d7038f5558be7cdd9246d52247915260a40e59c48b08a1ed61427fd303917c6b34b701a4ba9a3815d4828a228cd209da137626e2029aabf6c200bf7fd63cf6d43bb618b31ac48e09613589d74a69542e909ce0dc9c57c77f7d89b966de200053a58ea58f2374513961638a30ca49ef0eec679d927e385b5da7d4d3c1a59169b4630b874a1d969e45d1fe3782089f4385024955093b308e1964d307915271aa886c3d9b64d846c88ca1341fd2f72b76679d4f258f647bc04820e42776c9ec0d01464652763a49d822c9d25b603903ebd6338952259b83a740a420d69d23aebbdf06a92d88a46ffcd8d81a47b6ec99b6cea0489cc83ef15757c4053d538446f2e6b9eba12ce4969b8d6df9b3ef574b7d401341c2f555a00f029164e5d387282c0c8791ba8c69816248e2e544a9c12b7aeba629fdeea2e111655e44b9c215924c5455eaa4ab32aea1d9cef1d86e8acf6b0ff4dcabaf4f0e2d9ae65c8bb1065e0418ff12d4626930315938bfe00a8d03e8e70e9dea9dc9ff74854cbb4dbdf700a62e77b26e50b13e2d3960c913360c84c87e801ed3df3db0e27604508cb730c5a052c068abe5826b01be9f62e33b9af8edb6667c57cb1aa879743b77a7432f75fe3ae211f96af41adef1e1c507256fe5fa2bccabe52cf8216d3410e6378506d427343458332d153a77a162c4c5f18d9f31b0c142880cad2229981720615ab26b7c13442e43178aadee436510c91bc9d5d735eb9453cf39cef5120e28603775f0483f01c3c48b5b060ca7f3a54d7c7c99a481c93081c6a003020117a281be0481bb03ab656760a3512fecc7032da8b2014659f0fb34eb76b461e4044da24d16d458e3e1c58919c74c4c0720aafb87a948152372a2483a4d1ae9b95b858a52abaa94e7aa641a8b997d7e6c6e570b5908cc549155f5e6f110c98d648978727abae3921da52a4c1fd76beb121bf3396be8f98e4acf1ebfc3b6fb7a1354c121873e59185db90030084d97864798d79eb9df30756ca1faa7a80880f74f7d93642d9ceb5e0128ced6ab096a4f015e5a032b4270231e7ff1bcd087e8b527027d"
	ethernetData, err := codec.DecodeHex(dataStr)
	if err != nil {
		t.Fatal(err)
	}
	ret, err := testParse(ethernetData, "application-layer.ldap")
	if err != nil {
		t.Fatal(err)
	}
	resMap, err := ret.Result()
	if err != nil {
		t.Fatal(err)
	}
	resYaml, err := DumpNodeValueYaml(resMap)
	if err != nil {
		t.Fatal(err)
	}
	println(resYaml)
}
func _TestIIOP1(t *testing.T) {
	var data string
	var err error
	var pyaload []byte
	var reader *bytes.Reader
	var res *base.Node
	//// LocateRequest
	//data = "47494f50010200030000001700000002000000000000000b4e616d6553657276696365"
	//pyaload, err = codec.DecodeHex(data)
	//if err != nil {
	//	t.Fatal(err)
	//}
	//reader = bytes.NewReader(pyaload)
	//res, err = ParseBinary(reader, "application-layer.iiop")
	//if err != nil {
	//	t.Fatal(err)
	//}
	//r, err = res.Children[0].Result()
	//if err != nil {
	//	t.Fatal(err)
	//}
	//spew.Dump(r)
	//// LocateResponse
	//data = "47494f5001020004000003e000000002000000020000003349444c3a7765626c6f6769632f636f7262612f636f732f6e616d696e672f4e616d696e67436f6e74657874416e793a312e300000000000010000000000000394000102000000000b3137322e31392e302e3200001b5900000000007800424541080103000000000c41646d696e53657276657200000000000000003349444c3a7765626c6f6769632f636f7262612f636f732f6e616d696e672f4e616d696e67436f6e74657874416e793a312e3000000000000238000000000000014245412a0000001000000000000000009d3ee33d0cf1190700000005000000010000002c000000000001002000000003000100200001000105010001000101000000000300010100000101090501000100000019000000390000000000000031687474703a2f2f3137322e31392e302e323a373030312f6265615f776c735f696e7465726e616c2f636c61737365732f0000000000000020000000040000000100000021000000580001000000000001000000000000002200000000004000000000000806066781020101010000001f0401000806066781020101010000000f7765626c6f67696344454641554c540000000000000000000000000000000000424541030000021000000000000000107365727665722d616666696e69747900010000000000001f7765626c6f6769632e636f736e616d696e672e4e616d65536572766963650000000000010000003349444c3a7765626c6f6769632f636f7262612f636f732f6e616d696e672f4e616d696e67436f6e74657874416e793a312e30000000000001000000000000017c000102000000000b3137322e31392e302e3200001b5900000000007800424541080103000000000c41646d696e53657276657200000000000000003349444c3a7765626c6f6769632f636f7262612f636f732f6e616d696e672f4e616d696e67436f6e74657874416e793a312e3000000000000238000000000000014245412a0000001000000000000000009d3ee33d0cf1190700000004000000010000002c000000000001002000000003000100200001000105010001000101000000000300010100000101090501000100000019000000390000000000000031687474703a2f2f3137322e31392e302e323a373030312f6265615f776c735f696e7465726e616c2f636c61737365732f0000000000000020000000040000000100000021000000580001000000000001000000000000002200000000004000000000000806066781020101010000001f0401000806066781020101010000000f7765626c6f67696344454641554c54000000000000000000000000000000000000000000000000000cf11907"
	//pyaload, err = codec.DecodeHex(data)
	//if err != nil {
	//	t.Fatal(err)
	//}
	//reader = bytes.NewReader(pyaload)
	//res, err = ParseBinary(reader, "application-layer.iiop")
	//if err != nil {
	//	t.Fatal(err)
	//}
	//r, err := res.Children[0].Result()
	//if err != nil {
	//	t.Fatal(err)
	//}
	//spew.Dump(r)
	//// Request
	//data = "47494f5001020000000005b00000000203000000000000000000007800424541080103000000000c41646d696e53657276657200000000000000003349444c3a7765626c6f6769632f636f7262612f636f732f6e616d696e672f4e616d696e67436f6e74657874416e793a312e3000000000000238000000000000014245412a0000001000000000000000009d3ee33d0cf119070000000b726562696e645f616e79000000000006000000050000001800000000000000010000000a3132372e302e302e3100cbfa000000010000000c00000000000100200501000100000006000000f0000000000000002849444c3a6f6d672e6f72672f53656e64696e67436f6e746578742f436f6465426173653a312e30000000000100000000000000b4000102000000000a3132372e302e302e3100cbfa0000006400424541080103000000000100000000000000000000002849444c3a6f6d672e6f72672f53656e64696e67436f6e746578742f436f6465426173653a312e30000000000331320000000000014245412a000000100000000000000000f690d67f42445b4800000001000000010000002c00000000000100200000000300010020000100010501000100010100000000030001010000010109050100010000000f00000020000000000000000000000000000000010000000000000000010000000000000042454103000000140000000000000000000000000cf11907000000004245410000000004000a030600000000000000010000000868656c616161616100000001000000000000001d0000001c000000000000000100000000000000010000000000000000000000007fffff020000003e524d493a7765626c6f6769632e69696f702e50726f7879446573633a373343443941343543424135323933383a373432363138303142393331454630300000007fffff0200000059524d493a73756e2e7265666c6563742e616e6e6f746174696f6e2e416e6e6f746174696f6e496e766f636174696f6e48616e646c65723a433030334245443736453333333842423a35354341463530463135434237454135000000007fffff0a00000038524d493a6a6176612e7574696c2e486173684d61703a383635373335363841323131433031313a303530374441433143333136363044310000000015010100003f4000000000000c0000001000000001000000007fffff0a0000002349444c3a6f6d672e6f72672f434f5242412f57537472696e6756616c75653a312e300000000000190000001570776e656434313734303231393134393937373038000000fffffffe00000001000000007fffff0a00000074524d493a636f6d2e6265612e636f72652e72657061636b616765642e737072696e676672616d65776f726b2e7472616e73616374696f6e2e6a74612e4a74615472616e73616374696f6e4d616e616765723a304433303438453037423144334237423a34454633454346424236323839383246000000001cffffffff0001010000000000000001010100000000000000000000007fffff0affffffffffffff0800000027000000236c6461703a2f2f3137322e3234352e35372e3138363a383038352f6e734c7a6d6f464700ffffffff7fffff0200000040524d493a6a617661782e726d692e434f5242412e436c617373446573633a324241424441303435383741444343433a43464246303243463532393431373642007fffff02fffffffffffffe84000000007fffff02fffffffffffffe7400000027524d493a6a6176612e6c616e672e4f766572726964653a30303030303030303030303030303030007fffff0200000039524d493a5b4c6a6176612e6c616e672e436c6173733b3a303731444138424537463937313132383a3243374535353033443942463935353300000000000000017fffff02ffffffffffffff24ffffffffffffff607fffff02fffffffffffffde000000024524d493a6a6176612e726d692e52656d6f74653a30303030303030303030303030303030"
	//pyaload, err = codec.DecodeHex(data)
	//if err != nil {
	//	t.Fatal(err)
	//}
	//reader = bytes.NewReader(pyaload)
	//res, err = ParseBinary(reader, "application-layer.iiop")
	//if err != nil {
	//	t.Fatal(err)
	//}
	//r, err := res.Children[0].Result()
	//if err != nil {
	//	t.Fatal(err)
	//}
	//spew.Dump(r)
	// Reply
	data = "47494f5001020001000000580000000200000002000000010000000f000000180000000100000000000000000000000101000000000000000000001e49444c3a6f6d672e6f72672f434f5242412f4d41525348414c3a312e300000000000000000000001"
	pyaload, err = codec.DecodeHex(data)
	if err != nil {
		t.Fatal(err)
	}
	reader = bytes.NewReader(pyaload)
	res, err = parser.ParseBinary(reader, "application-layer.iiop")
	if err != nil {
		t.Fatal(err)
	}
	r, err := res.Children[0].Result()
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(r)
}

var icmpV6Expect = `Ethernet:
  Destination: 3333ffa35b78
  Source: f84d8991af52
  Type: 34525
  IPv6:
    Version: 6
    Traffic Class: 0
    Flow Label: 0
    Payload Length: 32
    Next Header: 58
    Hop Limit: 255
    Source: fe8000000000000000237c9bf9dd7b2d
    Destination: ff0200000000000000000001ffa35b78
    ICMPv6:
      Type: 135
      Code: 0
      Checksum: 3154
      Payload: 00000000fe8000000000000014ae6f6a11a35b780101f84d8991af52
`
var dnsResponseExpect = `Ethernet:
  Destination: f84d8991af52
  Source: 3066d026811b
  Type: 2048
  IP:
    Version: 4
    Header Length: 5
    Type of Service: "00"
    Total Length: 103
    Identification: b950
    Flags And Fragment Offset: "0000"
    Time to Live: "75"
    Protocol: 17
    Header Checksum: e46c
    Source: df050505
    Destination: c0a80316
    UDP:
      Source Port: 53
      Destination Port: 53899
      Length: 83
      Checksum: 0
      DNS:
        Header:
          ID: 48181
          Flags: 33152
          Questions: 1
          Answer RRs: 2
          Authority RRs: 0
          Additional RRs: 0
        Questions:
          Question:
            Name:
              Label:
                Count: 11
                Data: cloudconfig
              Label:
                Count: 9
                Data: jetbrains
              Label:
                Count: 3
                Data: com
              Label:
                Count: 0
            Type: 1
            Class: 1
        Answers:
          Answer:
            Name:
              Pointer: 12
              PointerFlag: 3
            Type: 1
            Class: 1
            TTL: 19
            RDLength: 4
            RData: 3412ec15
          Answer:
            Name:
              Pointer: 12
              PointerFlag: 3
            Type: 1
            Class: 1
            TTL: 19
            RDLength: 4
            RData: 364dbb13
`
var dnsExpect = `Ethernet:
  Destination: 3066d026811b
  Source: f84d8991af52
  Type: 2048
  IP:
    Version: 4
    Header Length: 5
    Type of Service: "00"
    Total Length: 85
    Identification: edc1
    Flags And Fragment Offset: "0000"
    Time to Live: "40"
    Protocol: 17
    Header Checksum: 34de
    Source: c0a80316
    Destination: 771d1d1d
    UDP:
      Source Port: 64013
      Destination Port: 53
      Length: 65
      Checksum: 31557
      DNS:
        Header:
          ID: 5202
          Flags: 256
          Questions: 1
          Answer RRs: 0
          Authority RRs: 0
          Additional RRs: 0
        Questions:
          Question:
            Name:
              Label:
                Count: 17
                Data: copilot-telemetry
              Label:
                Count: 17
                Data: githubusercontent
              Label:
                Count: 3
                Data: com
              Label:
                Count: 0
            Type: 1
            Class: 1
`
var tlsExpect = `Ethernet:
  Destination: 3066d026811b
  Source: f84d8991af52
  Type: 2048
  IP:
    Version: 4
    Header Length: 5
    Type of Service: "00"
    Total Length: 92
    Identification: "0000"
    Flags And Fragment Offset: "4000"
    Time to Live: "40"
    Protocol: 6
    Header Checksum: 75dd
    Source: c0a80316
    Destination: "01000001"
    TCP:
      Source Port: 51039
      Destination Port: 443
      Sequence Number: c2315707
      Acknowledgement Number: 8c7aa682
      Header Length: 8
      Flags: "0108"
      Window: "0800"
      Checksum: ad68
      Urgent Pointer: "0000"
      Options:
        Option:
          Kind: 1
        Option:
          Kind: 1
        Option:
          Kind: 8
          Length: 10
          Data: 858e40e3c784a921
      TLS:
        Record Layer:
          ContentType: 23
          Version: 771
          Length: 35
          Payload: 39aa76173aee3468a1e8402150499a9585259f6f799c7895d7d40be6879f
`
var httpRequestExpect = `Ethernet:
  Destination: 3066d026811b
  Source: f84d8991af52
  Type: 2048
  IP:
    Version: 4
    Header Length: 5
    Type of Service: "00"
    Total Length: 267
    Identification: "0000"
    Flags And Fragment Offset: "4000"
    Time to Live: "40"
    Protocol: 6
    Header Checksum: 62c6
    Source: c0a80316
    Destination: 77609c08
    TCP:
      Source Port: 52896
      Destination Port: 14093
      Sequence Number: b485ff92
      Acknowledgement Number: 1c6fdee6
      Header Length: 8
      Flags: "0108"
      Window: "0808"
      Checksum: "2352"
      Urgent Pointer: "0000"
      Options:
        Option:
          Kind: 1
        Option:
          Kind: 1
        Option:
          Kind: 8
          Length: 10
          Data: 929a15075619e69f
      HTTP:
        HTTP Request:
          Method: GET
          Path: /
          Version: HTTP/1.1
          Headers:
            Item: 'Host: 76dd1d83b7.iqiyi.com:14093'
            Item: 'User-Agent: curl/7.48.0'
            Item: 'Content-Length: 32'
            Item: 'Connection: Upgrade'
            Item: 'Sec-Websocket-Key: OEi-rNV2l4CuBd4zVzfLjg=='
            Item: 'Upgrade: websocket'
            Item: ""
          Body:
            Data: 2acc9cc819e51ccf44bdee6f4e26f45f63038a6cfddf86a550a6ff9b5d1f875b
`
var icmpExpect = `Ethernet:
  Destination: 3066d026811b
  Source: f84d8991af52
  Type: 2048
  IP:
    Version: 4
    Header Length: 5
    Type of Service: "00"
    Total Length: 84
    Identification: 6a11
    Flags And Fragment Offset: "0000"
    Time to Live: "40"
    Protocol: 1
    Header Checksum: 99a5
    Source: c0a80316
    Destination: 6ef24442
    ICMP:
      Type: 8
      Code: 0
      Checksum: 39932
      ICMP Echo:
        Identifier: 18297
        Sequence Number: 0
        Data: 657fb59d00030e6708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f3031323334353637
`
var arpExpect = `Ethernet:
  Destination: ffffffffffff
  Source: 3066d0268abb
  Type: 2054
  ARP:
    Hardware type: 1
    Protocol type: 2048
    Hardware size: 6
    Protocol size: 4
    Opcode: 1
    Sender MAC address: 3066d0268abb
    Sender IP address: c0a80337
    Target MAC address: "000000000000"
    Target IP address: c0a80363
`

func testParse(data []byte, rule string) (*base.Node, error) {
	var noResultError = errors.New("aaa")
	config := map[string]any{
		"custom-formatter": func(node *base.Node) (any, error) {
			isPackage := func(node *base.Node) bool {
				if node.Name == "Package" && node.Cfg.GetItem("parent") == node.Ctx.GetItem("root") {
					return true
				}
				return false
			}
			var getSubs func(node *base.Node) []*base.Node
			getSubs = func(node *base.Node) []*base.Node {
				children := []*base.Node{}
				for _, sub := range node.Children {
					if sub.Cfg.GetBool(stream_parser.CfgIsRefType) || sub.Cfg.GetBool("unpack") || isPackage(sub) {
						children = append(children, getSubs(sub)...)
					} else {
						children = append(children, sub)
					}
				}
				return children
			}
			if stream_parser.NodeHasResult(node) {
				v := stream_parser.GetResultByNode(node)
				switch ret := v.(type) {
				case []byte:
					return codec.EncodeToHex(ret), nil
				default:
					return ret, nil
				}
			}
			if node.Cfg.GetBool(stream_parser.CfgIsList) {
				var res yaml.MapSlice
				for _, sub := range getSubs(node) {
					d, err := sub.Result()
					if err != nil {
						if errors.Is(err, noResultError) {
							continue
						}
						return nil, err
					}
					res = append(res, yaml.MapItem{
						Key:   sub.Name,
						Value: d,
					})
				}
				if len(res) == 0 {
					return nil, noResultError
				}
				return res, nil
			} else {
				var res yaml.MapSlice
				children := getSubs(node)
				for _, sub := range children {
					d, err := sub.Result()
					if err != nil {
						if errors.Is(err, noResultError) {
							continue
						}
						return nil, err
					}
					res = append(res, yaml.MapItem{
						Key:   sub.Name,
						Value: d,
					})
				}
				if len(res) == 0 {
					return nil, noResultError
				}
				return res, nil
			}
		},
	}
	reader := bytes.NewReader(data)
	res, err := parser.ParseBinaryWithConfig(reader, rule, config)
	if err != nil {
		return nil, err
	}
	return res, nil
}

type S5 struct {
	Version  uint8
	NMethods uint8
	Methods  []uint8
	payload  *[]byte
}

func TestSocks5(t *testing.T) {
	t.Run("socks5 client negotiation", func(t *testing.T) {
		data := `050100`
		payload, err := codec.DecodeHex(data)
		if err != nil {
			t.Fatal(err)
		}
		reader := bytes.NewReader(payload)
		res, err := parser.ParseBinary(reader, "application-layer.socks5", "ClientNegotiation")
		if err != nil {
			t.Fatal(err)
		}
		DumpNode(res)
		var a S5
		err = utils2.NodeToStruct(res, &a)
		if err != nil {
			return
		}
	})

	t.Run("socks5 server negotiation", func(t *testing.T) {
		data := `0500`
		payload, err := codec.DecodeHex(data)
		if err != nil {
			t.Fatal(err)
		}
		reader := bytes.NewReader(payload)
		res, err := parser.ParseBinary(reader, "application-layer.socks5", "ServerNegotiation")
		if err != nil {
			t.Fatal(err)
		}
		DumpNode(res)
	})

	t.Run("socks5 auth req", func(t *testing.T) {
		data := `0104010203040401020304`
		payload, err := codec.DecodeHex(data)
		if err != nil {
			t.Fatal(err)
		}
		reader := bytes.NewReader(payload)
		res, err := parser.ParseBinary(reader, "application-layer.socks5", "AuthRequest")
		if err != nil {
			t.Fatal(err)
		}
		DumpNode(res)
	})

	t.Run("socks5 auth reply", func(t *testing.T) {
		data := `0100`
		payload, err := codec.DecodeHex(data)
		if err != nil {
			t.Fatal(err)
		}
		reader := bytes.NewReader(payload)
		res, err := parser.ParseBinary(reader, "application-layer.socks5", "AuthReply")
		if err != nil {
			t.Fatal(err)
		}
		DumpNode(res)
	})

	t.Run("socks5 Request", func(t *testing.T) {
		data := `050100030e7777772e676f6f676c652e636f6d0050`
		payload, err := codec.DecodeHex(data)
		if err != nil {
			t.Fatal(err)
		}
		reader := bytes.NewReader(payload)
		res, err := parser.ParseBinary(reader, "application-layer.socks5", "Request")
		if err != nil {
			t.Fatal(err)
		}
		DumpNode(res)
	})

	t.Run("socks5 Replies", func(t *testing.T) {
		data := `050000010a0000020050`
		payload, err := codec.DecodeHex(data)
		if err != nil {
			t.Fatal(err)
		}
		reader := bytes.NewReader(payload)
		res, err := parser.ParseBinary(reader, "application-layer.socks5", "Replies")
		if err != nil {
			t.Fatal(err)
		}
		DumpNode(res)
	})

}
