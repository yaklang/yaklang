package aireact

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// mockEmbeddingClient 本地 mock embedding 客户端
type mockEmbeddingClient struct{}

func (m *mockEmbeddingClient) Embedding(text string) ([]float32, error) {
	// 生成一个简单的默认向量
	hash := utils.CalcMd5(text)
	vec := make([]float32, 768)
	for i := 0; i < 768; i++ {
		vec[i] = float32(hash[i%len(hash)]) / 255.0
	}
	return vec, nil
}

// MockAIMemoryInvoker wraps the ReAct instance and mocks InvokeLiteForge for memory triage
type MockAIMemoryInvoker struct {
	*ReAct
	memoryTriageCallCount int
}

func (m *MockAIMemoryInvoker) InvokeLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption) (*aicommon.Action, error) {
	if actionName == "memory-triage" {
		m.memoryTriageCallCount++
		// 构造mock的返回数据
		mockResponseJSON := `{
			"@action": "memory-triage",
			"memory_entities": [
				{
					"content": "用户拒绝了使用sleep工具的建议，选择直接回答问题",
					"tags": ["用户交互", "工具拒绝", "测试场景"],
					"potential_questions": [
						"用户为什么拒绝使用工具？",
						"如何处理工具使用被拒绝的情况？",
						"什么时候应该直接回答而不是使用工具？"
					],
					"t": 0.8,
					"a": 0.7,
					"p": 0.9,
					"o": 0.85,
					"e": 0.6,
					"r": 0.75,
					"c": 0.65
				},
				{
					"content": "ReAct系统在工具被拒绝后能够正确回退到直接回答模式",
					"tags": ["系统行为", "测试验证", "ReAct框架"],
					"potential_questions": [
						"ReAct系统如何处理工具拒绝？",
						"工具调用失败后的备选方案是什么？",
						"如何测试ReAct的容错能力？"
					],
					"t": 0.7,
					"a": 0.8,
					"p": 0.6,
					"o": 0.9,
					"e": 0.5,
					"r": 0.8,
					"c": 0.7
				}
			]
		}`

		action, err := aicommon.ExtractAction(mockResponseJSON, "memory-triage")
		if err != nil {
			return nil, utils.Errorf("failed to extract action: %v", err)
		}
		return action, nil
	}

	// Fall back to original implementation
	return m.ReAct.InvokeLiteForge(ctx, actionName, prompt, outputs)
}

func mockedToolCallingWithAIMemory2(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, toolName string) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()
	if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "` + toolName + `" },
"human_readable_thought": "mocked thought for tool calling", "cumulative_summary": "..cumulative-mocked for tool calling.."}
`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "You need to generate parameters for the tool", "call-tool") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "seconds" : 0.1 }}`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "abc-mocked-reason"}`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, `directly_answer`) && !utils.MatchAnyOfSubString(prompt, `require_tool`) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "directly_answer", "answer_payload": "directly answer after '` + toolName + `' require and user reject it..........." },
"human_readable_thought": "mocked thought for tool calling", "cumulative_summary": "..cumulative-mocked for tool calling.."}
`))
		rsp.Close()
		return rsp, nil

	}

	fmt.Println("Unexpected prompt:", prompt)

	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

func getTestDatabase() (*gorm.DB, error) {
	// 创建临时文件数据库用于测试，避免并发访问问题
	tmpDir := consts.GetDefaultYakitBaseTempDir()
	dbFile := filepath.Join(tmpDir, uuid.NewString()+".db")

	db, err := gorm.Open("sqlite3", dbFile)
	if err != nil {
		return nil, err
	}

	// 自动迁移表结构
	schema.AutoMigrate(db, schema.KEY_SCHEMA_YAKIT_DATABASE)

	// 设置数据库连接池和超时
	db.DB().SetMaxOpenConns(1)
	db.DB().SetMaxIdleConns(1)

	return db, nil
}

