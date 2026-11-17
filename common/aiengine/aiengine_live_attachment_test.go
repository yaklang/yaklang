//go:build live

package aiengine_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/gorm"
	_ "github.com/yaklang/gorm/dialects/sqlite"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiconfig"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/aiengine"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	// Register the production ReAct engine factory.
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact"
	// Register the Yak runtime used by the production read_file tool.
	_ "github.com/yaklang/yaklang/common/yak"
)

func loadLiveAIConfigFromDatabase(t *testing.T) bool {
	t.Helper()
	dbPath := strings.TrimSpace(os.Getenv("YAK_LIVE_AI_DB"))
	if dbPath == "" {
		return false
	}
	db, err := gorm.Open("sqlite3", "file:"+dbPath+"?mode=ro")
	if err != nil {
		t.Fatalf("open live AI config database: %v", err)
	}
	defer func() { _ = db.Close() }()

	raw := yakit.GetKey(db, consts.AI_GLOBAL_CONFIG_KEY)
	if strings.TrimSpace(raw) == "" {
		t.Fatalf("database %s has no %s", dbPath, consts.AI_GLOBAL_CONFIG_KEY)
	}
	cfg := &ypb.AIGlobalConfig{}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		t.Fatalf("decode %s from %s: %v", consts.AI_GLOBAL_CONFIG_KEY, dbPath, err)
	}
	if err := yakit.ApplyAIGlobalConfig(nil, cfg); err != nil {
		t.Fatalf("apply %s from %s: %v", consts.AI_GLOBAL_CONFIG_KEY, dbPath, err)
	}

	describeModel := func(models []*ypb.AIModelConfig) string {
		if len(models) == 0 || models[0] == nil {
			return "<empty>"
		}
		providerType := "<nil>"
		if models[0].GetProvider() != nil {
			providerType = models[0].GetProvider().GetType()
		}
		return providerType + ":" + models[0].GetModelName()
	}
	t.Logf(
		"loaded AI config from %s without printing secrets: intelligent=%s lightweight=%s",
		dbPath, describeModel(cfg.GetIntelligentModels()), describeModel(cfg.GetLightweightModels()),
	)
	firstKey := ""
	if first := consts.FirstIntelligentModel(cfg.GetIntelligentModels()); first != nil && first.GetProvider() != nil {
		firstKey = first.GetProvider().GetAPIKey()
	}
	builtinKey := yakit.DefaultAIBalanceProviderConfig().GetAPIKey()
	t.Logf(
		"database API key applied: present=%v differs_from_builtin=%v",
		firstKey != "", firstKey != builtinKey,
	)
	return true
}

type livePromptRecord struct {
	phase  string
	prompt string
}

type liveAttachmentCapture struct {
	mu      sync.Mutex
	phase   string
	prompts []livePromptRecord
	streams map[string]*strings.Builder
	events  []string
}

func newLiveAttachmentCapture() *liveAttachmentCapture {
	return &liveAttachmentCapture{streams: make(map[string]*strings.Builder)}
}

func (c *liveAttachmentCapture) beginPhase(phase string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.phase = phase
	c.streams = make(map[string]*strings.Builder)
}

func (c *liveAttachmentCapture) callback(real aicommon.AICallbackType) aicommon.AICallbackType {
	return func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		prompt := req.GetPrompt()
		c.mu.Lock()
		c.prompts = append(c.prompts, livePromptRecord{phase: c.phase, prompt: prompt})
		c.mu.Unlock()
		return real(config, req)
	}
}

func (c *liveAttachmentCapture) recordStream(nodeID string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	builder := c.streams[nodeID]
	if builder == nil {
		builder = &strings.Builder{}
		c.streams[nodeID] = builder
	}
	builder.Write(data)
}

func (c *liveAttachmentCapture) recordEvent(event *schema.AiOutputEvent) {
	if event == nil {
		return
	}
	switch event.Type {
	case schema.EVENT_TYPE_STRUCTURED:
		if strings.HasPrefix(event.NodeId, "react_task_") {
			c.mu.Lock()
			c.events = append(c.events, event.NodeId+" "+string(event.Content))
			c.mu.Unlock()
		}
	case "error":
		c.mu.Lock()
		c.events = append(c.events, "error "+string(event.Content))
		c.mu.Unlock()
	}
}

