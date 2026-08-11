package ssaapi

import (
	"io"
	"os"
	"path/filepath"

	"github.com/yaklang/yaklang/common/consts"
)

// DebugOutputCleanup stops the pprof collector and closes the log file.
type DebugOutputCleanup func()

// noopDebugCleanup is returned when debug is disabled or setup fails non-fatally.
func noopDebugCleanup() {}

// SetupDebugDir wires debug/pprof output when debugDir is non-empty.
// Empty debugDir returns a no-op cleanup so callers can always:
//
//	cleanup := ssaapi.SetupDebugDir(config.GetDebugDir(), false)
//	defer cleanup()
//
// Setup failures are logged and treated as non-fatal (no-op cleanup).
// See StartDebugOutput for redirectSSADB semantics.
func SetupDebugDir(debugDir string, redirectSSADB bool) DebugOutputCleanup {
	if debugDir == "" {
		return noopDebugCleanup
	}
	cleanup, err := StartDebugOutput(debugDir, redirectSSADB)
	if err != nil {
		log.Warnf("[debug] setup debug dir %s failed: %v, continuing without debug", debugDir, err)
		return noopDebugCleanup
	}
	if cleanup == nil {
		return noopDebugCleanup
	}
	return cleanup
}

// StartDebugOutput wires up debug/pprof output for a scan or compile run.
// It is the shared implementation used by SetupDebugDir and the CLI --debug path:
//   - creates debugDir (MkdirAll)
//   - redirects log output to <dir>/log
//   - starts the periodic pprof collector (CPU/heap/goroutine snapshots)
//   - when redirectSSADB is true, redirects the SSA database to <dir>/ssadb.db
//
// For two-job compile -> scan flows the compile job must keep the shared
// Postgres SSA IR database so the scan job can reuse the compiled program, so
// callers should pass redirectSSADB=false for platform compile/scan jobs.
// Standalone CLI scans typically pass redirectSSADB=true.
func StartDebugOutput(debugDir string, redirectSSADB bool) (DebugOutputCleanup, error) {
	if debugDir == "" {
		return noopDebugCleanup, nil
	}
	if err := os.MkdirAll(debugDir, 0o755); err != nil {
		return nil, err
	}

	if redirectSSADB {
		ssadbPath := filepath.Join(debugDir, "ssadb.db")
		if err := consts.SetGormSSAProjectDatabaseByInfo(ssadbPath); err != nil {
			log.Warnf("[debug] set SSA database to %s failed: %v", ssadbPath, err)
		} else {
			log.Infof("[debug] SSA database: %s", ssadbPath)
		}
	}

	// Redirect log to <debugDir>/log (in addition to stdout).
	logPath := filepath.Join(debugDir, "log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		log.Warnf("[debug] create log file %s failed: %v", logPath, err)
		logFile = nil
	} else {
		log.Infof("[debug] log output: %s", logPath)
		log.SetOutput(io.MultiWriter(logFile, os.Stdout))
	}

	// Start pprof collector (periodic CPU/heap/goroutine snapshots).
	cleanup, err := StartPprofCollector(debugDir)
	if err != nil {
		log.Warnf("[debug] start pprof collector failed: %v, continuing without pprof", err)
		if logFile != nil {
			_ = logFile.Close()
		}
		return noopDebugCleanup, nil
	}

	return func() {
		cleanup()
		if logFile != nil {
			_ = logFile.Close()
		}
	}, nil
}