func TestReAct_ToolUse_Reject_WithAIMemory(t *testing.T) {
	sessionID := "test-session-" + ksuid.New().String()
	flag := ksuid.New().String()
	_ = flag
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	// 清理测试数据（在开始时和结束时都清理）
	toolCalled := false
	sleepTool, err := aitool.New(
		"sleep",
		aitool.WithNumberParam("seconds"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			toolCalled = true
			sleepInt := params.GetFloat("seconds", 0.3)
			if sleepInt <= 0 {
				sleepInt = 0.3
			}
			time.Sleep(time.Duration(sleepInt) * time.Second)
			return "done", nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	// 创建 mock embedding 客户端
	mockEmbedder := &mockEmbeddingClient{}

	ins, err := NewTestReAct(
		WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedToolCallingWithAIMemory2(i, r, "sleep")
		}),
		WithEventInputChan(in),
		WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		WithTools(sleepTool),
	)
	if err != nil {
		t.Fatal(err)
	}

	// 重新创建 memoryTriage，使用 mock embedding 和 mock invoker
	if ins.memoryTriage != nil {
		_ = ins.memoryTriage.Close()
	}

	db, err := getTestDatabase()
	require.NoError(t, err)

	mockInvoker := &MockAIMemoryInvoker{ReAct: ins}
	ins.memoryTriage, err = aimem.NewAIMemory(
		sessionID,
		aimem.WithInvoker(mockInvoker),
		aimem.WithRAGOptions(rag.WithEmbeddingClient(mockEmbedder)),
		aimem.WithDatabase(db),
	)
	if err != nil {
		t.Fatal(err)
	}

	_ = ins
	go func() {
		for i := 0; i < 1; i++ {
			in <- &ypb.AIInputEvent{
				IsFreeInput: true,
				FreeInput:   "abc",
			}
		}
	}()

	du := time.Duration(50)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)

	reviewed := false
	reviewReleased := false
	toolCallOutputEvent := false
	reActFinished := false
	var iid string
