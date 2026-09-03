package browsertools

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// BuildDynamicCapabilityTools creates browser tools whose device is selected at
// invocation time. This lets one long-lived AI Agent conversation reference a
// different explicitly attached browser instance on each user turn.
func BuildDynamicCapabilityTools(bridge Bridge) ([]*aitool.Tool, error) {
	factory := aitool.NewFactory()

	if err := factory.RegisterTool(
		"browser.capability.catalog",
		aitool.WithDescription("List the signed capability catalog of an already-open browser instance explicitly attached by the user through the Yakit browser extension. This is the correct entry point for an attached browser; it does not create or control a Rod browser session."),
		aitool.WithVerboseName("Browser Extension Capability Catalog"),
		aitool.WithVerboseNameZh("浏览器插件能力目录"),
		aitool.WithUsage("The runtime binds browser_ref to an instance explicitly attached by the user. browser_ref is optional for one attachment and required when several are attached. Query this catalog before browser.capability.call and follow the selected method's paramsSchema exactly. Never substitute use_browser/op=open for an attached instance."),
		aitool.WithKeywords([]string{"browser", "browser instance", "attached browser", "browser extension", "current website", "open tabs", "capability catalog", "schema", "浏览器", "浏览器实例", "已打开浏览器", "浏览器插件", "当前网站", "标签页", "能力目录", "参数"}),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithStringParam(
			"browser_ref",
			aitool.WithParam_Description("Exact Reference shown on the attached browser; required only when multiple browsers are attached"),
			aitool.WithParam_MaxLength(512),
		),
		aitool.WithStringParam(
			"domain",
			aitool.WithParam_Description("Optional capability-domain filter"),
			aitool.WithParam_EnumString("all", "page", "isolation", "authorization", "network", "recording", "callable", "debugger", "transform", "handoff", "proxy", "system"),
		),
		aitool.WithStringParam(
			"query",
			aitool.WithParam_Description("Optional keyword filter, for example tab, click, cookie, capture, or proxy"),
			aitool.WithParam_MaxLength(120),
		),
		aitool.WithNoRuntimeCallback(func(
			_ context.Context,
			params aitool.InvokeParams,
			_ io.Writer,
			_ io.Writer,
		) (interface{}, error) {
			if bridge == nil || !bridge.Available() {
				return nil, fmt.Errorf("browser extension bridge is not running")
			}
			deviceID := strings.TrimSpace(params.GetString("device_id"))
			catalog, connected := bridge.CapabilityCatalog(deviceID)
			if !connected {
				return nil, fmt.Errorf("browser instance %q is offline or has no signed capability catalog", deviceID)
			}
			if _, _, err := browserCapabilityDescriptors(catalog); err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"deviceId":      deviceID,
				"schemaVersion": catalog.Version,
				"schemaHash":    catalog.Hash,
				"schemaDialect": catalog.SchemaDialect,
				"reviewPolicy":  "Pairing grants instance-level access to HTTP(S) tabs. Each operation remains constrained by the current AI review policy, browser restrictions, and enterprise policy.",
				"capabilities": browserCapabilityCatalog(
					catalog,
					params.GetString("domain"),
					params.GetString("query"),
				),
			}, nil
		}),
	); err != nil {
		return nil, err
	}

	if err := factory.RegisterTool(
		"browser.capability.call",
		aitool.WithDescription("Call a signed capability on the already-open browser instance attached by the user through the Yakit browser extension. The runtime binds that exact instance; this tool never creates a Rod browser session."),
		aitool.WithVerboseName("Browser Extension Capability"),
		aitool.WithVerboseNameZh("浏览器插件能力"),
		aitool.WithUsage("Call browser.capability.catalog first. Pass the same browser_ref when multiple browser instances are attached; it is optional for one attachment. The runtime maps only to user-attached instances. Use browser.tab.open to open an HTTP(S) website, or browser.tabs to inspect existing tabs, then use returned target identifiers for tab or document methods. When login requires a QR code, MFA, CAPTCHA, or device confirmation, call browser.handoff.request and wait; Yakit presents it locally and the Agent must not extract the pixels. Never substitute use_browser/op=open."),
		aitool.WithKeywords([]string{"browser", "browser instance", "attached browser", "browser extension", "current website", "open tabs", "page interaction", "network", "debugging", "proxy", "浏览器", "浏览器实例", "已打开浏览器", "浏览器插件", "当前网站", "标签页", "页面操作", "网络", "调试"}),
		aitool.WithStringParam(
			"browser_ref",
			aitool.WithParam_Description("Exact Reference shown on the attached browser; required only when multiple browsers are attached"),
			aitool.WithParam_MaxLength(512),
		),
		aitool.WithStringParam(
			"method",
			aitool.WithParam_Description("Capability method declared by the selected browser instance's signed catalog"),
			aitool.WithParam_MaxLength(256),
			aitool.WithParam_Required(true),
		),
		aitool.WithRawParam(
			"params",
			map[string]any{
				"type":                 "object",
				"additionalProperties": true,
				"description":          "Parameters for the method, following the exact paramsSchema returned by browser.capability.catalog",
			},
		),
		aitool.WithCallback(func(
			ctx context.Context,
			params aitool.InvokeParams,
			runtimeConfig *aitool.ToolRuntimeConfig,
			_ io.Writer,
			_ io.Writer,
		) (interface{}, error) {
			if bridge == nil || !bridge.Available() {
				return nil, fmt.Errorf("browser extension bridge is not running")
			}
			deviceID := strings.TrimSpace(params.GetString("device_id"))
			method := strings.TrimSpace(params.GetString("method"))
			catalog, connected := bridge.CapabilityCatalog(deviceID)
			if !connected {
				return nil, fmt.Errorf("browser instance %q is offline or has no signed capability catalog", deviceID)
			}
			descriptors, _, err := browserCapabilityDescriptors(catalog)
			if err != nil {
				return nil, err
			}
			descriptor, ok := descriptors[method]
			if !ok {
				return nil, fmt.Errorf("browser capability %q is not declared by browser instance %q", method, deviceID)
			}
			callParams := params.GetObject("params")
			if err := catalog.ValidateCapabilityParams(method, map[string]interface{}(callParams)); err != nil {
				return nil, err
			}
			return callAgentCapability(
				ctx,
				bridge,
				deviceID,
				Target{},
				method,
				callParams,
				browserCapabilityTimeout(descriptor),
				false,
				runtimeConfig,
			)
		}),
	); err != nil {
		return nil, err
	}

	if err := factory.RegisterTool(
		"browser.handoff.request",
		aitool.WithDescription("Pause the attached browser workflow and show a local Yakit handoff card for a QR-code login, MFA, CAPTCHA, or device confirmation. Use this instead of telling the user to switch to the browser."),
		aitool.WithVerboseName("Browser User Handoff"),
		aitool.WithVerboseNameZh("浏览器登录接管"),
		aitool.WithUsage("After the requested login or verification UI is visible, call this tool and wait for the user. For a QR-code login, Yakit extracts and displays the QR code locally in the conversation. Do not finish the task, ask the user to scan in the browser, inspect the QR pixels, or call browser.handoff.status yourself."),
		aitool.WithKeywords([]string{"browser login", "QR code", "scan login", "MFA", "CAPTCHA", "user handoff", "浏览器登录", "扫码登录", "二维码", "验证码", "人工接管"}),
		aitool.WithStringParam(
			"browser_ref",
			aitool.WithParam_Description("Exact Reference shown on the attached browser; required only when multiple browsers are attached"),
			aitool.WithParam_MaxLength(512),
		),
		aitool.WithStringParam(
			"reason",
			aitool.WithParam_Description("The user action currently required by the page"),
			aitool.WithParam_EnumString("qr_code", "mfa", "captcha", "device_confirmation", "other"),
			aitool.WithParam_Default("qr_code"),
		),
		aitool.WithStringParam(
			"message",
			aitool.WithParam_Description("Short instruction shown in the local handoff card"),
			aitool.WithParam_MaxLength(500),
		),
		aitool.WithIntegerParam("tabId", aitool.WithParam_Description("Target tab ID; omit to use the active HTTP(S) tab"), aitool.WithParam_Min(1)),
		aitool.WithIntegerParam("frameId", aitool.WithParam_Description("Target frame ID"), aitool.WithParam_Min(0)),
		aitool.WithStringParam("documentId", aitool.WithParam_Description("Target document ID when known"), aitool.WithParam_MaxLength(512)),
		aitool.WithCallback(func(
			ctx context.Context,
			params aitool.InvokeParams,
			runtimeConfig *aitool.ToolRuntimeConfig,
			_ io.Writer,
			_ io.Writer,
		) (interface{}, error) {
			if bridge == nil || !bridge.Available() {
				return nil, fmt.Errorf("browser extension bridge is not running")
			}
			deviceID := strings.TrimSpace(params.GetString("device_id"))
			catalog, connected := bridge.CapabilityCatalog(deviceID)
			if !connected {
				return nil, fmt.Errorf("browser instance %q is offline or has no signed capability catalog", deviceID)
			}
			descriptors, _, err := browserCapabilityDescriptors(catalog)
			if err != nil {
				return nil, err
			}
			descriptor, ok := descriptors["browser.handoff.request"]
			if !ok {
				return nil, fmt.Errorf("browser instance %q does not support local user handoff", deviceID)
			}
			reason := strings.TrimSpace(params.GetString("reason"))
			if reason == "" {
				reason = "qr_code"
			}
			callParams := aitool.InvokeParams{
				"reason":  reason,
				"message": params.GetString("message"),
			}
			for _, key := range []string{"tabId", "frameId", "documentId"} {
				if params.Has(key) {
					callParams[key] = params[key]
				}
			}
			if err := catalog.ValidateCapabilityParams("browser.handoff.request", map[string]interface{}(callParams)); err != nil {
				return nil, err
			}
			return callAgentCapability(
				ctx,
				bridge,
				deviceID,
				Target{},
				"browser.handoff.request",
				callParams,
				browserCapabilityTimeout(descriptor),
				false,
				runtimeConfig,
			)
		}),
	); err != nil {
		return nil, err
	}

	return factory.Tools(), nil
}
