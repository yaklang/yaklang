package loop_vuln_verify

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

const (
	reachabilityOK          = "REACHABLE"
	reachabilityUnreachable = "UNREACHABLE"
	reachabilityPartial     = "PARTIAL"
)

// probeHTTPTarget performs a lightweight HTTP HEAD/GET probe to check connectivity.
// It uses the project's lowhttp library (not the Go standard http client) so that
// probe behaviour is consistent with the rest of the toolchain (TLS fingerprint,
// connection pool, proxy support, etc.).
// Returns the HTTP status code (0 on connection failure), a summary string, and an error.
func probeHTTPTarget(targetURL string) (statusCode int, summary string, err error) {
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "http://" + targetURL
	}

	parsed, parseErr := url.Parse(targetURL)
	if parseErr != nil {
		return 0, "", fmt.Errorf("invalid target URL %q: %w", targetURL, parseErr)
	}
	isHTTPS := strings.EqualFold(parsed.Scheme, "https")

	// Try HEAD first — lighter on bandwidth; fall back to GET on failure.
	for _, method := range []string{"HEAD", "GET"} {
		packet := lowhttp.UrlToRequestPacket(method, targetURL, nil, isHTTPS)

		resp, doErr := lowhttp.HTTP(
			lowhttp.WithRequest(packet),
			lowhttp.WithHttps(isHTTPS),
			lowhttp.WithTimeout(10*time.Second),
			// Do not follow redirects — a redirect already confirms reachability.
			lowhttp.WithRedirectTimes(0),
		)
		if doErr != nil {
			if method == "HEAD" {
				// HEAD failed; try GET before giving up.
				continue
			}
			return 0, "", doErr
		}

		code := resp.GetStatusCode()
		return code, fmt.Sprintf("HTTP %d", code), nil
	}

	return 0, "", fmt.Errorf("probe failed for %s", targetURL)
}

func buildCheckReachabilityAction(invoker aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"check_target_reachability",
		"验证目标环境是否可访问。在 assess_reproducibility 确认发现可复现后调用本动作。",
		[]aitool.ToolOption{
			aitool.WithStringParam("target_url",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description(
					"要探测的 URL 或 host:port，例如 http://192.168.1.10:8080/api 或 https://target.example.com"),
			),
			aitool.WithStringParam("expected_service",
				aitool.WithParam_Description("预期的服务类型：http | https | tcp（默认从 URL scheme 自动识别）"),
			),
		},
		verifyCheckReachability,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			handleCheckReachability(loop, action, op, invoker)
		},
	)
}

func verifyCheckReachability(_ *reactloops.ReActLoop, action *aicommon.Action) error {
	if strings.TrimSpace(action.GetString("target_url")) == "" {
		return utils.Error("target_url is required")
	}
	return nil
}

func handleCheckReachability(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator, invoker aicommon.AIInvokeRuntime) {
	targetURL := strings.TrimSpace(action.GetString("target_url"))

	// Store the resolved target for later steps.
	if loop.Get(keyTargetInfo) == "" || loop.Get(keyTargetInfo) == "not_provided" {
		loop.Set(keyTargetInfo, targetURL)
	}

	log.Infof("[VulnVerify] probing target: %s", targetURL)

	statusCode, summary, probeErr := probeHTTPTarget(targetURL)
	if probeErr != nil {
		// Connection-level failure.
		errMsg := probeErr.Error()
		loop.Set(keyReachabilityStatus, reachabilityUnreachable)
		loop.Set(keyVerificationPhase, "concluded_unreachable")
		invoker.AddToTimeline("check_target_reachability",
			fmt.Sprintf("UNREACHABLE target=%s error=%s", targetURL, errMsg))

		op.Feedback(fmt.Sprintf(
			"目标不可达：%s\n错误：%s\n"+
				"下一步：调用 directly_answer，说明连通性故障并建议用户检查目标地址或环境配置。",
			targetURL, errMsg))
		op.Continue()
		return
	}

	// Any HTTP response (including 4xx/5xx) means the service is up.
	loop.Set(keyReachabilityStatus, reachabilityOK)
	loop.Set(keyVerificationPhase, "phase3_execute")
	invoker.AddToTimeline("check_target_reachability",
		fmt.Sprintf("REACHABLE target=%s status=%s", targetURL, summary))

	// Provide two examples: require_tool for the first HTTP call (loads+caches the tool),
	// and directly_call_tool for subsequent calls (uses cached tool with exact params).
	feedback := fmt.Sprintf(
		"目标可达：%s → %s\n"+
			"下一步：发送验证请求。\n\n"+
			"**第一次 HTTP 请求**（do_http_request 尚未在缓存中）：使用 require_tool 加载并执行：\n"+
			`{"@action": "require_tool", "tool_require_payload": "do_http_request", "human_readable_thought": "发送首次验证请求到 %s"}`+"\n\n"+
			"**后续 HTTP 请求**（工具已在 CACHE_TOOL_CALL 缓存中）：使用 directly_call_tool 携带精确参数：\n"+
			`{"@action": "directly_call_tool", "directly_call_tool_name": "do_http_request", `+
			`"directly_call_tool_params": {"url": "%s", "method": "POST", `+
			`"content-type": "application/x-www-form-urlencoded", `+
			`"headers": "<Cookie 或其他 Header>", `+
			`"post-params": "<param=payload>", "timeout": 10, "show-request": "yes", "redirect-times": 0}, `+
			`"directly_call_identifier": "probe_1", `+
			`"directly_call_expectations": "<预期响应特征，例如：响应包含 uid= 则说明命令执行成功>"}`+"\n\n"+
			"每次工具调用返回结果后，立即调用 record_evidence 记录观察到的响应。",
		targetURL, summary, targetURL, targetURL)

	switch {
	case statusCode == 401 || statusCode == 403:
		feedback += "\n注意：服务器返回 401/403，可能需要身份验证。请确认是否需要凭证或 Token 才能继续测试。"
	case statusCode >= 500:
		feedback += "\n注意：服务器返回 5xx，服务可能处于部分降级状态，请谨慎操作。"
	}

	op.Feedback(feedback)
	op.Continue()
}
