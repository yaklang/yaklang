package yak

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools/metadata"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/yakcliconvert"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	yakscripttools.RegisterYakScriptPluginConvertHandle(ConvertYakScriptToAITool)
}

func ConvertYakScriptToAITool(script *schema.YakScript) (*aitool.Tool, error) {
	if script == nil {
		return nil, utils.Error("nil YakScript")
	}
	switch script.Type {
	case "yak":
		return convertNativeYakPluginToAITool(script)
	case "mitm":
		return convertMitmPluginToAITool(script)
	case "port-scan":
		return convertPortScanPluginToAITool(script)
	default:
		return nil, utils.Errorf("unsupported plugin type %q for AI conversion", script.Type)
	}
}

func yakScriptDesc(script *schema.YakScript) string {
	if script.AIDesc != "" {
		return script.AIDesc
	}
	return script.Help
}

func yakScriptKeywords(script *schema.YakScript) []string {
	if script.AIKeywords != "" {
		return strings.Split(script.AIKeywords, ",")
	}
	if script.Tags != "" {
		return strings.Split(script.Tags, ",")
	}
	return nil
}

// yakScriptUsage returns the usage text for secondary disclosure.
// Priority: schema.AIUsage > SSA-parsed __USAGE__ > fallback
func yakScriptUsage(script *schema.YakScript, fallback string) string {
	if script.AIUsage != "" {
		return script.AIUsage
	}
	return fallback
}

// convertNativeYakPluginToAITool converts a native yak plugin to an AI tool.
// It uses SSA to parse cli.* parameters from the script content and extracts
// metadata (__DESC__, __KEYWORDS__, __USAGE__) for AI tool configuration.
// Schema fields take priority over script-defined metadata.
func convertNativeYakPluginToAITool(script *schema.YakScript) (*aitool.Tool, error) {
	var mcpTool *mcp.Tool
	var desc, usage string
	var keywords []string

	prog, err := static_analyzer.SSAParse(script.Content, "yak")
	if err != nil {
		log.Warnf("SSA parse native yak plugin %q failed, using fallback params: %v", script.ScriptName, err)
		mcpTool = mcp.NewTool(script.ScriptName)
	} else {
		mcpTool = yakcliconvert.ConvertCliParameterToTool(script.ScriptName, prog)
		// Parse metadata once and use all fields
		meta, metaErr := metadata.ParseYakScriptMetadataProg(script.ScriptName, prog)
		if metaErr == nil {
			// Use script metadata as base, schema fields will override if set
			desc = meta.Description
			keywords = meta.Keywords
			usage = meta.Usage
		}
	}

	// Schema fields take priority over script-defined metadata
	if script.AIDesc != "" {
		desc = script.AIDesc
	} else if script.Help != "" && desc == "" {
		desc = script.Help
	}

	if script.AIKeywords != "" {
		keywords = strings.Split(script.AIKeywords, ",")
	} else if script.Tags != "" && len(keywords) == 0 {
		keywords = strings.Split(script.Tags, ",")
	}

	if script.AIUsage != "" {
		usage = script.AIUsage
	}

	mcpTool.Description = fmt.Sprintf("[Yakit Native Plugin] %s", desc)

	tool, err := aitool.NewFromMCPTool(
		mcpTool,
		aitool.WithKeywords(keywords),
		aitool.WithUsage(usage),
		aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, runtimeConfig *aitool.ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error) {
			return executeNativeYakPlugin(ctx, script, params, runtimeConfig, stdout, stderr)
		}),
	)
	if err != nil {
		return nil, utils.Errorf("create native yak plugin AI tool %q: %v", script.ScriptName, err)
	}
	return tool, nil
}

