package aicommon

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/schema"
)

func oversizedToolFixture() (string, string) {
	chunk := strings.Repeat("alpha beta gamma delta epsilon line-0123456789\n", 30000)
	combined := "COMBINED-HEAD\n" + chunk + "COMBINED-MIDDLE-SENTINEL\n" + chunk + "COMBINED-TAIL\n"
	result := "RESULT-HEAD\n" + chunk + "RESULT-MIDDLE-SENTINEL\n" + chunk + "RESULT-TAIL\n"
	return combined, result
}

func TestNormalizeToolResultDataHardTokenLimit(t *testing.T) {
	combined, result := oversizedToolFixture()
	require.Greater(t, ytoken.CalcTokenCount(combined)+ytoken.CalcTokenCount(result), 200000)

	toolResult := &aitool.ToolResult{
		ID:      1,
		Name:    "huge-output",
		Param:   map[string]any{"query": "example"},
		Success: true,
	}
	hint := "ARTIFACT:\n- combined: /tmp/artifacts/combined_output.txt\n- result: /tmp/artifacts/result.txt"
	normalizeToolResultData(toolResult, combined, result, hint)

	data, ok := toolResult.Data.(string)
	require.True(t, ok)
	require.LessOrEqual(t, ytoken.CalcTokenCount(data), ToolResultTokenLimit)
	require.LessOrEqual(t, ytoken.CalcTokenCount(toolResult.String()), ToolResultTokenLimit)
	require.Greater(t, ytoken.CalcTokenCount(data), 14000, "preview should fill most of the 16K budget")
	require.Contains(t, data, "RESULT-HEAD")
	require.Contains(t, data, "COMBINED-TAIL")
	require.Contains(t, data, hint)
	require.NotContains(t, data, "COMBINED-MIDDLE-SENTINEL")
	require.NotContains(t, data, "RESULT-MIDDLE-SENTINEL")
}

func TestToolResultOutputKindsOver200KTokens(t *testing.T) {
	huge := "KIND-HEAD\n" + strings.Repeat("record-0123456789 alpha beta gamma delta\n", 25000) + "KIND-MIDDLE-SENTINEL\n" + strings.Repeat("record-9876543210 epsilon zeta eta theta\n", 25000) + "KIND-TAIL\n"
	require.Greater(t, ytoken.CalcTokenCount(huge), 200000)
	hint := "ARTIFACT:\n- combined: /tmp/tool/combined_output.txt\n- result: /tmp/tool/result.txt"

	resultJSON, ext := stableResultText(map[string]any{"records": huge, "count": 50000})
	require.Equal(t, ".json", ext)
	cases := []struct {
		name     string
		combined string
		result   string
	}{
		{name: "stdout", combined: "[STDOUT]\n" + huge},
		{name: "stderr", combined: "[STDERR]\n" + huge},
		{name: "stdout_stderr_interleaved", combined: "[STDOUT]\n" + huge[:len(huge)/2] + "[STDERR]\n" + huge[len(huge)/2:]},
		{name: "result_string", result: huge},
		{name: "result_json", result: resultJSON},
		{name: "combined_and_result", combined: huge, result: huge + "RESULT-TAIL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			toolResult := &aitool.ToolResult{Name: tc.name, Success: true}
			normalizeToolResultData(toolResult, tc.combined, tc.result, hint)
			data := toolResult.Data.(string)
			require.LessOrEqual(t, ytoken.CalcTokenCount(data), ToolResultTokenLimit)
			require.LessOrEqual(t, ytoken.CalcTokenCount(toolResult.String()), ToolResultTokenLimit)
			require.Contains(t, data, "ARTIFACT:")
			require.NotContains(t, data, "KIND-MIDDLE-SENTINEL")
		})
	}
}

