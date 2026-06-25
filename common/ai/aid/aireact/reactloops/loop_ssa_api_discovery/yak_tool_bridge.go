package loop_ssa_api_discovery

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func appendSessionDBToolParams(params aitool.InvokeParams, rt *Runtime) {
	if rt == nil || params == nil {
		return
	}
	if dsn := strings.TrimSpace(rt.SessionDBDSN); dsn != "" {
		params["db-dsn"] = dsn
		delete(params, "sqlite-path")
	} else if rt.SQLitePath != "" {
		params["sqlite-path"] = rt.SQLitePath
		delete(params, "db-dsn")
	}
}

func executeYakTool(
	invoker aicommon.AIInvokeRuntime,
	ctx context.Context,
	toolName string,
	rt *Runtime,
	extraParams map[string]any,
) (string, error) {
	params := aitool.InvokeParams{
		"code-root":    rt.Session.CodeRootPath,
		"workdir":      rt.WorkDir,
		"session-uuid": rt.Session.UUID,
		"language":     rt.Session.Language,
	}
	if base := EffectiveTargetBaseURL(rt.Session); base != "" {
		params["target-base-url"] = base
	}
	appendSessionDBToolParams(params, rt)
	for k, v := range extraParams {
		params[k] = v
	}
	result, _, err := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, toolName, params)
	if err != nil {
		return "", err
	}
	if result != nil && !result.Success {
		msg := strings.TrimSpace(result.Error)
		if msg == "" {
			msg = toolName + " failed"
		}
		return "", utils.Error(msg)
	}
	if result != nil {
		return toolResultTextContent(result), nil
	}
	return "", nil
}
