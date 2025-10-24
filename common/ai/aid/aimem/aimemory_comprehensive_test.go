package aimem

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// AdvancedMockInvoker 高级Mock调用器，支持prompt验证和多种返回值
type AdvancedMockInvoker struct {
	ctx              context.Context
	capturedPrompts  []string
	capturedActions  []string
	returnValues     map[string]string
	shouldFail       map[string]bool
	promptValidators map[string]func(string) bool
}

func NewAdvancedMockInvoker(ctx context.Context) *AdvancedMockInvoker {
	return &AdvancedMockInvoker{
		ctx:              ctx,
		capturedPrompts:  []string{},
		capturedActions:  []string{},
		returnValues:     make(map[string]string),
		shouldFail:       make(map[string]bool),
		promptValidators: make(map[string]func(string) bool),
	}
}

// SetReturnValue 设置特定action的返回值
func (m *AdvancedMockInvoker) SetReturnValue(action, value string) {
	m.returnValues[action] = value
}

// SetShouldFail 设置特定action是否应该失败
func (m *AdvancedMockInvoker) SetShouldFail(action string, shouldFail bool) {
	m.shouldFail[action] = shouldFail
}

// SetPromptValidator 设置prompt验证器
func (m *AdvancedMockInvoker) SetPromptValidator(action string, validator func(string) bool) {
	m.promptValidators[action] = validator
}

// GetCapturedPrompts 获取捕获的prompts
func (m *AdvancedMockInvoker) GetCapturedPrompts() []string {
	return m.capturedPrompts
}

// GetCapturedActions 获取捕获的actions
func (m *AdvancedMockInvoker) GetCapturedActions() []string {
	return m.capturedActions
}

func (m *AdvancedMockInvoker) GetBasicPromptInfo(tools []*aitool.Tool) (string, map[string]any, error) {
	return "Mock Basic Prompt Template: {{ .Query }}", map[string]any{
		"Query": "test query",
	}, nil
}

func (m *AdvancedMockInvoker) InvokeLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption) (*aicommon.Action, error) {
	// 记录调用
	m.capturedActions = append(m.capturedActions, actionName)
	m.capturedPrompts = append(m.capturedPrompts, prompt)

	// 验证prompt
	if validator, exists := m.promptValidators[actionName]; exists {
		if !validator(prompt) {
			return nil, utils.Errorf("prompt validation failed for action: %s", actionName)
		}
	}

	// 检查是否应该失败
	if shouldFail, exists := m.shouldFail[actionName]; exists && shouldFail {
		return nil, utils.Errorf("mock failure for action: %s", actionName)
	}

	// 返回自定义值或默认值
	var mockResponseJSON string
	if customValue, exists := m.returnValues[actionName]; exists {
		mockResponseJSON = customValue
	} else {
		// 默认返回值
		switch actionName {
		case "memory-triage":
			mockResponseJSON = `{
				"@action": "memory-triage",
				"memory_entities": [
					{
						"content": "用户在实现一个复杂的AI记忆系统，使用C.O.R.E. P.A.C.T.框架进行记忆评分",
						"tags": ["AI开发", "记忆系统", "C.O.R.E. P.A.C.T."],
						"potential_questions": ["如何实现AI记忆系统？", "什么是C.O.R.E. P.A.C.T.框架？", "如何评估记忆的重要性？"],
						"t": 0.8, "a": 0.7, "p": 0.9, "o": 0.85, "e": 0.6, "r": 0.75, "c": 0.65
					},
					{
						"content": "系统需要支持语义搜索、按分数搜索和按标签搜索功能",
						"tags": ["搜索功能", "AI开发"],
						"potential_questions": ["如何实现语义搜索？", "什么是按分数搜索？", "如何按标签过滤记忆？"],
						"t": 0.7, "a": 0.8, "p": 0.6, "o": 0.9, "e": 0.5, "r": 0.8, "c": 0.7
					}
				]
			}`
		case "tag-selection":
			mockResponseJSON = `{
				"@action": "tag-selection",
				"tags": ["编程", "Go语言", "AI开发"]
			}`
		case "memory-deduplication":
			mockResponseJSON = `{
				"@action": "memory-deduplication",
				"is_duplicate": false,
				"reason": "新记忆提供了独特的信息和视角",
				"similarity_score": 0.3
			}`
		case "batch-memory-deduplication":
			mockResponseJSON = `{
				"@action": "batch-memory-deduplication",
				"non_duplicate_indices": ["0", "1"],
				"analysis": "所有记忆都提供了独特价值，建议保留"
			}`
		default:
			return nil, utils.Errorf("unknown action: %s", actionName)
		}
	}

	action, err := aicommon.ExtractAction(mockResponseJSON, actionName)
	if err != nil {
		return nil, utils.Errorf("failed to extract action: %v", err)
	}

	return action, nil
}

