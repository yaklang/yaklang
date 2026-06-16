package yakcmds

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/schema"
	cli "github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
)

// constHotPatchValue 复刻 grpc_mitm.go 中的 constClujore: 把一个值/闭包包成 func() interface{}
// 关键词: hotpatch, constClujore, MITM hijack callback
func constHotPatchValue(i interface{}) func() interface{} {
	return func() interface{} {
		return i
	}
}

// MITMHotPatchResult MITM/全局热加载验证执行后的证据结构
type MITMHotPatchResult struct {
	Scope            string
	URL              string
	OriginRequest    []byte
	BeforeRequest    []byte
	ModifiedRequest  []byte
	OriginResponse   []byte
	ModifiedResponse []byte
	AfterResponse    []byte
	MockedResponse   []byte
	Dropped          bool
	DropStage        string
	SaveTags         string
	RequestHooked    bool
	ResponseHooked   bool
	SaveHooked       bool
}

// RunMITMHotPatch 复用 MixPluginCaller 的真实执行路径运行 MITM/全局热加载脚本。
// scope 仅用于输出标识 (mitm / global)，两类热加载在引擎侧共享同一套 hook 执行链。
// 关键词: hotpatch-mitm, hotpatch-global, MixPluginCaller, LoadHotPatch, CallHijackRequest
func RunMITMHotPatch(ctx context.Context, scope, code string, isHttps bool, urlStr string, req, rsp []byte) (*MITMHotPatchResult, error) {
	if len(req) > 0 {
		req = lowhttp.FixHTTPRequest(req)
	}
	if urlStr == "" && len(req) > 0 {
		scheme := "http"
		if isHttps {
			scheme = "https"
		}
		urlStr = lowhttp.GetUrlFromHTTPRequest(scheme, req)
	}

	caller, err := yak.NewMixPluginCaller()
	if err != nil {
		return nil, utils.Wrap(err, "create mix plugin caller failed")
	}
	caller.SetCtx(ctx)
	caller.SetLoadPluginTimeout(15)
	caller.SetCallPluginTimeout(20)

	if err := caller.LoadHotPatch(ctx, nil, code); err != nil {
		return nil, utils.Wrap(err, "load hot patch code failed")
	}

	res := &MITMHotPatchResult{
		Scope:          scope,
		URL:            urlStr,
		OriginRequest:  req,
		OriginResponse: rsp,
	}

	currentReq := req
	// 全局管线: beforeRequest 在劫持之前
	if before := caller.CallBeforeRequestWithCtx(ctx, isHttps, urlStr, req, currentReq); len(before) > 0 {
		res.BeforeRequest = before
		currentReq = before
	}

	// 请求劫持 hijackHTTPRequest(isHttps, url, req, forward, drop)
	caller.CallHijackRequestWithCtx(ctx, isHttps, urlStr,
		constHotPatchValue(currentReq),
		constHotPatchValue(func(replaced interface{}) {
			if res.Dropped || replaced == nil {
				return
			}
			ret := utils.InterfaceToBytes(replaced)
			if len(ret) > 0 {
				res.ModifiedRequest = lowhttp.FixHTTPRequest(ret)
				res.RequestHooked = true
				currentReq = res.ModifiedRequest
			}
		}),
		constHotPatchValue(func() {
			res.Dropped = true
			res.DropStage = "request"
		}),
	)

	currentRsp := rsp
	if !res.Dropped {
		if len(rsp) > 0 {
			// 响应劫持 hijackHTTPResponseEx(isHttps, url, req, rsp, forward, drop)
			caller.CallHijackResponseExWithCtx(ctx, isHttps, urlStr,
				constHotPatchValue(currentReq),
				constHotPatchValue(currentRsp),
				constHotPatchValue(func(replaced interface{}) {
					if res.Dropped || replaced == nil {
						return
					}
					ret := utils.InterfaceToBytes(replaced)
					if len(ret) > 0 {
						res.ModifiedResponse = ret
						res.ResponseHooked = true
						currentRsp = ret
					}
				}),
				constHotPatchValue(func() {
					res.Dropped = true
					res.DropStage = "response"
				}),
			)

			// 全局管线: afterRequest 在响应之后
			if after := caller.CallAfterRequestWithCtx(ctx, isHttps, urlStr, req, currentReq, rsp, currentRsp); len(after) > 0 {
				res.AfterResponse = after
			}
		} else {
			// 无真实响应时尝试 mockHTTPRequest(isHttps, url, req, mockResponse)
			caller.CallMockHTTPRequestWithCtx(ctx, isHttps, urlStr,
				constHotPatchValue(currentReq),
				func(mock interface{}) {
					if mock == nil {
						return
					}
					res.MockedResponse = utils.InterfaceToBytes(mock)
				},
			)
		}
	}

	// 镜像 mirrorHTTPFlow(isHttps, url, req, rsp, body)
	if !res.Dropped {
		body := lowhttp.GetHTTPPacketBody(currentRsp)
		caller.MirrorHTTPFlow(isHttps, urlStr, currentReq, currentRsp, body)
	}

	// 入库劫持 hijackSaveHTTPFlow(flow, modify, drop)
	flow := &schema.HTTPFlow{Url: urlStr, IsHTTPS: isHttps}
	flow.SetRequest(string(currentReq))
	flow.SetResponse(string(currentRsp))
	caller.HijackSaveHTTPFlow(flow,
		func(modified *schema.HTTPFlow) {
			res.SaveHooked = true
			if modified != nil {
				flow = modified
			}
		},
		func() {
			res.Dropped = true
			res.DropStage = "save"
		},
	)
	res.SaveTags = flow.Tags

	caller.Wait()
	return res, nil
}

