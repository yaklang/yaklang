package scannode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/utils"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	"github.com/yaklang/yaklang/scannode/inputresolver"
)

func legionForgeCapabilityOptions(
	ctx context.Context,
	release *aiv1.ContextForgeRelease,
	binding aiSessionBinding,
) ([]aicommon.ConfigOption, *legionServerFocusRuntime, error) {
	switch release.GetCapabilityProfile() {
	case legionForgeAdvisoryProfile:
		return append(restrictedLegionForgeToolOptions(nil),
			aicommon.WithDisableToolUse(true),
			aicommon.WithReActActionPolicy(func(_, action string) bool {
				return action == "directly_answer" || action == "finish"
			}),
		), nil, nil
	case legionForgeReportProfile:
		options, err := legionForgeReportOptions(ctx, release, binding.InputWorkspace)
		return options, nil, err
	case legionForgeHTTPProfile:
		return legionForgeHTTPOptions(ctx, release)
	default:
		return nil, nil, fmt.Errorf("unsupported Forge capability profile %q", release.GetCapabilityProfile())
	}
}

func legionForgeReportOptions(
	ctx context.Context,
	release *aiv1.ContextForgeRelease,
	workspace *inputresolver.Workspace,
) ([]aicommon.ConfigOption, error) {
	resources, err := legionForgeResourcePaths(release)
	if err != nil {
		return nil, err
	}
	if len(resources) > 0 && workspace == nil {
		return nil, fmt.Errorf("report Forge release is missing its managed input workspace")
	}
	if workspace != nil {
		available := make(map[string]struct{})
		for _, file := range workspace.Files() {
			available[file.RelativePath] = struct{}{}
		}
		for _, resource := range resources {
			if _, ok := available[resource]; !ok {
				return nil, fmt.Errorf("report Forge resource binding is unavailable")
			}
		}
	}
	tools := make([]*aitool.Tool, 0, len(legionForgeReportTools))
	for _, name := range legionForgeReportTools {
		name := name
		tool, err := aitool.New(name,
			aitool.WithDescription(legionForgeReportToolDescription(name)),
			aitool.WithStringParam("path"),
			aitool.WithIntegerParam("offset"),
			aitool.WithIntegerParam("max_bytes"),
			aitool.WithIntegerParam("start_line"),
			aitool.WithIntegerParam("end_line"),
			aitool.WithNoRuntimeCallback(func(callCtx context.Context, params aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
				if workspace == nil {
					return nil, fmt.Errorf("managed input workspace is unavailable")
				}
				if callCtx.Err() != nil || ctx.Err() != nil {
					return nil, context.Canceled
				}
				path := strings.TrimSpace(utils.InterfaceToString(params["path"]))
				switch name {
				case "query_file_meta":
					return workspace.List(callCtx, path)
				case "read_file":
					return workspace.Read(callCtx, path, int64(utils.InterfaceToInt(params["offset"])), int64(utils.InterfaceToInt(params["max_bytes"])))
				case "read_file_lines":
					return workspace.ReadLines(callCtx, path, utils.InterfaceToInt(params["start_line"]), utils.InterfaceToInt(params["end_line"]))
				case "parse_office_to_text":
					return workspace.ExtractOfficeText(callCtx, path)
				default:
					return nil, fmt.Errorf("unsupported managed report tool")
				}
			}),
		)
		if err != nil {
			return nil, err
		}
		tools = append(tools, tool)
	}
	return restrictedLegionForgeToolOptions(tools), nil
}