// 实现其他必需的接口方法
func (m *AdvancedMockInvoker) ExecuteToolRequiredAndCall(ctx context.Context, name string) (*aitool.ToolResult, bool, error) {
	return nil, false, nil
}

func (m *AdvancedMockInvoker) AskForClarification(ctx context.Context, question string, payloads []string) string {
	return ""
}

func (m *AdvancedMockInvoker) DirectlyAnswer(ctx context.Context, query string, tools []*aitool.Tool) (string, error) {
	return "", nil
}

func (m *AdvancedMockInvoker) EnhanceKnowledgeAnswer(ctx context.Context, s string) (string, error) {
	return "", nil
}

func (m *AdvancedMockInvoker) VerifyUserSatisfaction(ctx context.Context, query string, isToolCall bool, payload string) (bool, error) {
	return true, nil
}

func (m *AdvancedMockInvoker) RequireAIForgeAndAsyncExecute(ctx context.Context, forgeName string, onFinish func(error)) {
}

func (m *AdvancedMockInvoker) AsyncPlanAndExecute(ctx context.Context, planPayload string, onFinish func(error)) {
}

func (m *AdvancedMockInvoker) AddToTimeline(entry, content string) {
}

func (m *AdvancedMockInvoker) GetConfig() aicommon.AICallerConfigIf {
	return nil
}

func (m *AdvancedMockInvoker) EmitFileArtifactWithExt(name, ext string, data any) string {
	return ""
}

func (m *AdvancedMockInvoker) EmitResultAfterStream(any) {
}

func (m *AdvancedMockInvoker) EmitResult(any) {
}

func (m *AdvancedMockInvoker) EmitStreamResult(any) {
}

// TestAIMemoryTriage_NewAIMemory 测试创建AI记忆系统
func TestAIMemoryTriage_NewAIMemory(t *testing.T) {

	sessionID := "new-memory-test-" + uuid.New().String()
	defer cleanupComprehensiveTestData(t, sessionID)

	t.Run("ValidCreation", func(t *testing.T) {
		memory, err := CreateTestAIMemory(sessionID,
			WithInvoker(NewAdvancedMockInvoker(context.Background())),
		)
		if err != nil {
			t.Fatalf("create AI memory failed: %v", err)
		}
		defer memory.Close()

		// 验证基本属性
		if memory.GetSessionID() != sessionID {
			t.Errorf("expected session ID %s, got %s", sessionID, memory.GetSessionID())
		}

		// 验证HNSW后端初始化
		stats := memory.GetHNSWStats()
		if stats == nil {
			t.Errorf("HNSW stats should not be nil")
		}
	})

	t.Run("MissingInvoker", func(t *testing.T) {
		_, err := CreateTestAIMemory(sessionID)
		if err == nil {
			t.Errorf("expected error when invoker is missing")
		}
		if !strings.Contains(err.Error(), "invoker") {
			t.Errorf("error should mention invoker, got: %v", err)
		}
	})

	t.Run("EmptySessionID", func(t *testing.T) {
		_, err := NewAIMemory("")
		if err == nil {
			t.Errorf("expected error for empty session ID")
		}
		if !strings.Contains(err.Error(), "sessionId is required") {
			t.Errorf("error should mention sessionId requirement, got: %v", err)
		}
	})
}