// WebFuzzerHotPatchResult Web Fuzzer 热加载验证执行后的证据结构
type WebFuzzerHotPatchResult struct {
	OriginRequest    []byte
	ModifiedRequest  []byte
	OriginResponse   []byte
	ModifiedResponse []byte
	MirrorData       map[string]string
	FailureReasons   []string
	RetryRequests    [][]byte
	RenderedFuzzTag  []string
	BeforeHooked     bool
	AfterHooked      bool
}

// RunWebFuzzerHotPatch 复用 MutateHookCaller 的真实执行路径运行 Web Fuzzer 热加载脚本。
// 关键词: hotpatch-webfuzzer, MutateHookCaller, beforeRequest, afterRequest, retryHandler, customFailureChecker
func RunWebFuzzerHotPatch(ctx context.Context, code string, isHttps bool, req, rsp []byte, fuzztag string) (*WebFuzzerHotPatchResult, error) {
	if len(req) > 0 {
		req = lowhttp.FixHTTPRequest(req)
	}
	before, after, mirrorFlow, retryHandler, customFailureChecker, _ := yak.MutateHookCaller(ctx, code, nil)

	res := &WebFuzzerHotPatchResult{
		OriginRequest:  req,
		OriginResponse: rsp,
	}

	currentReq := req
	if before != nil {
		ret := before(isHttps, req, currentReq)
		if len(ret) > 0 {
			res.ModifiedRequest = ret
			res.BeforeHooked = true
			currentReq = ret
		}
	}

	if len(rsp) > 0 {
		if after != nil {
			ret := after(isHttps, req, currentReq, rsp, rsp)
			if len(ret) > 0 {
				res.ModifiedResponse = ret
				res.AfterHooked = true
			}
		}
		if mirrorFlow != nil {
			res.MirrorData = mirrorFlow(currentReq, rsp, map[string]string{})
		}
		if customFailureChecker != nil {
			customFailureChecker(isHttps, currentReq, rsp, func(reason string) {
				res.FailureReasons = append(res.FailureReasons, reason)
			})
		}
		if retryHandler != nil {
			retryHandler(isHttps, 0, currentReq, rsp, func(newReqs ...[]byte) {
				for _, r := range newReqs {
					res.RetryRequests = append(res.RetryRequests, r)
				}
			})
		}
	}

	if fuzztag != "" {
		rendered, err := mutate.QuickMutate(fuzztag, consts.GetGormProfileDatabase(), yak.MutateWithYaklang(code))
		if err != nil {
			log.Errorf("render fuzztag failed: %s", err)
		} else {
			res.RenderedFuzzTag = rendered
		}
	}

	return res, nil
}

// RunCodecPlugin 复用 codec 插件 (右键) 的 handle(input) 执行路径。
// 关键词: codec-plugin, handle, SafeCallYakFunction, 右键 codec 调试
func RunCodecPlugin(ctx context.Context, code, input string) (string, error) {
	engine, err := yak.NewScriptEngine(1).ExecuteEx(code, map[string]interface{}{
		"YAK_FILENAME": "codec-plugin",
		"PLUGIN_NAME":  "codec-plugin",
	})
	if err != nil {
		return "", utils.Wrap(err, "execute codec plugin code failed")
	}
	result, err := engine.SafeCallYakFunction(ctx, "handle", []interface{}{input})
	if err != nil {
		return "", utils.Wrap(err, "call codec plugin handle(input) failed")
	}
	return utils.InterfaceToString(result), nil
}

// ---------- CLI helpers ----------

