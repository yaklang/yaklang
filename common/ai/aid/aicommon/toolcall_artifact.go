package aicommon

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	ToolResultTokenLimit       = 16 * 1024
	toolResultHintReserve      = 1024
	toolCapturePreviewBytes    = 256 * 1024
	toolOutputSnapshotMaxBytes = 128 * 1024
	toolUIStreamHeadBytes      = 12 * 1024
	toolUIStreamTailBytes      = 4 * 1024
)

type boundedToolUIWriter struct {
	mu         sync.Mutex
	dst        io.Writer
	headLeft   int
	tail       []byte
	tailLimit  int
	suppressed int64
}

func newBoundedToolUIWriter(dst io.Writer) *boundedToolUIWriter {
	return &boundedToolUIWriter{dst: dst, headLeft: toolUIStreamHeadBytes, tailLimit: toolUIStreamTailBytes}
}

func (w *boundedToolUIWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	originalLen := len(p)
	if w.headLeft > 0 {
		take := w.headLeft
		if take > len(p) {
			take = len(p)
		}
		if take > 0 {
			if _, err := w.dst.Write(p[:take]); err != nil {
				return 0, err
			}
			w.headLeft -= take
			p = p[take:]
		}
	}
	if len(p) > 0 {
		w.suppressed += int64(len(p))
		w.tail = append(w.tail, p...)
		if len(w.tail) > w.tailLimit {
			w.tail = append([]byte(nil), w.tail[len(w.tail)-w.tailLimit:]...)
		}
	}
	return originalLen, nil
}

func (w *boundedToolUIWriter) Finish() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.suppressed == 0 {
		return nil
	}
	omitted := w.suppressed - int64(len(w.tail))
	marker := fmt.Sprintf("\n... [UI stream omitted %d middle bytes; complete output is in the tool artifact] ...\n", omitted)
	if _, err := io.WriteString(w.dst, marker); err != nil {
		return err
	}
	_, err := w.dst.Write(w.tail)
	return err
}

type boundedHeadTailBuffer struct {
	mu        sync.RWMutex
	head      []byte
	tail      []byte
	total     int64
	headLimit int
	tailLimit int
}

func newBoundedHeadTailBuffer(limit int) *boundedHeadTailBuffer {
	if limit < 2 {
		limit = 2
	}
	return &boundedHeadTailBuffer{headLimit: limit / 2, tailLimit: limit - limit/2}
}

func (b *boundedHeadTailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	b.total += int64(n)
	if remain := b.headLimit - len(b.head); remain > 0 {
		take := remain
		if take > len(p) {
			take = len(p)
		}
		b.head = append(b.head, p[:take]...)
		p = p[take:]
	}
	if len(p) > 0 {
		b.tail = append(b.tail, p...)
		if len(b.tail) > b.tailLimit {
			b.tail = append([]byte(nil), b.tail[len(b.tail)-b.tailLimit:]...)
		}
	}
	return n, nil
}

func (b *boundedHeadTailBuffer) Snapshot() []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.total <= int64(len(b.head)+len(b.tail)) {
		return append(bytes.Clone(b.head), b.tail...)
	}
	omitted := b.total - int64(len(b.head)+len(b.tail))
	marker := fmt.Sprintf("\n\n... [artifact preview omitted %d bytes from the middle] ...\n\n", omitted)
	out := make([]byte, 0, len(b.head)+len(marker)+len(b.tail))
	out = append(out, b.head...)
	out = append(out, marker...)
	out = append(out, b.tail...)
	return out
}

func (b *boundedHeadTailBuffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return int(b.total)
}

type toolArtifactStream int

const (
	artifactStdout toolArtifactStream = iota
	artifactStderr
)

type toolArtifactWriter struct {
	bundle *toolCallArtifactBundle
	stream toolArtifactStream
}

func (w toolArtifactWriter) Write(p []byte) (int, error) {
	return w.bundle.writeStream(w.stream, p)
}

