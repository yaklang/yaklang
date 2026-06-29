package ssaapi

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

// Environment knobs for the SSA compile adaptive GC/CPU profiling. All are opt-in;
// when unset, compile runs with the Go runtime defaults.
const (
	ssaCompileAdaptiveGCEnv = "YAK_SSA_COMPILE_ADAPTIVE_GC"
	ssaCompileGOGCEnv       = "YAK_SSA_COMPILE_GOGC"
	ssaCompileMemLimitEnv   = "YAK_SSA_COMPILE_MEM_LIMIT"
)

const (
	defaultSSACompileGOGC     = 1000
	defaultSSACompileMemLimit = 680 * 1024 * 1024
)

var ssaCompileGCMu sync.Mutex

// startSSACompileCPUProfile starts a CPU profile when YAK_SSA_CPU_PROFILE is
// set to a writable path; otherwise it is a no-op. The returned func stops and
// closes the profile and must be deferred by the caller.
func startSSACompileCPUProfile() func() {
	target := strings.TrimSpace(os.Getenv("YAK_SSA_CPU_PROFILE"))
	if target == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "[ssa.cpu] profile mkdir failed %s: %v\n", filepath.Dir(target), err)
		return nil
	}
	f, err := os.Create(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ssa.cpu] profile create failed %s: %v\n", target, err)
		return nil
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		fmt.Fprintf(os.Stderr, "[ssa.cpu] profile start failed %s: %v\n", target, err)
		_ = f.Close()
		return nil
	}
	fmt.Fprintf(os.Stderr, "[ssa.cpu] profile started %s\n", target)
	return func() {
		pprof.StopCPUProfile()
		if err := f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "[ssa.cpu] profile close failed %s: %v\n", target, err)
			return
		}
		fmt.Fprintf(os.Stderr, "[ssa.cpu] profile saved %s\n", target)
	}
}

// startSSACompileAdaptiveGC applies compile-time GC percent / soft memory limit
// tuning when explicitly requested. It is a no-op unless YAK_SSA_COMPILE_ADAPTIVE_GC
// is enabled or one of YAK_SSA_COMPILE_GOGC / YAK_SSA_COMPILE_MEM_LIMIT is set.
func startSSACompileAdaptiveGC(logf func(string, ...any)) func() {
	if raw := strings.TrimSpace(os.Getenv(ssaCompileAdaptiveGCEnv)); raw != "" && !envFlagEnabled(ssaCompileAdaptiveGCEnv) {
		return nil
	}

	adaptiveEnabled := envFlagEnabled(ssaCompileAdaptiveGCEnv)
	gcPercent, setGC := ssaCompileGCPercent()
	memLimit, setMemLimit := ssaCompileMemoryLimit()
	if adaptiveEnabled {
		if !setGC && strings.TrimSpace(os.Getenv("GOGC")) == "" {
			gcPercent, setGC = defaultSSACompileGOGC, true
		}
		if !setMemLimit && strings.TrimSpace(os.Getenv("GOMEMLIMIT")) == "" {
			memLimit, setMemLimit = defaultSSACompileMemLimit, true
		}
	}
	if !setGC && !setMemLimit {
		return nil
	}

	ssaCompileGCMu.Lock()
	var oldGC int
	var oldMemLimit int64
	if setGC {
		oldGC = debug.SetGCPercent(gcPercent)
	}
	if setMemLimit {
		oldMemLimit = debug.SetMemoryLimit(memLimit)
	}
	if logf != nil {
		logf("ssa compile adaptive GC enabled gogc=%s mem_limit=%s", gcPolicyValue(setGC, gcPercent), gcPolicyBytesValue(setMemLimit, memLimit))
	}
	return func() {
		if setMemLimit {
			debug.SetMemoryLimit(oldMemLimit)
		}
		if setGC {
			debug.SetGCPercent(oldGC)
		}
		ssaCompileGCMu.Unlock()
	}
}

func ssaCompileGCPercent() (int, bool) {
	if raw := strings.TrimSpace(os.Getenv(ssaCompileGOGCEnv)); raw != "" {
		switch strings.ToLower(raw) {
		case "0", "false", "no", "off", "disable", "disabled":
			return 0, false
		default:
			if v, err := strconv.Atoi(raw); err == nil && v >= 0 {
				return v, true
			}
		}
	}
	if strings.TrimSpace(os.Getenv("GOGC")) != "" {
		return 0, false
	}
	return defaultSSACompileGOGC, false
}

func ssaCompileMemoryLimit() (int64, bool) {
	if raw := strings.TrimSpace(os.Getenv(ssaCompileMemLimitEnv)); raw != "" {
		switch strings.ToLower(raw) {
		case "0", "false", "no", "off", "disable", "disabled":
			return 0, false
		default:
			if v, err := utils.ToBytes(raw); err == nil && v > 0 {
				return int64(v), true
			}
			if v, err := strconv.ParseInt(raw, 10, 64); err == nil && v > 0 {
				return v, true
			}
		}
	}
	if strings.TrimSpace(os.Getenv("GOMEMLIMIT")) != "" {
		return 0, false
	}
	return defaultSSACompileMemLimit, false
}

func gcPolicyValue(enabled bool, value int) string {
	if !enabled {
		return "unchanged"
	}
	return strconv.Itoa(value)
}

func gcPolicyBytesValue(enabled bool, value int64) string {
	if !enabled {
		return "unchanged"
	}
	return utils.ByteSize(uint64(value))
}