func readPacketFile(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, utils.Wrapf(err, "read file %s failed", path)
	}
	return raw, nil
}

func printPacketSection(title string, data []byte) {
	fmt.Printf("===== %s (len=%d) =====\n", title, len(data))
	if len(data) > 0 {
		fmt.Println(string(data))
	} else {
		fmt.Println("<empty / not changed>")
	}
	fmt.Println()
}

func printMITMHotPatchResult(res *MITMHotPatchResult) {
	fmt.Printf("########## %s hot-patch validation ##########\n", strings.ToUpper(res.Scope))
	fmt.Printf("url: %s\n", res.URL)
	fmt.Printf("dropped: %v (stage: %s)\n", res.Dropped, res.DropStage)
	fmt.Printf("request-hooked: %v, response-hooked: %v, save-hooked: %v\n", res.RequestHooked, res.ResponseHooked, res.SaveHooked)
	fmt.Printf("save-tags: %s\n\n", res.SaveTags)

	printPacketSection("origin request", res.OriginRequest)
	if len(res.BeforeRequest) > 0 {
		printPacketSection("beforeRequest rewritten", res.BeforeRequest)
	}
	printPacketSection("modified request", res.ModifiedRequest)
	if len(res.OriginResponse) > 0 {
		printPacketSection("origin response", res.OriginResponse)
	}
	if len(res.ModifiedResponse) > 0 {
		printPacketSection("modified response", res.ModifiedResponse)
	}
	if len(res.AfterResponse) > 0 {
		printPacketSection("afterRequest rewritten", res.AfterResponse)
	}
	if len(res.MockedResponse) > 0 {
		printPacketSection("mocked response", res.MockedResponse)
	}
}

func printWebFuzzerHotPatchResult(res *WebFuzzerHotPatchResult) {
	fmt.Println("########## WEBFUZZER hot-patch validation ##########")
	fmt.Printf("before-hooked: %v, after-hooked: %v\n", res.BeforeHooked, res.AfterHooked)
	fmt.Printf("failure-reasons: %v\n", res.FailureReasons)
	fmt.Printf("retry-requests: %d\n", len(res.RetryRequests))
	if len(res.MirrorData) > 0 {
		fmt.Printf("mirror-data: %v\n", res.MirrorData)
	}
	if len(res.RenderedFuzzTag) > 0 {
		fmt.Printf("rendered-fuzztag: %v\n", res.RenderedFuzzTag)
	}
	fmt.Println()

	printPacketSection("origin request", res.OriginRequest)
	if len(res.ModifiedRequest) > 0 {
		printPacketSection("beforeRequest rewritten", res.ModifiedRequest)
	}
	if len(res.OriginResponse) > 0 {
		printPacketSection("origin response", res.OriginResponse)
	}
	if len(res.ModifiedResponse) > 0 {
		printPacketSection("afterRequest rewritten", res.ModifiedResponse)
	}
	for i, r := range res.RetryRequests {
		printPacketSection(fmt.Sprintf("retry request #%d", i+1), r)
	}
}

func newHotPatchContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 60*time.Second)
}