func (c *liveAttachmentCapture) promptsInPhase(phase string) []livePromptRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	var result []livePromptRecord
	for _, record := range c.prompts {
		if record.phase == phase {
			result = append(result, record)
		}
	}
	return result
}

func (c *liveAttachmentCapture) answerText() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var builder strings.Builder
	for nodeID, stream := range c.streams {
		if strings.Contains(nodeID, "answer") {
			builder.WriteString(stream.String())
		}
	}
	return builder.String()
}

func (c *liveAttachmentCapture) streamSummary() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var parts []string
	for nodeID, stream := range c.streams {
		parts = append(parts, fmt.Sprintf("%s=%q", nodeID, stream.String()))
	}
	return strings.Join(parts, "\n")
}

func markerExcerpt(prompt, marker string) string {
	index := strings.Index(prompt, marker)
	if index < 0 {
		return "(marker absent)"
	}
	start := index - 220
	if start < 0 {
		start = 0
	}
	end := index + len(marker) + 220
	if end > len(prompt) {
		end = len(prompt)
	}
	return prompt[start:end]
}

func truncateLiveLog(content string, limit int) string {
	if len(content) <= limit {
		return content
	}
	return content[:limit] + fmt.Sprintf("\n... (truncated, total %d bytes)", len(content))
}

func TestLiveAIChatSmoke(t *testing.T) {
	if os.Getenv("YAK_LIVE_ATTACHMENT_TEST") != "1" {
		t.Skip("set YAK_LIVE_ATTACHMENT_TEST=1 to run the real AI smoke check")
	}
	provider := os.Getenv("YAK_LIVE_ATTACHMENT_PROVIDER")
	if provider == "default" {
		_ = consts.GetGormProfileDatabase()
		if !loadLiveAIConfigFromDatabase(t) {
			aiconfig.EnsureConfigLoaded()
		}
		answer, err := ai.Chat("Reply with exactly: OK")
		t.Logf("default provider answer=%q err=%v", answer, err)
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(answer) == "" {
			t.Fatal("real ai.Chat returned an empty answer")
		}
		return
	}
	if provider == "" {
		provider = "openai"
	}
	keyEnv := "OPENAI_API_KEY"
	if provider == "aibalance" {
		keyEnv = "AIBALANCE_API_KEY"
	}
	key := strings.TrimSpace(os.Getenv(keyEnv))
	if key == "" {
		t.Fatalf("%s is not set", keyEnv)
	}
	model := strings.TrimSpace(os.Getenv("YAK_LIVE_ATTACHMENT_MODEL"))
	if model == "" {
		model = "gpt-4o-mini"
	}
	answer, err := ai.Chat(
		"Reply with exactly: OK",
		aispec.WithType(provider),
		aispec.WithAPIKey(key),
		aispec.WithModel(model),
		aispec.WithTimeout(45),
	)
	t.Logf("provider=%s model=%s answer=%q err=%v", provider, model, answer, err)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(answer) == "" {
		t.Fatal("real ai.Chat returned an empty answer")
	}
}