// TestAIMemoryTriage_AddRawText 测试原始文本处理
func TestAIMemoryTriage_AddRawText(t *testing.T) {

	sessionID := "add-raw-text-test-" + uuid.New().String()
	defer cleanupComprehensiveTestData(t, sessionID)

	t.Run("SuccessfulProcessing", func(t *testing.T) {
		mockInvoker := NewAdvancedMockInvoker(context.Background())

		// 设置prompt验证器
		mockInvoker.SetPromptValidator("memory-triage", func(prompt string) bool {
			// 验证prompt包含关键信息
			return strings.Contains(prompt, "C.O.R.E. P.A.C.T.") &&
				strings.Contains(prompt, "Go语言并发编程")
		})

		memory, err := CreateTestAIMemory(sessionID, WithInvoker(mockInvoker))
		if err != nil {
			t.Fatalf("create AI memory failed: %v", err)
		}
		defer memory.Close()

		// 测试处理原始文本
		testInput := "Go语言并发编程使用goroutine和channel实现高效的并发处理"
		entities, err := memory.AddRawText(testInput)
		if err != nil {
			t.Fatalf("AddRawText failed: %v", err)
		}

		// 验证返回结果
		if len(entities) == 0 {
			t.Fatalf("expected entities to be generated")
		}

		// 验证实体内容
		for _, entity := range entities {
			if entity.Id == "" {
				t.Errorf("entity ID should not be empty")
			}
			if entity.Content == "" {
				t.Errorf("entity content should not be empty")
			}
			if len(entity.CorePactVector) != 7 {
				t.Errorf("CorePactVector should have 7 dimensions, got %d", len(entity.CorePactVector))
			}
			if entity.CreatedAt.IsZero() {
				t.Errorf("CreatedAt should be set")
			}
		}

		// 验证prompt被正确调用
		actions := mockInvoker.GetCapturedActions()
		if len(actions) == 0 || actions[0] != "memory-triage" {
			t.Errorf("expected memory-triage action to be called")
		}
	})

	t.Run("InvokerFailure", func(t *testing.T) {
		mockInvoker := NewAdvancedMockInvoker(context.Background())
		mockInvoker.SetShouldFail("memory-triage", true)

		memory, err := CreateTestAIMemory(sessionID, WithInvoker(mockInvoker))
		if err != nil {
			t.Fatalf("create AI memory failed: %v", err)
		}
		defer memory.Close()

		_, err = memory.AddRawText("test input")
		if err == nil {
			t.Errorf("expected error when invoker fails")
		}
	})

	t.Run("EmptyInput", func(t *testing.T) {
		mockInvoker := NewAdvancedMockInvoker(context.Background())
		memory, err := CreateTestAIMemory(sessionID, WithInvoker(mockInvoker))
		if err != nil {
			t.Fatalf("create AI memory failed: %v", err)
		}
		defer memory.Close()

		entities, err := memory.AddRawText("")
		if err != nil {
			t.Fatalf("AddRawText with empty input should not fail: %v", err)
		}

		// 空输入可能返回空结果或默认结果，都是合理的
		t.Logf("Empty input resulted in %d entities", len(entities))
	})
}

// TestAIMemoryTriage_SelectTags 测试标签选择
func TestAIMemoryTriage_SelectTags(t *testing.T) {

	sessionID := "select-tags-test-" + uuid.New().String()
	defer cleanupComprehensiveTestData(t, sessionID)

	t.Run("SuccessfulTagSelection", func(t *testing.T) {
		mockInvoker := NewAdvancedMockInvoker(context.Background())

		// 设置prompt验证器
		mockInvoker.SetPromptValidator("tag-selection", func(prompt string) bool {
			return strings.Contains(prompt, "编程语言特性")
		})

		memory, err := CreateTestAIMemory(sessionID, WithInvoker(mockInvoker))
		if err != nil {
			t.Fatalf("create AI memory failed: %v", err)
		}
		defer memory.Close()

		ctx := context.Background()
		tags, err := memory.SelectTags(ctx, "编程语言特性和应用场景")
		if err != nil {
			t.Fatalf("SelectTags failed: %v", err)
		}

		// 验证返回的标签
		if len(tags) == 0 {
			t.Errorf("expected tags to be returned")
		}

		// 验证标签内容
		expectedTags := []string{"编程", "Go语言", "AI开发"}
		for _, expectedTag := range expectedTags {
			found := false
			for _, tag := range tags {
				if tag == expectedTag {
					found = true
					break
				}
			}
			if !found {
				t.Logf("expected tag '%s' not found in result: %v", expectedTag, tags)
			}
		}

		// 验证action被调用
		actions := mockInvoker.GetCapturedActions()
		if len(actions) == 0 || actions[len(actions)-1] != "tag-selection" {
			t.Errorf("expected tag-selection action to be called")
		}
	})

	t.Run("InvokerFailure", func(t *testing.T) {
		mockInvoker := NewAdvancedMockInvoker(context.Background())
		mockInvoker.SetShouldFail("tag-selection", true)

		memory, err := CreateTestAIMemory(sessionID, WithInvoker(mockInvoker))
		if err != nil {
			t.Fatalf("create AI memory failed: %v", err)
		}
		defer memory.Close()

		ctx := context.Background()
		_, err = memory.SelectTags(ctx, "test input")
		if err == nil {
			t.Errorf("expected error when invoker fails")
		}
	})
}