// HotPatchValidatorCommands 暴露给 yak.go 注册的热加载/codec 验证命令组
var HotPatchValidatorCommands = []*cli.Command{
	{
		Name:        "hotpatch-mitm",
		Usage:       "yak hotpatch-mitm --script x.yak --request req.txt [--response rsp.txt] [--https] [--url URL]",
		Description: "Validate a MITM hot-patch script (hijackHTTPRequest/hijackHTTPResponseEx/mirrorHTTPFlow/hijackSaveHTTPFlow) reusing the real MixPluginCaller pipeline.",
		Flags: []cli.Flag{
			cli.StringFlag{Name: "script,s", Usage: "hot-patch yak script file"},
			cli.StringFlag{Name: "request,r", Usage: "raw http request file"},
			cli.StringFlag{Name: "response", Usage: "raw http response file (optional)"},
			cli.StringFlag{Name: "url", Usage: "override url (optional, inferred from request)"},
			cli.BoolFlag{Name: "https", Usage: "treat traffic as https"},
		},
		Action: func(c *cli.Context) error {
			code, err := readPacketFile(c.String("script"))
			if err != nil {
				return err
			}
			if len(code) == 0 {
				return utils.Error("script is required, use --script x.yak")
			}
			req, err := readPacketFile(c.String("request"))
			if err != nil {
				return err
			}
			rsp, err := readPacketFile(c.String("response"))
			if err != nil {
				return err
			}
			ctx, cancel := newHotPatchContext()
			defer cancel()
			res, err := RunMITMHotPatch(ctx, "mitm", string(code), c.Bool("https"), c.String("url"), req, rsp)
			if err != nil {
				return err
			}
			printMITMHotPatchResult(res)
			return nil
		},
	},
	{
		Name:        "hotpatch-global",
		Usage:       "yak hotpatch-global --script x.yak --request req.txt [--response rsp.txt] [--https] [--url URL]",
		Description: "Validate a global hot-patch script (beforeRequest/afterRequest + hijack + save) reusing the real global hot-patch pipeline.",
		Flags: []cli.Flag{
			cli.StringFlag{Name: "script,s", Usage: "hot-patch yak script file"},
			cli.StringFlag{Name: "request,r", Usage: "raw http request file"},
			cli.StringFlag{Name: "response", Usage: "raw http response file (optional)"},
			cli.StringFlag{Name: "url", Usage: "override url (optional, inferred from request)"},
			cli.BoolFlag{Name: "https", Usage: "treat traffic as https"},
		},
		Action: func(c *cli.Context) error {
			code, err := readPacketFile(c.String("script"))
			if err != nil {
				return err
			}
			if len(code) == 0 {
				return utils.Error("script is required, use --script x.yak")
			}
			req, err := readPacketFile(c.String("request"))
			if err != nil {
				return err
			}
			rsp, err := readPacketFile(c.String("response"))
			if err != nil {
				return err
			}
			ctx, cancel := newHotPatchContext()
			defer cancel()
			res, err := RunMITMHotPatch(ctx, "global", string(code), c.Bool("https"), c.String("url"), req, rsp)
			if err != nil {
				return err
			}
			printMITMHotPatchResult(res)
			return nil
		},
	},
	{
		Name:        "hotpatch-webfuzzer",
		Usage:       "yak hotpatch-webfuzzer --script x.yak --request req.txt [--response rsp.txt] [--fuzztag TAG] [--https]",
		Description: "Validate a Web Fuzzer hot-patch script (beforeRequest/afterRequest/mirrorHTTPFlow/retryHandler/customFailureChecker + {{yak(...)}} fuzztag) reusing MutateHookCaller.",
		Flags: []cli.Flag{
			cli.StringFlag{Name: "script,s", Usage: "hot-patch yak script file"},
			cli.StringFlag{Name: "request,r", Usage: "raw http request file"},
			cli.StringFlag{Name: "response", Usage: "raw http response file (optional)"},
			cli.StringFlag{Name: "fuzztag", Usage: "fuzztag template to render with the script's {{yak(...)}} handlers (optional)"},
			cli.BoolFlag{Name: "https", Usage: "treat traffic as https"},
		},
		Action: func(c *cli.Context) error {
			code, err := readPacketFile(c.String("script"))
			if err != nil {
				return err
			}
			if len(code) == 0 {
				return utils.Error("script is required, use --script x.yak")
			}
			req, err := readPacketFile(c.String("request"))
			if err != nil {
				return err
			}
			rsp, err := readPacketFile(c.String("response"))
			if err != nil {
				return err
			}
			ctx, cancel := newHotPatchContext()
			defer cancel()
			res, err := RunWebFuzzerHotPatch(ctx, string(code), c.Bool("https"), req, rsp, c.String("fuzztag"))
			if err != nil {
				return err
			}
			printWebFuzzerHotPatchResult(res)
			return nil
		},
	},
	{
		Name:        "codec-plugin",
		Usage:       "yak codec-plugin --script x.yak --input STRING | --input-file file",
		Description: "Validate a codec plugin (right-click codec) by calling its handle(input) function and printing the output.",
		Flags: []cli.Flag{
			cli.StringFlag{Name: "script,s", Usage: "codec plugin yak script file (must define handle)"},
			cli.StringFlag{Name: "input,i", Usage: "input string passed to handle(input)"},
			cli.StringFlag{Name: "input-file", Usage: "read input from file instead of --input"},
		},
		Action: func(c *cli.Context) error {
			code, err := readPacketFile(c.String("script"))
			if err != nil {
				return err
			}
			if len(code) == 0 {
				return utils.Error("script is required, use --script x.yak")
			}
			input := c.String("input")
			if f := c.String("input-file"); f != "" {
				raw, err := readPacketFile(f)
				if err != nil {
					return err
				}
				input = string(raw)
			}
			ctx, cancel := newHotPatchContext()
			defer cancel()
			out, err := RunCodecPlugin(ctx, string(code), input)
			if err != nil {
				return err
			}
			fmt.Println("########## CODEC plugin validation ##########")
			fmt.Printf("input  (len=%d): %s\n", len(input), input)
			fmt.Printf("output (len=%d): %s\n", len(out), out)
			return nil
		},
	},
}