func TestLiveAttachedFileExternalChange(t *testing.T) {
	// Run with:
	// YAK_LIVE_ATTACHMENT_TEST=1 go test -tags live ./common/aiengine \
	//   -run '^TestLiveAttachedFileExternalChange$' -count=1 -v -timeout 10m
	if os.Getenv("YAK_LIVE_ATTACHMENT_TEST") != "1" {
		t.Skip("set YAK_LIVE_ATTACHMENT_TEST=1 to run the real-agent attached-file check")
	}
	_ = consts.GetGormProfileDatabase()
	if !loadLiveAIConfigFromDatabase(t) {
		aiconfig.EnsureConfigLoaded()
	}

	const (
		markerV1 = "REAL_FILE_MARKER_V1_7f3a"
		markerV2 = "REAL_FILE_MARKER_V2_9c1e"
		markerV3 = "REAL_FILE_MARKER_V3_4b8d"
	)

	workdir := t.TempDir()
	path := filepath.Join(workdir, "live-changing.txt")
	if err := os.WriteFile(path, []byte(markerV1+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	capture := newLiveAttachmentCapture()
	realCallback := aicommon.AIChatToAICallbackType(func(prompt string, opts ...aispec.AIConfigOption) (string, error) {
		return ai.Chat(prompt, opts...)
	})
	callback := capture.callback(realCallback)

	mutated := false
	var mutatedAt time.Time
	wrappedCallback := func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		capture.mu.Lock()
		firstAttachmentPrompt := !mutated && strings.Contains(req.GetPrompt(), markerV1)
		if firstAttachmentPrompt {
			mutated = true
			mutatedAt = time.Now()
		}
		capture.mu.Unlock()

		if firstAttachmentPrompt {
			// The prompt has already been constructed from the V1 snapshot.
			// Mutate only after that, while the same ReAct task is still running.
			if err := os.WriteFile(path, []byte(markerV2+"\n"), 0o600); err != nil {
				return nil, err
			}
			t.Logf("external file changed to V2 at %s while the ReAct task is running", mutatedAt.Format(time.RFC3339Nano))
		}
		return callback(config, req)
	}

	engine, err := aiengine.NewAIEngine(
		aiengine.WithAICallback(wrappedCallback),
		aiengine.WithSessionID(fmt.Sprintf("live-attachment-%d", time.Now().UnixNano())),
		aiengine.WithMaxIteration(5),
		aiengine.WithTimeout(300),
		aiengine.WithWorkdir(workdir),
		aiengine.WithYOLOMode(),
		aiengine.WithDisableToolUse(true),
		aiengine.WithDisableAIForge(true),
		aiengine.WithDisableMCPServers(true),
		aiengine.WithEnableAISearchTool(false),
		aiengine.WithEnableForgeSearchTool(false),
		aiengine.WithAttachedFilePath(path),
		aiengine.WithOnStream(func(_ aicommon.AIEngineOperator, _ *schema.AiOutputEvent, nodeID string, data []byte) {
			capture.recordStream(nodeID, data)
		}),
		aiengine.WithOnEvent(func(_ aicommon.AIEngineOperator, event *schema.AiOutputEvent) {
			capture.recordEvent(event)
		}),
		aiengine.WithExtOptions(
			aicommon.WithAICallback(wrappedCallback),
			aicommon.WithDisableAutoSkills(true),
			aicommon.WithDisableIntentRecognition(true),
			aicommon.WithDisableSessionTitleGeneration(true),
			aicommon.WithEnableFunctionCallMode(false),
			aicommon.WithNoOpMemoryTriage(),
			aicommon.WithDisablePerception(true),
			aicommon.WithDisableCreateDBRuntime(true),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	const turn1Question = "The user pinned a file. Find the exact marker string beginning with REAL_FILE_MARKER_ in the attached file preview in THIS prompt. Use the directly_answer action and put only that full marker string in answer_payload. Do not call any tools."
	capture.beginPhase("turn1")
	if err := engine.SendMsg(turn1Question); err != nil {
		t.Fatalf("turn 1 failed: %v\nstreams:\n%s\nevents:\n%s", err, truncateLiveLog(capture.streamSummary(), 4000), strings.Join(capture.events, "\n"))
	}

	turn1Prompts := capture.promptsInPhase("turn1")
	if len(turn1Prompts) == 0 {
		t.Fatal("turn 1 made no real model calls")
	}
	var turn1PromptCountV1, turn1PromptCountV2 int
	for _, record := range turn1Prompts {
		if strings.Contains(record.prompt, markerV1) {
			turn1PromptCountV1++
		}
		if strings.Contains(record.prompt, markerV2) {
			turn1PromptCountV2++
		}
	}
	if turn1PromptCountV1 == 0 {
		t.Fatalf("turn 1 never saw the prepared V1 snapshot; prompts=%d", len(turn1Prompts))
	}
	if !mutated {
		t.Fatal("the external file mutation was never triggered")
	}
	diskAfterTurn1, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(diskAfterTurn1)) != markerV2 {
		t.Fatalf("disk marker after turn 1 = %q, want V2", strings.TrimSpace(string(diskAfterTurn1)))
	}
	if turn1PromptCountV2 != 0 {
		t.Fatalf("within the same task, a later prompt unexpectedly saw the external V2 change")
	}
	turn1Answer := capture.answerText()
	if !strings.Contains(turn1Answer, markerV1) {
		t.Fatalf("turn 1 real answer did not report V1: %q\nstreams:\n%s", turn1Answer, truncateLiveLog(capture.streamSummary(), 4000))
	}
	if strings.Contains(turn1Answer, markerV2) {
		t.Fatalf("turn 1 real answer reported V2 even though no V2 prompt existed: %q", turn1Answer)
	}
	t.Logf("turn 1: model_calls=%d prompts_with_V1=%d prompts_with_V2=%d answer=%q", len(turn1Prompts), turn1PromptCountV1, turn1PromptCountV2, strings.TrimSpace(turn1Answer))
	t.Logf("turn 1 V1 prompt excerpt:\n%s", markerExcerpt(turn1Prompts[0].prompt, markerV1))

	if err := os.WriteFile(path, []byte(markerV3+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const turn2Question = "The user pinned a file again. If this prompt contains more than one attached-file snapshot, use the snapshot appended LAST in the prompt. Use the directly_answer action and put only the full exact marker string from that newest snapshot in answer_payload. Do not call any tools."
	capture.beginPhase("turn2")
	if err := engine.SendMsg(turn2Question); err != nil {
		t.Fatalf("turn 2 failed: %v\nstreams:\n%s\nevents:\n%s", err, truncateLiveLog(capture.streamSummary(), 4000), strings.Join(capture.events, "\n"))
	}

	turn2Prompts := capture.promptsInPhase("turn2")
	if len(turn2Prompts) == 0 {
		t.Fatal("turn 2 made no real model calls")
	}
	var turn2PromptCountV1, turn2PromptCountV2, turn2PromptCountV3 int
	for _, record := range turn2Prompts {
		if strings.Contains(record.prompt, markerV1) {
			turn2PromptCountV1++
		}
		if strings.Contains(record.prompt, markerV2) {
			turn2PromptCountV2++
		}
		if strings.Contains(record.prompt, markerV3) {
			turn2PromptCountV3++
		}
	}
	if turn2PromptCountV3 == 0 {
		t.Fatalf("turn 2 never re-read the externally changed V3 file; prompts=%d", len(turn2Prompts))
	}
	turn2Answer := capture.answerText()
	if !strings.Contains(turn2Answer, markerV3) {
		t.Fatalf("turn 2 real answer did not report V3: %q\nstreams:\n%s", turn2Answer, truncateLiveLog(capture.streamSummary(), 4000))
	}
	t.Logf("turn 2: model_calls=%d prompts_with_V1=%d prompts_with_V2=%d prompts_with_V3=%d answer=%q", len(turn2Prompts), turn2PromptCountV1, turn2PromptCountV2, turn2PromptCountV3, strings.TrimSpace(turn2Answer))
	t.Logf("turn 2 V3 prompt excerpt:\n%s", markerExcerpt(turn2Prompts[0].prompt, markerV3))
}

type liveToolObservation struct {
	mu         sync.Mutex
	calls      []string
	lastResult string
}

func (o *liveToolObservation) record(name string, result any) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.calls = append(o.calls, name)
	if result != nil {
		o.lastResult = fmt.Sprint(result)
	}
}

func (o *liveToolObservation) count() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.calls)
}

func (o *liveToolObservation) result() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.lastResult
}

