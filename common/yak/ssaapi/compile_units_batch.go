package ssaapi

import (
	"os"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

// Environment knobs for compile-unit batching and the writer-cache mode. All
// are opt-in; defaults keep small/medium projects on the resident fast path.
const (
	compileUnitWriterCacheEnv   = "YAK_SSA_COMPILE_UNIT_WRITER_CACHE"
	compileUnitHoldSCCIREnv     = "YAK_SSA_COMPILE_UNIT_HOLD_SCC_IR"
	compileUnitBatchMinFilesEnv = "YAK_SSA_COMPILE_UNIT_BATCH_MIN_FILES"
	compileUnitBatchMinBytesEnv = "YAK_SSA_COMPILE_UNIT_BATCH_MIN_BYTES"
	compileUnitBatchMaxFilesEnv = "YAK_SSA_COMPILE_UNIT_BATCH_MAX_FILES"
)

const (
	// Compile-unit split thresholds. We can't fully clear memory between
	// batches (Instruction objects hold references to Function/Block/Program),
	// so each batch is kept small enough that accumulation stays bounded.
	defaultCompileUnitBatchMinFiles = 32
	defaultCompileUnitBatchMinBytes = 256 * 1024
	defaultCompileUnitBatchMaxFiles = 100
	// Keep in sync with the SSA IR cache resident fast-path threshold. Below
	// this size a single compile-unit batch is cheaper in resident mode than
	// forcing the async writer cache.
	compileUnitResidentFastPathMaxBytes = 2 * 1024 * 1024
)

// compileUnitExecutionBatch is a contiguous run of SCCs from the unit plan that
// the engine compiles in one pass before flushing.
type compileUnitExecutionBatch struct {
	startSCC int
	endSCC   int
	units    []*CompileUnit
	unitKeys []string
	files    int
	bytes    int64
}

func compileUnitBatchThresholds() (int, int64) {
	minFiles := defaultCompileUnitBatchMinFiles
	if raw := strings.TrimSpace(os.Getenv(compileUnitBatchMinFilesEnv)); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			minFiles = v
		}
	}
	if minFiles <= 0 {
		minFiles = 1
	}

	minBytes := int64(defaultCompileUnitBatchMinBytes)
	if raw := strings.TrimSpace(os.Getenv(compileUnitBatchMinBytesEnv)); raw != "" {
		switch strings.ToLower(raw) {
		case "0", "false", "no", "off", "disable", "disabled":
			minBytes = 0
		default:
			if v, err := utils.ToBytes(raw); err == nil {
				minBytes = int64(v)
			} else if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
				minBytes = v
			}
		}
	}
	if minBytes < 0 {
		minBytes = 0
	}
	return minFiles, minBytes
}

func compileUnitBatchMaxFiles() int {
	maxFiles := defaultCompileUnitBatchMaxFiles
	if raw := strings.TrimSpace(os.Getenv(compileUnitBatchMaxFilesEnv)); raw != "" {
		switch strings.ToLower(raw) {
		case "0", "false", "no", "off", "disable", "disabled":
			return 0
		default:
			if v, err := strconv.Atoi(raw); err == nil && v > 0 {
				maxFiles = v
			}
		}
	}
	return maxFiles
}

