package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/davecgh/go-spew/spew"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_MITM_HotPatch_Drop(t *testing.T) {
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(5))
	defer cancel()

	mockHost, mockPort := utils.DebugMockHTTPHandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("Hello"))
	})

	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&ypb.MITMRequest{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})
	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		if data.GetMessage().GetIsMessage() {
			msg := string(data.GetMessage().GetMessage())
			if !strings.Contains(msg, "starting mitm server") {
				continue
			}
			// load hot-patch mitm plugin
			stream.Send(&ypb.MITMRequest{
				SetYakScript: true,
				YakScriptContent: `hijackHTTPResponseEx = func(isHttps, url, req, rsp, forward, drop) { drop() }
afterRequest = func(ishttps, oreq ,req ,orsp ,rsp){
}
`,
			})
		} else if data.GetCurrentHook && len(data.GetHooks()) > 0 {
			// send packet
			packet := `GET / HTTP/1.1
Host: ` + utils.HostPort(mockHost, mockPort) + `

`
			packetBytes := lowhttp.FixHTTPRequest([]byte(packet))
			_, err := yak.Execute(`
rsp, req, err = poc.HTTPEx(packet, poc.proxy(mitmProxy))
assert rsp.RawPacket.Contains("响应被用户丢弃")
`, map[string]any{
				"packet":    string(packetBytes),
				"mitmProxy": `http://` + utils.HostPort("127.0.0.1", mitmPort),
			})
			if err != nil {
				t.Fatal(err)
			}
			cancel()
		}
	}
}

func TestGRPCMUSTPASS_MITM_HotPatch_Dangerous_FuzzTag(t *testing.T) {
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(5))
	defer cancel()

	// create a temporary file to test
	token1 := utils.RandStringBytes(16)
	fileName, err := utils.SaveTempFile(token1, "fuzztag-test-file")
	require.NoError(t, err)
	fileName = strings.ReplaceAll(fileName, "\\", "\\\\")
	// create a codec script to test
	token2 := utils.RandStringBytes(16)
	scriptName, clearFunc, err := yakit.CreateTemporaryYakScriptEx("codec", fmt.Sprintf(`
	handle = func(origin)  {
		return "%s"
	}`, token2))

	require.NoError(t, err)
	defer clearFunc()

	mockHost, mockPort := utils.DebugMockHTTPHandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("Hello"))
	})

	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	require.NoError(t, err)
	stream, err := client.MITM(ctx)
	require.NoError(t, err)
	stream.Send(&ypb.MITMRequest{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})
	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		if data.GetMessage().GetIsMessage() {
			msg := string(data.GetMessage().GetMessage())
			if !strings.Contains(msg, "starting mitm server") {
				continue
			}
			// load hot-patch mitm plugin
			stream.Send(&ypb.MITMRequest{
				SetYakScript: true,
				YakScriptContent: fmt.Sprintf(`hijackHTTPResponseEx = func(isHttps, url, req, rsp, forward, drop) {
	token1, token2 = "%s", "%s"
	file_fuzztag = fuzz.Strings("{{file(%s)}}")
	codec_fuzztag = fuzz.Strings("{{codec(%s)}}")
	if file_fuzztag[0].Contains(token1) || codec_fuzztag[0].Contains(token2) {
		forward(poc.ReplaceBody(rsp, "no", false))
	} else {
		forward(poc.ReplaceBody(rsp, "yes", false))
	}
}`, token1, token2, fileName, scriptName),
			})
		} else if data.GetCurrentHook && len(data.GetHooks()) > 0 {
			// send packet
			packet := `GET / HTTP/1.1
Host: ` + utils.HostPort(mockHost, mockPort) + `

`
			packetBytes := lowhttp.FixHTTPRequest([]byte(packet))
			_, err := yak.Execute(`
rsp, req = poc.HTTPEx(packet, poc.proxy(mitmProxy))~
assert rsp.RawPacket.Contains("yes")
`, map[string]any{
				"packet":    string(packetBytes),
				"mitmProxy": `http://` + utils.HostPort("127.0.0.1", mitmPort),
			})
			require.NoError(t, err)
			cancel()
		}
	}
}