func TestNormalizeToolResultDataOmitsHugeTimelineParams(t *testing.T) {
	toolResult := &aitool.ToolResult{
		Name:    "huge-params",
		Param:   map[string]any{"payload": strings.Repeat("unique-param-value-0123456789 ", 30000)},
		Error:   strings.Repeat("large-error-0123456789 ", 5000),
		Success: false,
	}
	normalizeToolResultData(toolResult, "small output", "small result", "HINT:\nartifact_persist_failed")
	require.True(t, toolResult.OmitParamsInTimeline)
	require.LessOrEqual(t, ytoken.CalcTokenCount(toolResult.Data.(string)), ToolResultTokenLimit)
	require.LessOrEqual(t, ytoken.CalcTokenCount(toolResult.String()), ToolResultTokenLimit)
}

func TestLateInvocationErrorCannotPushTimelineItemOverLimit(t *testing.T) {
	combined, result := oversizedToolFixture()
	toolResult := &aitool.ToolResult{Name: "late-error", Success: true}
	normalizeToolResultData(toolResult, combined, result, "ARTIFACT:\n- combined: /tmp/late/combined_output.txt")
	toolResult.Success = false
	toolResult.Error = strings.Repeat("late-checkpoint-error-0123456789 ", 30000)

	enforceCanonicalToolResultLimit(toolResult)
	require.LessOrEqual(t, ytoken.CalcTokenCount(toolResult.Data.(string)), ToolResultTokenLimit)
	require.LessOrEqual(t, ytoken.CalcTokenCount(toolResult.String()), ToolResultTokenLimit)
	require.Contains(t, toolResult.Data.(string), "ARTIFACT:")
}

func TestToolArtifactCombinedOutputPreservesInterleaving(t *testing.T) {
	dir := t.TempDir()
	b := &toolCallArtifactBundle{
		dir:          dir,
		combinedPath: filepath.Join(dir, "combined_output.txt"),
		stdoutPath:   filepath.Join(dir, "stdout.txt"),
		stderrPath:   filepath.Join(dir, "stderr.txt"),
		preview:      newBoundedHeadTailBuffer(toolCapturePreviewBytes),
	}
	var err error
	b.combined, err = os.Create(b.combinedPath)
	require.NoError(t, err)
	b.stdout, err = os.Create(b.stdoutPath)
	require.NoError(t, err)
	b.stderr, err = os.Create(b.stderrPath)
	require.NoError(t, err)

	stdout := b.Writer(artifactStdout)
	stderr := b.Writer(artifactStderr)
	_, _ = ioWriteString(stdout, "out-1\n")
	_, _ = ioWriteString(stderr, "err-1\n")
	_, _ = ioWriteString(stdout, "out-2\n")
	_, _ = ioWriteString(stderr, "err-2\n")
	b.closeStreams()

	combined, err := os.ReadFile(b.combinedPath)
	require.NoError(t, err)
	require.Equal(t, "out-1\nerr-1\nout-2\nerr-2\n", string(combined))
	stdoutData, err := os.ReadFile(b.stdoutPath)
	require.NoError(t, err)
	require.Equal(t, "out-1\nout-2\n", string(stdoutData))
	require.Equal(t, sha256.Sum256([]byte("out-1\nout-2\n")), sha256.Sum256(stdoutData))
	stderrData, err := os.ReadFile(b.stderrPath)
	require.NoError(t, err)
	require.Equal(t, "err-1\nerr-2\n", string(stderrData))
	require.Equal(t, sha256.Sum256([]byte("err-1\nerr-2\n")), sha256.Sum256(stderrData))
}

func ioWriteString(w interface{ Write([]byte) (int, error) }, s string) (int, error) {
	return w.Write([]byte(s))
}

func TestBoundedToolUIWriterEmitsHeadMarkerAndTail(t *testing.T) {
	var dst bytes.Buffer
	w := newBoundedToolUIWriter(&dst)
	raw := "HEAD-SENTINEL\n" + strings.Repeat("head-fill-", 2000) +
		"OMITTED-MIDDLE-SENTINEL\n" + strings.Repeat("middle-fill-", 5000) + "\nTAIL-SENTINEL"
	n, err := w.Write([]byte(raw))
	require.NoError(t, err)
	require.Equal(t, len(raw), n)
	require.NoError(t, w.Finish())
	out := dst.String()
	require.Contains(t, out, "HEAD-SENTINEL")
	require.Contains(t, out, "TAIL-SENTINEL")
	require.Contains(t, out, "UI stream omitted")
	require.NotContains(t, out, "OMITTED-MIDDLE-SENTINEL")
	require.Less(t, len(out), len(raw)/2)
}

