package browsertools

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func callDynamicBrowserCapability(
	ctx context.Context,
	bridge Bridge,
	deviceID string,
	browserRef string,
	method string,
	callParams aitool.InvokeParams,
	runtimeConfig *aitool.ToolRuntimeConfig,
) (interface{}, error) {
	instance, err := resolveBrowserDevice(bridge, deviceID, browserRef)
	if err != nil {
		return nil, err
	}
	deviceID = instance.DeviceID
	catalog, connected := bridge.CapabilityCatalog(deviceID)
	if !connected {
		return nil, fmt.Errorf("browser %s is offline or has no signed capability catalog", instance.Reference)
	}
	descriptors, _, err := browserCapabilityDescriptors(catalog)
	if err != nil {
		return nil, err
	}
	descriptor, ok := descriptors[method]
	if !ok {
		return nil, fmt.Errorf("browser capability %q is not declared by browser %q", method, instance.Reference)
	}
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
}

// BuildDynamicCapabilityTools creates browser tools whose device is selected at
// invocation time, either from an explicit @ reference or current live state.
func BuildDynamicCapabilityTools(bridge Bridge) ([]*aitool.Tool, error) {
	factory := aitool.NewFactory()

	if err := factory.RegisterTool(
		"browser.instances.list",
		aitool.WithDescription("List already-open browsers currently connected to this Yak engine through the Yakit browser extension. Use this when the user refers to YTray browsers without an explicit @ mention."),
		aitool.WithVerboseName("Connected Browser Instances"),
		aitool.WithVerboseNameZh("在线浏览器实例"),
		aitool.WithUsage("References such as A, B, and C are stable user-facing selectors. Device IDs are intentionally hidden. With one online instance, other browser tools select it automatically; with several, pass browser_ref for singular operations."),
		aitool.WithKeywords([]string{"browser instances", "YTray", "connected browser", "A/B browser", "浏览器实例", "在线浏览器", "多浏览器"}),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithNoRuntimeCallback(func(
			_ context.Context,
			_ aitool.InvokeParams,
			_ io.Writer,
			_ io.Writer,
		) (interface{}, error) {
			instances := connectedBrowsers(bridge)
			result := make([]map[string]interface{}, 0, len(instances))
			for _, instance := range instances {
				result = append(result, map[string]interface{}{
					"browserRef":        instance.Reference,
					"online":            true,
					"manager":           instance.Manager,
					"client":            instance.Client,
					"capabilityDomains": instance.Domains,
				})
			}
			return result, nil
		}),
	); err != nil {
		return nil, err
	}

	if err := factory.RegisterTool(
		"browser.capability.catalog",
		aitool.WithDescription("List the signed capability catalog of an already-open browser connected through the Yakit browser extension. This does not create or control a Rod browser session."),
		aitool.WithVerboseName("Browser Extension Capability Catalog"),
		aitool.WithVerboseNameZh("浏览器插件能力目录"),
		aitool.WithUsage("An explicit @ attachment is bound by the runtime. Otherwise browser_ref selects an online A/B/C instance, and the sole online instance is selected automatically. Query only the relevant domain before browser.capability.call and follow paramsSchema exactly. For one page cryptography operation, use browser.crypto.inspect directly after browser.context. For encrypted HTTP testing, use browser.transform.prepare and pass validationDraft.id to browser.http.test. Never substitute use_browser/op=open for a connected extension browser."),
		aitool.WithKeywords([]string{"browser", "browser instance", "attached browser", "browser extension", "current website", "open tabs", "capability catalog", "schema", "浏览器", "浏览器实例", "已打开浏览器", "浏览器插件", "当前网站", "标签页", "能力目录", "参数"}),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithStringParam(
			"browser_ref",
			aitool.WithParam_Description("Online browser reference such as A or B; optional when exactly one browser is online"),
			aitool.WithParam_MaxLength(512),
		),
		aitool.WithStringParam(
			"domain",
			aitool.WithParam_Description("Required capability-domain filter; request one domain at a time"),
			aitool.WithParam_EnumString("page", "isolation", "authorization", "network", "recording", "transform", "handoff", "proxy", "system"),
			aitool.WithParam_Required(true),
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
			instance, err := resolveBrowserDevice(
				bridge,
				strings.TrimSpace(params.GetString("device_id")),
				params.GetString("browser_ref"),
			)
			if err != nil {
				return nil, err
			}
			deviceID := instance.DeviceID
			catalog, connected := bridge.CapabilityCatalog(deviceID)
			if !connected {
				return nil, fmt.Errorf("browser %s is offline or has no signed capability catalog", instance.Reference)
			}
			if _, _, err := browserCapabilityDescriptors(catalog); err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"browserRef":    instance.Reference,
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
		aitool.WithDescription("Call a signed capability on an already-open browser connected through the Yakit browser extension. This tool never creates a Rod browser session."),
		aitool.WithVerboseName("Browser Extension Capability"),
		aitool.WithVerboseNameZh("浏览器插件能力"),
		aitool.WithUsage("Call browser.capability.catalog for the relevant domain first. Pass browser_ref when several instances are online; omit it when only one is online. Use browser.tab.open or browser.tabs for navigation. For one page encryption, decryption, signature, or encoding operation, get a fresh browser.context and use browser.crypto.inspect. For encrypted HTTP, use browser.transform.prepare; recording, callable, debugger, and browser.profile.* are internal workflow steps. For QR/MFA/CAPTCHA call browser.handoff.request and wait. Never substitute use_browser/op=open or reopen a page because a dialog appeared."),
		aitool.WithKeywords([]string{"browser", "browser instance", "attached browser", "browser extension", "current website", "open tabs", "page interaction", "network", "debugging", "proxy", "浏览器", "浏览器实例", "已打开浏览器", "浏览器插件", "当前网站", "标签页", "页面操作", "网络", "调试"}),
		aitool.WithStringParam(
			"browser_ref",
			aitool.WithParam_Description("Online browser reference such as A or B; optional when exactly one browser is online"),
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
			deviceID := strings.TrimSpace(params.GetString("device_id"))
			method := strings.TrimSpace(params.GetString("method"))
			callParams := params.GetObject("params")
			return callDynamicBrowserCapability(
				ctx,
				bridge,
				deviceID,
				params.GetString("browser_ref"),
				method,
				callParams,
				runtimeConfig,
			)
		}),
	); err != nil {
		return nil, err
	}

	if err := factory.RegisterTool(
		"browser.crypto.inspect",
		aitool.WithDescription("Inspect one real cryptographic page operation in the attached browser. It atomically records a visible-node click, page crypto/encoding calls, request bodies, and modal messages, then cleans up. It does not create a plaintext gateway."),
		aitool.WithVerboseName("Inspect Browser Crypto Operation"),
		aitool.WithVerboseNameZh("检查页面加解密操作"),
		aitool.WithUsage("First call browser.capability.call with method=browser.context and includeDom=true. Select the exact visible node that triggers the operation, then call this tool once with that captureId and nodeId. Use the returned recording and network evidence to answer the user. Do not manually orchestrate recording/deep-capture/profile tools, and do not create a plaintext gateway unless the user explicitly asks for one."),
		aitool.WithKeywords([]string{"browser crypto", "page encryption", "decrypt", "AES", "RSA", "signature", "页面加密", "页面解密", "加密算法", "签名"}),
		aitool.WithStringParam(
			"browser_ref",
			aitool.WithParam_Description("Online browser reference such as A or B; optional when exactly one browser is online"),
			aitool.WithParam_MaxLength(512),
		),
		aitool.WithStringParam("captureId", aitool.WithParam_Description("captureId returned by browser.context"), aitool.WithParam_MaxLength(160), aitool.WithParam_Required(true)),
		aitool.WithStringParam("nodeId", aitool.WithParam_Description("Visible trigger nodeId from the same browser.context result"), aitool.WithParam_MaxLength(80), aitool.WithParam_Required(true)),
		aitool.WithIntegerParam("settleMs", aitool.WithParam_Description("Maximum time to wait for the operation and request to settle"), aitool.WithParam_Min(250), aitool.WithParam_Max(5_000)),
		aitool.WithIntegerParam("tabId", aitool.WithParam_Description("Target tab ID from browser.context"), aitool.WithParam_Min(1)),
		aitool.WithIntegerParam("frameId", aitool.WithParam_Description("Target frame ID from browser.context"), aitool.WithParam_Min(0)),
		aitool.WithStringParam("documentId", aitool.WithParam_Description("Target document ID from browser.context"), aitool.WithParam_MaxLength(512)),
		aitool.WithCallback(func(
			ctx context.Context,
			params aitool.InvokeParams,
			runtimeConfig *aitool.ToolRuntimeConfig,
			_ io.Writer,
			_ io.Writer,
		) (interface{}, error) {
			callParams := aitool.InvokeParams{
				"captureId": params.GetString("captureId"),
				"nodeId":    params.GetString("nodeId"),
			}
			for _, key := range []string{"settleMs", "tabId", "frameId", "documentId"} {
				if params.Has(key) {
					callParams[key] = params[key]
				}
			}
			return callDynamicBrowserCapability(
				ctx,
				bridge,
				strings.TrimSpace(params.GetString("device_id")),
				params.GetString("browser_ref"),
				"browser.crypto.inspect",
				callParams,
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
			aitool.WithParam_Description("Online browser reference such as A or B; optional when exactly one browser is online"),
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
			instance, err := resolveBrowserDevice(
				bridge,
				strings.TrimSpace(params.GetString("device_id")),
				params.GetString("browser_ref"),
			)
			if err != nil {
				return nil, err
			}
			deviceID := instance.DeviceID
			catalog, connected := bridge.CapabilityCatalog(deviceID)
			if !connected {
				return nil, fmt.Errorf("browser %s is offline or has no signed capability catalog", instance.Reference)
			}
			descriptors, _, err := browserCapabilityDescriptors(catalog)
			if err != nil {
				return nil, err
			}
			descriptor, ok := descriptors["browser.handoff.request"]
			if !ok {
				return nil, fmt.Errorf("browser %q does not support local user handoff", instance.Reference)
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