// TestAIMemoryTriage_ShouldSaveMemoryEntities 测试去重保存判断
func TestAIMemoryTriage_ShouldSaveMemoryEntities(t *testing.T) {

	sessionID := "should-save-test-" + uuid.New().String()
	defer cleanupComprehensiveTestData(t, sessionID)

	t.Run("AllEntitiesUnique", func(t *testing.T) {
		mockInvoker := NewAdvancedMockInvoker(context.Background())
		memory, err := CreateTestAIMemory(sessionID, WithInvoker(mockInvoker))
		if err != nil {
			t.Fatalf("create AI memory failed: %v", err)
		}
		defer memory.Close()

		// 创建测试实体
		entities := []*MemoryEntity{
			{
				Id:                 "test-1",
				Content:            "Go语言并发编程",
				Tags:               []string{"编程", "Go语言"},
				PotentialQuestions: []string{"什么是Go语言？"},
				C_Score:            0.8, O_Score: 0.9, R_Score: 0.7, E_Score: 0.6,
				P_Score: 0.8, A_Score: 0.7, T_Score: 0.8,
				CorePactVector: []float32{0.8, 0.9, 0.7, 0.6, 0.8, 0.7, 0.8},
			},
			{
				Id:                 "test-2",
				Content:            "Python数据分析",
				Tags:               []string{"编程", "Python", "数据分析"},
				PotentialQuestions: []string{"Python如何做数据分析？"},
				C_Score:            0.7, O_Score: 0.8, R_Score: 0.8, E_Score: 0.5,
				P_Score: 0.7, A_Score: 0.8, T_Score: 0.7,
				CorePactVector: []float32{0.7, 0.8, 0.8, 0.5, 0.7, 0.8, 0.7},
			},
		}

		worthSaving := memory.ShouldSaveMemoryEntities(entities)

		// 由于没有现有记忆，所有实体都应该被保存
		if len(worthSaving) != len(entities) {
			t.Errorf("expected %d entities to be worth saving, got %d", len(entities), len(worthSaving))
		}
	})

	t.Run("EmptyEntities", func(t *testing.T) {
		mockInvoker := NewAdvancedMockInvoker(context.Background())
		memory, err := CreateTestAIMemory(sessionID, WithInvoker(mockInvoker))
		if err != nil {
			t.Fatalf("create AI memory failed: %v", err)
		}
		defer memory.Close()

		worthSaving := memory.ShouldSaveMemoryEntities([]*MemoryEntity{})
		if len(worthSaving) != 0 {
			t.Errorf("expected 0 entities for empty input, got %d", len(worthSaving))
		}
	})
}

// TestAIMemoryTriage_HandleMemory 测试记忆处理
func TestAIMemoryTriage_HandleMemory(t *testing.T) {

	sessionID := "handle-memory-comprehensive-test-" + uuid.New().String()
	defer cleanupComprehensiveTestData(t, sessionID)

	t.Run("SuccessfulHandling", func(t *testing.T) {
		mockInvoker := NewAdvancedMockInvoker(context.Background())
		memory, err := CreateTestAIMemory(sessionID, WithInvoker(mockInvoker))
		if err != nil {
			t.Fatalf("create AI memory failed: %v", err)
		}
		defer memory.Close()

		err = memory.HandleMemory("Go语言是一种现代编程语言")
		if err != nil {
			t.Fatalf("HandleMemory failed: %v", err)
		}

		// 验证记忆被保存
		memories, err := memory.ListAllMemories(10)
		if err != nil {
			t.Fatalf("ListAllMemories failed: %v", err)
		}

		if len(memories) == 0 {
			t.Errorf("expected memories to be saved")
		}
	})

	t.Run("EmptyInput", func(t *testing.T) {
		mockInvoker := NewAdvancedMockInvoker(context.Background())
		memory, err := CreateTestAIMemory(sessionID, WithInvoker(mockInvoker))
		if err != nil {
			t.Fatalf("create AI memory failed: %v", err)
		}
		defer memory.Close()

		err = memory.HandleMemory("")
		if err != nil {
			t.Errorf("HandleMemory with empty input should not fail: %v", err)
		}
	})

	t.Run("NilInput", func(t *testing.T) {
		mockInvoker := NewAdvancedMockInvoker(context.Background())
		memory, err := CreateTestAIMemory(sessionID, WithInvoker(mockInvoker))
		if err != nil {
			t.Fatalf("create AI memory failed: %v", err)
		}
		defer memory.Close()

		err = memory.HandleMemory(nil)
		if err != nil {
			t.Errorf("HandleMemory with nil input should not fail: %v", err)
		}
	})
}