func TestStableResultTextJSON(t *testing.T) {
	first, ext := stableResultText(map[string]any{"z": 1, "a": []any{"x", 2}})
	second, secondExt := stableResultText(map[string]any{"a": []any{"x", 2}, "z": 1})
	require.Equal(t, ".json", ext)
	require.Equal(t, ext, secondExt)
	require.Equal(t, first, second)
}

func TestLegacyCheckpointMapDecodesForArtifactMigration(t *testing.T) {
	combined, stdout, stderr, result, ok := legacyExecutionParts(map[string]any{
		"stdout":          "old-stdout",
		"stderr":          "old-stderr",
		"combined_output": "old-stdout\nold-stderr",
		"result":          map[string]any{"status": "ok"},
	})
	require.True(t, ok)
	require.Equal(t, "old-stdout\nold-stderr", combined)
	require.Equal(t, "old-stdout", stdout)
	require.Equal(t, "old-stderr", stderr)
	require.Equal(t, map[string]any{"status": "ok"}, result)
}

func TestToolArtifactFinalizeReplacesDataAndPersistsRawFiles(t *testing.T) {
	dir := t.TempDir()
	events := make([]*schema.AiOutputEvent, 0)
	emitter := NewEmitter("artifact-test", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		events = append(events, event)
		return event, nil
	})
	caller := &ToolCaller{emitter: emitter}
	b := &toolCallArtifactBundle{
		dir:          dir,
		reportPath:   filepath.Join(dir, "report.md"),
		combinedPath: filepath.Join(dir, "combined_output.txt"),
		stdoutPath:   filepath.Join(dir, "stdout.txt"),
		stderrPath:   filepath.Join(dir, "stderr.txt"),
		manifestPath: filepath.Join(dir, "manifest.json"),
		preview:      newBoundedHeadTailBuffer(toolCapturePreviewBytes),
	}
	var err error
	b.combined, err = os.Create(b.combinedPath)
	require.NoError(t, err)
	b.stdout, err = os.Create(b.stdoutPath)
	require.NoError(t, err)
	b.stderr, err = os.Create(b.stderrPath)
	require.NoError(t, err)

	combined := "stdout-1\nstderr-1\nraw-middle-sentinel\nstdout-tail\n"
	_, err = b.Writer(artifactStdout).Write([]byte("stdout-1\n"))
	require.NoError(t, err)
	_, err = b.Writer(artifactStderr).Write([]byte("stderr-1\nraw-middle-sentinel\n"))
	require.NoError(t, err)
	_, err = b.Writer(artifactStdout).Write([]byte("stdout-tail\n"))
	require.NoError(t, err)
	resultValue := map[string]any{"status": "ok", "items": []int{1, 2, 3}}
	toolResult := &aitool.ToolResult{
		Name:       "artifact-test-tool",
		ToolCallID: "call-1",
		Param:      aitool.InvokeParams{"query": "x"},
		Success:    true,
		Data:       &aitool.ToolExecutionResult{Result: resultValue},
	}
	tool, err := aitool.New("artifact-test-tool", aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) { return nil, nil }))
	require.NoError(t, err)
	require.NoError(t, b.finalize(caller, tool, "call-1", "fixture", toolResult.Param.(aitool.InvokeParams), toolResult, 0, ""))

	require.IsType(t, "", toolResult.Data)
	require.Contains(t, toolResult.Data.(string), "ARTIFACT:")
	require.Contains(t, toolResult.Data.(string), b.combinedPath)
	raw, err := os.ReadFile(b.combinedPath)
	require.NoError(t, err)
	require.Equal(t, combined, string(raw))
	require.Equal(t, sha256.Sum256([]byte(combined)), sha256.Sum256(raw))
	require.FileExists(t, b.resultPath)
	require.FileExists(t, b.manifestPath)
	require.FileExists(t, b.reportPath)
	require.NotEmpty(t, events)
}

