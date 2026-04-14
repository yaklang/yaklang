package loop_plan

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestGetBaseFrameContext_BasicFields(t *testing.T) {
	ctx := context.Background()
	cfg := &aicommon.Config{}
	cfg.Ctx = ctx
	invoker := mock.NewMockInvoker(ctx)
	loop := reactloops.NewMinimalReActLoop(cfg, invoker)

	result := loop.GetBaseFrameContext()

	currentTime, ok := result["CurrentTime"]
	require.True(t, ok, "CurrentTime must be present")
	assert.NotEmpty(t, currentTime, "CurrentTime should not be empty")
	timeStr, ok := currentTime.(string)
	require.True(t, ok)
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`, timeStr)

	osArch, ok := result["OSArch"]
	require.True(t, ok, "OSArch must be present")
	assert.Contains(t, osArch, "/")
}

func TestGetBaseFrameContext_WithWorkdir(t *testing.T) {
	ctx := context.Background()
	cfg := &aicommon.Config{Workdir: "/tmp/test-workdir"}
	cfg.Ctx = ctx
	invoker := mock.NewMockInvoker(ctx)
	loop := reactloops.NewMinimalReActLoop(cfg, invoker)

	result := loop.GetBaseFrameContext()

	workDir, ok := result["WorkingDir"]
	require.True(t, ok, "WorkingDir must be present when Workdir is set")
	assert.Equal(t, "/tmp/test-workdir", workDir)
}

func TestGetBaseFrameContext_WithoutWorkdir(t *testing.T) {
	ctx := context.Background()
	cfg := &aicommon.Config{}
	cfg.Ctx = ctx
	invoker := mock.NewMockInvoker(ctx)
	loop := reactloops.NewMinimalReActLoop(cfg, invoker)

	result := loop.GetBaseFrameContext()

	_, ok := result["WorkingDir"]
	assert.False(t, ok, "WorkingDir should not be present when Workdir is empty")
}

func TestGetBaseFrameContext_WithTimeline(t *testing.T) {
	ctx := context.Background()
	timeline := aicommon.NewTimeline(nil, nil)
	cfg := &aicommon.Config{Timeline: timeline}
	cfg.Ctx = ctx
	invoker := mock.NewMockInvoker(ctx)
	loop := reactloops.NewMinimalReActLoop(cfg, invoker)

	result := loop.GetBaseFrameContext()

	_, ok := result["Timeline"]
	assert.True(t, ok, "Timeline should be present when Timeline is set")
}

func TestGetBaseFrameContext_MockedConfig(t *testing.T) {
	ctx := context.Background()
	invoker := mock.NewMockInvoker(ctx)
	loop := reactloops.NewMinimalReActLoop(invoker.GetConfig(), invoker)

	result := loop.GetBaseFrameContext()
	assert.NotEmpty(t, result["CurrentTime"])
	assert.NotEmpty(t, result["OSArch"])
}

func TestGuidanceDocumentTemplate_RendersBaseContext(t *testing.T) {
	data := map[string]any{
		"CurrentTime": "2026-04-14 12:00:00",
		"OSArch":      "darwin/arm64",
		"WorkingDir":  "/Users/test/project",
		"UserInput":   "optimize the crawler performance",
		"Nonce":       "ABCD",
	}
	rendered, err := utils.RenderTemplate(guidanceDocumentPrompt, data)
	require.NoError(t, err)

	assert.Contains(t, rendered, "2026-04-14 12:00:00")
	assert.Contains(t, rendered, "darwin/arm64")
	assert.Contains(t, rendered, "/Users/test/project")
	assert.Contains(t, rendered, "optimize the crawler performance")
}

func TestGuidanceDocumentTemplate_RendersTimeline(t *testing.T) {
	data := map[string]any{
		"CurrentTime": "2026-04-14 12:00:00",
		"OSArch":      "darwin/arm64",
		"UserInput":   "test task",
		"Timeline":    "- [12:00] Started scanning\n- [12:01] Found 50 endpoints",
		"Nonce":       "ABCD",
	}
	rendered, err := utils.RenderTemplate(guidanceDocumentPrompt, data)
	require.NoError(t, err)

	assert.Contains(t, rendered, "Timeline Memory")
	assert.Contains(t, rendered, "Started scanning")
	assert.Contains(t, rendered, "Found 50 endpoints")
}

func TestGuidanceDocumentTemplate_NoTimelineWhenEmpty(t *testing.T) {
	data := map[string]any{
		"CurrentTime": "2026-04-14 12:00:00",
		"OSArch":      "darwin/arm64",
		"UserInput":   "test task",
		"Nonce":       "ABCD",
	}
	rendered, err := utils.RenderTemplate(guidanceDocumentPrompt, data)
	require.NoError(t, err)

	assert.NotContains(t, rendered, "Timeline Memory")
}

func TestGuidanceDocumentTemplate_NoWorkingDirWhenEmpty(t *testing.T) {
	data := map[string]any{
		"CurrentTime": "2026-04-14 12:00:00",
		"OSArch":      "darwin/arm64",
		"UserInput":   "test task",
		"Nonce":       "ABCD",
	}
	rendered, err := utils.RenderTemplate(guidanceDocumentPrompt, data)
	require.NoError(t, err)

	assert.NotContains(t, rendered, "Working Dir")
}

func TestGuidanceDocumentTemplate_WithFactsAndEvidence(t *testing.T) {
	data := map[string]any{
		"CurrentTime": "2026-04-14 12:00:00",
		"OSArch":      "darwin/arm64",
		"UserInput":   "analyze the target",
		"Facts":       "- Target is running on port 8080\n- Framework: Spring Boot",
		"Evidence":    "Nmap scan results show open ports 80, 443, 8080",
		"Context":     "## Scan results\nDetailed scan output here",
		"Timeline":    "- [12:00] Task started",
		"WorkingDir":  "/home/user/project",
		"Nonce":       "ABCD",
	}
	rendered, err := utils.RenderTemplate(guidanceDocumentPrompt, data)
	require.NoError(t, err)

	assert.Contains(t, rendered, "Current Time: 2026-04-14 12:00:00")
	assert.Contains(t, rendered, "OS/Arch: darwin/arm64")
	assert.Contains(t, rendered, "Working Dir: /home/user/project")
	assert.Contains(t, rendered, "Timeline Memory")
	assert.Contains(t, rendered, "Task started")
	assert.Contains(t, rendered, "analyze the target")
	assert.Contains(t, rendered, "Target is running on port 8080")
	assert.Contains(t, rendered, "Nmap scan results")
	assert.Contains(t, rendered, "Detailed scan output here")
}

func TestEmitDocumentMarkdown_NonEmptyDocument(t *testing.T) {
	ctx := context.Background()
	var mu sync.Mutex
	var events []*schema.AiOutputEvent
	cfg := &aicommon.Config{
		Emitter: aicommon.NewEmitter("test-emit-doc", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, e)
			return e, nil
		}),
	}
	cfg.Ctx = ctx
	invoker := mock.NewMockInvoker(ctx)
	loop := reactloops.NewMinimalReActLoop(cfg, invoker)

	emitDocumentMarkdown(loop, "# Test Document\n\nSome content here.")
	if cfg.Emitter != nil {
		cfg.Emitter.WaitForStream()
	}

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, events, "should emit at least one event")
}

func TestEmitDocumentMarkdown_EmptyDocument(t *testing.T) {
	ctx := context.Background()
	var mu sync.Mutex
	var events []*schema.AiOutputEvent
	cfg := &aicommon.Config{
		Emitter: aicommon.NewEmitter("test-emit-doc-empty", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, e)
			return e, nil
		}),
	}
	cfg.Ctx = ctx
	invoker := mock.NewMockInvoker(ctx)
	loop := reactloops.NewMinimalReActLoop(cfg, invoker)

	emitDocumentMarkdown(loop, "")

	mu.Lock()
	defer mu.Unlock()
	assert.Empty(t, events, "should not emit events for empty document")
}

func TestEmitDocumentMarkdown_WhitespaceOnly(t *testing.T) {
	ctx := context.Background()
	var mu sync.Mutex
	var events []*schema.AiOutputEvent
	cfg := &aicommon.Config{
		Emitter: aicommon.NewEmitter("test-emit-doc-ws", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, e)
			return e, nil
		}),
	}
	cfg.Ctx = ctx
	invoker := mock.NewMockInvoker(ctx)
	loop := reactloops.NewMinimalReActLoop(cfg, invoker)

	emitDocumentMarkdown(loop, "   \n\t  \n  ")

	mu.Lock()
	defer mu.Unlock()
	assert.Empty(t, events, "should not emit events for whitespace-only document")
}

func TestGuidanceDocumentTemplate_SectionOrder(t *testing.T) {
	data := map[string]any{
		"CurrentTime": "2026-04-14 12:00:00",
		"OSArch":      "linux/amd64",
		"WorkingDir":  "/opt/project",
		"Timeline":    "timeline content",
		"UserInput":   "user request",
		"Facts":       "some facts",
		"Evidence":    "some evidence",
		"Context":     "some context",
		"Nonce":       "TEST",
	}
	rendered, err := utils.RenderTemplate(guidanceDocumentPrompt, data)
	require.NoError(t, err)

	timeIdx := strings.Index(rendered, "Current Time:")
	timelineIdx := strings.Index(rendered, "Timeline Memory")
	userInputIdx := strings.Index(rendered, "user request")
	factsIdx := strings.Index(rendered, "some facts")
	outputIdx := strings.Index(rendered, "# 输出要求")

	assert.Greater(t, timelineIdx, timeIdx, "Timeline should come after CurrentTime header")
	assert.Greater(t, userInputIdx, timelineIdx, "UserInput should come after Timeline")
	assert.Greater(t, factsIdx, userInputIdx, "Facts should come after UserInput")
	assert.Greater(t, outputIdx, factsIdx, "Output requirements should come after Facts")
}
