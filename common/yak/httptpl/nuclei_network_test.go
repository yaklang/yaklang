package httptpl

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strings"
	"testing"
)

func TestCreateYakTemplateFromNucleiTemplateRaw_Network_Smoking(t *testing.T) {
	flag, _ := codec.DecodeHex(`0700000200000002000000`)
	server, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\n" +
		"Content-Length: 111\r\n" +
		"Server: nginx\r\n\r\n" +
		"" +
		"Kernel Version 1.11.111  " + string(flag)))
	var demo = `id: tidb-unauth

info:
  name: TiDB - Unauthenticated Access
  author: lu4nx
  severity: high
  description: TiDB server was able to be accessed because no authentication was required.
  metadata:
    zoomeye-query: tidb +port:"4000"
  tags: network,tidb,unauth

network:
  - inputs:
      - read: 1024              # skip handshake packet
      - data: b200000185a6ff0900000001ff0000000000000000000000000000000000000000000000726f6f7400006d7973716c5f6e61746976655f70617373776f72640075045f70696406313337353030095f706c6174666f726d067838365f3634035f6f73054c696e75780c5f636c69656e745f6e616d65086c69626d7973716c076f735f757365720578787878780f5f636c69656e745f76657273696f6e06382e302e32360c70726f6772616d5f6e616d65056d7973716c  # authentication
        type: hex

    host:
      - "{{Hostname}}"
      - "{{Host}}:4000"

    read-size: 1024

    matchers:
      - type: binary
        binary:
          # resp format:
          # 07: length, 02: sequence number, 00: success
          - "0700000200000002000000"

    extractors:
      - type: regex
        regex:
          - 'Kernel Version \d\.\d\d\.\d\d\d'

      - type: regex
        regex:
          - 'Kernel 111Version \d\.\d\d\.\d\d\d'

# Enhanced by mp on 2022/07/20`
	data, err := CreateYakTemplateFromNucleiTemplateRaw(demo)
	if err != nil {
		panic(err)
	}

	if len(data.TCPRequestSequences) != 1 {
		panic("len(data.TCPRequestSequences) != 1")
	}

	if ret := data.TCPRequestSequences[0].Inputs; len(ret) != 2 {
		panic("len(data.TCPRequestSequences[0].Inputs) != 2")
	} else {
		if ret[0].Read != 1024 {
			panic("ret[0].Read != 1024")
		}

		if !strings.Contains(ret[1].Data, "b200000185a6ff0900000001ff000000000") {
			spew.Dump(ret[1])
			panic("strings.Contains(ret[1].Data, \"b200000185a6ff0900000001ff000000000\")")
		}
	}

	n, err := data.Exec(nil, false, []byte("GET /bai/path HTTP/1.1\r\n"+
		"Host: www.baidu.com\r\n\r\n"), lowhttp.WithHost(server), lowhttp.WithPort(port))
	if err != nil {
		panic(err)
	}
	if n != 2 {
		panic(1)
	}
}