func TestNormalizeToolResultDataDeduplicatesExactCombinedAndResult(t *testing.T) {
	result := strings.Repeat("same-result-line\n", 100)
	toolResult := &aitool.ToolResult{Name: "dedupe", Success: true}
	normalizeToolResultData(toolResult, result, result, "ARTIFACT:\n/path")
	data := toolResult.Data.(string)
	// When observations equal the result, emit the semantic RESULT exactly once.
	require.Equal(t, 1, strings.Count(data, result))
	require.Contains(t, data, "RESULT:")
	require.NotContains(t, data, "OBSERVATIONS:")
}

func TestToolArtifactFailureNeverFallsBackToOversizedInlineData(t *testing.T) {
	combined, result := oversizedToolFixture()
	b := &toolCallArtifactBundle{
		prepare: fmt.Errorf("disk unavailable"),
		preview: newBoundedHeadTailBuffer(toolCapturePreviewBytes),
	}
	_, _ = b.preview.Write([]byte(combined))
	toolResult := &aitool.ToolResult{
		Name:    "artifact-failure",
		Success: true,
		Data:    &aitool.ToolExecutionResult{Result: result},
	}
	tool, err := aitool.New("artifact-failure", aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) { return nil, nil }))
	require.NoError(t, err)
	err = b.finalize(&ToolCaller{}, tool, "call-failure", "", nil, toolResult, 0, "")
	require.NoError(t, err)
	require.True(t, toolResult.Success, "artifact persistence is not invocation protocol completion")
	require.Empty(t, toolResult.Error)
	require.Contains(t, toolResult.Data.(string), "artifact_persist_failed")
	require.LessOrEqual(t, ytoken.CalcTokenCount(toolResult.Data.(string)), ToolResultTokenLimit)
	require.LessOrEqual(t, ytoken.CalcTokenCount(toolResult.String()), ToolResultTokenLimit)
	require.NotContains(t, toolResult.Data.(string), "COMBINED-MIDDLE-SENTINEL")
}

func TestSmallToolResultCanContinueWhenArtifactPersistenceFails(t *testing.T) {
	b := &toolCallArtifactBundle{
		prepare: fmt.Errorf("disk unavailable"),
		preview: newBoundedHeadTailBuffer(toolCapturePreviewBytes),
	}
	_, _ = b.preview.Write([]byte("small combined output"))
	toolResult := &aitool.ToolResult{
		Name:    "small-artifact-failure",
		Success: true,
		Data:    &aitool.ToolExecutionResult{Result: "small result"},
	}
	tool, err := aitool.New("small-artifact-failure", aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) { return nil, nil }))
	require.NoError(t, err)
	require.NoError(t, b.finalize(&ToolCaller{}, tool, "call-small-failure", "", nil, toolResult, 0, ""))
	require.True(t, toolResult.Success)
	require.Contains(t, toolResult.Data.(string), "small combined output")
	require.Contains(t, toolResult.Data.(string), "small result")
	require.Contains(t, toolResult.Data.(string), "artifact_persist_failed")
}

