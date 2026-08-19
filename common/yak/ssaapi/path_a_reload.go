package ssaapi

import (
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
)

// pathAEnabled returns true when YAK_SSA_PATH_A_RELOAD is set to a
// positive value. When enabled, after compile + SaveToDatabase, the
// DBWrite Program's cache is closed and a DBRead Program is loaded
// from the database for scanning. This reduces scan-phase RSS by
// releasing compile-time SSA objects (~14GB on Hadoop) and using
// lazy-loaded instructions instead.
//
// Default: disabled (empty or non-positive → false).
// This is a function (not a global var) to avoid test pollution.
func PathAEnabled() bool {
	raw := os.Getenv("YAK_SSA_PATH_A_RELOAD")
	if raw == "" {
		return false
	}
	v, err := strconv.Atoi(raw)
	return err == nil && v > 0
}

// reloadProgramFromDatabase closes the DBWrite Program's cache and
// creates a fresh DBRead Program from the database. This releases
// compile-time SSA objects (instructions, types, variables, scopes,
// ANTLR tokens, source editors) that are no longer needed for
// SyntaxFlow scanning.
//
// Safety:
//   - SaveToDatabase must have completed successfully (caller checks).
//   - The DBWrite cache is closed (Cache.CloseWithoutSave) to release
//     resident instruction/type/index/source maps.
//   - The Program is removed from ProgramCache to avoid stale references.
//   - FromDatabase creates a new Program with ProgramCacheDBRead mode,
//     which lazily loads instructions from DB during SyntaxFlow queries.
//   - The program name, types, indices, sources, and all ir_codes are
//     persisted in the DB and available to the DBRead Program.
//
// Returns the new DBRead Program, or the original Program if reload
// fails (fallback to Path B for safety).
func ReloadProgramFromDatabase(prog *Program) *Program {
	if prog == nil {
		return nil
	}

	programName := prog.GetProgramName()
	if programName == "" {
		log.Warnf("[path-a] cannot reload: program name is empty")
		return prog
	}

	// CleanBaseline: release ALL compilation state (v3 step E/F).
	// CloseWithoutSave only closed the instruction store; CleanBaseline
	// nil's out all stores and program references, allowing GC to
	// reclaim all compile-time SSA objects.
	if prog.Program != nil && prog.Program.Cache != nil {
		if !prog.Program.Cache.IsCleaned() {
			log.Infof("[path-a] CleanBaseline for program %s to release compile memory", programName)
			prog.Program.Cache.CleanBaseline()
		}
	}
	// Force collection and return memory to the OS BEFORE the DBRead scan
	// program starts allocating Values/editors. Without this, compile-time SSA
	// objects may still be pending GC while scan memory grows, so peak RSS does
	// not drop even though CleanBaseline released the references.
	runtime.GC()
	debug.FreeOSMemory()
	log.Infof("[path-a] released compile memory for program %s before DBRead reload", programName)

	// Remove from ProgramCache to avoid returning the stale DBWrite Program.
	ProgramCache.Remove(programName)

	// Load a fresh DBRead Program from the database.
	newProg, err := FromDatabase(programName)
	if err != nil {
		log.Errorf("[path-a] failed to reload program %s from database: %v, falling back to DBWrite Program", programName, err)
		// Fallback: return the original Program. Its cache is closed,
		// so SyntaxFlow will lazily load from DB anyway (GetInstruction
		// falls back to DB when not in resident cache).
		return prog
	}

	log.Infof("[path-a] reloaded program %s from database (DBRead mode) for scanning", programName)

	// Set diagnostics recorder to nil on the new program to avoid
	// referencing the compile-time recorder.
	if newProg.Program != nil {
		newProg.Program.SetDiagnosticsRecorder(nil)
	}

	return newProg
}