func executeNativeYakPlugin(ctx context.Context, script *schema.YakScript, params aitool.InvokeParams, runtimeConfig *aitool.ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var runtimeId string
	var runtimeFeedBacker func(result *ypb.ExecResult) error
	if runtimeConfig != nil {
		runtimeId = runtimeConfig.RuntimeID
		runtimeFeedBacker = runtimeConfig.FeedBacker
	}

	yakitClient := yaklib.NewVirtualYakitClientWithRuntimeID(func(result *ypb.ExecResult) error {
		if ret := yaklib.ConvertExecResultIntoAIToolCallStdoutLog(result); ret != "" {
			stdout.Write([]byte(ret))
			stdout.Write([]byte("\n"))
		}
		if runtimeFeedBacker != nil {
			return runtimeFeedBacker(result)
		}
		return nil
	}, runtimeId)

	engine := NewYakitVirtualClientScriptEngine(yakitClient)

	stdout.Write([]byte(fmt.Sprintf("[info] executing native yak plugin: %s\n", script.ScriptName)))
	var paramDesc []string
	var args []string
	for k, v := range params {
		switch ret := v.(type) {
		case bool:
			if ret {
				args = append(args, "--"+k)
				paramDesc = append(paramDesc, fmt.Sprintf("%s=true", k))
			}
		default:
			args = append(args, "--"+k, fmt.Sprint(ret))
			valStr := fmt.Sprint(ret)
			if len(valStr) > 80 {
				valStr = valStr[:80] + "..."
			}
			paramDesc = append(paramDesc, fmt.Sprintf("%s=%q", k, valStr))
		}
	}
	if len(paramDesc) > 0 {
		stdout.Write([]byte(fmt.Sprintf("[info] parameters: %s\n", strings.Join(paramDesc, ", "))))
	}

	cliApp := GetHookCliApp(args)
	engine.RegisterEngineHooks(func(ae *antlr4yak.Engine) error {
		BindYakitPluginContextToEngine(
			ae,
			CreateYakitPluginContext(runtimeId).
				WithContext(ctx).
				WithContextCancel(cancel).
				WithCliApp(cliApp).
				WithYakitClient(yakitClient),
		)
		return nil
	})

	_, err := engine.ExecuteExWithContext(ctx, script.Content, map[string]interface{}{
		"RUNTIME_ID":   runtimeId,
		"CTX":          ctx,
		"PLUGIN_NAME":  script.ScriptName + ".yak",
		"YAK_FILENAME": script.ScriptName + ".yak",
	})
	if err != nil {
		log.Errorf("execute native yak plugin %q failed: %v", script.ScriptName, err)
		stderr.Write([]byte(err.Error()))
		return nil, err
	}

	return collectNativePluginResult(runtimeId, script.ScriptName, stdout), nil
}

func collectNativePluginResult(runtimeId string, pluginName string, stdout io.Writer) string {
	var result strings.Builder
	result.WriteString(fmt.Sprintf("Native yak plugin execution completed: %s\n", pluginName))

	if runtimeId == "" {
		return result.String()
	}

	db := consts.GetGormProjectDatabase()
	if db == nil {
		return result.String()
	}

	risks, err := yakit.GetRisksByRuntimeId(db, runtimeId)
	if err != nil {
		log.Warnf("query risks by runtimeId %q failed: %v", runtimeId, err)
		return result.String()
	}

	if len(risks) == 0 {
		return result.String()
	}

	result.WriteString(fmt.Sprintf("\n=== Vulnerabilities Found: %d ===\n", len(risks)))
	stdout.Write([]byte(fmt.Sprintf("[info] found %d vulnerability(ies) from plugin execution\n", len(risks))))
	for i, r := range risks {
		severity := r.Severity
		if severity == "" {
			severity = "info"
		}
		title := r.Title
		if title == "" {
			title = r.TitleVerbose
		}
		riskLine := fmt.Sprintf("[%d] [%s] %s", i+1, strings.ToUpper(severity), title)
		if r.Url != "" {
			riskLine += fmt.Sprintf(" | URL: %s", r.Url)
		}
		result.WriteString(riskLine + "\n")

		if r.RiskType != "" {
			result.WriteString(fmt.Sprintf("    Type: %s", r.RiskType))
			if r.RiskTypeVerbose != "" {
				result.WriteString(fmt.Sprintf(" (%s)", r.RiskTypeVerbose))
			}
			result.WriteString("\n")
		}
		if r.Payload != "" {
			payload := r.Payload
			if len(payload) > 200 {
				payload = payload[:200] + "..."
			}
			result.WriteString(fmt.Sprintf("    Payload: %s\n", payload))
		}
	}
	result.WriteString("=== End of Vulnerability Report ===\n")

	return result.String()
}