func buildCompileUnitExecutionBatches(order [][]*CompileUnit, minFiles int, minBytes int64) []compileUnitExecutionBatch {
	if len(order) == 0 {
		return nil
	}
	if minFiles <= 0 {
		minFiles = 1
	}
	if minBytes < 0 {
		minBytes = 0
	}
	maxFiles := compileUnitBatchMaxFiles()

	// Calculate total project size
	totalFiles := 0
	totalBytes := int64(0)
	for _, scc := range order {
		for _, unit := range scc {
			if unit != nil {
				totalFiles += len(unit.Files)
				totalBytes += unit.Bytes
			}
		}
	}

	// Estimate desired batch count based on both dimensions
	var estimatedBatchCount int
	if minFiles > 0 && totalFiles > minFiles {
		estimatedBatchCount = max(estimatedBatchCount, (totalFiles+minFiles-1)/minFiles)
	}
	if minBytes > 0 && totalBytes > minBytes {
		estimatedBatchCount = max(estimatedBatchCount, int((totalBytes+minBytes-1)/minBytes))
	}
	if maxFiles > 0 && totalFiles > maxFiles {
		estimatedBatchCount = max(estimatedBatchCount, (totalFiles+maxFiles-1)/maxFiles)
	}
	if estimatedBatchCount <= 1 {
		// Single batch - take everything
		return []compileUnitExecutionBatch{buildSingleBatch(order)}
	}

	// Adaptive target: aim for balanced batches
	targetFilesPerBatch := (totalFiles + estimatedBatchCount - 1) / estimatedBatchCount
	targetBytesPerBatch := (totalBytes + int64(estimatedBatchCount) - 1) / int64(estimatedBatchCount)

	// Use 80% of target as the threshold to allow some headroom for the last batch
	softMinFiles := int(float64(targetFilesPerBatch) * 0.8)
	softMinBytes := int64(float64(targetBytesPerBatch) * 0.8)
	if softMinFiles < 1 {
		softMinFiles = 1
	}

	batches := make([]compileUnitExecutionBatch, 0, estimatedBatchCount)
	current := compileUnitExecutionBatch{startSCC: -1, endSCC: -1}
	flush := func() {
		if len(current.units) == 0 {
			current = compileUnitExecutionBatch{startSCC: -1, endSCC: -1}
			return
		}
		batches = append(batches, current)
		current = compileUnitExecutionBatch{startSCC: -1, endSCC: -1}
	}

	for sccIndex, scc := range order {
		if current.startSCC < 0 {
			current.startSCC = sccIndex
		}
		current.endSCC = sccIndex
		for _, unit := range scc {
			if unit == nil {
				continue
			}
			// Check max files limit before adding unit
			if maxFiles > 0 && current.files+len(unit.Files) > maxFiles && len(current.units) > 0 {
				flush()
				current.startSCC = sccIndex
				current.endSCC = sccIndex
			}
			current.units = append(current.units, unit)
			current.unitKeys = append(current.unitKeys, unit.Key)
			current.files += len(unit.Files)
			current.bytes += unit.Bytes
		}

		// Check if we should flush this batch
		remainingSCCs := len(order) - sccIndex - 1
		remainingBatches := estimatedBatchCount - len(batches) - 1

		shouldFlush := false
		if remainingSCCs == 0 {
			// Last SCC - always flush to close the final batch
			shouldFlush = true
		} else if remainingBatches <= 0 {
			// No more batches planned - continue accumulating
			shouldFlush = false
		} else if compileUnitBatchReadyAdaptive(current, softMinFiles, softMinBytes) {
			// Reached soft threshold - flush
			shouldFlush = true
		}

		if shouldFlush {
			flush()
		}
	}
	flush()
	return batches
}

func buildSingleBatch(order [][]*CompileUnit) compileUnitExecutionBatch {
	batch := compileUnitExecutionBatch{startSCC: 0, endSCC: len(order) - 1}
	for _, scc := range order {
		for _, unit := range scc {
			if unit == nil {
				continue
			}
			batch.units = append(batch.units, unit)
			batch.unitKeys = append(batch.unitKeys, unit.Key)
			batch.files += len(unit.Files)
			batch.bytes += unit.Bytes
		}
	}
	return batch
}

func compileUnitBatchReadyAdaptive(batch compileUnitExecutionBatch, softMinFiles int, softMinBytes int64) bool {
	if len(batch.units) == 0 {
		return false
	}
	// Both dimensions must reach threshold (AND logic for better balance)
	filesReady := softMinFiles <= 1 || batch.files >= softMinFiles
	bytesReady := softMinBytes <= 0 || batch.bytes >= softMinBytes
	return filesReady && bytesReady
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func compileUnitBatchReady(batch compileUnitExecutionBatch, minFiles int, minBytes int64) bool {
	if len(batch.units) == 0 {
		return false
	}
	if minFiles <= 1 && minBytes <= 0 {
		return true
	}
	// Use OR logic for backward compatibility (deprecated path)
	if minFiles > 0 && batch.files >= minFiles {
		return true
	}
	if minBytes > 0 && batch.bytes >= minBytes {
		return true
	}
	return false
}

// compileUnitWriterCacheEnabled decides whether the compile-unit run uses the
// async writer cache. It requires the caller to opt in via
// YAK_SSA_COMPILE_UNIT_WRITER_CACHE; it then enables the writer for multi-batch
// projects or for single-batch projects above the resident fast-path size.
func compileUnitWriterCacheEnabled(requested bool, batches []compileUnitExecutionBatch, projectBytes int64) bool {
	if !requested {
		return false
	}
	if len(batches) > 1 {
		return true
	}
	return projectBytes > compileUnitResidentFastPathMaxBytes
}