func TestGRPCMUSTPASS_MITM_HotPatch_BeforeRequest_AfterRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(100))
	defer cancel()

	originReqToken := utils.RandStringBytes(16)
	hijackReqToken := utils.RandStringBytes(16)
	reqToken := utils.RandStringBytes(16)
	originRspToken := utils.RandStringBytes(16)
	hijackRspToken := utils.RandStringBytes(16)
	rspToken := utils.RandStringBytes(16)

	mockHost, mockPort := utils.DebugMockHTTPEx(func(req []byte) []byte {
		spew.Dump(req)
		if !bytes.Contains(req, []byte(reqToken)) {
			panic("req token not found")
		}
		return []byte("HTTP/1.1 200 OK\r\nContent-Length: 16\r\n" + originRspToken + "\r\n\r\n")
	})

	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&ypb.MITMRequest{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})

	hotPatchScript := `hijackHTTPRequest = func(isHttps, url, req, forward , drop) {
    req = poc.ReplaceHTTPPacketBody(req,"` + hijackReqToken + `")
    forward(req)
}

beforeRequest = func(ishttps,oreq,req){
	if !oreq.Contains("` + originReqToken + `") { // check oreq correct
		return req
	}
    return poc.ReplaceHTTPPacketBody(req, "` + reqToken + `")
}

hijackHTTPResponse = func(isHttps, url, rsp, forward, drop) {
    rsp = poc.ReplaceHTTPPacketBody(rsp,"` + hijackRspToken + `")
    forward(rsp)
}

afterRequest = func(ishttps,oreq,req,orsp,rsp){

	if !oreq.Contains("` + originReqToken + `") { // check oreq correct
		println("oreq error")
		return rsp
	}	
	
	if !req.Contains("` + reqToken + `") { // check req correct
		println("req error")
		return rsp
	}

	if !orsp.Contains("` + originRspToken + `") { // check orsp correct
		println("orsp error")
		return rsp
	}

	if !rsp.Contains("` + hijackRspToken + `") { // check hijack req correct
		println("rsp error")
		return rsp
	}
    return poc.ReplaceHTTPPacketBody(rsp, "` + rspToken + `")
}



`

	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		if data.GetMessage().GetIsMessage() {
			msg := string(data.GetMessage().GetMessage())
			if !strings.Contains(msg, "starting mitm server") {
				continue
			}
			// load hot-patch mitm plugin
			stream.Send(&ypb.MITMRequest{
				SetYakScript:     true,
				YakScriptContent: hotPatchScript,
			})
			stream.Send(&ypb.MITMRequest{
				SetAutoForward:   true,
				AutoForwardValue: true,
			})
		} else if data.GetCurrentHook && len(data.GetHooks()) > 0 {
			// send packet
			go func() {
				packet := `GET / HTTP/1.1
Host: ` + utils.HostPort(mockHost, mockPort) + `

` + originReqToken + `
`
				packetBytes := lowhttp.FixHTTPRequest([]byte(packet))
				_, err := yak.Execute(`
rsp, req = poc.HTTPEx(packet, poc.proxy(mitmProxy))~
dump(rsp.RawPacket)
assert rsp.RawPacket.Contains("`+rspToken+`")
`, map[string]any{
					"packet":    string(packetBytes),
					"mitmProxy": `http://` + utils.HostPort("127.0.0.1", mitmPort),
				})
				if err != nil {
					t.Fatal(err)
				}
				cancel()
			}()
		} else if data.Request != nil && !data.ForResponse {
			// send packet
			if !bytes.Contains(data.Request, []byte(hijackReqToken)) {
				t.Fatal("hijack req token not found")
			}
			stream.Send(&ypb.MITMRequest{
				HijackResponse: true,
				Forward:        true,
			})
		} else if data.Response != nil {
			// send packet
			if !bytes.Contains(data.Response, []byte(hijackRspToken)) {
				t.Fatal("hijack rsp token not found")
			}

			stream.Send(&ypb.MITMRequest{
				Forward: true,
			})
		}

	}
}

func TestGRPCMUSTPASS_MITM_HotPatch_HijackAndMirrorURL(t *testing.T) {
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(5))
	defer cancel()

	hookURLCheck := false
	mockHost, mockPort := utils.DebugMockHTTPHandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/notify" {
			hookURLCheck = true
		}
		writer.Write([]byte("Hello"))
	})

	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&ypb.MITMRequest{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})
	for {
		data, err := stream.Recv()
		log.Infof("data: %v", data)
		if err != nil {
			break
		}
		if data.GetMessage().GetIsMessage() {
			msg := string(data.GetMessage().GetMessage())
			log.Infof("msg: %v", msg)
			if !strings.Contains(msg, "starting mitm server") {
				continue
			}
			// load hot-patch mitm plugin
			stream.Send(&ypb.MITMRequest{
				SetYakScript: true,
				YakScriptContent: `
				hijackHTTPRequest = func(isHttps, url, req, forward, drop) {
					modified = str.ReplaceAll(string(req), "/origin", "/modify")
					forward(poc.FixHTTPRequest(modified))
				}
				mirrorHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
					yakit.Output(url)
					yakit.Output(req)

					if str.Contains(url, "/modify") {
						modified = str.ReplaceAll(string(req), "/modify", "/notify")
						req := poc.FixHTTPRequest(modified)
						poc.HTTPEx(req)
					}
				}
				`,
			})
		} else if data.GetCurrentHook && len(data.GetHooks()) > 0 {
			// send packet
			packet := `GET /origin HTTP/1.1
Host: ` + utils.HostPort(mockHost, mockPort) + `

`
			packetBytes := lowhttp.FixHTTPRequest([]byte(packet))
			_, err := yak.Execute(`
rsp, req, err = poc.HTTPEx(packet, poc.proxy(mitmProxy))
`, map[string]any{
				"packet":    string(packetBytes),
				"mitmProxy": `http://` + utils.HostPort("127.0.0.1", mitmPort),
			})
			time.Sleep(1 * time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if !hookURLCheck {
				t.Fatalf("hook url check failed")
			}
			cancel()
		}
	}
}