// TestAIMemoryTriage_SearchMemory 测试记忆搜索
func TestAIMemoryTriage_SearchMemory(t *testing.T) {

	sessionID := "search-memory-comprehensive-test-" + uuid.New().String()
	defer cleanupComprehensiveTestData(t, sessionID)

	t.Run("SuccessfulSearch", func(t *testing.T) {
		mockInvoker := NewAdvancedMockInvoker(context.Background())
		memory, err := CreateTestAIMemory(sessionID, WithInvoker(mockInvoker))
		if err != nil {
			t.Fatalf("create AI memory failed: %v", err)
		}
		defer memory.Close()

		// 先添加一些记忆
		err = memory.HandleMemory("Go语言并发编程")
		if err != nil {
			t.Fatalf("HandleMemory failed: %v", err)
		}

		// 搜索记忆
		result, err := memory.SearchMemory("编程语言", 500)
		if err != nil {
			t.Fatalf("SearchMemory failed: %v", err)
		}

		// 验证搜索结果
		if result == nil {
			t.Fatalf("search result should not be nil")
		}

		if result.ContentBytes > 500 {
			t.Errorf("content bytes %d exceeds limit 500", result.ContentBytes)
		}

		if result.SearchSummary == "" {
			t.Errorf("search summary should not be empty")
		}
	})

	t.Run("ZeroBytesLimit", func(t *testing.T) {
		mockInvoker := NewAdvancedMockInvoker(context.Background())
		memory, err := CreateTestAIMemory(sessionID, WithInvoker(mockInvoker))
		if err != nil {
			t.Fatalf("create AI memory failed: %v", err)
		}
		defer memory.Close()

		result, err := memory.SearchMemory("test", 0)
		if err != nil {
			t.Fatalf("SearchMemory with zero limit failed: %v", err)
		}

		if len(result.Memories) != 0 {
			t.Errorf("expected no memories with zero bytes limit")
		}
	})

	t.Run("NegativeBytesLimit", func(t *testing.T) {
		mockInvoker := NewAdvancedMockInvoker(context.Background())
		memory, err := CreateTestAIMemory(sessionID, WithInvoker(mockInvoker))
		if err != nil {
			t.Fatalf("create AI memory failed: %v", err)
		}
		defer memory.Close()

		result, err := memory.SearchMemory("test", -100)
		if err != nil {
			t.Fatalf("SearchMemory with negative limit failed: %v", err)
		}

		if len(result.Memories) != 0 {
			t.Errorf("expected no memories with negative bytes limit")
		}
	})
}

