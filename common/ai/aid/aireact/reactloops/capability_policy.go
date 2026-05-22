package reactloops

import "github.com/yaklang/yaklang/common/ai/aid/aicommon"

type enablePlanAndExecGetter interface {
	GetEnablePlanAndExec() bool
}

type planExecPolicyState int

const (
	planExecPolicyUnspecified planExecPolicyState = iota
	planExecPolicyEnabled
	disabledByExplicitConfig
)

// IsPlanAndExecAllowed reports whether the current loop/runtime policy still
// allows the plan capability set. This includes both explicit PE actions and
// runnable AI Blueprint / Forge execution.
func IsPlanAndExecAllowed(loop *ReActLoop, invoker aicommon.AIInvokeRuntime) bool {
	if loop != nil {
		if getter := loop.AllowPlanAndExec(); getter != nil && !getter() {
			return false
		}
		if resolveExplicitEnablePlanAndExec(loop.GetConfig()) == disabledByExplicitConfig {
			return false
		}
	}
	if invoker == nil {
		return true
	}
	switch resolveExplicitEnablePlanAndExec(invoker.GetConfig()) {
	case disabledByExplicitConfig:
		return false
	case planExecPolicyEnabled:
		return true
	}
	return true
}

// IsMCPServersAllowed reports whether MCP tools may be discovered or recommended
// for this runtime. When Config.DisallowMCPServers is true (WithDisallowMCPServers),
// ReAct does not preload or connect MCP servers; intent/capability paths should
// not surface MCP matches either, so the model is not steered toward unreachable tools.
func IsMCPServersAllowed(invoker aicommon.AIInvokeRuntime) bool {
	return aicommon.IsMCPServersAllowedRuntime(invoker)
}

func resolveExplicitEnablePlanAndExec(cfg aicommon.AICallerConfigIf) planExecPolicyState {
	if cfg == nil {
		return planExecPolicyUnspecified
	}
	getter, ok := cfg.(enablePlanAndExecGetter)
	if !ok {
		return planExecPolicyUnspecified
	}
	// Many unit tests use raw &aicommon.Config{} zero-values instead of a fully
	// constructed runtime config. Treat those as "unspecified" so tests keep the
	// historical default-allow behavior, while real configs created through
	// NewConfig/WithEnablePlanAndExec still remain explicit and enforceable.
	if originProvider, ok := cfg.(interface {
		OriginOptions() []aicommon.ConfigOption
	}); ok {
		if len(originProvider.OriginOptions()) == 0 {
			if _, isRawConfig := cfg.(*aicommon.Config); isRawConfig {
				return planExecPolicyUnspecified
			}
		}
	}
	if getter.GetEnablePlanAndExec() {
		return planExecPolicyEnabled
	}
	return disabledByExplicitConfig
}
