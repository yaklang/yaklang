package loop_ssa_api_discovery

import (
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

// Runtime holds DB handles for one loop execution (stored in ReActLoop vars).
type Runtime struct {
	DB           *gorm.DB
	Repo         *store.Repository
	WorkDir      string
	SQLitePath   string
	SessionDBDSN string
	Session      *store.DiscoverySession
	// UserAuthUsername / UserAuthPassword legacy primary account (first of AuthCredentialGroups or auth: line).
	UserAuthUsername string
	UserAuthPassword string
	// UserAuthCredentialGroups multi-group credentials from user input (each group may have many accounts).
	UserAuthCredentialGroups []UserCredentialGroup
	// Phase4ModeRaw from user input; default deep_mining when empty.
	Phase4ModeRaw string
	// SkipDirectoryAnalysis skips directory BFS when true (also via YAK_SSA_SKIP_DIR_ANALYSIS=1).
	SkipDirectoryAnalysis bool
	// AllowPartialAuth when true (also via partial_auth user input or YAK_SSA_AUTH_PARTIAL_OK=1)
	// continues API probing with verified realms only; unverified realms are skipped programmatically.
	AllowPartialAuth bool
	// FrameworkToolkitEnabled enables programmatic framework toolkit fast path when true.
	FrameworkToolkitEnabled bool
	// SelectedFrameworkID is set by framework router (e.g. publiccms, other).
	SelectedFrameworkID string
	// FrameworkToolkitMode is "fast" (skip Phase1 AI + V-phase) or "fallback_ai" (generic extract + full AI).
	FrameworkToolkitMode string
	// PipelineLogger provides persistent execution logging to workDir/ssa_discovery/pipeline.log.
	PipelineLogger *PipelineLogger
	// ExecutionLogger provides structured JSONL execution logging to workDir/ssa_discovery/execution_log.jsonl.
	ExecutionLogger *ExecutionLogger
}

func setRuntime(loop *reactloops.ReActLoop, rt *Runtime) {
	loop.Set(runtimeKey, rt)
}

func getRuntime(loop *reactloops.ReActLoop) *Runtime {
	v := loop.GetVariable(runtimeKey)
	if v == nil {
		return nil
	}
	rt, _ := v.(*Runtime)
	return rt
}

func (rt *Runtime) pipelineLog() *PipelineLogger {
	if rt == nil {
		return nil
	}
	return rt.PipelineLogger
}

func (rt *Runtime) logInfof(format string, args ...any) {
	if l := rt.pipelineLog(); l != nil {
		l.Infof(format, args...)
	}
}

func (rt *Runtime) logWarnf(format string, args ...any) {
	if l := rt.pipelineLog(); l != nil {
		l.Warnf(format, args...)
	}
}

func (rt *Runtime) logErrorf(format string, args ...any) {
	if l := rt.pipelineLog(); l != nil {
		l.Errorf(format, args...)
	}
}

func (rt *Runtime) logDebugf(format string, args ...any) {
	if l := rt.pipelineLog(); l != nil {
		l.Debugf(format, args...)
	}
}

func (rt *Runtime) execLog() *ExecutionLogger {
	if rt == nil {
		return nil
	}
	if rt.ExecutionLogger != nil {
		return rt.ExecutionLogger
	}
	return ForRuntime(rt)
}

func (rt *Runtime) execStepStart(step, executionType string) {
	if l := rt.execLog(); l != nil {
		l.StepStart(step, executionType)
	}
}

func (rt *Runtime) execStepEnd(step, executionType string, started time.Time, outputFiles []string) {
	if l := rt.execLog(); l != nil {
		l.StepEnd(step, executionType, started, outputFiles)
	}
}

func (rt *Runtime) execStepError(step, executionType string, started time.Time, err error, outputFiles []string) {
	if l := rt.execLog(); l != nil {
		l.StepError(step, executionType, started, err, outputFiles)
	}
}

func (rt *Runtime) execInfo(step, executionType, message string) {
	if l := rt.execLog(); l != nil {
		l.Info(step, executionType, message)
	}
}

// execTimed runs fn with start/end/error execution logging.
func (rt *Runtime) execTimed(step, executionType string, outputFiles []string, fn func() error) error {
	if rt == nil {
		return fn()
	}
	started := time.Now()
	rt.execStepStart(step, executionType)
	err := fn()
	if err != nil {
		rt.execStepError(step, executionType, started, err, outputFiles)
		return err
	}
	rt.execStepEnd(step, executionType, started, outputFiles)
	return nil
}