// TestAIMemoryTriage_StorageOperations 测试存储操作
func TestAIMemoryTriage_StorageOperations(t *testing.T) {

	sessionID := "storage-ops-test-" + uuid.New().String()
	defer cleanupComprehensiveTestData(t, sessionID)

	mockInvoker := NewAdvancedMockInvoker(context.Background())
	memory, err := CreateTestAIMemory(sessionID, WithInvoker(mockInvoker))
	if err != nil {
		t.Fatalf("create AI memory failed: %v", err)
	}
	defer memory.Close()

	t.Run("SaveAndRetrieve", func(t *testing.T) {
		// 创建测试实体
		testEntity := &MemoryEntity{
			Id:                 "storage-save-retrieve-" + uuid.New().String(),
			Content:            "测试存储操作的记忆实体",
			Tags:               []string{"测试", "存储"},
			PotentialQuestions: []string{"如何测试存储？"},
			C_Score:            0.7, O_Score: 0.8, R_Score: 0.6, E_Score: 0.5,
			P_Score: 0.7, A_Score: 0.6, T_Score: 0.8,
			CorePactVector: []float32{0.7, 0.8, 0.6, 0.5, 0.7, 0.6, 0.8},
		}

		// 保存实体
		err := memory.SaveMemoryEntities(testEntity)
		if err != nil {
			t.Fatalf("SaveMemoryEntities failed: %v", err)
		}

		// 检索实体
		retrieved, err := memory.GetMemoryEntity(testEntity.Id)
		if err != nil {
			t.Fatalf("GetMemoryEntity failed: %v", err)
		}

		// 验证检索的实体
		if retrieved.Id != testEntity.Id {
			t.Errorf("expected ID %s, got %s", testEntity.Id, retrieved.Id)
		}
		if retrieved.Content != testEntity.Content {
			t.Errorf("expected content %s, got %s", testEntity.Content, retrieved.Content)
		}
	})

	t.Run("UpdateEntity", func(t *testing.T) {
		// 创建并保存实体
		testEntity := &MemoryEntity{
			Id:                 "storage-update-" + uuid.New().String(),
			Content:            "原始记忆内容",
			Tags:               []string{"测试", "存储"},
			PotentialQuestions: []string{"如何测试存储？"},
			C_Score:            0.7, O_Score: 0.8, R_Score: 0.6, E_Score: 0.5,
			P_Score: 0.7, A_Score: 0.6, T_Score: 0.8,
			CorePactVector: []float32{0.7, 0.8, 0.6, 0.5, 0.7, 0.6, 0.8},
		}

		err := memory.SaveMemoryEntities(testEntity)
		if err != nil {
			t.Fatalf("SaveMemoryEntities failed: %v", err)
		}

		// 更新实体
		updatedEntity := &MemoryEntity{
			Id:                 testEntity.Id,
			Content:            "更新后的记忆内容",
			Tags:               []string{"测试", "存储", "更新"},
			PotentialQuestions: []string{"如何测试更新？"},
			C_Score:            0.8, O_Score: 0.9, R_Score: 0.7, E_Score: 0.6,
			P_Score: 0.8, A_Score: 0.7, T_Score: 0.9,
			CorePactVector: []float32{0.8, 0.9, 0.7, 0.6, 0.8, 0.7, 0.9},
		}

		err = memory.UpdateMemoryEntity(updatedEntity)
		if err != nil {
			t.Fatalf("UpdateMemoryEntity failed: %v", err)
		}

		// 验证更新
		updated, err := memory.GetMemoryEntity(testEntity.Id)
		if err != nil {
			t.Fatalf("GetMemoryEntity after update failed: %v", err)
		}

		if updated.Content != "更新后的记忆内容" {
			t.Errorf("content was not updated, got: %s", updated.Content)
		}
		if len(updated.Tags) != 3 {
			t.Errorf("expected 3 tags after update, got %d", len(updated.Tags))
		}
	})

	t.Run("DeleteEntity", func(t *testing.T) {
		// 创建并保存实体
		testEntity := &MemoryEntity{
			Id:                 "storage-delete-" + uuid.New().String(),
			Content:            "待删除的记忆实体",
			Tags:               []string{"测试", "删除"},
			PotentialQuestions: []string{"如何测试删除？"},
			C_Score:            0.7, O_Score: 0.8, R_Score: 0.6, E_Score: 0.5,
			P_Score: 0.7, A_Score: 0.6, T_Score: 0.8,
			CorePactVector: []float32{0.7, 0.8, 0.6, 0.5, 0.7, 0.6, 0.8},
		}

		err := memory.SaveMemoryEntities(testEntity)
		if err != nil {
			t.Fatalf("SaveMemoryEntities failed: %v", err)
		}

		// 删除实体
		err = memory.DeleteMemoryEntity(testEntity.Id)
		if err != nil {
			t.Fatalf("DeleteMemoryEntity failed: %v", err)
		}

		// 验证删除
		_, err = memory.GetMemoryEntity(testEntity.Id)
		if err == nil {
			t.Errorf("expected error when getting deleted entity")
		}
	})

	t.Run("ListAllMemories", func(t *testing.T) {
		// 添加多个实体
		entities := []*MemoryEntity{
			{
				Id: "list-test-1-" + uuid.New().String(), Content: "第一个测试记忆",
				Tags: []string{"测试"}, PotentialQuestions: []string{"测试1？"},
				C_Score: 0.5, O_Score: 0.6, R_Score: 0.7, E_Score: 0.5,
				P_Score: 0.6, A_Score: 0.5, T_Score: 0.7,
				CorePactVector: []float32{0.5, 0.6, 0.7, 0.5, 0.6, 0.5, 0.7},
			},
			{
				Id: "list-test-2-" + uuid.New().String(), Content: "第二个测试记忆",
				Tags: []string{"测试"}, PotentialQuestions: []string{"测试2？"},
				C_Score: 0.6, O_Score: 0.7, R_Score: 0.8, E_Score: 0.6,
				P_Score: 0.7, A_Score: 0.6, T_Score: 0.8,
				CorePactVector: []float32{0.6, 0.7, 0.8, 0.6, 0.7, 0.6, 0.8},
			},
		}

		err := memory.SaveMemoryEntities(entities...)
		if err != nil {
			t.Fatalf("SaveMemoryEntities failed: %v", err)
		}

		// 列出所有记忆
		allMemories, err := memory.ListAllMemories(10)
		if err != nil {
			t.Fatalf("ListAllMemories failed: %v", err)
		}

		if len(allMemories) < 2 {
			t.Errorf("expected at least 2 memories, got %d", len(allMemories))
		}

		// 测试限制
		limitedMemories, err := memory.ListAllMemories(1)
		if err != nil {
			t.Fatalf("ListAllMemories with limit failed: %v", err)
		}

		if len(limitedMemories) != 1 {
			t.Errorf("expected 1 memory with limit, got %d", len(limitedMemories))
		}
	})
}