func TestCurrentCheckpointReplayKeepsCompactedDataByteStable(t *testing.T) {
	canonicalData := []struct {
		name string
		data string
	}{
		{
			name: "distinct_result",
			data: "RESULT:\nresult\n\nOBSERVATIONS:\npreview\n\nARTIFACT:\n- combined: /stable/original/combined_output.txt\n- result: /stable/original/result.txt",
		},
		{
			name: "combined_equals_result_keeps_only_result",
			data: "RESULT:\nsame compact bytes\n\nARTIFACT:\n- combined: /stable/original/combined_output.txt\n- result: /stable/original/result.txt",
		},
		{
			name: "empty_result_keeps_only_observations",
			data: "OBSERVATIONS:\nstdout only\n\nHINT:\nartifact_persist_failed: historical fixture",
		},
		{
			name: "legacy_combined_output_checkpoint",
			data: "COMBINED OUTPUT:\nlegacy preview\n\nARTIFACT:\n- combined: /stable/original/combined_output.txt\n- result: /stable/original/result.txt",
		},
	}
	for _, tc := range canonicalData {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			originalDir := filepath.Join(root, "tool_calls", "1_replay")
			replayDir := originalDir + "_2"
			require.NoError(t, os.MkdirAll(originalDir, 0o755))
			originalArtifact := filepath.Join(originalDir, "combined_output.txt")
			require.NoError(t, os.WriteFile(originalArtifact, []byte("stable-original-artifact"), 0o644))
			require.NoError(t, os.MkdirAll(replayDir, 0o755))
			b := &toolCallArtifactBundle{
				dir:          replayDir,
				combinedPath: filepath.Join(replayDir, "combined_output.txt"),
				stdoutPath:   filepath.Join(replayDir, "stdout.txt"),
				stderrPath:   filepath.Join(replayDir, "stderr.txt"),
				preview:      newBoundedHeadTailBuffer(toolCapturePreviewBytes),
				ownsDir:      true,
			}
			var err error
			b.combined, err = os.Create(b.combinedPath)
			require.NoError(t, err)
			b.stdout, err = os.Create(b.stdoutPath)
			require.NoError(t, err)
			b.stderr, err = os.Create(b.stderrPath)
			require.NoError(t, err)
			toolResult := &aitool.ToolResult{Name: "replay", Success: true, Data: tc.data}
			tool, err := aitool.New("replay", aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) { return nil, nil }))
			require.NoError(t, err)
			require.NoError(t, b.finalize(&ToolCaller{}, tool, "replay-call", "", nil, toolResult, 0, ""))
			require.Equal(t, tc.data, toolResult.Data, "checkpoint replay must be byte-stable")
			require.NoDirExists(t, replayDir, "replay must remove the speculative _2 bundle")
			originalBytes, err := os.ReadFile(originalArtifact)
			require.NoError(t, err)
			require.Equal(t, "stable-original-artifact", string(originalBytes))
		})
	}
}

func TestCanonicalToolResultDataRequiresFrameworkTailEnvelope(t *testing.T) {
	require.True(t, isCanonicalToolResultData(
		"RESULT:\nsemantic\n\nOBSERVATIONS:\nplain\n\nARTIFACT:\n- combined: /tmp/combined_output.txt\n- result: /tmp/result.json",
	))
	require.True(t, isCanonicalToolResultData(
		"OBSERVATIONS:\nplain\n\nHINT:\nartifact_persist_failed: disk unavailable. This is a bounded preview; omitted content is unrecoverable.",
	))
	require.True(t, isCanonicalToolResultData(
		"COMBINED OUTPUT:\nlegacy\n\nARTIFACT:\n- combined: /tmp/combined_output.txt\n- result: /tmp/result.txt",
	))
	for _, rawToolString := range []string{
		"RESULT:\nordinary text without a framework tail",
		"OBSERVATIONS:\nordinary text without a framework tail",
		"COMBINED OUTPUT:\nthis is ordinary tool text",
		"COMBINED OUTPUT:\nordinary text mentions\n\nARTIFACT:\nwithout framework paths",
		"COMBINED OUTPUT:\nordinary text\n\nARTIFACT:\n- combined: /tmp/only-one-path",
		"COMBINED OUTPUT:\nordinary text\n\nHINT:\nuser-authored hint",
		"prefix COMBINED OUTPUT:\nplain\n\nARTIFACT:\n- combined: /tmp/a\n- result: /tmp/b",
	} {
		require.False(t, isCanonicalToolResultData(rawToolString), rawToolString)
	}
}