func liveFileSHA256(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum[:])
}

func replaceLiveFileAtomically(t *testing.T, path string, content string) {
	t.Helper()
	tempPath := path + ".next"
	if err := os.WriteFile(tempPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		t.Fatal(err)
	}
}

func loadLiveReadFileTool(t *testing.T) *aitool.Tool {
	t.Helper()
	source, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/fs/read_file.yak")
	if err != nil {
		t.Fatalf("load production read_file tool source: %v", err)
	}
	spec := yakscripttools.LoadYakScriptToAiTools("read_file", string(source))
	if spec == nil {
		t.Fatal("parse production read_file tool metadata")
	}
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{spec})
	if len(tools) != 1 || tools[0] == nil {
		t.Fatalf("convert production read_file tool: got %d tools", len(tools))
	}
	return tools[0]
}

func newLiveObservedReadFileTool(t *testing.T, observation *liveToolObservation) *aitool.Tool {
	t.Helper()
	tool := loadLiveReadFileTool(t)
	callback := tool.Callback
	tool.Callback = func(ctx context.Context, params aitool.InvokeParams, runtimeConfig *aitool.ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error) {
		result, err := callback(ctx, params, runtimeConfig, stdout, stderr)
		observation.record("read_file", result)
		return result, err
	}
	return tool
}

