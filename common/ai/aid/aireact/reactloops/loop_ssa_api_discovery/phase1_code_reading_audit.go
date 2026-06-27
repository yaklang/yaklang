package loop_ssa_api_discovery

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	// largeFileChunkLines is the number of lines per chunk when auto-slicing a large file.
	// read_file in lines mode outputs ~10KB per 200-line chunk, giving the agent ~10KB of
	// structured JSON per read — enough for meaningful analysis without overwhelming the context.
	largeFileChunkLines = 200
	// largeFileByteThreshold is the file size (in bytes) above which a file is considered
	// "large" and auto-sliced. 30KB is chosen because code_unit_registry.json (312KB, 1130 units)
	// would produce ~15 chunks of 200 lines each — manageable for the agent to read sequentially.
	largeFileByteThreshold = 30 * 1024
)

func normalizeReadFilePath(action *aicommon.Action) string {
	if action == nil {
		return ""
	}
	file := strings.TrimSpace(action.GetString("file"))
	if file != "" {
		return file
	}
	return strings.TrimSpace(action.GetString("path"))
}

func readFileParamsForBuiltin(action *aicommon.Action) aitool.InvokeParams {
	params := aitool.InvokeParams{}
	for k, v := range action.GetParams() {
		if k == "path" {
			continue
		}
		params[k] = v
	}
	if strings.TrimSpace(params.GetString("file")) == "" {
		if p := normalizeReadFilePath(action); p != "" {
			params["file"] = p
		}
	}
	return params
}

func buildCodeReadingReadFileAudit(rt *Runtime) reactloops.ReActLoopOption {
	return buildCodeReadingReadFileAuditWithAllowed(rt, nil)
}

// largeFileChunkHint provides the AI with a complete line-based pagination plan for
// a file that exceeds largeFileByteThreshold bytes. It scans the file once to count
// lines, then returns a markdown table describing each chunk (offset, line range,
// estimated byte range). The agent can then issue sequential read_file calls with the
// appropriate `offset` and `lines` parameters to consume the entire file in order.
func largeFileChunkHint(absPath string, fileSize int64) string {
	f, err := os.Open(absPath)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	lineCount := 0
	for scanner.Scan() {
		lineCount++
	}
	if err := scanner.Err(); err != nil {
		return ""
	}
	if lineCount == 0 {
		return ""
	}

	chunkLines := largeFileChunkLines
	chunks := (lineCount + chunkLines - 1) / chunkLines
	if chunks < 2 {
		return ""
	}

	var rows []string
	for i := 0; i < chunks; i++ {
		startLine := i*chunkLines + 1
		endLine := startLine + chunkLines - 1
		if endLine > lineCount {
			endLine = lineCount
		}
		rows = append(rows, fmt.Sprintf(
			"| chunk-%d | offset=%d lines=%d | lines %d-%d / %d |",
			i+1, startLine, chunkLines, startLine, endLine, lineCount,
		))
	}

	return fmt.Sprintf(
		"**Large file detected** (%d bytes, %d lines).\n"+
			"This file has been auto-sliced into **%d chunks of ~%d lines each**.\n\n"+
			"**Pagination plan** (use `read_file` with `offset` and `lines` params):\n\n"+
			"| chunk | read_file params | covers |\n"+
			"| ------ | ---------------- | ------ |\n"+
			"%s\n\n"+
			"**Recommended approach**: Read sequentially from chunk 1 to chunk %d.\n"+
			"For `code_unit_registry.json`, read the `units[]` array entries across chunks to identify all `rel_path` values for `entry_files`.\n",
		fileSize, lineCount, chunks, chunkLines, strings.Join(rows, "\n"), chunks,
	)
}

func buildCodeReadingReadFileAuditWithAllowed(rt *Runtime, allowedRelPaths map[string]struct{}) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"read_file",
		"Read a local text file (Phase1B hybrid audit hook). Parameter name for path MUST be `file`, not `path`. When batch_files are set, only those repo-relative paths are allowed.",
		[]aitool.ToolOption{
			aitool.WithStringParam("file", aitool.WithParam_Required(true), aitool.WithParam_Description("file path (MUST be exact rel_path from batch_files; do NOT guess absolute paths)")),
			aitool.WithIntegerParam("offset"),
			aitool.WithIntegerParam("chunk_size"),
			aitool.WithIntegerParam("lines"),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			path := normalizeReadFilePath(action)
			if path == "" {
				op.Feedback("read_file: missing required param `file` (do NOT use `path`; see fs_builtin_tool_params in persistent instruction)")
				op.Continue()
				return
			}
			canon := normalizePlanFileRef(rt, path)
			if len(allowedRelPaths) > 0 {
				if _, ok := allowedRelPaths[canon]; !ok {
					var allowed []string
					for k := range allowedRelPaths {
						allowed = append(allowed, k)
					}
					op.Feedback(fmt.Sprintf("read_file blocked: %q is not in current batch_files. Use one of: %s", canon, strings.Join(allowed, ", ")))
					logFileOp(rt, FileOpInput{
						Stage: store.FileOpStagePhase1BReact, Operation: store.FileOpReactReadFile,
						RelPath: canon, ToolName: "read_file",
						Outcome: store.FileOpOutcomeFailed, Summary: "path not in batch worklist",
					})
					op.Continue()
					return
				}
				if rt != nil && rt.Session != nil && rt.Session.CodePathOK {
					path = filepath.Join(rt.Session.CodeRootPath, filepath.FromSlash(canon))
				} else {
					path = canon
				}
			}

			// Intercept large files without an explicit offset: compute a pagination plan
			// and return it to the agent instead of executing the read immediately.
			// This prevents the agent from reading only the first ~10KB of a 312KB
			// code_unit_registry.json and then guessing the remaining paths.
			if action.GetInt("offset") == 0 && path != "" {
				if info, err := os.Stat(path); err == nil && info.Size() >= largeFileByteThreshold {
					if hint := largeFileChunkHint(path, info.Size()); hint != "" {
						op.Feedback(hint)
						op.Continue()
						return
					}
				}
			}

			invoker := loop.GetInvoker()
			ctx := loop.GetConfig().GetContext()
			task := loop.GetCurrentTask()
			if task != nil && !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}
			params := readFileParamsForBuiltin(action)
			params["file"] = path
			result, _, err := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, "read_file", params)
			if err != nil {
				logFileOp(rt, FileOpInput{
					Stage: store.FileOpStagePhase1BReact, Operation: store.FileOpReactReadFile,
					RelPath: canon, ToolName: "read_file",
					Outcome: store.FileOpOutcomeFailed, Summary: err.Error(),
				})
				op.Feedback(fmt.Sprintf("read_file failed: %v", err))
				op.Continue()
				return
			}
			content := toolResultTextContent(result)
			logFileOp(rt, FileOpInput{
				Stage: store.FileOpStagePhase1BReact, Operation: store.FileOpReactReadFile,
				RelPath: canon, ToolName: "read_file",
				Outcome: store.FileOpOutcomeProcessed,
				Summary: fmt.Sprintf("read %d bytes", len(content)),
			})
			op.Feedback(content)
			op.Continue()
		},
	)
}

func worklistBatchAllowedPaths(rt *Runtime, batch []WorklistSeedItem) map[string]struct{} {
	out := map[string]struct{}{}
	for _, item := range batch {
		rel := normalizePlanFileRef(rt, item.RelPath)
		if rel != "" {
			out[rel] = struct{}{}
		}
	}
	return out
}