func TestToolArtifactDiscardUnfinishedClosesStreamsAndRemovesBundle(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tool_calls", "1_cancelled")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	b := &toolCallArtifactBundle{
		dir:          dir,
		combinedPath: filepath.Join(dir, "combined_output.txt"),
		stdoutPath:   filepath.Join(dir, "stdout.txt"),
		stderrPath:   filepath.Join(dir, "stderr.txt"),
		preview:      newBoundedHeadTailBuffer(toolCapturePreviewBytes),
		ownsDir:      true,
	}
	var err error
	b.combined, err = os.Create(b.combinedPath)
	require.NoError(t, err)
	b.stdout, err = os.Create(b.stdoutPath)
	require.NoError(t, err)
	b.stderr, err = os.Create(b.stderrPath)
	require.NoError(t, err)
	stdout := b.Writer(artifactStdout)
	_, err = stdout.Write([]byte("partial-before-cancel"))
	require.NoError(t, err)

	b.discardIfUnfinished()
	require.NoDirExists(t, dir)
	// Simulate a non-cooperative callback unwinding after ToolCaller returned.
	// Late writes are consumed and dropped instead of touching closed files.
	n, err := stdout.Write([]byte("late-after-cancel"))
	require.NoError(t, err)
	require.Equal(t, len("late-after-cancel"), n)
	require.NoDirExists(t, dir)
}

func TestToolArtifactDiscardDoesNotRemoveUnownedExistingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tool_calls", "existing")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	sentinel := filepath.Join(dir, "sentinel.txt")
	require.NoError(t, os.WriteFile(sentinel, []byte("owned elsewhere"), 0o644))
	b := &toolCallArtifactBundle{dir: dir, prepare: fmt.Errorf("reservation failed")}
	require.NoError(t, b.discardIfUnfinished())
	require.FileExists(t, sentinel, "failed reservation must never delete a directory the bundle did not create")
}

func TestToolArtifactLateFinalizeFailureDoesNotLeaveGhostArtifactHint(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tool_calls", "late-persist-failure")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	b := &toolCallArtifactBundle{
		dir:          dir,
		reportPath:   filepath.Join(dir, "report.md"),
		combinedPath: filepath.Join(dir, "combined_output.txt"),
		stdoutPath:   filepath.Join(dir, "stdout.txt"),
		stderrPath:   filepath.Join(dir, "stderr.txt"),
		manifestPath: filepath.Join(dir, "missing-parent", "manifest.json"),
		preview:      newBoundedHeadTailBuffer(toolCapturePreviewBytes),
		ownsDir:      true,
	}
	var err error
	b.combined, err = os.Create(b.combinedPath)
	require.NoError(t, err)
	b.stdout, err = os.Create(b.stdoutPath)
	require.NoError(t, err)
	b.stderr, err = os.Create(b.stderrPath)
	require.NoError(t, err)
	_, err = b.Writer(artifactStdout).Write([]byte("small output"))
	require.NoError(t, err)
	toolResult := &aitool.ToolResult{Name: "late-persist", Success: true, Data: &aitool.ToolExecutionResult{Result: "small result"}}
	tool, err := aitool.New("late-persist", aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) { return nil, nil }))
	require.NoError(t, err)
	err = b.finalize(&ToolCaller{}, tool, "late-call", "", nil, toolResult, 0, "")
	require.NoError(t, err, "artifact persistence is an observation failure after protocol completion")
	require.True(t, toolResult.Success)
	data := toolResult.Data.(string)
	require.Contains(t, data, "HINT:\nartifact_persist_failed:")
	require.NotContains(t, data, "\n\nARTIFACT:\n")
	require.NoError(t, b.discardIfUnfinished())
	require.NoDirExists(t, dir)
}

func TestReserveToolArtifactDirNeverOverwritesExistingBundle(t *testing.T) {
	base := filepath.Join(t.TempDir(), "tool_calls", "1_replay")
	require.NoError(t, os.MkdirAll(base, 0o755))
	original := filepath.Join(base, "combined_output.txt")
	require.NoError(t, os.WriteFile(original, []byte("original-artifact"), 0o644))

	reserved, err := reserveToolArtifactDir(base)
	require.NoError(t, err)
	require.Equal(t, base+"_2", reserved)
	content, err := os.ReadFile(original)
	require.NoError(t, err)
	require.Equal(t, "original-artifact", string(content))
}