// TestAIMemoryTriage_HNSWOperations 测试HNSW操作
func TestAIMemoryTriage_HNSWOperations(t *testing.T) {

	sessionID := "hnsw-ops-test-" + uuid.New().String()
	defer cleanupComprehensiveTestData(t, sessionID)

	mockInvoker := NewAdvancedMockInvoker(context.Background())
	memory, err := CreateTestAIMemory(sessionID, WithInvoker(mockInvoker))
	if err != nil {
		t.Fatalf("create AI memory failed: %v", err)
	}
	defer memory.Close()

	t.Run("GetHNSWStats", func(t *testing.T) {
		stats := memory.GetHNSWStats()
		if stats == nil {
			t.Errorf("HNSW stats should not be nil")
		}

		// 验证stats包含预期的字段
		if _, exists := stats["session_id"]; !exists {
			t.Errorf("stats should contain session_id")
		}
	})

	t.Run("RebuildHNSWIndex", func(t *testing.T) {
		// 先添加一些数据
		err := memory.HandleMemory("测试HNSW重建索引")
		if err != nil {
			t.Fatalf("HandleMemory failed: %v", err)
		}

		// 重建索引
		err = memory.RebuildHNSWIndex()
		if err != nil {
			t.Fatalf("RebuildHNSWIndex failed: %v", err)
		}

		// 验证重建后仍能正常工作
		stats := memory.GetHNSWStats()
		if stats == nil {
			t.Errorf("HNSW stats should not be nil after rebuild")
		}
	})

	t.Run("SearchByScoreVector", func(t *testing.T) {
		// 先添加一些数据
		err := memory.HandleMemory("测试向量搜索功能")
		if err != nil {
			t.Fatalf("HandleMemory failed: %v", err)
		}

		// 创建查询向量
		queryEntity := &MemoryEntity{
			C_Score: 0.7, O_Score: 0.8, R_Score: 0.6, E_Score: 0.5,
			P_Score: 0.7, A_Score: 0.6, T_Score: 0.8,
		}

		// 执行向量搜索
		results, err := memory.SearchByScoreVector(queryEntity, 5)
		if err != nil {
			t.Fatalf("SearchByScoreVector failed: %v", err)
		}

		// 验证结果
		if len(results) > 5 {
			t.Errorf("expected at most 5 results, got %d", len(results))
		}

		for _, result := range results {
			if result.Entity == nil {
				t.Errorf("result entity should not be nil")
			}
			if result.Score < 0 || result.Score > 1 {
				t.Errorf("result score should be between 0 and 1, got %f", result.Score)
			}
		}
	})
}

// TestAIMemoryTriage_TagOperations 测试标签操作
func TestAIMemoryTriage_TagOperations(t *testing.T) {

	sessionID := "tag-ops-test-" + uuid.New().String()
	defer cleanupComprehensiveTestData(t, sessionID)

	mockInvoker := NewAdvancedMockInvoker(context.Background())
	memory, err := CreateTestAIMemory(sessionID, WithInvoker(mockInvoker))
	if err != nil {
		t.Fatalf("create AI memory failed: %v", err)
	}
	defer memory.Close()

	t.Run("GetAllTags", func(t *testing.T) {
		// 初始状态应该没有标签
		tags, err := memory.GetAllTags()
		if err != nil {
			t.Fatalf("GetAllTags failed: %v", err)
		}

		initialTagCount := len(tags)

		// 添加一些带标签的记忆
		err = memory.HandleMemory("Go语言并发编程学习")
		if err != nil {
			t.Fatalf("HandleMemory failed: %v", err)
		}

		// 再次获取标签
		tags, err = memory.GetAllTags()
		if err != nil {
			t.Fatalf("GetAllTags after adding memory failed: %v", err)
		}

		if len(tags) <= initialTagCount {
			t.Logf("Expected more tags after adding memory, initial: %d, current: %d", initialTagCount, len(tags))
		}
	})

	t.Run("SearchByTags", func(t *testing.T) {
		// 搜索特定标签
		results, err := memory.SearchByTags([]string{"AI开发"}, false, 10)
		if err != nil {
			t.Fatalf("SearchByTags failed: %v", err)
		}

		// 验证结果
		for _, result := range results {
			if result == nil {
				t.Errorf("search result should not be nil")
			}
		}

		// 测试精确匹配
		exactResults, err := memory.SearchByTags([]string{"AI开发", "记忆系统"}, true, 10)
		if err != nil {
			t.Fatalf("SearchByTags with exact match failed: %v", err)
		}

		// 精确匹配的结果应该不多于模糊匹配
		if len(exactResults) > len(results) {
			t.Errorf("exact match should not return more results than fuzzy match")
		}
	})

	t.Run("GetDynamicContextWithTags", func(t *testing.T) {
		context, err := memory.GetDynamicContextWithTags()
		if err != nil {
			t.Fatalf("GetDynamicContextWithTags failed: %v", err)
		}

		if context == "" {
			t.Errorf("dynamic context should not be empty")
		}

		// 验证上下文包含标签信息
		if !strings.Contains(context, "标签") && !strings.Contains(context, "没有已存储") {
			t.Errorf("context should mention tags or indicate no stored tags")
		}
	})
}

