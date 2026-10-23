package syntaxflow_scan

import (
	"context"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func Scan(ctx context.Context, option ...ssaconfig.Option) (retErr error) {
	defer func() {
		if e := recover(); e != nil {
			log.Errorf("syntaxflow scan panic: %v", e)
			utils.PrintCurrentGoroutineRuntimeStack()
			retErr = utils.Errorf("syntaxflow scan panic: %v", e)
		}
	}()

	config, err := NewConfig(option...)
	if err != nil {
		return err
	}

	// The call that creates the runtime owns its consumers. A nested stage
	// receives the runtime already and must not register a second report or
	// database saver.
	ownsRuntime := config.scanRuntime == nil
	rt := ensureScanRuntime(config)
	memory := config.GetSyntaxFlowMemory() ||
		config.GetSyntaxFlowResultKind() == ssaconfig.SFResultSaveMemory
	if ownsRuntime {
		rt.RegisterRiskUpdateHandler(config.Reporter)
		if !memory && !config.IsNoSaveRisk() {
			rt.RegisterRiskUpdateHandler(ssaapi.NewRiskDBSaver())
		}
	}

	// Wire up debug/pprof output when debug_dir is set.
	// Keep the shared Postgres SSA IR DB (redirectSSADB=false) for platform
	// two-job compile -> scan reuse; CLI --debug redirects SSADB separately.
	debugCleanup := ssaapi.SetupDebugDir(config.GetDebugDir(), false)
	defer debugCleanup()

	var taskId string
	var m *scanManager
	var success bool

	runningID := uuid.NewString()
	defer func() {
		if m == nil {
			if ownsRuntime {
				retErr = utils.JoinErrors(retErr, rt.Close())
			}
			return
		}
		if success && m.status != schema.SYNTAXFLOWSCAN_PAUSED {
			m.SetFinishedQuery(m.GetTotalQuery())
			if m.GetTotalQuery() == 0 {
				m.status = schema.SYNTAXFLOWSCAN_DONE
			}
		}
		m.StatusTask()
		m.Stop(runningID)
		// Stop waited for every queued risk callback, so the pending batch is
		// complete. Write it before the task row so a caller that sees the
		// finished task can also read every finding.
		retErr = utils.JoinErrors(retErr, rt.Flush())
		if ownsRuntime {
			retErr = utils.JoinErrors(retErr, rt.Close())
		}
		// Stop waits for queued result callbacks, so the final task row includes
		// the complete in-memory risk count even when risks are not persisted.
		// SaveTask flushes the coalesced snapshot. A failed write is returned and
		// must not be followed by the terminal callback: callers load this row by
		// task ID as soon as they see done.
		if err := m.SaveTask(); err != nil {
			log.Errorf("save syntaxflow task failed: %v", err)
			retErr = utils.JoinErrors(retErr, err)
		} else if success && m.status == schema.SYNTAXFLOWSCAN_DONE {
			m.notifyDone()
		}
		// Stop waits for the queued risk callbacks, so the report already holds
		// every finding this task accepted. Saving here publishes that document.
		m.saveReport()
	}()
	errC := make(chan error)
	switch ssaconfig.ControlMode(config.GetScanControlMode()) {
	case ssaconfig.ControlModeStart:
		taskId = uuid.New().String()
		m, err = createSyntaxflowTaskById(ctx, runningID, taskId, config)
		if err != nil {
			return err
		}
		log.Info("start to create syntaxflow scan")
		go func() {
			err := m.ScanNewTask()
			if err != nil {
				utils.TryWriteChannel(errC, err)
			}
			close(errC)
		}()
	case ssaconfig.ControlModeStatus:
		taskId = config.GetScanResumeTaskId()
		m, err = LoadSyntaxflowTaskFromDB(ctx, runningID, config)
		if err != nil {
			return err
		}
		m.StatusTask()
		close(errC)
	case ssaconfig.ControlModeResume:
		taskId = config.GetScanResumeTaskId()
		m, err = LoadSyntaxflowTaskFromDB(ctx, runningID, config)
		if err != nil {
			return err
		}
		go func() {
			err := m.ResumeTask()
			if err != nil {
				utils.TryWriteChannel(errC, err)
			}
			close(errC)
		}()
	default:
		return utils.Error("invalid syntaxFlow scan mode")
	}
	RemoveSyntaxFlowTaskByID(taskId)

	// wait result
	select {
	case err, ok := <-errC:
		if ok {
			m.status = schema.SYNTAXFLOWSCAN_ERROR
			success = false
			return err
		}
		success = true
		return nil
	case <-ctx.Done():
		m.status = schema.SYNTAXFLOWSCAN_DONE
		success = false
		return utils.Error("client canceled")
	}
}