func legionForgeHTTPOptions(ctx context.Context, release *aiv1.ContextForgeRelease) ([]aicommon.ConfigOption, *legionServerFocusRuntime, error) {
	target := ""
	for _, parameter := range release.GetParameters() {
		if parameter.GetKey() == "target-url" && parameter.GetValueKind() == "string" {
			target = strings.TrimSpace(parameter.GetValue())
		}
	}
	runtime, err := newLegionForgeHTTPRuntime(ctx, target, defaultLegionForgeLookupIP)
	if err != nil {
		return nil, nil, fmt.Errorf("HTTP Forge target: %w", err)
	}
	tools := make([]*aitool.Tool, 0, len(legionForgeHTTPTools))
	for _, name := range legionForgeHTTPTools {
		name := name
		tool, err := aitool.New(name,
			aitool.WithDescription(legionForgeHTTPToolDescription(name)),
			aitool.WithStringParam("url"),
			aitool.WithStringParam("method"),
			aitool.WithNoRuntimeCallback(func(callCtx context.Context, params aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
				if callCtx.Err() != nil || ctx.Err() != nil {
					return nil, context.Canceled
				}
				request := map[string]any(params)
				if request["url"] == nil || strings.TrimSpace(utils.InterfaceToString(request["url"])) == "" {
					request["url"] = target
				}
				result, err := runtime.executeHTTPRequestContext(callCtx, request)
				if err != nil {
					return nil, err
				}
				if name == "simple_crawler" {
					refs, extractErr := runtime.extractReferences(map[string]any{"base_url": result["url"], "document": result["body"]})
					if extractErr != nil {
						return nil, extractErr
					}
					result["references"] = refs
				}
				if name == "web_fingerprint" {
					delete(result, "body")
				}
				return result, nil
			}),
		)
		if err != nil {
			return nil, nil, err
		}
		tools = append(tools, tool)
	}
	return restrictedLegionForgeToolOptions(tools), runtime, nil
}

func restrictedLegionForgeToolOptions(tools []*aitool.Tool) []aicommon.ConfigOption {
	manager := buildinaitools.NewToolManagerByToolGetter(
		func() []*aitool.Tool { return tools },
		buildinaitools.WithOnlyTools(tools...),
	)
	return []aicommon.ConfigOption{
		// Immutable applications already pin their plan and capability set.
		// Host skills and generic capability recommendation are not part of
		// that release and must not be discovered during its execution.
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableIntentRecognition(true),
		aicommon.WithDisableMemoryTriage(true),
		aicommon.WithDisableToolUse(false),
		aicommon.WithAiToolManager(manager),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithShowForgeListInPrompt(false),
		aicommon.WithReActActionPolicy(managedInputActionAllowed),
	}
}

func legionForgeResourcePaths(release *aiv1.ContextForgeRelease) ([]string, error) {
	paths := make([]string, 0)
	for _, parameter := range release.GetParameters() {
		if parameter.GetValueKind() != "resource" {
			continue
		}
		value := strings.TrimSpace(parameter.GetValue())
		if strings.HasPrefix(value, "[") {
			var values []string
			if json.Unmarshal([]byte(value), &values) != nil || len(values) == 0 {
				return nil, fmt.Errorf("report Forge resource binding is invalid")
			}
			paths = append(paths, values...)
			continue
		}
		paths = append(paths, value)
	}
	for _, path := range paths {
		if !inputresolver.SafeInputPath(path) {
			return nil, fmt.Errorf("report Forge resource binding is not a managed path")
		}
	}
	return paths, nil
}

func legionForgeReportToolDescription(name string) string {
	switch name {
	case "query_file_meta":
		return "List only the authorized run-local input metadata and logical paths."
	case "read_file", "read_file_lines":
		return "Read bounded UTF-8 content from an authorized run-local input path."
	case "parse_office_to_text":
		return "Extract bounded text from an authorized XLSX, UTF-8 text, or JSON input without macros or external links."
	default:
		return "Use a server-managed report input capability."
	}
}

func legionForgeHTTPToolDescription(name string) string {
	switch name {
	case "simple_crawler":
		return "Fetch one authorized page with GET or HEAD and extract bounded same-origin references."
	case "web_fingerprint":
		return "Inspect bounded response metadata for the authorized target origin."
	default:
		return "Send a bounded GET or HEAD request only to the authorized target origin."
	}
}