type artifactFileStats struct {
	Path       string `json:"path"`
	Bytes      int64  `json:"bytes"`
	Tokens     int    `json:"tokens"`
	Lines      int64  `json:"lines"`
	SHA256     string `json:"sha256"`
	Persistent bool   `json:"persistent"`
}

type toolCallArtifactManifest struct {
	Tool       string                       `json:"tool"`
	CallToolID string                       `json:"call_tool_id"`
	Identifier string                       `json:"identifier,omitempty"`
	Status     string                       `json:"status"`
	Success    bool                         `json:"success"`
	Error      string                       `json:"error,omitempty"`
	Params     any                          `json:"params"`
	CreatedAt  time.Time                    `json:"created_at"`
	Files      map[string]artifactFileStats `json:"files"`
}

type toolCallArtifactBundle struct {
	mu sync.Mutex

	dir          string
	reportPath   string
	combinedPath string
	stdoutPath   string
	stderrPath   string
	resultPath   string
	manifestPath string

	combined *os.File
	stdout   *os.File
	stderr   *os.File
	preview  *boundedHeadTailBuffer
	prepare  error
	closed   bool
}

func (t *ToolCaller) newToolCallArtifactBundle(tool *aitool.Tool, callToolID, identifier string) *toolCallArtifactBundle {
	b := &toolCallArtifactBundle{preview: newBoundedHeadTailBuffer(toolCapturePreviewBytes)}
	workdir := ""
	if cfg, ok := t.config.(*Config); ok {
		workdir = cfg.Workdir
	}
	if workdir == "" {
		workdir = t.config.GetOrCreateWorkDir()
	}
	if workdir == "" {
		workdir = consts.GetDefaultBaseHomeDir()
	}
	taskIndex, taskName := "0", ""
	if t.task != nil {
		if t.task.GetIndex() != "" {
			taskIndex = t.task.GetIndex()
		}
		taskName = t.task.GetSemanticIdentifier()
	}
	callNumber := 1
	if t.task != nil {
		callNumber = len(t.task.GetAllToolCallResults()) + 1
	}
	name := sanitizeFilename(tool.Name)
	if name == "" {
		name = "unknown_tool"
	}
	parts := []string{fmt.Sprintf("%d", callNumber), name}
	if identifier != "" {
		parts = append(parts, sanitizeFilename(identifier))
	}
	baseDir := filepath.Join(workdir, BuildTaskDirName(taskIndex, taskName), "tool_calls", strings.Join(parts, "_"))
	b.dir, b.prepare = reserveToolArtifactDir(baseDir)
	if b.prepare != nil {
		return b
	}
	b.reportPath = filepath.Join(b.dir, "report.md")
	b.combinedPath = filepath.Join(b.dir, "combined_output.txt")
	b.stdoutPath = filepath.Join(b.dir, "stdout.txt")
	b.stderrPath = filepath.Join(b.dir, "stderr.txt")
	b.manifestPath = filepath.Join(b.dir, "manifest.json")
	var err error
	if b.combined, err = os.Create(b.combinedPath); err != nil {
		b.prepare = err
		return b
	}
	if b.stdout, err = os.Create(b.stdoutPath); err != nil {
		b.prepare = err
		b.closeStreams()
		return b
	}
	if b.stderr, err = os.Create(b.stderrPath); err != nil {
		b.prepare = err
		b.closeStreams()
		return b
	}
	return b
}

// reserveToolArtifactDir never reuses an existing bundle. This matters during
// checkpoint replay: the caller prepares a bundle before it discovers the
// stored result, and truncating the original artifact here would make the HINT
// in the replayed Data point at destroyed files.
func reserveToolArtifactDir(base string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(base), 0o755); err != nil {
		return base, err
	}
	for suffix := 1; suffix <= 10_000; suffix++ {
		candidate := base
		if suffix > 1 {
			candidate = fmt.Sprintf("%s_%d", base, suffix)
		}
		if err := os.Mkdir(candidate, 0o755); err == nil {
			return candidate, nil
		} else if !os.IsExist(err) {
			return candidate, err
		}
	}
	return base, utils.Errorf("cannot reserve unique tool artifact directory: %s", base)
}

