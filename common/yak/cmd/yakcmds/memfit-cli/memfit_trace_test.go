package memfitcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

func TestMemfitTraceWritesAndRedactsDiagnosticFiles(t *testing.T) {
	const secret = "sk-trace-secret"
	dir := t.TempDir()
	trace, err := newMemfitTrace(dir, secret)
	require.NoError(t, err)
	require.NotNil(t, trace)

	trace.recordStartConfig(memfitStartConfig{
		APIKey:  secret,
		Workdir: "/tmp/project",
		Focus:   "coding",
	})
	trace.recordProtocol("parent_to_worker", "start", "start-1", map[string]any{
		"api_key": secret,
		"focus":   "coding",
	})
	trace.recordAIEvent(&schema.AiOutputEvent{
		Type:    schema.EVENT_TYPE_STREAM,
		NodeId:  "answer",
		Content: []byte("content " + secret),
	})
	trace.recordAICall(aicommon.AICallTraceRecord{
		Prompt: "prompt " + secret,
		Output: "output " + secret,
	})
	trace.recordLine(memfitTraceWorkerLogFile, "worker log "+secret)
	trace.Close()

	for _, name := range []string{
		memfitTraceLifecycleFile,
		memfitTraceProtocolFile,
		memfitTraceAIEventsFile,
		memfitTraceAICallsFile,
		memfitTraceWorkerLogFile,
	} {
		path := filepath.Join(dir, name)
		raw, readErr := os.ReadFile(path)
		require.NoError(t, readErr, "expected trace file %s", name)
		require.NotContains(t, string(raw), secret, "trace file %s leaked the configured API key", name)
		if name != memfitTraceLifecycleFile {
			require.Contains(t, string(raw), "[REDACTED]", "trace file %s should record redaction evidence", name)
		}
	}
}

func TestMemfitTraceDisabledByEmptyDir(t *testing.T) {
	trace, err := newMemfitTrace("", "")
	require.NoError(t, err)
	require.Nil(t, trace)
	trace.recordJSONL("ignored.jsonl", "ignored", "value")
	trace.Close()
}

func TestMemfitTraceProducesAnalyzableAIEventText(t *testing.T) {
	trace, err := newMemfitTrace(t.TempDir(), "")
	require.NoError(t, err)
	trace.recordAIEvent(&schema.AiOutputEvent{
		Type:        schema.EVENT_TYPE_STRUCTURED,
		NodeId:      "tool_call_result",
		CallToolID:  "call-1",
		Content:     []byte(`{"ok":true}`),
		StreamDelta: []byte("delta"),
	})
	trace.Close()

	raw, err := os.ReadFile(filepath.Join(trace.Dir(), memfitTraceAIEventsFile))
	require.NoError(t, err)
	text := string(raw)
	require.Contains(t, text, `"tool_call_result"`)
	require.Contains(t, text, `"call_tool_id":"call-1"`)
	require.Contains(t, text, `"content":"{\"ok\":true}"`)
	require.False(t, strings.Contains(text, `"content":"eyJ`), "content must be plain text, not base64")
}
