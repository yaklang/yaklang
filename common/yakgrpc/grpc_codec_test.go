package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_COMMON_CODEC_AUTODECODE(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	t.Run("test auto decode ", func(t *testing.T) {
		rsp, err := client.AutoDecode(utils.TimeoutContextSeconds(1), &ypb.AutoDecodeRequest{Data: `MTI3LjAuMC4xICAgbG9jYWxob3N0IGxvY2FsaG9zdC5sb2NhbGRvbWFpbiBsb2NhbGhvc3Q0IGxvY2FsaG9zdDQubG9jYWxkb21haW40Cjo6MSAgICAgICAgIGxvY2FsaG9zdCBsb2NhbGhvc3QubG9jYWxkb21haW4gbG9jYWxob3N0NiBsb2NhbGhvc3Q2LmxvY2FsZG9tYWluNgoxMC4xOC4zLjEzNyBob3N0LTEwLTE4LTMtMTM3CjEwLjE4LjMuNzcgeXVubmFuCjEwLjE4LjMuMjU0IG1hc3Rlcgo=`})
		if err != nil {
			panic(err)
		}
		check := false
		for _, r := range rsp.GetResults() {
			if strings.Contains(string(r.Result), `ocalhost.localdoma`) {
				check = true
			}
		}
		if !check {
			t.Fatal("AUTO DECODE BASE64 SMOKING TEST FAILED")
		}
	})

	t.Run("test repeated auto decode", func(t *testing.T) {
		rsp, err := client.AutoDecode(context.Background(), &ypb.AutoDecodeRequest{Data: `%7B%22pctag%22%3A%22pc%22%2C%22appid%22%3A%221026%22%7D`})
		if err != nil {
			panic(err)
		}
		require.Len(t, rsp.GetResults(), 1)
		require.Equal(t, `UrlDecode`, string(rsp.GetResults()[0].Type))
	})

}