func TestArtifactFileStatsCountsFinalLineWithoutNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result.txt")
	require.NoError(t, os.WriteFile(path, []byte("one\ntwo"), 0o644))
	require.EqualValues(t, 2, fileStats(path).Lines)
}

func TestLegacyTimelineToolResultMigratesToArtifactAndCanonicalData(t *testing.T) {
	workdir := t.TempDir()
	huge := "LEGACY-HEAD\n" + strings.Repeat("legacy-record alpha beta gamma\n", 30000) + "LEGACY-MIDDLE-SENTINEL\n" + strings.Repeat("legacy-tail-record delta epsilon\n", 30000) + "LEGACY-TAIL\n"
	timeline := NewTimeline(nil, nil)
	timeline.PushToolResult(&aitool.ToolResult{
		ID:      77,
		Name:    "legacy/tool",
		Success: true,
		Data: &aitool.ToolExecutionResult{
			Stdout:         huge,
			CombinedOutput: huge,
			Result:         map[string]any{"status": "ok", "tail": "result-tail"},
		},
	})

	serialized, err := MarshalTimeline(timeline)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(serialized)
	require.NoError(t, err)
	require.True(t, restored.migrateLegacyToolResults(workdir))
	require.False(t, restored.migrateLegacyToolResults(workdir), "canonical Data must not bootstrap twice")

	item, ok := restored.idToTimelineItem.Get(77)
	require.True(t, ok)
	result := item.GetValue().(*aitool.ToolResult)
	data, ok := result.Data.(string)
	require.True(t, ok)
	require.Contains(t, data, "RESULT:\n")
	require.Contains(t, data, "OBSERVATIONS:\nLEGACY-HEAD")
	require.Contains(t, data, "result-tail")
	require.Less(t, strings.Index(data, "RESULT:\n"), strings.Index(data, "OBSERVATIONS:\n"))
	require.Contains(t, data, "ARTIFACT:\n- combined:")
	require.NotContains(t, data, "LEGACY-MIDDLE-SENTINEL")
	require.LessOrEqual(t, ytoken.CalcTokenCount(data), ToolResultTokenLimit)
	require.LessOrEqual(t, ytoken.CalcTokenCount(result.String()), ToolResultTokenLimit)

	bundleDir := filepath.Join(workdir, "task_legacy", "tool_calls", "77_legacy_tool_migrated")
	rawCombined, err := os.ReadFile(filepath.Join(bundleDir, "combined_output.txt"))
	require.NoError(t, err)
	require.Equal(t, huge, string(rawCombined))
	require.FileExists(t, filepath.Join(bundleDir, "stdout.txt"))
	require.FileExists(t, filepath.Join(bundleDir, "stderr.txt"))
	require.FileExists(t, filepath.Join(bundleDir, "result.json"))
	require.FileExists(t, filepath.Join(bundleDir, "manifest.json"))
	require.FileExists(t, filepath.Join(bundleDir, "report.md"))

	var timestampValue *aitool.ToolResult
	restored.tsToTimelineItem.ForEach(func(_ int64, timelineItem *TimelineItem) bool {
		if timelineItem.GetID() == 77 {
			timestampValue = timelineItem.GetValue().(*aitool.ToolResult)
		}
		return true
	})
	require.Same(t, result, timestampValue, "all restored Timeline indexes must share the migrated value")

	remarshaled, err := MarshalTimeline(restored)
	require.NoError(t, err)
	require.NotContains(t, remarshaled, "LEGACY-MIDDLE-SENTINEL")
}

func BenchmarkNormalizeToolResultData200KTokens(b *testing.B) {
	combined, result := oversizedToolFixture()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		toolResult := &aitool.ToolResult{Name: fmt.Sprintf("bench-%d", i), Success: true}
		normalizeToolResultData(toolResult, combined, result, "ARTIFACT:\n/tmp/artifact")
	}
}