const defaultMitmUsage = `MITM Plugin Usage Guide for AI Parameter Generation

This is a Yakit MITM (Man-In-The-Middle) plugin that analyzes HTTP traffic for security vulnerabilities.

## Parameter Selection Rules
1. If you have a raw HTTP request packet, provide it via "requestPacket" (highest priority).
   Set "isHttps" to "true" if the target uses HTTPS.
2. If you only have a URL, provide it via "url". A GET request will be auto-generated.
   HTTPS will be auto-detected from the URL scheme.
3. At least one of "url" or "requestPacket" must be provided.

## Parameter Details
- url: Target URL (e.g. "https://example.com/path?param=value"). Used when requestPacket is empty.
- requestPacket: Raw HTTP request packet as a string, including request line, headers, and optional body.
  Example: "GET /path HTTP/1.1\r\nHost: example.com\r\n\r\n"
- isHttps: "true" or "false". Only needed when providing requestPacket; auto-detected for url.

## Important Notes
- The plugin sends the request and analyzes both request and response for vulnerabilities.
- Results are reported via the plugin's built-in reporting mechanism.
- This plugin is designed for security testing; only use against authorized targets.`

func convertMitmPluginToAITool(script *schema.YakScript) (*aitool.Tool, error) {
	desc := yakScriptDesc(script)
	keywords := yakScriptKeywords(script)
	usage := yakScriptUsage(script, defaultMitmUsage)

	tool, err := aitool.New(
		script.ScriptName,
		aitool.WithDescription(fmt.Sprintf("[Yakit MITM Plugin] %s", desc)),
		aitool.WithKeywords(keywords),
		aitool.WithUsage(usage),
		aitool.WithStringParam("url",
			aitool.WithParam_Description("Target URL. If requestPacket is empty, a GET request will be generated from this URL."),
			aitool.WithParam_Required(false),
		),
		aitool.WithStringParam("requestPacket",
			aitool.WithParam_Description("Raw HTTP request packet to send and analyze. Takes priority over url."),
			aitool.WithParam_Required(false),
		),
		aitool.WithStringParam("isHttps",
			aitool.WithParam_Description("Whether the target uses HTTPS. Use 'true' or 'false'."),
			aitool.WithParam_Required(false),
		),
		aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, runtimeConfig *aitool.ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error) {
			return executeMitmPlugins(ctx, []*schema.YakScript{script}, params, runtimeConfig, stdout, stderr)
		}),
	)
	if err != nil {
		return nil, utils.Errorf("create mitm plugin AI tool %q: %v", script.ScriptName, err)
	}
	return tool, nil
}