func newLiveSnapshotInspectTool(t *testing.T, baseline map[string]string, observation *liveToolObservation) *aitool.Tool {
	t.Helper()
	tool, err := aitool.New(
		"inspect_attached_file",
		aitool.WithDescription("Compare a currently attached file with the exact SHA256 snapshot captured when it was attached."),
		aitool.WithUsage("Use this tool to verify whether an attached file changed externally. Return changed=true only when the content SHA256 differs from the attachment snapshot."),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithStringParam(
			"path",
			aitool.WithParam_Description("Absolute path of the previously attached file."),
			aitool.WithParam_Required(true),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			rawPath, hasPath := params["path"]
			path := strings.TrimSpace(utils.InterfaceToString(rawPath))
			baselineHash, ok := baseline[path]
			if !ok {
				return nil, fmt.Errorf("path %q has no attached-file baseline (has_path=%v raw_params=%#v)", path, hasPath, params)
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			currentHash := liveFileSHA256(content)
			result := map[string]any{
				"path":            path,
				"changed":         currentHash != baselineHash,
				"snapshot_sha256": baselineHash,
				"current_sha256":  currentHash,
				"current_marker":  strings.TrimSpace(string(content)),
			}
			raw, err := json.Marshal(result)
			if err != nil {
				return nil, err
			}
			observation.record("inspect_attached_file", string(raw))
			return string(raw), nil
		}),
	)
	if err != nil {
		t.Fatalf("create inspect_attached_file experiment tool: %v", err)
	}
	return tool
}

func liveAnswerReportsChangedTrue(answer string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(answer, " ", ""))
	return strings.Contains(normalized, "changed=true") ||
		strings.Contains(normalized, "changed:true") ||
		strings.Contains(normalized, `"changed":true`)
}

