package bin_parser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"gopkg.in/yaml.v2"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestTOA(t *testing.T) {
	remoteAddr := "59.56.179.46"
	//iptables -A OUTPUT -p tcp -d 93.184.216.34 --tcp-flags RST RST -j DROP
	raddr, err := net.ResolveIPAddr("ip", remoteAddr)
	if err != nil {
		t.Fatal(err)
	}
	ipConn, err := net.DialIP("ip:6", nil, raddr)
	if err != nil {
		t.Fatal(err)
	}
	dataHex := "3066d026811bf84d8991af5208004500010b00004000400662c6c0a8031677609c08cea0370db485ff921c6fdee680180808235200000101080a929a15075619e69f474554202f20485454502f312e310d0a486f73743a20373664643164383362372e69716979692e636f6d3a31343039330d0a557365722d4167656e743a206375726c2f372e34382e300d0a436f6e74656e742d4c656e6774683a2033320d0a436f6e6e656374696f6e3a20557067726164650d0a5365632d576562736f636b65742d4b65793a204f45692d724e56326c3443754264347a567a664c6a673d3d0d0a557067726164653a20776562736f636b65740d0a0d0a2acc9cc819e51ccf44bdee6f4e26f45f63038a6cfddf86a550a6ff9b5d1f875b"
	data, err := codec.DecodeHex(dataHex)
	if err != nil {
		t.Fatal(err)
	}
	node, err := parser.ParseBinary(bytes.NewReader(data), "ethernet")
	if err != nil {
		t.Fatal(err)
	}
	resMap, err := node.Result()
	if err != nil {
		t.Fatal(err)
	}
	modifyBytes := func(f func(d map[string]any)) []byte {
		itcpMap, ok := getSubData(resMap, "Ethernet/Internet Protocol/TCP")
		if !ok {
			t.Fatal("get tcp failed")
		}
		tcpMap := itcpMap.(map[string]any)
		f(tcpMap)
		node, err = parser.GenerateBinary(resMap, "ethernet")
		if err != nil {
			t.Fatal(err)
		}
		tcpNode := GetSubNode(node, "Ethernet/Internet Protocol/TCP")
		data = stream_parser.GetBytesByNode(tcpNode)
		return data
	}
	iipMap, ok := getSubData(resMap, "Ethernet/Internet Protocol")
	if !ok {
		t.Fatal("get ip failed")
	}
	ipMap := iipMap.(map[string]any)

	getPayload := func(f func(map[string]any)) []byte {
		data := modifyBytes(func(d map[string]any) {
			f(d)
			d["Checksum"] = 0
		})
		return modifyBytes(func(d map[string]any) {
			f(d)
			d["Checksum"] = Checksum("192.168.3.22", remoteAddr, data)
		})
	}
	port := utils.GetRandomAvailableTCPPort()
	_ = port
	data = getPayload(func(tcpMap map[string]any) {
		tcpMap["Source Port"] = 64893
		tcpMap["Destination Port"] = 80
		tcpMap["Flags"] = 4
		tcpMap["Header Length"] = 5
		//tcpMap["Acknowledgement Number"] = 0
		delete(tcpMap, "Options")
		delete(tcpMap, "HTTP")
		ipMap["Total Length"] = 40
	})
	ipConn.Write(data)
}

func TestParser(t *testing.T) {
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
			resYaml, err := ResultToYaml(resMap)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, tt.args.expect, string(resYaml))
		})
	}
}