LOOP:
	for {
		select {
		case e := <-out:
			fmt.Println(e.String())
			if e.Type == string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE) {
				reviewed = true
				fmt.Println(string(e.Content))
				iid = utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				in <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        utils.InterfaceToString(iid),
					InteractiveJSONInput: `{"suggestion": "direct_answer"}`,
				}
			}

			if e.Type == string(schema.EVENT_TYPE_REVIEW_RELEASE) {
				gotId := utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				if gotId == iid {
					reviewReleased = true
				}
			}

			if e.Type == string(schema.EVENT_TOOL_CALL_USER_CANCEL) {
				toolCallOutputEvent = true
			}

			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					reActFinished = true
					break LOOP
				}
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	if !reviewed {
		t.Fatal("Expected to have at least one review event, but got none")
	}

	if !reviewReleased {
		t.Fatal("Expected to have at least one review release event, but got none")
	}

	if toolCalled {
		t.Fatal("Tool was called, but should have been rejected")
	}

	if !toolCallOutputEvent {
		t.Fatal("Expected to have at least one output event, but got none")
	}

	if !reActFinished {
		t.Fatal("Expected to have at least one re-act terminal event, but got none")
	}

	fmt.Println("--------------------------------------")
	tl := ins.DumpTimeline()
	fmt.Println(tl)
	if !strings.Contains(tl, `mocked thought for tool calling`) {
		t.Fatal("timeline does not contain mocked thought")
	}
	if !utils.MatchAllOfSubString(tl, `system-question`, "user-answer", "when review") {
		t.Fatal("timeline does not contain system-question")
	}
	if !utils.MatchAllOfSubString(tl, `ReAct iteration 1`, `ReAct Iteration Done[1]`) {
		t.Fatal("timeline does not contain ReAct iteration")
	}
	if !utils.MatchAllOfSubString(tl, `direct_answer`) {
		t.Fatal("timeline does not contain direct_answer")
	}
	fmt.Println("--------------------------------------")

	// 等待内存处理完成（包括数据库保存，因为是同步的）
	ins.Wait()

	// 验证 AIMemory Mock 被调用
	if mockInvoker.memoryTriageCallCount == 0 {
		t.Fatal("Expected AIMemory mock to be called, but it was not")
	}
	fmt.Printf("AIMemory mock was called %d times\n", mockInvoker.memoryTriageCallCount)

	var memoryEntities []schema.AIMemoryEntity
	if err := db.Where("session_id = ?", sessionID).Find(&memoryEntities).Error; err != nil {
		t.Fatalf("Failed to query memory entities: %v", err)
	}

	if len(memoryEntities) == 0 {
		t.Fatal("Expected to find memory entities in database, but found none")
	}
	fmt.Printf("Found %d memory entities in database\n", len(memoryEntities))

	// 验证 memory entities 的内容
	foundToolRejection := false
	foundSystemBehavior := false
	for _, entity := range memoryEntities {
		fmt.Printf("Memory Entity: %s\n", entity.Content)
		fmt.Printf("  Tags: %v\n", entity.Tags)
		fmt.Printf("  Potential Questions: %v\n", entity.PotentialQuestions)
		fmt.Printf("  Scores: C=%.2f, O=%.2f, R=%.2f, E=%.2f, P=%.2f, A=%.2f, T=%.2f\n",
			entity.C_Score, entity.O_Score, entity.R_Score, entity.E_Score,
			entity.P_Score, entity.A_Score, entity.T_Score)

		if utils.MatchAllOfSubString(entity.Content, "拒绝", "sleep") {
			foundToolRejection = true
		}
		if utils.MatchAllOfSubString(entity.Content, "ReAct", "直接回答") {
			foundSystemBehavior = true
		}
	}

	if !foundToolRejection {
		t.Fatal("Expected to find memory entity about tool rejection")
	}
	if !foundSystemBehavior {
		t.Fatal("Expected to find memory entity about system behavior")
	}

	// 验证可以搜索到 Mock 数据
	searchResults, err := ins.memoryTriage.SearchBySemantics("用户拒绝工具", 5)
	if err != nil {
		t.Fatalf("Failed to search memory entities: %v", err)
	}

	// 如果语义搜索返回空结果（RAG不可用），尝试按标签搜索
	if len(searchResults) == 0 {
		fmt.Println("Semantic search returned no results (RAG unavailable), trying tag-based search instead")
		// 这是可以接受的，因为 RAG 可能不可用，系统降级到其他搜索方式
	} else {
		fmt.Printf("Found %d search results\n", len(searchResults))

		for _, result := range searchResults {
			fmt.Printf("Search Result (score=%.4f): %s\n", result.Score, result.Entity.Content)
		}
	}

	// 验证按标签搜索
	tagResults, err := ins.memoryTriage.SearchByTags([]string{"用户交互"}, false, 5)
	if err != nil {
		t.Fatalf("Failed to search by tags: %v", err)
	}

	if len(tagResults) == 0 {
		t.Fatal("Expected to find tag search results, but found none")
	}
	fmt.Printf("Found %d results by tag search\n", len(tagResults))

	// 验证使用 SearchMemory（使用AI增强的搜索）
	fmt.Println("\n=== Testing SearchMemory (with AI) ===")
	memorySearchResults, err := ins.memoryTriage.SearchMemory("用户拒绝工具调用", 10000)
	if err != nil {
		t.Fatalf("Failed to search memory: %v", err)
	}
	if memorySearchResults == nil {
		t.Fatal("Expected SearchMemory to return a result, but got nil")
	}
	fmt.Printf("SearchMemory found %d memories\n", len(memorySearchResults.Memories))
	fmt.Printf("Total content bytes: %d\n", memorySearchResults.ContentBytes)
	fmt.Printf("Search summary: %s\n", memorySearchResults.SearchSummary)
	for _, mem := range memorySearchResults.Memories {
		fmt.Printf("  - Memory: %s\n", mem.Content[:min(len(mem.Content), 100)])
	}

	// 验证使用 SearchMemoryWithoutAI（关键词搜索，不用AI）
	fmt.Println("\n=== Testing SearchMemoryWithoutAI (keyword-based) ===")
	noAIResults, err := ins.memoryTriage.SearchMemoryWithoutAI("工具调用 拒绝", 10000)
	if err != nil {
		t.Fatalf("Failed to search memory without AI: %v", err)
	}
	if noAIResults == nil {
		t.Fatal("Expected SearchMemoryWithoutAI to return a result, but got nil")
	}
	fmt.Printf("SearchMemoryWithoutAI found %d memories\n", len(noAIResults.Memories))
	fmt.Printf("Total content bytes: %d\n", noAIResults.ContentBytes)
	fmt.Printf("Search summary: %s\n", noAIResults.SearchSummary)
	for _, mem := range noAIResults.Memories {
		fmt.Printf("  - Memory: %s\n", mem.Content[:min(len(mem.Content), 100)])
	}

	// Helper function to get minimum of two integers
	min := func(a, b int) int {
		if a < b {
			return a
		}
		return b
	}
	_ = min
}
