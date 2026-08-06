package browsertools

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/browser"
)

func browserCapabilityCatalog(
	catalog *browser.ExtensionBridgeCapabilityCatalog,
	domain string,
	query string,
) []browser.ExtensionBridgeCapabilityDescriptor {
	domain = strings.ToLower(strings.TrimSpace(domain))
	query = strings.ToLower(strings.TrimSpace(query))
	result := make([]browser.ExtensionBridgeCapabilityDescriptor, 0, len(catalog.Capabilities))
	for _, descriptor := range catalog.Capabilities {
		if domain != "" && domain != "all" && descriptor.Domain != domain {
			continue
		}
		searchable := strings.ToLower(strings.Join([]string{
			descriptor.Method,
			descriptor.Domain,
			descriptor.Access,
			descriptor.Summary,
			strings.Join(descriptor.Scopes, " "),
			string(descriptor.ParamsSchema),
		}, " "))
		if query != "" && !strings.Contains(searchable, query) {
			continue
		}
		result = append(result, descriptor)
	}
	return result
}

func browserCapabilityDescriptors(
	catalog *browser.ExtensionBridgeCapabilityCatalog,
) (map[string]browser.ExtensionBridgeCapabilityDescriptor, []string, error) {
	if catalog == nil || strings.TrimSpace(catalog.Hash) == "" || len(catalog.Capabilities) == 0 {
		return nil, nil, fmt.Errorf("browser extension does not advertise a capability catalog")
	}
	descriptors := make(map[string]browser.ExtensionBridgeCapabilityDescriptor, len(catalog.Capabilities))
	methods := make([]string, 0, len(catalog.Capabilities))
	for _, descriptor := range catalog.Capabilities {
		method := strings.TrimSpace(descriptor.Method)
		if method == "" {
			return nil, nil, fmt.Errorf("browser extension capability catalog contains an empty method")
		}
		if _, duplicate := descriptors[method]; duplicate {
			return nil, nil, fmt.Errorf("browser extension capability catalog contains duplicate method %q", method)
		}
		descriptors[method] = descriptor
		methods = append(methods, method)
	}
	sort.Strings(methods)
	return descriptors, methods, nil
}

func augmentBrowserCapabilityTarget(
	descriptor browser.ExtensionBridgeCapabilityDescriptor,
	params aitool.InvokeParams,
	target Target,
) {
	switch descriptor.TargetMode {
	case "tab":
		if !params.Has("tabId") {
			params["tabId"] = target.TabID
		}
	case "document":
		hasTab := params.Has("tabId")
		hasFrame := params.Has("frameId")
		hasDocument := params.Has("documentId")
		if !hasTab && !hasFrame && !hasDocument {
			for key, value := range target.Params() {
				params[key] = value
			}
			return
		}
		if !hasTab {
			params["tabId"] = target.TabID
		}
		if params.GetInteger("tabId") != target.TabID {
			return
		}
		if !hasFrame {
			params["frameId"] = target.FrameID
		}
		if params.GetInteger("frameId") != target.FrameID {
			return
		}
		if !hasDocument && target.DocumentID != "" {
			params["documentId"] = target.DocumentID
		}
	}
}

func browserCapabilityTimeout(
	descriptor browser.ExtensionBridgeCapabilityDescriptor,
) time.Duration {
	timeout := time.Duration(descriptor.DefaultTimeoutMS) * time.Millisecond
	if timeout < 250*time.Millisecond {
		return ReadTimeout
	}
	if timeout > 120*time.Second {
		return 120 * time.Second
	}
	return timeout
}

func RegisterCapabilityTools(
	factory *aitool.ToolFactory,
	caller Caller,
	deviceID string,
	target Target,
	catalog *browser.ExtensionBridgeCapabilityCatalog,
) error {
	descriptors, methods, err := browserCapabilityDescriptors(catalog)
	if err != nil {
		return err
	}

	if err := factory.RegisterTool(
		"browser.capability.catalog",
		aitool.WithDescription("List every capability and parameter schema declared by the connected browser extension. This tool does not read page data."),
		aitool.WithUsage("Query this catalog before using browser capabilities outside the dedicated cryptography workflow, such as page interaction, network capture, Deep Capture, Eval, proxy, or Profile management."),
		aitool.WithKeywords([]string{"browser", "capability catalog", "schema", "debugging", "review", "浏览器", "能力目录", "参数", "调试", "权限"}),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithStringParam(
			"domain",
			aitool.WithParam_Description("Optional capability-domain filter"),
			aitool.WithParam_EnumString("all", "page", "isolation", "authorization", "network", "recording", "callable", "debugger", "transform", "handoff", "proxy", "system"),
		),
		aitool.WithStringParam(
			"query",
			aitool.WithParam_Description("Optional keyword filter, for example eval, cookie, capture, or proxy"),
			aitool.WithParam_MaxLength(120),
		),
		aitool.WithNoRuntimeCallback(func(
			_ context.Context,
			params aitool.InvokeParams,
			_ io.Writer,
			_ io.Writer,
		) (interface{}, error) {
			return map[string]interface{}{
				"defaultTarget": target.Params(),
				"schemaVersion": catalog.Version,
				"schemaHash":    catalog.Hash,
				"schemaDialect": catalog.SchemaDialect,
				"reviewPolicy":  "Each capability.call enters the current AI session's manual, ai, or yolo Review flow and remains constrained by the extension grant scope",
				"capabilities": browserCapabilityCatalog(
					catalog,
					params.GetString("domain"),
					params.GetString("query"),
				),
			}, nil
		}),
	); err != nil {
		return err
	}

	return factory.RegisterTool(
		"browser.capability.call",
		aitool.WithDescription("Call any capability declared by the connected browser extension. Parameters are checked against that extension version's signed schema before dispatch. The extension grant, target, origin, document, and scope remain the final authority."),
		aitool.WithUsage("Use browser.capability.catalog first and construct params from the selected descriptor's paramsSchema. This tool can inspect or create browser identity-isolation contexts, interact with pages, read authorized Cookie or context data, capture traffic, control recording and Deep Capture, run invoke or eval, manage callables and Profiles, or switch proxies."),
		aitool.WithKeywords([]string{"browser", "identity isolation", "page interaction", "network", "debugging", "eval", "proxy", "浏览器", "身份隔离", "页面操作", "网络", "调试", "代理"}),
		aitool.WithStringParam(
			"method",
			aitool.WithParam_Description("Bridge capability declared by the connected extension schema"),
			aitool.WithParam_EnumString(methods...),
			aitool.WithParam_Required(true),
		),
		aitool.WithRawParam(
			"params",
			map[string]any{
				"type":                 "object",
				"additionalProperties": true,
				"description":          "Parameters for the selected method; use the exact paramsSchema returned by browser.capability.catalog",
			},
		),
		aitool.WithNoRuntimeCallback(func(
			ctx context.Context,
			params aitool.InvokeParams,
			_ io.Writer,
			_ io.Writer,
		) (interface{}, error) {
			method := strings.TrimSpace(params.GetString("method"))
			descriptor, ok := descriptors[method]
			if !ok {
				return nil, fmt.Errorf("browser capability %q is not declared by the selected extension schema", method)
			}
			callParams := params.GetObject("params")
			augmentBrowserCapabilityTarget(descriptor, callParams, target)
			if err := catalog.ValidateCapabilityParams(
				method,
				map[string]interface{}(callParams),
			); err != nil {
				return nil, err
			}
			return CallCapability(
				ctx,
				caller,
				deviceID,
				target,
				method,
				callParams,
				browserCapabilityTimeout(descriptor),
				false,
			)
		}),
	)
}