func TestLiveAttachedFileChangeAwareness(t *testing.T) {
	if os.Getenv("YAK_LIVE_ATTACHMENT_TEST") != "1" {
		t.Skip("set YAK_LIVE_ATTACHMENT_TEST=1 to run the real-agent attached-file change experiment")
	}
	_ = consts.GetGormProfileDatabase()
	if !loadLiveAIConfigFromDatabase(t) {
		aiconfig.EnsureConfigLoaded()
	}

	const (
		markerV1 = "LIVE_CHANGE_MARKER_V1_7f3a"
		markerV2 = "LIVE_CHANGE_MARKER_V2_9c1e"
	)
	initialContent := markerV1 + "\n"
	updatedContent := markerV2 + "\n"
	baselineHash := liveFileSHA256([]byte(initialContent))

	realCallback := aicommon.AIChatToAICallbackType(func(prompt string, opts ...aispec.AIConfigOption) (string, error) {
		return ai.Chat(prompt, opts...)
	})

	runScenario := func(
		name string,
		disableToolUse bool,
		toolFactory func(path string, observation *liveToolObservation) []*aitool.Tool,
		turn2Question func(path string) string,
		verify func(t *testing.T, observation *liveToolObservation, prompts []livePromptRecord, answer string),
	) {
		t.Run(name, func(t *testing.T) {
			workdir := t.TempDir()
			path := filepath.Join(workdir, "live-change-awareness.txt")
			if err := os.WriteFile(path, []byte(initialContent), 0o600); err != nil {
				t.Fatal(err)
			}

			capture := newLiveAttachmentCapture()
			observation := &liveToolObservation{}
			var tools []*aitool.Tool
			if toolFactory != nil {
				tools = toolFactory(path, observation)
			}
			wrappedCallback := capture.callback(realCallback)
			extOptions := []aicommon.ConfigOption{
				aicommon.WithAICallback(wrappedCallback),
				aicommon.WithDisableAutoSkills(true),
				aicommon.WithDisableIntentRecognition(true),
				aicommon.WithDisableSessionTitleGeneration(true),
				aicommon.WithEnableFunctionCallMode(false),
				aicommon.WithNoOpMemoryTriage(),
				aicommon.WithDisablePerception(true),
				aicommon.WithDisableCreateDBRuntime(true),
			}
			if len(tools) > 0 {
				extOptions = append(extOptions, aicommon.WithAiToolManagerOptions(buildinaitools.WithOnlyTools(tools...)))
			} else {
				extOptions = append(extOptions, aicommon.WithAiToolManagerOptions(buildinaitools.WithOnlyTools()))
			}

			engine, err := aiengine.NewAIEngine(
				aiengine.WithAICallback(wrappedCallback),
				aiengine.WithSessionID(fmt.Sprintf("live-change-awareness-%s-%d", name, time.Now().UnixNano())),
				aiengine.WithMaxIteration(6),
				aiengine.WithTimeout(300),
				aiengine.WithWorkdir(workdir),
				aiengine.WithYOLOMode(),
				aiengine.WithDisableToolUse(disableToolUse),
				aiengine.WithDisableAIForge(true),
				aiengine.WithDisableMCPServers(true),
				aiengine.WithEnableAISearchTool(false),
				aiengine.WithEnableForgeSearchTool(false),
				aiengine.WithOnStream(func(_ aicommon.AIEngineOperator, _ *schema.AiOutputEvent, nodeID string, data []byte) {
					capture.recordStream(nodeID, data)
				}),
				aiengine.WithOnEvent(func(_ aicommon.AIEngineOperator, event *schema.AiOutputEvent) {
					capture.recordEvent(event)
				}),
				aiengine.WithExtOptions(extOptions...),
			)
			if err != nil {
				t.Fatal(err)
			}
			defer engine.Close()

			const turn1Question = "The user attached a file for this message only. Find the exact marker beginning with LIVE_CHANGE_MARKER_ in the attached-file preview in THIS prompt. Use directly_answer and put only that marker in answer_payload. Do not call any tools."
			capture.beginPhase("turn1")
			if err := engine.SendMsg(turn1Question, aiengine.WithAttachedFilePath(path)); err != nil {
				t.Fatalf("turn 1 failed: %v\nstreams:\n%s\nevents:\n%s", err, truncateLiveLog(capture.streamSummary(), 4000), strings.Join(capture.events, "\n"))
			}

			turn1Prompts := capture.promptsInPhase("turn1")
			turn1Answer := strings.TrimSpace(capture.answerText())
			if len(turn1Prompts) == 0 || !strings.Contains(turn1Answer, markerV1) {
				t.Fatalf("turn 1 did not consume the V1 attachment: prompts=%d answer=%q", len(turn1Prompts), turn1Answer)
			}
			for _, record := range turn1Prompts {
				if strings.Contains(record.prompt, markerV2) {
					t.Fatalf("turn 1 saw V2 before the external mutation")
				}
			}

			replaceLiveFileAtomically(t, path, updatedContent)
			t.Logf("external file changed after turn 1: %s -> %s", markerV1, markerV2)
			// Give the pinned-file monitor at least one polling cycle to append
			// its timeline notice before the next user turn starts.
			time.Sleep(1500 * time.Millisecond)

			capture.beginPhase("turn2")
			if err := engine.SendMsg(turn2Question(path)); err != nil {
				t.Fatalf("turn 2 without reattachment failed: %v\nstreams:\n%s\nevents:\n%s", err, truncateLiveLog(capture.streamSummary(), 4000), strings.Join(capture.events, "\n"))
			}
			turn2Prompts := capture.promptsInPhase("turn2")
			turn2Answer := strings.TrimSpace(capture.answerText())
			if len(turn2Prompts) == 0 {
				t.Fatal("turn 2 made no real model calls")
			}

			var promptCountV1, promptCountV2 int
			for _, record := range turn2Prompts {
				if strings.Contains(record.prompt, markerV1) {
					promptCountV1++
				}
				if strings.Contains(record.prompt, markerV2) {
					promptCountV2++
				}
			}
			t.Logf(
				"turn 2: model_calls=%d prompts_with_V1=%d prompts_with_V2=%d tool_calls=%d answer=%q",
				len(turn2Prompts), promptCountV1, promptCountV2, observation.count(), turn2Answer,
			)
			if result := observation.result(); result != "" {
				t.Logf("last experiment tool result: %s", truncateLiveLog(result, 2000))
			}
			verify(t, observation, turn2Prompts, turn2Answer)
		})
	}

	runScenario(
		"monitor_without_tools",
		true,
		nil,
		func(string) string {
			return "Do not call any tools. Inspect the conversation timeline only. If it contains an attached_file_changed notice for the user-pinned file, use directly_answer and put exactly NOTICE_PRESENT in answer_payload. Otherwise put exactly NOTICE_MISSING."
		},
		func(t *testing.T, observation *liveToolObservation, prompts []livePromptRecord, answer string) {
			t.Helper()
			if observation.count() != 0 {
				t.Fatalf("monitor scenario unexpectedly called tools: %v", observation.calls)
			}
			var promptsWithChangeNotice int
			for _, record := range prompts {
				if strings.Contains(record.prompt, markerV2) {
					t.Fatal("monitor scenario unexpectedly observed V2 without reattachment or a tool read")
				}
				if strings.Contains(record.prompt, "[attached_file_changed]") {
					promptsWithChangeNotice++
				}
			}
			if promptsWithChangeNotice == 0 {
				t.Fatal("the pinned-file monitor did not add its change notice to a later prompt")
			}
			if !strings.Contains(answer, "NOTICE_PRESENT") || strings.Contains(answer, "NOTICE_MISSING") {
				t.Fatalf("real agent did not report the monitor notice cleanly: %q", answer)
			}
			t.Logf("pinned-file monitor notice appeared in %d prompts without reattachment", promptsWithChangeNotice)
		},
	)

	runScenario(
		"read_file_tool",
		false,
		func(_ string, observation *liveToolObservation) []*aitool.Tool {
			return []*aitool.Tool{newLiveObservedReadFileTool(t, observation)}
		},
		func(path string) string {
			return fmt.Sprintf(
				"Call the read_file tool exactly once with file=%q. This is the same file that was attached in the previous message. Compare the current marker with the marker from the original attachment. Do not call any other tool. Then use directly_answer and output exactly: changed=<true|false>; marker=<current marker>.",
				path,
			)
		},
		func(t *testing.T, observation *liveToolObservation, prompts []livePromptRecord, answer string) {
			t.Helper()
			if observation.count() == 0 {
				t.Fatal("read_file was available but the real agent did not call it")
			}
			if !strings.Contains(answer, markerV2) {
				t.Fatalf("read_file scenario did not report the current V2 marker: %q", answer)
			}
			t.Logf("read_file scenario observed current content; explicit changed=%v", liveAnswerReportsChangedTrue(answer))
		},
	)

	runScenario(
		"snapshot_inspect_tool",
		false,
		func(path string, observation *liveToolObservation) []*aitool.Tool {
			return []*aitool.Tool{
				newLiveSnapshotInspectTool(t, map[string]string{path: baselineHash}, observation),
			}
		},
		func(path string) string {
			return fmt.Sprintf(
				"Call inspect_attached_file exactly once with path=%q. It compares the current file with the SHA256 snapshot captured when the file was attached. Do not call any other tool. Then use directly_answer and output exactly: changed=<true|false>; marker=<current marker>.",
				path,
			)
		},
		func(t *testing.T, observation *liveToolObservation, prompts []livePromptRecord, answer string) {
			t.Helper()
			if observation.count() == 0 {
				t.Fatal("inspect_attached_file was available but the real agent did not call it")
			}
			if !strings.Contains(strings.ToLower(observation.result()), `"changed":true`) {
				t.Fatalf("snapshot probe did not detect the external change: %s", observation.result())
			}
			if !strings.Contains(answer, markerV2) || !liveAnswerReportsChangedTrue(answer) {
				t.Fatalf("real agent did not report the detected change and V2 marker: %q", answer)
			}
		},
	)
}