func (b *toolCallArtifactBundle) Writer(stream toolArtifactStream) io.Writer {
	return toolArtifactWriter{bundle: b, stream: stream}
}

func (b *toolCallArtifactBundle) writeStream(stream toolArtifactStream, p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, _ = b.preview.Write(p)
	if b.prepare != nil {
		return len(p), nil
	}
	var target *os.File
	if stream == artifactStderr {
		target = b.stderr
	} else {
		target = b.stdout
	}
	if _, err := target.Write(p); err != nil {
		b.prepare = err
		return len(p), nil
	}
	if _, err := b.combined.Write(p); err != nil {
		b.prepare = err
	}
	return len(p), nil
}

func (b *toolCallArtifactBundle) closeStreams() {
	if b.closed {
		return
	}
	b.closed = true
	for _, f := range []*os.File{b.combined, b.stdout, b.stderr} {
		if f != nil {
			if err := f.Close(); err != nil && b.prepare == nil {
				b.prepare = err
			}
		}
	}
}

func stableResultText(v any) (text, ext string) {
	if v == nil {
		return "", ".txt"
	}
	if s, ok := v.(string); ok {
		return s, ".txt"
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err == nil {
		return string(data), ".json"
	}
	return utils.InterfaceToString(v), ".txt"
}

func legacyExecutionParts(data any) (combined, stdout, stderr string, result any, ok bool) {
	switch v := data.(type) {
	case *aitool.ToolExecutionResult:
		combined = v.CombinedOutput
		stdout, stderr = v.Stdout, v.Stderr
		if combined == "" {
			if stdout != "" {
				combined += "[STDOUT]\n" + stdout
			}
			if stderr != "" {
				combined += "[STDERR]\n" + stderr
			}
		}
		return combined, stdout, stderr, v.Result, true
	case map[string]any:
		// A plain structured tool result may also be a map containing a
		// "result" field. Treat it as the legacy execution envelope only when
		// at least one capture field identifies that envelope unambiguously.
		_, hasCombined := v["combined_output"]
		_, hasStdout := v["stdout"]
		_, hasStderr := v["stderr"]
		if !hasCombined && !hasStdout && !hasStderr {
			return "", "", "", data, false
		}
		combined = utils.InterfaceToString(v["combined_output"])
		stdout = utils.InterfaceToString(v["stdout"])
		stderr = utils.InterfaceToString(v["stderr"])
		if combined == "" {
			if stdout != "" {
				combined += "[STDOUT]\n" + stdout
			}
			if stderr != "" {
				combined += "[STDERR]\n" + stderr
			}
		}
		return combined, stdout, stderr, v["result"], true
	default:
		return "", "", "", data, false
	}
}

func fileStats(path string) artifactFileStats {
	st := artifactFileStats{Path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		return st
	}
	h := sha256.Sum256(data)
	st.Bytes = int64(len(data))
	st.Tokens = ytoken.CalcTokenCount(string(data))
	st.Lines = int64(bytes.Count(data, []byte{'\n'}))
	if len(data) > 0 && data[len(data)-1] != '\n' {
		st.Lines++
	}
	st.SHA256 = hex.EncodeToString(h[:])
	st.Persistent = true
	return st
}

func toolArtifactHint(b *toolCallArtifactBundle, persistErr error) string {
	if persistErr != nil || b == nil {
		return fmt.Sprintf("HINT:\nComplete output could not be persisted (artifact_persist_failed: %v). This is a bounded preview; omitted content is unrecoverable.", persistErr)
	}
	return fmt.Sprintf(`HINT:
Complete tool output is stored in artifacts:
- combined output: %s
- stdout: %s
- stderr: %s
- result: %s
Use grep first, or read_file(file=%q, mode="lines", offset=..., lines=...).
Do not load or cat the complete artifact unless necessary.`, b.combinedPath, b.stdoutPath, b.stderrPath, b.resultPath, b.combinedPath)
}

func shrinkBodyWithStats(body string, budget int) string {
	if budget <= 0 {
		return ""
	}
	tokens := ytoken.Encode(body)
	if len(tokens) <= budget {
		return body
	}
	marker := fmt.Sprintf("\n\n... [truncated %d tokens, %d bytes and %d lines from the middle; inspect artifacts from HINT] ...\n\n",
		len(tokens)-budget, len(body), strings.Count(body, "\n"))
	markerTokens := ytoken.CalcTokenCount(marker)
	available := budget - markerTokens
	if available <= 0 {
		return ytoken.Decode(tokens[:budget])
	}
	head := available / 2
	tail := available - head
	return ytoken.Decode(tokens[:head]) + marker + ytoken.Decode(tokens[len(tokens)-tail:])
}

func normalizeToolResultData(toolResult *aitool.ToolResult, combined, resultText, hint string) {
	if toolResult == nil {
		return
	}
	// Data is the sole prompt representation after tool completion. Historical
	// shrink fields must not bypass it during Timeline rendering.
	toolResult.ShrinkResult = ""
	toolResult.ShrinkSimilarResult = ""
	if ytoken.CalcTokenCount(toolResult.CallExpectations) > toolResultHintReserve/2 {
		toolResult.CallExpectations = ShrinkTextBlockByTokens(toolResult.CallExpectations, toolResultHintReserve/2)
	}
	if combined == "" {
		combined = "(empty)"
	}
	if resultText == "" {
		resultText = "(empty)"
	}
	if combined == resultText {
		resultText = "[duplicate of COMBINED OUTPUT omitted]"
	}
	body := "COMBINED OUTPUT:\n" + combined + "\n\nRESULT:\n" + resultText
	staticTokens := ytoken.CalcTokenCount("\n\n" + hint)
	bodyBudget := ToolResultTokenLimit - toolResultHintReserve
	if bodyBudget > ToolResultTokenLimit-staticTokens {
		bodyBudget = ToolResultTokenLimit - staticTokens
	}
	if bodyBudget < 0 {
		bodyBudget = 0
	}

	// An enormous error cannot be allowed to consume the whole Timeline item.
	if ytoken.CalcTokenCount(toolResult.Error) > toolResultHintReserve {
		toolResult.Error = ShrinkTextBlockByTokens(toolResult.Error, toolResultHintReserve)
	}
	toolResult.Data = shrinkBodyWithStats(body, bodyBudget) + "\n\n" + hint
	if ytoken.CalcTokenCount(toolResult.Data.(string)) > ToolResultTokenLimit {
		toolResult.Data = shrinkBodyWithStats(body, ToolResultTokenLimit-staticTokens) + "\n\n" + hint
	}

	enforceCanonicalToolResultLimit(toolResult)
}

// enforceCanonicalToolResultLimit is safe to call after finalize. The caller
// can still attach a checkpoint/invocation error after Data was normalized; we
// must not let that late outer error push the final Timeline item above 16K.
func enforceCanonicalToolResultLimit(toolResult *aitool.ToolResult) {
	if toolResult == nil {
		return
	}
	if ytoken.CalcTokenCount(toolResult.Error) > toolResultHintReserve {
		toolResult.Error = ShrinkTextBlockByTokens(toolResult.Error, toolResultHintReserve)
	}
	// Params remain on ToolResult for audit/checkpoint use, but an exceptionally
	// large param block is omitted from Timeline rendering to preserve the same
	// 16K hard ceiling as Data.
	for attempts := 0; attempts < 8 && ytoken.CalcTokenCount(toolResult.String()) > ToolResultTokenLimit; attempts++ {
		if !toolResult.OmitParamsInTimeline {
			toolResult.OmitParamsInTimeline = true
			continue
		}
		over := ytoken.CalcTokenCount(toolResult.String()) - ToolResultTokenLimit
		data, ok := toolResult.Data.(string)
		if !ok || data == "" {
			break
		}
		hintIndex := strings.LastIndex(data, "\n\nHINT:\n")
		if hintIndex < 0 {
			break
		}
		body, hint := data[:hintIndex], data[hintIndex+2:]
		bodyBudget := ytoken.CalcTokenCount(body) - over - 32
		if bodyBudget < 0 {
			bodyBudget = 0
		}
		toolResult.Data = shrinkBodyWithStats(body, bodyBudget) + "\n\n" + hint
	}
}

func (b *toolCallArtifactBundle) finalize(
	t *ToolCaller,
	tool *aitool.Tool,
	callToolID, identifier string,
	params aitool.InvokeParams,
	toolResult *aitool.ToolResult,
	paramGenDuration time.Duration,
	rawAIParamResponse string,
) error {
	if toolResult == nil {
		return nil
	}
	if data, ok := toolResult.Data.(string); ok && strings.Contains(data, "COMBINED OUTPUT:\n") && strings.Contains(data, "\n\nRESULT:\n") && strings.Contains(data, "\n\nHINT:\n") {
		// A current-format checkpoint already owns stable artifact paths. Do not
		// wrap the preview again or rewrite its bytes during replay.
		b.closeStreams()
		for _, path := range []string{b.combinedPath, b.stdoutPath, b.stderrPath} {
			if path != "" {
				_ = os.Remove(path)
			}
		}
		if b.dir != "" {
			_ = os.Remove(b.dir)
		}
		return nil
	}
	b.closeStreams()
	combined, legacyStdout, legacyStderr, rawResult, legacy := legacyExecutionParts(toolResult.Data)
	if combined == "" {
		combined = string(b.preview.Snapshot())
	}
	if !legacy && rawResult == nil {
		rawResult = toolResult.Data
	}
	resultText, resultExt := stableResultText(rawResult)
	b.resultPath = filepath.Join(b.dir, "result"+resultExt)

	persistErr := b.prepare
	if persistErr == nil {
		if err := os.WriteFile(b.resultPath, []byte(resultText), 0o644); err != nil {
			persistErr = err
		}
	}

	// Legacy checkpoints contain their full combined output in Data rather than
	// in the just-created stream files. Materialize that content exactly once.
	if persistErr == nil && legacy {
		if st, err := os.Stat(b.combinedPath); err == nil && st.Size() == 0 && combined != "" {
			if err := os.WriteFile(b.combinedPath, []byte(combined), 0o644); err != nil {
				persistErr = err
			}
		}
		if st, err := os.Stat(b.stdoutPath); err == nil && st.Size() == 0 && legacyStdout != "" {
			if err := os.WriteFile(b.stdoutPath, []byte(legacyStdout), 0o644); err != nil {
				persistErr = err
			}
		}
		if st, err := os.Stat(b.stderrPath); err == nil && st.Size() == 0 && legacyStderr != "" {
			if err := os.WriteFile(b.stderrPath, []byte(legacyStderr), 0o644); err != nil {
				persistErr = err
			}
		}
	}

	hint := toolArtifactHint(b, persistErr)
	normalizeToolResultData(toolResult, combined, resultText, hint)
	rawTokens := ytoken.CalcTokenCount(combined) + ytoken.CalcTokenCount(resultText)
	if persistErr != nil && rawTokens > ToolResultTokenLimit {
		toolResult.Success = false
		if toolResult.Error != "" {
			toolResult.Error += "; "
		}
		toolResult.Error += "artifact_persist_failed: complete oversized output is unavailable"
		normalizeToolResultData(toolResult, combined, resultText, hint)
		return persistErr
	}
	if persistErr != nil {
		log.Warnf("tool artifact persistence failed for bounded result: %v", persistErr)
		return nil
	}

	files := map[string]artifactFileStats{
		"combined_output": fileStats(b.combinedPath),
		"stdout":          fileStats(b.stdoutPath),
		"stderr":          fileStats(b.stderrPath),
		"result":          fileStats(b.resultPath),
	}
	manifest := toolCallArtifactManifest{
		Tool:       tool.Name,
		CallToolID: callToolID,
		Identifier: identifier,
		Status:     map[bool]string{true: "success", false: "failed"}[toolResult.Success],
		Success:    toolResult.Success,
		Error:      toolResult.Error,
		Params:     params,
		CreatedAt:  time.Now(),
		Files:      files,
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err == nil {
		err = os.WriteFile(b.manifestPath, manifestData, 0o644)
	}
	if err != nil {
		return err
	}

	report := b.renderReport(tool, identifier, params, toolResult, paramGenDuration, rawAIParamResponse)
	if err := os.WriteFile(b.reportPath, []byte(report), 0o644); err != nil {
		return err
	}
	t.emitter.EmitToolCallLogDir(callToolID, b.reportPath)
	t.emitter.EmitPinFilename(b.reportPath)
	t.emitter.EmitPinFilename(b.dir)
	log.Infof("saved tool call artifact bundle to: %s", b.dir)
	return nil
}

func (b *toolCallArtifactBundle) renderReport(tool *aitool.Tool, identifier string, params aitool.InvokeParams, toolResult *aitool.ToolResult, paramGenDuration time.Duration, rawAIParamResponse string) string {
	var md strings.Builder
	md.WriteString(fmt.Sprintf("# Tool Call Report: %s\n\n", tool.Name))
	md.WriteString("## Basic Info\n\n")
	md.WriteString(fmt.Sprintf("- **Tool**: %s\n- **Call ID**: %s\n", tool.Name, toolResult.ToolCallID))
	if identifier != "" {
		md.WriteString(fmt.Sprintf("- **Identifier**: %s\n", identifier))
	}
	md.WriteString("\n## Parameters\n\n")
	if paramGenDuration > 0 {
		md.WriteString(fmt.Sprintf("Parameter generation took **%.2fs**\n\n", paramGenDuration.Seconds()))
	}
	if rawAIParamResponse != "" {
		md.WriteString("### Raw AI Response\n\n" + markdownCodeFence + "\n" + ShrinkTextBlockByTokens(rawAIParamResponse, 2048) + "\n" + markdownCodeFence + "\n\n")
	}
	md.WriteString("### Parsed Parameters (YAML)\n\n" + markdownCodeFence + "yaml\n" + renderParamsAsYAML(params) + markdownCodeFence + "\n\n")
	md.WriteString("## Execution Result Preview\n\n" + markdownCodeFence + "\n" + utils.InterfaceToString(toolResult.Data) + "\n" + markdownCodeFence + "\n\n")
	md.WriteString("## Artifact Files\n\n")
	md.WriteString(fmt.Sprintf("- [Combined output](%s)\n- [Stdout](%s)\n- [Stderr](%s)\n- [Result](%s)\n- [Manifest](%s)\n", b.combinedPath, b.stdoutPath, b.stderrPath, b.resultPath, b.manifestPath))
	return md.String()
}

// migrateLegacyToolResults externalizes historical ToolExecutionResult
// envelopes after a persistent Timeline has been restored and its workdir is
// available. It deliberately leaves already-canonical string Data untouched.
// The caller persists the Timeline once when this returns true.
func (m *Timeline) migrateLegacyToolResults(workdir string) bool {
	if m == nil || strings.TrimSpace(workdir) == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	migrated := make(map[int64]*aitool.ToolResult)
	m.idToTimelineItem.ForEach(func(id int64, item *TimelineItem) bool {
		if item == nil {
			return true
		}
		result, ok := item.value.(*aitool.ToolResult)
		if !ok || result == nil {
			return true
		}
		if !migrateLegacyTimelineToolResult(workdir, result) {
			return true
		}
		migrated[id] = result
		return true
	})
	if len(migrated) == 0 {
		return false
	}

	// The serialized Timeline stores id and timestamp indexes separately, so
	// JSON restore creates distinct TimelineItem pointers. Rebind the timestamp
	// index to the migrated ToolResult to prevent the old envelope surviving in
	// UI/diff paths that traverse that index.
	m.tsToTimelineItem.ForEach(func(_ int64, item *TimelineItem) bool {
		if item == nil {
			return true
		}
		if result, ok := migrated[item.GetID()]; ok {
			item.value = result
		}
		return true
	})
	return true
}

func migrateLegacyTimelineToolResult(workdir string, toolResult *aitool.ToolResult) bool {
	combined, stdout, stderr, rawResult, legacy := legacyExecutionParts(toolResult.Data)
	if !legacy {
		return false
	}

	dir := filepath.Join(
		workdir,
		"task_legacy",
		"tool_calls",
		fmt.Sprintf("%d_%s_migrated", toolResult.ID, sanitizeFilename(toolResult.Name)),
	)
	b := &toolCallArtifactBundle{
		dir:          dir,
		reportPath:   filepath.Join(dir, "report.md"),
		combinedPath: filepath.Join(dir, "combined_output.txt"),
		stdoutPath:   filepath.Join(dir, "stdout.txt"),
		stderrPath:   filepath.Join(dir, "stderr.txt"),
		manifestPath: filepath.Join(dir, "manifest.json"),
	}
	resultText, resultExt := stableResultText(rawResult)
	b.resultPath = filepath.Join(dir, "result"+resultExt)

	var persistErr error
	if err := os.MkdirAll(dir, 0o755); err != nil {
		persistErr = err
	}
	for path, content := range map[string]string{
		b.combinedPath: combined,
		b.stdoutPath:   stdout,
		b.stderrPath:   stderr,
		b.resultPath:   resultText,
	} {
		if persistErr != nil {
			break
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			persistErr = err
		}
	}

	hint := toolArtifactHint(b, persistErr)
	normalizeToolResultData(toolResult, combined, resultText, hint)
	if persistErr != nil {
		if ytoken.CalcTokenCount(combined)+ytoken.CalcTokenCount(resultText) > ToolResultTokenLimit {
			toolResult.Success = false
			if toolResult.Error != "" {
				toolResult.Error += "; "
			}
			toolResult.Error += "artifact_persist_failed: historical oversized output is unavailable"
			normalizeToolResultData(toolResult, combined, resultText, hint)
		}
		log.Warnf("failed to migrate legacy Timeline tool result %d: %v", toolResult.ID, persistErr)
		return true
	}

	manifest := toolCallArtifactManifest{
		Tool:       toolResult.Name,
		CallToolID: toolResult.ToolCallID,
		Identifier: "legacy-timeline-migration",
		Status:     map[bool]string{true: "success", false: "failed"}[toolResult.Success],
		Success:    toolResult.Success,
		Error:      toolResult.Error,
		Params:     toolResult.Param,
		CreatedAt:  time.Now(),
		Files: map[string]artifactFileStats{
			"combined_output": fileStats(b.combinedPath),
			"stdout":          fileStats(b.stdoutPath),
			"stderr":          fileStats(b.stderrPath),
			"result":          fileStats(b.resultPath),
		},
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err == nil {
		err = os.WriteFile(b.manifestPath, manifestData, 0o644)
	}
	if err == nil {
		report := fmt.Sprintf(
			"# Migrated Tool Call: %s\n\n## Execution Result Preview\n\n%s\n\n## Artifact Files\n\n- %s\n- %s\n- %s\n- %s\n- %s\n",
			toolResult.Name, utils.InterfaceToString(toolResult.Data), b.combinedPath, b.stdoutPath, b.stderrPath, b.resultPath, b.manifestPath,
		)
		err = os.WriteFile(b.reportPath, []byte(report), 0o644)
	}
	if err != nil {
		log.Warnf("legacy Timeline tool output migrated, but artifact index write failed for %d: %v", toolResult.ID, err)
	}
	return true
}