var icmpV6Expect = `Ethernet:
  Destination: 3333ffa35b78
  Source: f84d8991af52
  Type: 34525
  Internet Protocol Version 6:
    Version: 6
    Traffic Class: 0
    Flow Label: 0
    Payload Length: 32
    Next Header: 58
    Hop Limit: 255
    Source: fe8000000000000000237c9bf9dd7b2d
    Destination: ff0200000000000000000001ffa35b78
    ICMPV6:
      Type: 135
      Code: 0
      Checksum: 3154
      Payload: 00000000fe8000000000000014ae6f6a11a35b780101f84d8991af52
`
var dnsResponseExpect = `Ethernet:
  Destination: f84d8991af52
  Source: 3066d026811b
  Type: 2048
  Internet Protocol:
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
            String:
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
  Internet Protocol:
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
            String:
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
  Internet Protocol:
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
      Transport Layer Security:
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
  Internet Protocol:
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
  Internet Protocol:
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
  Address Resolution Protocol:
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

func TestICMP(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	data := `"0f\xd0&\x81\x1b\xf8M\x89\x91\xafR\b\x00E\x00\x00@\x00\x00@\x00@\x06f\xea\xc0\xa8\x03\x16\b\b\b\bÞ\x01\xbb\x97k\xf4\xec\x00\x00\x00\x00\xb0\x02\xff\xffy\xeb\x00\x00\x02\x04\x05\xb4\x01\x03\x03\x06\x01\x01\b\nWy@\x17\x00\x00\x00\x00\x04\x02\x00\x00"`
	ethernetData, err := strconv.Unquote(data)
	if err != nil {
		t.Fatal(err)
	}
	ret, err := testParse([]byte(ethernetData), "ethernet")
	if err != nil {
		t.Fatal(err)
	}
	resMap, err := ret.Result()
	resYaml, err := ResultToYaml(resMap)
	if err != nil {
		t.Fatal(err)
	}
	println(resYaml)

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
	res, err := parser.ParseBinary(reader, "http_request")
	if err != nil {
		t.Fatal(err)
	}
	_ = res
	//spew.Dump(codec.EncodeToHex(res.Bytes))
	//assert.Equal(t, codec.EncodeToHex(data), codec.EncodeToHex(res.Bytes))
}
func TestParseInternetProtocol(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	//data := `"0f\xd0&\x81\x1b\xf8M\x89\x91\xafR\b\x00E\x00\x00M\x00\x00@\x00@\x06\xee\xa0\xc0\xa8\x03\x16e[\"\xf1\xd9\xc1\x1f\x90l\x159\xad\x91&U+P\x18\x10\x00)\xa1\x00\x00\x17\xf1\x03\x00 \x8b\xff(\x84U\x82\xa1OĽ\xb7ƙ>g\x8f\xcb\vݔ,l\x7f\xb1\x97\n\xea\xcb?he\xc4"`
	//payload, err := strconv.Unquote(data)
	//if err != nil {
	//	t.Fatal(err)
	//}
	data := "3066d026811bf84d8991af5208004500010b00004000400662c6c0a8031677609c08cea0370db485ff921c6fdee680180808235200000101080a929a15075619e69f474554202f20485454502f312e310d0a486f73743a20373664643164383362372e69716979692e636f6d3a31343039330d0a557365722d4167656e743a206375726c2f372e34382e300d0a436f6e74656e742d4c656e6774683a2033320d0a436f6e6e656374696f6e3a20557067726164650d0a5365632d576562736f636b65742d4b65793a204f45692d724e56326c3443754264347a567a664c6a673d3d0d0a557067726164653a20776562736f636b65740d0a0d0a2acc9cc819e51ccf44bdee6f4e26f45f63038a6cfddf86a550a6ff9b5d1f875b"

	payload, err := codec.DecodeHex(data)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader([]byte(payload))
	res, err := parser.ParseBinary(reader, "ethernet")
	if err != nil {
		t.Fatal(err)
	}
	r, err := res.Result()
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(r)
}
func TestTLSSession(t *testing.T) {
	data := `16030100fd010000f9030379fd1d601a4710f618dd7354ff98d638017afcec683102320d34abaa54359380204fdec12241340d59cd98b4e7dfc2c6f6d594180e1fb95c6c1e2307b1da00abbf0026c02bc02fc02cc030cca9cca8c009c013c00ac014009c009d002f0035c012000a1301130213030100008a000500050100000000000a000a0008001d001700180019000b00020100000d001a0018080404030807080508060401050106010503060302010203ff010001000010000b000908687474702f312e3100120000002b0009080304030303020301003300260024001d00203e8c72a8d10a674c47bccb04b043cdce74e9fe3b44de3025a44fa2f093618862160303007a020000760303e2cad785acd6b2fb94f3b6b9ef1673b76db389fadd23e6edfd5354048c9ef3dc204fdec12241340d59cd98b4e7dfc2c6f6d594180e1fb95c6c1e2307b1da00abbf130100002e002b0002030400330024001d002015d77e937e5f1376b7c3e48d6d45443a227d439706bb5c3a3da412c39e17917914030300010117030300171fff0e6cda94f9c35ddb9d35767a4a11e0f11a5ac6af591703030370096010dc5b172904f9a02f60aea1d6f6f4fd0e0ae974414b04169ab9693e3cdee925252678b945c690cd2444b40d9b84a9bb3aec06c659e4f54991bf6df561360412aca03568fa1f8ca32b62652cf7e4e21c42572893e83574fc5906249b10d53ce5beb99087c9e3462265eef5055eafbf1372885069eea9be28c7d7e23c935bbd810af3de0610199c8af38853b2c94720ef1f1cd89b5e31860058f661605d3318a6238a027027ece4cc88f4fff0d096fb0a1d9804edb217a40156ec346417ceb56e5ee36352e2a98e0f522d6eebfd3d2cf1fc9300a25460e6691680167240878b4d265c6672bbc92023cedd361a875660f16cc06389bea2e1183a40c8912d5a37d22eff407e6b28751953f29956808f378e88f0c9c9b2fcff7c0fa54cb655586ccfb197fcae7099981a8d8f5aef90815b58f2626c27a416e1ee627453a983039ba37003aa45f22b94873885c1f7bb5bb16b043214cd752338b42daa62a646098b9ad0670021c868bdb7e6124f581c1c611f9e9cd898a04a8a8964ca1fb9f472315e91899d56a00e5ef968f52ec53f710e18717f6a44bf6b5aec91761c3037e50f7aa2e5594d03030e2660446ab9ee7e353775e233ba68183abd5dd426134c7664a5e6727107333d456cc4949ec4163e9329a066996d0ea151d2b87b40db8dd843fe2f157897a4f6a53c7141a5716a8e9bd89a88dbb5e93893c423ea6f38681fd6e0725634c24e7ea8264b488b209cb68a1be7c41345c12ca98d36dec0614312802ac93303e73ebd8df2fadd8c0766a8ca537881abc0bd3a58f84a5a6ef9b217ec920af5a28dbd7e73b95fd41242a648e74df44612f1b6dd48676c355c338f1bfccb4ba98cd003d22098599c9847a0cf0e941f96d9f54ff7f6082016d181ac47566eda7645eae264d1e4736315a05d5f319d143393423de7fa4e7ae0afda80c62c96fc3373c5ace00cae3f059697a1c8273ab97c1aa8b37946326217db884e32396b84ecb8c0bcbe8426c1b7226562cbf8d3f07ab6f22d4232c118e23c2f8896fdcff285ddd9dd7ca8a88073f365945177f919a14dd57a83789b01641a02d29bc8f572ca23b45b2eb75c2d8ce198ca6c1350516021ea9d45194c6e6137cb526cc666b4f5605a6894cec7048945184df2f611c1a80b0009e93868b3ca0ed55d5d57e8ca3a495d23e2bff84eb71122fe08eebd9737814c393deea364624be45940bb6e13be7c4f11d94ceca77ca9096dad17030301198e95550e802f52761754acfa6668caee51a796d97dc03f968efeaaaa906a2cf853e14bc677c940dc6ecf49d7d8fe55f1562bfd46ad5bdc5f56086152fd7c8ed77694505b8959041a1daaed1867e14e74a7d55965cd300e8d3295ac5b0a1370e267252cb0c42894d54944592aaf02744f5449934fa46cf7b19353d1af2a904937b618da67286b8e8bc9d3b0480e933447f9cee4014528ef34ecf9282dea9014e7d1a58ccc28e02e17549fb7ce3e047b56969dc95ac1cd9e8d74ff707743d23e3eb843521cd27cdc7ae2bba84b43cb821ced27b6c1dac5ca9117ce427ffb905e84a6bd1220d1db223a905913f0179b161be3e881011a590090e1911c75d8fb59a2b2f8113f3b1bc4daffa83c703415eebc8200210037a272bb5a17030300355e19196552b8615f986c8ea69efda113b08c20a69289bf3b218816d7c709640f0429ac28999c499639f22474a63310760bac666239140303000101170303003553a7ea2b544498dc2c02be3a77839645513ee9f5e6f2eba783e80f1d64c4e40c59c4d931caa7aa5b0b7f624c91faa4104e7356035b1703030047d03f2b0417080e282366abca13dc861c26d12880c483874129700b6dddf73f0c106413572bbbe877a56035615b5d8bf6ace73a96aac4638abcaf8f1e9018510faaf5b641880e6617030300315380b458d112f9d178f44e268377719f0addfdd29461562313e0b39d9c720c682b5ed16ca4306c26433e0f06493286012f1703030013338063e857c0efa52bc1b6573a682732dbcd3a`
	payload, err := codec.DecodeHex(data)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(payload)
	res, err := parser.ParseBinary(reader, "application-layer.tls")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Children) != 12 {
		t.Fatal("tls session parse failed")
	}
	DumpNode(res)
	dict1 := NodeToMap(res)
	res, err = parser.GenerateBinary(dict1, "application-layer.tls")
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
	res, err := parser.ParseBinary(reader, "data_link")
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
	res, err := parser.ParseBinary(reader, "application-layer.ntlm", "NegotiateMessage")
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
	res, err = parser.GenerateBinary(mapData, "application-layer.ntlm", "NegotiateMessage")
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
	res, err := parser.ParseBinary(reader, "application-layer.ntlm", "AuthenticationMessage")
	if err != nil {
		t.Fatal(err)
	}
	DumpNode(res)
	byts := NodeToBytes(res)
	assert.Equal(t, "TlRMTVNTUAADAAAAGAAYAFgAAAAWARYBcAAAAAAAAACGAQAACAAIAIYBAAAyADIAjgEAAAAAAABYAAAA", codec.EncodeBase64(byts))
}
func TestT3(t *testing.T) {
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
func TestIIOP1(t *testing.T) {
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
func TestHTTP(t *testing.T) {
	var data string
	var err error
	var pyaload []byte
	var reader *bytes.Reader
	var res *base.Node
	// Request
	//data = "474554202f20485454502f312e310d0a486f73743a20666130323639386263652e69716979692e636f6d3a31343039310d0a557365722d4167656e743a206375726c2f372e31372e300d0a436f6e74656e742d4c656e6774683a2033320d0a436f6e6e656374696f6e3a20557067726164650d0a5365632d576562736f636b65742d4b65793a204349576a774c4f776d666756425561556c34446556773d3d0d0a557067726164653a20776562736f636b65740d0a0d0ab462f8eda760a02ae79c4781eed151b1d410661b7329303071efaa83f2b9fd5e"
	//pyaload, err = codec.DecodeHex(data)
	//if err != nil {
	//	t.Fatal(err)
	//}
	//reader = bytes.NewReader(pyaload)
	//res, err = ParseBinary(reader, "application-layer.http")
	//if err != nil {
	//	t.Fatal(err)
	//}
	//r, err := res.Children[0].Result()
	//if err != nil {
	//	t.Fatal(err)
	//}
	//spew.Dump(r)
	// Response
	//data = "485454502f312e3120323030204f4b0d0a436f6e6e656374696f6e3a20636c6f73650d0a436f6e74656e742d547970653a206170706c69636174696f6e2f6f637465742d73747265616d0d0a436f6e74656e742d4c656e6774683a203234380d0a0d0a16f104002e0000002a0204f100a80cc4b7dffc81d5a39eb1a8add09e7a88c3cd0a76cf2171db6530579dbb55767e000000010016f104003735fc81fdbb363b73b197be0a92b2c0c2a3313edb14ce8e8cf44449dc72d895b2f429d10965d4e4c93fd28665f81db92761fa54ae89397917f10400686c74728d8356d26d9cf1edbbf7185f1ab07ab0af28dd0df99118643d91d52a9f887545505ea32efcbad72650894b68f5353eafa8ec6cb6b23b80e6ceccf879cad7570e831b623ceb9280e7d59b665bbf8a907d4ef5f5f6211de2334a600ba7bdb2ffbc919c2ab13915f1040017919b39d0aa21828a9f734d5251a240bd4bb4597f6009b5"
	//pyaload, err = codec.DecodeHex(data)
	//if err != nil {
	//	t.Fatal(err)
	//}
	//reader = bytes.NewReader(pyaload)
	//res, err = ParseBinary(reader, "application-layer.http")
	//if err != nil {
	//	t.Fatal(err)
	//}
	//r, err := res.Children[0].Result()
	//if err != nil {
	//	t.Fatal(err)
	//}
	//spew.Dump(r)
	// Chunked Request
	data = "504f5354202f20485454502f312e310d0a436f6e74656e742d547970653a206170706c69636174696f6e2f6a736f6e0d0a486f73743a207777772e6578616d706c652e636f6d0d0a5472616e736665722d456e636f64696e673a206368756e6b65640d0a0d0a330d0a613d310d0a300d0a0d0a"
	pyaload, err = codec.DecodeHex(data)
	if err != nil {
		t.Fatal(err)
	}
	reader = bytes.NewReader(pyaload)
	res, err = parser.ParseBinary(reader, "application-layer.http")
	if err != nil {
		t.Fatal(err)
	}
	r, err := res.Children[0].Result()
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(r)

}
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
					if sub.Cfg.GetBool("isRefType") || sub.Cfg.GetBool("unpack") || isPackage(sub) {
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