func executeMitmPlugins(ctx context.Context, scripts []*schema.YakScript, params aitool.InvokeParams, runtimeConfig *aitool.ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error) {
	urlStr, _ := params["url"].(string)
	reqPacket, _ := params["requestPacket"].(string)
	isHttpsStr, _ := params["isHttps"].(string)
	isHttps := isHttpsStr == "true"

	if reqPacket == "" && urlStr == "" {
		return nil, utils.Error("at least one of 'url' or 'requestPacket' must be provided")
	}

	if reqPacket == "" && urlStr != "" {
		detectedHttps, raw, err := lowhttp.ParseUrlToHttpRequestRaw("GET", urlStr)
		if err != nil {
			return nil, utils.Errorf("parse URL to HTTP request failed: %v", err)
		}
		reqPacket = string(raw)
		isHttps = detectedHttps
	}

	var runtimeId string
	if runtimeConfig != nil {
		runtimeId = runtimeConfig.RuntimeID
	}

	manager, err := NewMixPluginCaller()
	if err != nil {
		return nil, utils.Errorf("create MixPluginCaller failed: %v", err)
	}
	if runtimeId != "" {
		manager.SetRuntimeId(runtimeId)
	}

	manager.SetFeedback(func(result *ypb.ExecResult) error {
		if ret := yaklib.ConvertExecResultIntoAIToolCallStdoutLog(result); ret != "" {
			stdout.Write([]byte(ret))
			stdout.Write([]byte("\n"))
		}
		if runtimeConfig != nil && runtimeConfig.FeedBacker != nil {
			return runtimeConfig.FeedBacker(result)
		}
		return nil
	})

	var pluginNames []string
	var loadWg sync.WaitGroup
	var loadErrs []string
	for _, script := range scripts {
		pluginNames = append(pluginNames, script.ScriptName)
		loadWg.Add(1)
		go func(s *schema.YakScript) {
			defer loadWg.Done()
			if loadErr := manager.LoadPluginEx(ctx, s); loadErr != nil {
				loadErrs = append(loadErrs, fmt.Sprintf("load plugin %q failed: %v", s.ScriptName, loadErr))
				log.Warnf("load mitm plugin %q for AI failed: %v", s.ScriptName, loadErr)
			}
		}(script)
	}
	loadWg.Wait()

	if len(loadErrs) > 0 && len(loadErrs) == len(scripts) {
		return nil, utils.Errorf("all plugins failed to load: %s", strings.Join(loadErrs, "; "))
	}

	scheme := "http"
	if isHttps {
		scheme = "https"
	}
	targetURL := lowhttp.GetUrlFromHTTPRequest(scheme, []byte(reqPacket))
	stdout.Write([]byte(fmt.Sprintf("[info] loaded %d plugin(s): %s\n", len(pluginNames)-len(loadErrs), strings.Join(pluginNames, ", "))))
	stdout.Write([]byte(fmt.Sprintf("[info] sending HTTP request to: %s (https=%v)\n", targetURL, isHttps)))

	var pocOpts []poc.PocConfigOption
	if isHttps {
		pocOpts = append(pocOpts, poc.WithForceHTTPS(true))
	}
	rspBytes, reqBytes, err := poc.HTTP(reqPacket, pocOpts...)
	if err != nil {
		stderr.Write([]byte(fmt.Sprintf("HTTP request failed: %v\n", err)))
		return nil, utils.Errorf("send HTTP request failed: %v", err)
	}

	statusCode := lowhttp.ExtractStatusCodeFromResponse(rspBytes)
	body := lowhttp.GetHTTPPacketBody(rspBytes)
	urlForMirror := lowhttp.GetUrlFromHTTPRequest(scheme, reqBytes)
	stdout.Write([]byte(fmt.Sprintf("[info] HTTP response: status=%d, body_length=%d\n", statusCode, len(body))))
	stdout.Write([]byte(fmt.Sprintf("[info] analyzing HTTP flow for vulnerabilities via plugin(s)...\n")))

	manager.MirrorHTTPFlow(isHttps, urlForMirror, reqBytes, rspBytes, body)
	manager.GetNativeCaller().Wait()

	return collectMitmPluginResult(runtimeId, pluginNames, urlForMirror, stdout), nil
}

func collectMitmPluginResult(runtimeId string, pluginNames []string, targetURL string, stdout io.Writer) string {
	var result strings.Builder
	result.WriteString(fmt.Sprintf("MITM plugin execution completed for target: %s\n", targetURL))
	result.WriteString(fmt.Sprintf("Plugins executed: %s\n", strings.Join(pluginNames, ", ")))

	if runtimeId == "" {
		result.WriteString("No risks collected (runtimeId unavailable)\n")
		return result.String()
	}

	db := consts.GetGormProjectDatabase()
	if db == nil {
		result.WriteString("No risks collected (database unavailable)\n")
		return result.String()
	}

	risks, err := yakit.GetRisksByRuntimeId(db, runtimeId)
	if err != nil {
		log.Warnf("query risks by runtimeId %q failed: %v", runtimeId, err)
		result.WriteString("No risks collected (query failed)\n")
		return result.String()
	}

	if len(risks) == 0 {
		result.WriteString("Scan completed: no vulnerabilities found.\n")
		stdout.Write([]byte("[info] scan completed: no vulnerabilities found\n"))
		return result.String()
	}

	result.WriteString(fmt.Sprintf("\n=== Vulnerabilities Found: %d ===\n", len(risks)))
	stdout.Write([]byte(fmt.Sprintf("[info] scan completed: found %d vulnerability(ies)\n", len(risks))))
	for i, r := range risks {
		severity := r.Severity
		if severity == "" {
			severity = "info"
		}
		title := r.Title
		if title == "" {
			title = r.TitleVerbose
		}
		riskLine := fmt.Sprintf("[%d] [%s] %s", i+1, strings.ToUpper(severity), title)
		if r.Url != "" {
			riskLine += fmt.Sprintf(" | URL: %s", r.Url)
		}
		result.WriteString(riskLine + "\n")
		stdout.Write([]byte(fmt.Sprintf("[risk] %s\n", riskLine)))

		if r.RiskType != "" {
			result.WriteString(fmt.Sprintf("    Type: %s", r.RiskType))
			if r.RiskTypeVerbose != "" {
				result.WriteString(fmt.Sprintf(" (%s)", r.RiskTypeVerbose))
			}
			result.WriteString("\n")
		}
		if r.Payload != "" {
			payload := r.Payload
			if len(payload) > 200 {
				payload = payload[:200] + "..."
			}
			result.WriteString(fmt.Sprintf("    Payload: %s\n", payload))
		}
	}
	result.WriteString("=== End of Vulnerability Report ===\n")

	return result.String()
}