func TestGRPCMUSTPASS_COMMON_CODEC_Filetag(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	fp, err := consts.TempFile("test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	token := utils.RandStringBytes(10)
	fp.WriteString("asdfasdfas\r\nabc\r\n" + token)
	fp.Close()

	rsp, err := client.Codec(context.Background(), &ypb.CodecRequest{
		Text: "{{file:line(" + fp.Name() + ")}}",
		Type: "fuzz",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.GetResult(), token) {
		t.Fatal("filetag codec fail")
	}
}

func TestGRPCMUSTPASS_COMMON_CODEC_request_from_url(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	rsp, err := client.Codec(context.Background(), &ypb.CodecRequest{
		Text: "https://www.example.com/abc",
		Type: "packet-from-url",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.GetResult(), "/abc HTTP/") {
		t.Fatal("filetag codec fail")
	}
	if !strings.Contains(rsp.GetResult(), "User-Agent: ") {
		t.Fatal("filetag codec fail")
	}
}

func TestGRPCNewCodec(t *testing.T) {
	workFlow := []*ypb.CodecWork{
		{
			CodecType:  "HtmlEncode",
			Script:     "",
			PluginName: "",
			Params: []*ypb.ExecParamItem{
				{
					Key:   "entityRef",
					Value: "named",
				},
				{
					Key:   "fullEncode",
					Value: "false",
				},
			},
		},
	}
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	rsp, err := client.NewCodec(utils.TimeoutContextSeconds(1),
		&ypb.CodecRequestFlow{
			Text:       "<a>",
			Auto:       false,
			WorkFlow:   workFlow,
			InputBytes: nil,
		},
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(rsp.GetResult())
	if rsp.GetResult() != "&lt;a&gt;" {
		t.Fatal("workflow codec fail")
	}
}

func TestGRPCNewCodec_YakScript(t *testing.T) {
	workFlow := []*ypb.CodecWork{
		{
			CodecType: "CustomCodecPlugin",
			Params: []*ypb.ExecParamItem{
				{
					Key: "pluginContent",
					Value: `
handle = func(origin /*string*/) {
    if type(origin).String() != "string"{
		println(type(origin))
        return "no"
    }
    return "ok"
}
`,
				},
			},
		},
	}
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	rsp, err := client.NewCodec(utils.TimeoutContextSeconds(1),
		&ypb.CodecRequestFlow{
			Text:       "text",
			Auto:       false,
			WorkFlow:   workFlow,
			InputBytes: nil,
		},
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(rsp.GetResult())
	if rsp.GetResult() != "ok" {
		t.Fatal("check yak script input type fail")
	}
}

func TestGRPCNewCodec_Find(t *testing.T) {
	workFlow := []*ypb.CodecWork{
		{
			CodecType: "Find",
			Params: []*ypb.ExecParamItem{
				{
					Key:   "find",
					Value: "a.*",
				}, {
					Key:   "findType",
					Value: "regexp",
				}, {
					Key:   "Global",
					Value: "",
				}, {
					Key:   "Multiline",
					Value: "",
				}, {
					Key:   "IgnoreCase",
					Value: "",
				},
			},
		},
	}
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	rsp, err := client.NewCodec(utils.TimeoutContextSeconds(1),
		&ypb.CodecRequestFlow{
			Text:       "acccccc",
			Auto:       false,
			WorkFlow:   workFlow,
			InputBytes: nil,
		},
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(rsp.GetResult())
	if rsp.GetResult() != "acccccc" {
		t.Fatal("check find method fail")
	}
	client, err = NewLocalClient()
	if err != nil {
		panic(err)
	}
	rsp, err = client.NewCodec(utils.TimeoutContextSeconds(1),
		&ypb.CodecRequestFlow{
			Text:       "cccccc",
			Auto:       false,
			WorkFlow:   workFlow,
			InputBytes: nil,
		},
	)
	if err != nil {
		panic(err)
	}
	if rsp.GetResult() != "" {
		t.Fatal("check find method fail")
	}
}

func TestGRPCNewCodec_Replace(t *testing.T) {
	workFlow := []*ypb.CodecWork{
		{
			CodecType: "Replace",
			Params: []*ypb.ExecParamItem{
				{
					Key:   "find",
					Value: "a.*",
				},
				{
					Key:   "replace",
					Value: "c",
				},
				{
					Key:   "findType",
					Value: "regexp",
				},
				{
					Key:   "Global",
					Value: "true",
				},
				{
					Key:   "Multiline",
					Value: "",
				},
				{
					Key:   "IgnoreCase",
					Value: "",
				},
			},
		},
	}
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	rsp, err := client.NewCodec(utils.TimeoutContextSeconds(1),
		&ypb.CodecRequestFlow{
			Text: `abc
acb
aaa
bbb
`,
			Auto:       false,
			WorkFlow:   workFlow,
			InputBytes: nil,
		},
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(rsp.GetResult())
	if rsp.GetResult() != `c
c
c
bbb
` {
		t.Fatal("check replace method fail")
	}
	client, err = NewLocalClient()
	if err != nil {
		panic(err)
	}
	rsp, err = client.NewCodec(utils.TimeoutContextSeconds(1),
		&ypb.CodecRequestFlow{
			Text:       "cccccc",
			Auto:       false,
			WorkFlow:   workFlow,
			InputBytes: nil,
		},
	)
	if err != nil {
		panic(err)
	}
	if rsp.GetResult() != "cccccc" {
		t.Fatal("check find method fail")
	}
}

func TestGRPCCodecFlowFuzztag(t *testing.T) {
	workFlow := []*ypb.CodecWork{
		{
			CodecType:  "Base64Encode",
			Script:     "",
			PluginName: "",
			Params: []*ypb.ExecParamItem{
				{
					Key:   "Alphabet",
					Value: "standard",
				},
			},
		},
		{
			CodecType:  "URLEncode",
			Script:     "",
			PluginName: "",
			Params: []*ypb.ExecParamItem{
				{
					Key:   "fullEncode",
					Value: "true",
				},
			},
		},
	}
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	t.Run("test fuzz tag", func(t *testing.T) {
		flowName := utils.RandStringBytes(10)
		_, err = client.SaveCodecFlow(utils.TimeoutContextSeconds(1),
			&ypb.CustomizeCodecFlow{
				FlowName: flowName,
				WorkFlow: workFlow,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		data := utils.RandStringBytes(10)
		expected := codec.EncodeUrlCode(codec.EncodeBase64(data))

		res, err := mutate.FuzzTagExec("{{codecflow(" + flowName + "|" + data + ")}}")
		if err != nil {
			t.Fatal(err)
		}
		if len(res) == 0 {
			t.Fatal("fuzztag exec failed")
		}
		require.Equal(t, expected, res[0])
	})

	t.Run("test fuzz tag suspend", func(t *testing.T) {
		flowName := utils.RandStringBytes(10)
		_, err = client.SaveCodecFlow(utils.TimeoutContextSeconds(1),
			&ypb.CustomizeCodecFlow{
				FlowName: flowName,
				WorkFlow: workFlow,
				WorkFlowUI: `{
    "rightItems": [
        {
          "status": "suspend" 
        },
        {
        }
    ]
}`,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		data := utils.RandStringBytes(10)

		res, err := mutate.FuzzTagExec("{{codecflow(" + flowName + "|" + data + ")}}")
		if err != nil {
			t.Fatal(err)
		}
		if len(res) == 0 {
			t.Fatal("fuzztag exec failed")
		}
		require.Equal(t, data, res[0])
	})

	t.Run("test fuzz tag shield", func(t *testing.T) {
		flowName := utils.RandStringBytes(10)
		_, err = client.SaveCodecFlow(utils.TimeoutContextSeconds(1),
			&ypb.CustomizeCodecFlow{
				FlowName: flowName,
				WorkFlow: workFlow,
				WorkFlowUI: `{
    "rightItems": [
        {
          "status": "shield" 
        },
        {
        }
    ]
}`,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		data := utils.RandStringBytes(10)
		expected := codec.EncodeUrlCode(data)

		res, err := mutate.FuzzTagExec("{{codecflow(" + flowName + "|" + data + ")}}")
		if err != nil {
			t.Fatal(err)
		}
		if len(res) == 0 {
			t.Fatal("fuzztag exec failed")
		}
		require.Equal(t, expected, res[0])
	})
}

func TestGRPCCodecFlow_Normal(t *testing.T) {
	workFlow := []*ypb.CodecWork{
		{
			CodecType:  "Base64Encode",
			Script:     "",
			PluginName: "",
			Params: []*ypb.ExecParamItem{
				{
					Key:   "Alphabet",
					Value: "standard",
				},
			},
		},
	}
	workFlow2 := []*ypb.CodecWork{
		{
			CodecType:  "URLEncode",
			Script:     "",
			PluginName: "",
			Params: []*ypb.ExecParamItem{
				{
					Key:   "fullEncode",
					Value: "true",
				},
			},
		},
	}
	client, err := NewLocalClient(true)
	if err != nil {
		panic(err)
	}

	t.Run("test save", func(t *testing.T) {
		flowName := utils.RandStringBytes(10)
		defer func() {
			yakit.DeleteCodecFlow(consts.GetGormProfileDatabase(), flowName)
		}()
		_, err = client.SaveCodecFlow(utils.TimeoutContextSeconds(1),
			&ypb.CustomizeCodecFlow{
				FlowName: flowName,
				WorkFlow: workFlow,
			},
		)
		require.NoError(t, err)
		_, err = client.SaveCodecFlow(utils.TimeoutContextSeconds(1),
			&ypb.CustomizeCodecFlow{
				FlowName: flowName,
				WorkFlow: workFlow,
			},
		)
		require.Error(t, err, fmt.Sprintf("Codec Flow: %s already exists", flowName))
	})

	t.Run("test update", func(t *testing.T) {
		flowName := utils.RandStringBytes(10)
		defer func() {
			yakit.DeleteCodecFlow(consts.GetGormProfileDatabase(), flowName)
		}()
		_, err = client.UpdateCodecFlow(utils.TimeoutContextSeconds(1),
			&ypb.UpdateCodecFlowRequest{
				FlowId:   uuid.New().String(),
				FlowName: flowName,
				WorkFlow: workFlow,
			},
		)
		require.Error(t, err, fmt.Sprintf("Codec Flow: %s not find", flowName))
		_, err = client.SaveCodecFlow(utils.TimeoutContextSeconds(1),
			&ypb.CustomizeCodecFlow{
				FlowName: flowName,
				WorkFlow: workFlow,
			},
		)
		require.NoError(t, err)
		_, err = client.UpdateCodecFlow(utils.TimeoutContextSeconds(1),
			&ypb.UpdateCodecFlowRequest{
				FlowName: flowName,
				WorkFlow: workFlow2,
			},
		)
		require.NoError(t, err)
		codecFlow, err := yakit.GetCodecFlowByName(consts.GetGormProfileDatabase(), flowName)
		jsonData, _ := json.Marshal(workFlow2)
		require.NoError(t, err)
		require.Equal(t, codecFlow.WorkFlow, jsonData)

		// codecFlows, err := yakit.GetAllCodecFlow(consts.GetGormProfileDatabase())
		// jsonData, _ = json.Marshal(workFlow2)
		// require.NoError(t, err)
		// require.Equal(t, codecFlows[0].WorkFlow, jsonData)

		// codecFlow, err = yakit.GetCodecFlowByID(consts.GetGormProfileDatabase(), flowID_find)
		// jsonData, _ = json.Marshal(workFlow2)
		// require.NoError(t, err)
		// require.Equal(t, codecFlow.WorkFlow, jsonData)

	})
}

func TestGRPCCodecFlow(t *testing.T) {
	workFlow := []*ypb.CodecWork{
		{
			CodecType:  "Base64Decode",
			Script:     "",
			PluginName: "",
			Params: []*ypb.ExecParamItem{
				{
					Key:   "Alphabet",
					Value: "standard",
				},
			},
		},
	}
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	codeData := "\u202etest\x00\n\xff你好"
	expectViewData := "test\n\\xff你好"

	rsp, err := client.NewCodec(utils.TimeoutContextSeconds(1),
		&ypb.CodecRequestFlow{
			Text:       codec.EncodeBase64(codeData),
			Auto:       false,
			WorkFlow:   workFlow,
			InputBytes: nil,
		},
	)
	if err != nil || rsp == nil {
		t.Fatal(err)
	}

	require.Equal(t, codeData, string(rsp.GetRawResult()), "rawRes decode error")
	require.Equal(t, true, rsp.GetIsFalseAppearance(), "IsFalseAppearance check error")
	require.Equal(t, expectViewData, rsp.GetResult(), "result check error")

	// test is precise
	codeData = `ABC`
	rsp, err = client.NewCodec(utils.TimeoutContextSeconds(1),
		&ypb.CodecRequestFlow{
			Text:       codec.EncodeBase64(codeData),
			Auto:       false,
			WorkFlow:   workFlow,
			InputBytes: nil,
		},
	)
	if err != nil || rsp == nil {
		t.Fatal(err)
	}

	require.Equal(t, codeData, string(rsp.GetRawResult()), "rawRes decode error")
	require.Equal(t, false, rsp.GetIsFalseAppearance(), "IsFalseAppearance check error")
	require.Equal(t, codeData, rsp.GetResult(), "result check error")
}

func TestGRPCNewCodec_HTTPRequestMutate(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	check := func(packet, transform string, callback func(string, []byte)) {
		workFlow := []*ypb.CodecWork{
			{
				CodecType: "HTTPRequestMutate",
				Params: []*ypb.ExecParamItem{
					{
						Key:   "transform",
						Value: transform,
					},
				},
			},
		}
		rsp, err := client.NewCodec(utils.TimeoutContextSeconds(1),
			&ypb.CodecRequestFlow{
				Text:       packet,
				Auto:       false,
				WorkFlow:   workFlow,
				InputBytes: nil,
			},
		)
		require.NoError(t, err)
		callback(rsp.GetResult(), rsp.GetRawResult())
	}

	t.Run("GET to Form", func(t *testing.T) {
		check(`GET /?aaa=bbb HTTP/1.1
Host: www.example.com
`, "上传数据包", func(packet string, packetBytes []byte) {
			require.Equal(t, "POST", lowhttp.GetHTTPRequestMethod(packetBytes))

			contentType := lowhttp.GetHTTPPacketHeader(packetBytes, "content-type")
			require.Contains(t, contentType, "multipart/form-data")

			body := string(lowhttp.GetHTTPPacketBody(packetBytes))
			body = strings.TrimSpace(body)
			require.True(t, strings.HasPrefix(body, "--"), "expect body is a multipart/form-data, got "+body)
			require.True(t, strings.HasSuffix(body, "--"), "expect body is a multipart/form-data, got "+body)
			boundary := lowhttp.ExtractBoundaryFromBody(body)
			require.Contains(t, contentType, boundary, "expect boundary in content-type, got "+contentType)
			require.Contains(t, body, `name="aaa"`)
			require.Contains(t, body, "\r\n\r\nbbb\r\n")
		})
	})

	t.Run("Form to POST", func(t *testing.T) {
		check(`POST /ofcms-admin/admin/cms/template/save.json HTTP/1.1
Host: localhost:8080
Content-Type: multipart/form-data; boundary=b4287c56364c86452c746bc63feb846cd10a9ddc1e9ed979996b3519a5a3

--b4287c56364c86452c746bc63feb846cd10a9ddc1e9ed979996b3519a5a3
Content-Disposition: form-data; name="key"

value
--b4287c56364c86452c746bc63feb846cd10a9ddc1e9ed979996b3519a5a3--`, "POST", func(packet string, packetBytes []byte) {
			require.Equal(t, "POST", lowhttp.GetHTTPRequestMethod(packetBytes))

			contentType := lowhttp.GetHTTPPacketHeader(packetBytes, "content-type")
			require.Equal(t, "application/x-www-form-urlencoded", contentType)

			body := string(lowhttp.GetHTTPPacketBody(packetBytes))
			body = strings.TrimSpace(body)
			require.Equal(t, "key=value", body)
		})
	})

	t.Run("POST to GET same key", func(t *testing.T) {
		check(`POST / HTTP/1.1
Host: www.baidu.com
Content-Type: application/x-www-form-urlencoded
Content-Length: 11

key=1&key=1`, "GET", func(packet string, packetBytes []byte) {
			require.Equal(t, "GET", lowhttp.GetHTTPRequestMethod(packetBytes))
			require.Equal(t, map[string][]string{
				"key": {"1", "1"},
			}, lowhttp.GetFullHTTPRequestQueryParams(packetBytes))

			contentType := lowhttp.GetHTTPPacketHeader(packetBytes, "content-type")
			require.Equal(t, "", contentType)

			body := string(lowhttp.GetHTTPPacketBody(packetBytes))
			body = strings.TrimSpace(body)
			require.Equal(t, "", body)
		})
	})

	t.Run("fix invalid host cause mutate failed", func(t *testing.T) {
		check(`GET /?a=1 HTTP/1.1
Host: {{payload(test)}}
`, "POST", func(packet string, packetBytes []byte) {
			require.Equal(t, "POST", lowhttp.GetHTTPRequestMethod(packetBytes))
			require.Equal(t, "{{payload(test)}}", lowhttp.GetHTTPPacketHeader(packetBytes, "Host"))

			contentType := lowhttp.GetHTTPPacketHeader(packetBytes, "content-type")
			require.Equal(t, "application/x-www-form-urlencoded", contentType)

			body := string(lowhttp.GetHTTPPacketBody(packetBytes))
			body = strings.TrimSpace(body)
			require.Equal(t, "a=1", body)
		})
	})

	t.Run("Form only post params", func(t *testing.T) {
		check(`POST /?q=w&e=r HTTP/1.1
Host: www.baidu.com
Content-Type: application/x-www-form-urlencoded
Content-Length: 11

aaa=bbb&ccc=ddd`, "上传数据包(仅POST参数)", func(packet string, packetBytes []byte) {
			require.Equal(t, "POST", lowhttp.GetHTTPRequestMethod(packetBytes))
			require.Equal(t, map[string]string{
				"q": "w",
				"e": "r",
			}, lowhttp.GetAllHTTPRequestQueryParams(packetBytes))

			contentType := lowhttp.GetHTTPPacketHeader(packetBytes, "content-type")
			require.Contains(t, contentType, "multipart/form-data")

			body := string(lowhttp.GetHTTPPacketBody(packetBytes))
			body = strings.TrimSpace(body)
			require.True(t, strings.HasPrefix(body, "--"), "expect body is a multipart/form-data, got "+body)
			require.True(t, strings.HasSuffix(body, "--"), "expect body is a multipart/form-data, got "+body)
			boundary := lowhttp.ExtractBoundaryFromBody(body)
			require.Contains(t, contentType, boundary, "expect boundary in content-type, got "+contentType)
			require.Contains(t, body, `name="aaa"`)
			require.Contains(t, body, "\r\n\r\nbbb\r\n")
			require.Contains(t, body, `name="ccc"`)
			require.Contains(t, body, "\r\n\r\nddd\r\n")
			require.NotContains(t, body, `name="q"`)
			require.NotContains(t, body, "\r\n\r\nw\r\n")
			require.NotContains(t, body, `name="e"`)
			require.NotContains(t, body, "\r\n\r\nr\r\n")
		})
	})
}

func Tes() {

}