// TestAIMemoryTriage_ErrorHandling 测试错误处理
func TestAIMemoryTriage_ErrorHandling(t *testing.T) {

	sessionID := "error-handling-test-" + uuid.New().String()
	defer cleanupComprehensiveTestData(t, sessionID)

	t.Run("InvalidMemoryEntity", func(t *testing.T) {
		mockInvoker := NewAdvancedMockInvoker(context.Background())
		memory, err := CreateTestAIMemory(sessionID, WithInvoker(mockInvoker))
		if err != nil {
			t.Fatalf("create AI memory failed: %v", err)
		}
		defer memory.Close()

		// 测试保存nil实体
		err = memory.SaveMemoryEntities(nil)
		if err != nil {
			t.Errorf("SaveMemoryEntities with nil should not fail: %v", err)
		}

		// 测试保存无效向量的实体
		invalidEntity := &MemoryEntity{
			Id:             "invalid-entity",
			Content:        "测试无效实体",
			CorePactVector: []float32{0.1, 0.2}, // 错误的维度
		}

		err = memory.SaveMemoryEntities(invalidEntity)
		// 注意：系统可能会保存到数据库但在HNSW索引时失败，这是预期的行为
		if err != nil {
			t.Logf("SaveMemoryEntities with invalid vector returned error (expected): %v", err)
		} else {
			t.Logf("SaveMemoryEntities with invalid vector succeeded (may fail later in HNSW)")
		}
	})

	t.Run("NonExistentEntity", func(t *testing.T) {
		mockInvoker := NewAdvancedMockInvoker(context.Background())
		memory, err := CreateTestAIMemory(sessionID, WithInvoker(mockInvoker))
		if err != nil {
			t.Fatalf("create AI memory failed: %v", err)
		}
		defer memory.Close()

		// 尝试获取不存在的实体
		_, err = memory.GetMemoryEntity("non-existent-id")
		if err == nil {
			t.Errorf("expected error when getting non-existent entity")
		}

		// 尝试更新不存在的实体
		nonExistentEntity := &MemoryEntity{
			Id:             "non-existent-update",
			Content:        "不存在的实体",
			CorePactVector: []float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7},
		}

		err = memory.UpdateMemoryEntity(nonExistentEntity)
		if err != nil {
			t.Logf("UpdateMemoryEntity for non-existent entity returned error (expected): %v", err)
		}

		// 尝试删除不存在的实体
		err = memory.DeleteMemoryEntity("non-existent-delete")
		if err != nil {
			t.Errorf("DeleteMemoryEntity for non-existent entity should not fail: %v", err)
		}
	})
}

// TestAIMemoryTriage_ConcurrentOperations 测试并发操作
func TestAIMemoryTriage_ConcurrentOperations(t *testing.T) {

	sessionID := "concurrent-test-" + uuid.New().String()
	defer cleanupComprehensiveTestData(t, sessionID)

	mockInvoker := NewAdvancedMockInvoker(context.Background())
	memory, err := CreateTestAIMemory(sessionID, WithInvoker(mockInvoker))
	if err != nil {
		t.Fatalf("create AI memory failed: %v", err)
	}
	defer memory.Close()

	t.Run("ConcurrentHandleMemory", func(t *testing.T) {
		// 并发处理多个记忆
		done := make(chan bool, 3)

		go func() {
			err := memory.HandleMemory("并发测试记忆1")
			if err != nil {
				t.Errorf("concurrent HandleMemory 1 failed: %v", err)
			}
			done <- true
		}()

		go func() {
			err := memory.HandleMemory("并发测试记忆2")
			if err != nil {
				t.Errorf("concurrent HandleMemory 2 failed: %v", err)
			}
			done <- true
		}()

		go func() {
			err := memory.HandleMemory("并发测试记忆3")
			if err != nil {
				t.Errorf("concurrent HandleMemory 3 failed: %v", err)
			}
			done <- true
		}()

		// 等待所有goroutine完成
		for i := 0; i < 3; i++ {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatalf("concurrent operation timed out")
			}
		}
	})

	t.Run("ConcurrentSearch", func(t *testing.T) {
		// 并发搜索
		done := make(chan bool, 2)

		go func() {
			_, err := memory.SearchMemory("并发搜索1", 500)
			if err != nil {
				t.Errorf("concurrent SearchMemory 1 failed: %v", err)
			}
			done <- true
		}()

		go func() {
			_, err := memory.SearchMemory("并发搜索2", 500)
			if err != nil {
				t.Errorf("concurrent SearchMemory 2 failed: %v", err)
			}
			done <- true
		}()

		// 等待完成
		for i := 0; i < 2; i++ {
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatalf("concurrent search timed out")
			}
		}
	})
}

// cleanupComprehensiveTestData 清理测试数据
func cleanupComprehensiveTestData(t *testing.T, sessionID string) {
	db := consts.GetGormProjectDatabase()
	if db != nil {
		// 清理测试数据
		if err := db.Where("session_id = ?", sessionID).Delete(&schema.AIMemoryEntity{}).Error; err != nil {
			t.Logf("cleanup AIMemoryEntity failed: %v", err)
		}
		if err := db.Where("session_id = ?", sessionID).Delete(&schema.AIMemoryCollection{}).Error; err != nil {
			t.Logf("cleanup AIMemoryCollection failed: %v", err)
		}
	}
}