const defaultPortScanUsage = `Port-Scan Plugin Usage Guide for AI Parameter Generation

This is a Yakit Port-Scan plugin that analyzes service scan results for security vulnerabilities.

## Parameter Details
- target: Target host IP address or domain name (e.g. "192.168.1.1" or "example.com"). Required.
- port: Target port number as a string (e.g. "80", "443", "8080"). Required.

## How It Works
1. The plugin receives a target:port pair representing a discovered open port.
2. It analyzes the service running on that port for known vulnerabilities.
3. Results (risks/vulnerabilities) are reported via the plugin's built-in reporting mechanism.

## Important Notes
- Both "target" and "port" are required parameters.
- The plugin simulates receiving a port scan result with state=OPEN.
- Only use against authorized targets for security testing.`

func convertPortScanPluginToAITool(script *schema.YakScript) (*aitool.Tool, error) {
	desc := yakScriptDesc(script)
	keywords := yakScriptKeywords(script)
	usage := yakScriptUsage(script, defaultPortScanUsage)

	tool, err := aitool.New(
		script.ScriptName,
		aitool.WithDescription(fmt.Sprintf("[Yakit Port-Scan Plugin] %s", desc)),
		aitool.WithKeywords(keywords),
		aitool.WithUsage(usage),
		aitool.WithStringParam("target",
			aitool.WithParam_Description("Target host IP or domain."),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam("port",
			aitool.WithParam_Description("Target port number."),
			aitool.WithParam_Required(true),
		),
		aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, runtimeConfig *aitool.ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error) {
			return executePortScanPlugins(ctx, []*schema.YakScript{script}, params, runtimeConfig, stdout, stderr)
		}),
	)
	if err != nil {
		return nil, utils.Errorf("create port-scan plugin AI tool %q: %v", script.ScriptName, err)
	}
	return tool, nil
}

func executePortScanPlugins(ctx context.Context, scripts []*schema.YakScript, params aitool.InvokeParams, runtimeConfig *aitool.ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error) {
	target, _ := params["target"].(string)
	portStr, _ := params["port"].(string)

	if target == "" {
		return nil, utils.Error("target is required for port-scan plugin")
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, utils.Errorf("invalid port %q: %v", portStr, err)
	}

	var runtimeId string
	if runtimeConfig != nil {
		runtimeId = runtimeConfig.RuntimeID
	}

	manager, err := NewMixPluginCaller()
	if err != nil {
		return nil, utils.Errorf("create MixPluginCaller failed: %v", err)
	}
	if runtimeId != "" {
		manager.SetRuntimeId(runtimeId)
	}

	manager.SetFeedback(func(result *ypb.ExecResult) error {
		if ret := yaklib.ConvertExecResultIntoAIToolCallStdoutLog(result); ret != "" {
			stdout.Write([]byte(ret))
			stdout.Write([]byte("\n"))
		}
		if runtimeConfig != nil && runtimeConfig.FeedBacker != nil {
			return runtimeConfig.FeedBacker(result)
		}
		return nil
	})

	var pluginNames []string
	var loadWg sync.WaitGroup
	var loadErrs []string
	for _, script := range scripts {
		pluginNames = append(pluginNames, script.ScriptName)
		loadWg.Add(1)
		go func(s *schema.YakScript) {
			defer loadWg.Done()
			if loadErr := manager.LoadPluginEx(ctx, s); loadErr != nil {
				loadErrs = append(loadErrs, fmt.Sprintf("load plugin %q failed: %v", s.ScriptName, loadErr))
				log.Warnf("load port-scan plugin %q for AI failed: %v", s.ScriptName, loadErr)
			}
		}(script)
	}
	loadWg.Wait()

	if len(loadErrs) > 0 && len(loadErrs) == len(scripts) {
		return nil, utils.Errorf("all plugins failed to load: %s", strings.Join(loadErrs, "; "))
	}

	stdout.Write([]byte(fmt.Sprintf("[info] loaded %d plugin(s): %s\n", len(pluginNames)-len(loadErrs), strings.Join(pluginNames, ", "))))
	stdout.Write([]byte(fmt.Sprintf("[info] scanning target %s:%d (state=OPEN)\n", target, port)))

	matchResult := &fp.MatchResult{
		Target: target,
		Port:   port,
		State:  fp.OPEN,
	}

	manager.HandleServiceScanResult(matchResult)
	manager.GetNativeCaller().Wait()

	return collectPortScanPluginResult(runtimeId, pluginNames, target, port, stdout), nil
}