func TestGRPCMUSTPASS_MITM_HotPatch_Output(t *testing.T) {
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(100))
	defer cancel()

	token1 := utils.RandStringBytes(16)
	token2 := utils.RandStringBytes(16)
	mockHost, mockPort := utils.DebugMockHTTPHandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("Hello"))
	})

	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&ypb.MITMRequest{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})
	var mirrorCheck bool        // load hotpatch hook
	var beforeRequestCheck bool // MutateHookCaller
	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		if data.GetMessage().GetIsMessage() {
			msg := string(data.GetMessage().GetMessage())
			if !strings.Contains(msg, "starting mitm server") {
				if strings.Contains(msg, token1) {
					mirrorCheck = true
				}
				if strings.Contains(msg, token2) {
					beforeRequestCheck = true
				}
				continue
			}

			// load hot-patch mitm plugin
			stream.Send(&ypb.MITMRequest{
				SetYakScript: true,
				YakScriptContent: fmt.Sprintf(`mirrorHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
    yakit.Output("%s")
}
beforeRequest = func(ishttps, oreq, req){
    yakit_output("%s")
}`, token1, token2),
			})
		} else if data.GetCurrentHook && len(data.GetHooks()) > 0 {
			go func() {
				// send packet
				packet := `GET / HTTP/1.1
Host: ` + utils.HostPort(mockHost, mockPort) + `

`
				packetBytes := lowhttp.FixHTTPRequest([]byte(packet))
				_, err = lowhttp.HTTPWithoutRedirect(lowhttp.WithPacketBytes(packetBytes), lowhttp.WithProxy(`http://`+utils.HostPort("127.0.0.1", mitmPort)))
				require.NoError(t, err)
				time.Sleep(1 * time.Second)
				cancel()
			}()
		}
	}

	require.True(t, mirrorCheck, "mirrorHttpFlow hook yakit.output fail")
	require.True(t, beforeRequestCheck, "beforeRequest hook yakit.output fail")
}

func TestGRPCMUSTPASS_MITM_HotPatch_HijackSaveHTTPFlow(t *testing.T) {
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(10))
	defer cancel()

	mockHost, mockPort := utils.DebugMockHTTPHandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("Hello"))
	})

	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	token := utils.RandStringBytes(16)

	RunMITMTestServerEx(client, ctx, func(stream ypb.Yak_MITMClient) {
		stream.Send(&ypb.MITMRequest{
			Host: "127.0.0.1",
			Port: uint32(mitmPort),
		})
	}, func(stream ypb.Yak_MITMClient) {
		stream.Send(&ypb.MITMRequest{
			SetYakScript:     true,
			YakScriptContent: `hijackSaveHTTPFlow = func(flow, modify, drop) {flow.Blue();modify(flow)}`,
		})
		stream.Send(&ypb.MITMRequest{
			SetContentReplacers: true,
			Replacers:           make([]*ypb.MITMContentReplacer, 0),
		})
	}, func(stream ypb.Yak_MITMClient, msg *ypb.MITMResponse) {
		if msg.GetCurrentHook && len(msg.GetHooks()) > 0 {
			// send packet
			_, err := yak.Execute(`
			for i in 10 {
				url = f"${target}?token=${token}&randstr=${str.RandStr(10)}"
				rsp, req, _ = poc.Get(url, poc.proxy(mitmProxy), poc.save(false))
			}
			`, map[string]any{
				"mitmProxy": `http://` + utils.HostPort("127.0.0.1", mitmPort),
				"target":    `http://` + utils.HostPort(mockHost, mockPort),
				"token":     token,
			})
			if err != nil {
				t.Fatal(err)
			}
			cancel()
		}
	})

	rsp, err := QueryHTTPFlows(utils.TimeoutContextSeconds(2), client, &ypb.QueryHTTPFlowRequest{
		Keyword:    token,
		SourceType: "mitm",
	}, 10)
	require.NoError(t, err)
	for _, flow := range rsp.GetData() {
		require.Containsf(t, flow.Tags, "YAKIT_COLOR_BLUE", "flow tags not contains COLOR_BLUE")
	}
}