func collectPortScanPluginResult(runtimeId string, pluginNames []string, target string, port int, stdout io.Writer) string {
	var result strings.Builder
	result.WriteString(fmt.Sprintf("Port-scan plugin execution completed for target: %s:%d\n", target, port))
	result.WriteString(fmt.Sprintf("Plugins executed: %s\n", strings.Join(pluginNames, ", ")))

	if runtimeId == "" {
		result.WriteString("No risks collected (runtimeId unavailable)\n")
		return result.String()
	}

	db := consts.GetGormProjectDatabase()
	if db == nil {
		result.WriteString("No risks collected (database unavailable)\n")
		return result.String()
	}

	risks, err := yakit.GetRisksByRuntimeId(db, runtimeId)
	if err != nil {
		log.Warnf("query risks by runtimeId %q failed: %v", runtimeId, err)
		result.WriteString("No risks collected (query failed)\n")
		return result.String()
	}

	if len(risks) == 0 {
		result.WriteString("Scan completed: no vulnerabilities found.\n")
		stdout.Write([]byte("[info] scan completed: no vulnerabilities found\n"))
		return result.String()
	}

	result.WriteString(fmt.Sprintf("\n=== Vulnerabilities Found: %d ===\n", len(risks)))
	stdout.Write([]byte(fmt.Sprintf("[info] scan completed: found %d vulnerability(ies)\n", len(risks))))
	for i, r := range risks {
		severity := r.Severity
		if severity == "" {
			severity = "info"
		}
		title := r.Title
		if title == "" {
			title = r.TitleVerbose
		}
		riskLine := fmt.Sprintf("[%d] [%s] %s", i+1, strings.ToUpper(severity), title)
		if r.Url != "" {
			riskLine += fmt.Sprintf(" | URL: %s", r.Url)
		}
		result.WriteString(riskLine + "\n")
		stdout.Write([]byte(fmt.Sprintf("[risk] %s\n", riskLine)))

		if r.RiskType != "" {
			result.WriteString(fmt.Sprintf("    Type: %s", r.RiskType))
			if r.RiskTypeVerbose != "" {
				result.WriteString(fmt.Sprintf(" (%s)", r.RiskTypeVerbose))
			}
			result.WriteString("\n")
		}
		if r.Payload != "" {
			payload := r.Payload
			if len(payload) > 200 {
				payload = payload[:200] + "..."
			}
			result.WriteString(fmt.Sprintf("    Payload: %s\n", payload))
		}
	}
	result.WriteString("=== End of Vulnerability Report ===\n")

	return result.String()
}

// dumpToolParamsJSON is a helper for debugging/testing the tool's input schema.
func dumpToolParamsJSON(tool *aitool.Tool) string {
	if tool == nil || tool.Tool == nil {
		return "{}"
	}
	m := tool.InputSchema.ToMap()
	bs, _ := json.Marshal(m)
	return string(bs)
}
