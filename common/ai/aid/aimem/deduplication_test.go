package aimem

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

func TestShouldSaveMemoryEntities_BatchDeduplication(t *testing.T) {
	t.Parallel()

	sessionID := "batch-dedup-test-" + uuid.New().String()
	defer cleanupDeduplicationTestData(t, sessionID)

	// 创建AI记忆系统（使用测试专用的创建函数）
	memory, err := createTestAIMemory(sessionID,
		WithInvoker(NewMockInvoker(context.Background())),
	)
	if err != nil {
		t.Fatalf("create AI memory failed: %v", err)
	}
	defer memory.Close()

	// 创建测试记忆实体（不再使用分数过滤，测试批量去重）
	entities := []*MemoryEntity{
		{
			Id:                 "memory-1",
			CreatedAt:          time.Now(),
			Content:            "Go语言是一种编程语言",
			Tags:               []string{"编程", "Go语言"},
			PotentialQuestions: []string{"什么是Go语言？", "Go语言有什么特点？"},
			C_Score:            0.8,
			O_Score:            0.9,
			R_Score:            0.7,
			E_Score:            0.6,
			P_Score:            0.8,
			A_Score:            0.7,
			T_Score:            0.8,
			CorePactVector:     []float32{0.8, 0.9, 0.7, 0.6, 0.8, 0.7, 0.8},
		},
		{
			Id:                 "memory-2",
			CreatedAt:          time.Now(),
			Content:            "Python是一种编程语言",
			Tags:               []string{"编程", "Python"},
			PotentialQuestions: []string{"什么是Python？", "Python有什么优势？"},
			C_Score:            0.7,
			O_Score:            0.8,
			R_Score:            0.8,
			E_Score:            0.5,
			P_Score:            0.7,
			A_Score:            0.8,
			T_Score:            0.7,
			CorePactVector:     []float32{0.7, 0.8, 0.8, 0.5, 0.7, 0.8, 0.7},
		},
		{
			Id:                 "memory-3",
			CreatedAt:          time.Now(),
			Content:            "JavaScript是一种编程语言",
			Tags:               []string{"编程", "JavaScript", "前端"},
			PotentialQuestions: []string{"什么是JavaScript？", "JavaScript用于什么？"},
			C_Score:            0.6,
			O_Score:            0.7,
			R_Score:            0.8,
			E_Score:            0.4,
			P_Score:            0.6,
			A_Score:            0.7,
			T_Score:            0.6,
			CorePactVector:     []float32{0.6, 0.7, 0.8, 0.4, 0.6, 0.7, 0.6},
		},
	}

	// 测试批量去重功能
	worthSaving := memory.ShouldSaveMemoryEntities(entities)

	// 验证结果 - 现在不再基于分数过滤，而是基于重复检测
	if len(worthSaving) == 0 {
		t.Fatalf("expected some memories to be worth saving, got none")
	}

	// 由于没有现有记忆，所有记忆都应该被保留（除非它们之间互相重复）
	t.Logf("Batch deduplication result: %d/%d memories worth saving", len(worthSaving), len(entities))

	// 验证返回的记忆都是原始实体中的
	for _, mem := range worthSaving {
		found := false
		for _, original := range entities {
			if mem.Id == original.Id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("returned memory %s not found in original entities", mem.Id)
		}
	}
}

func TestBatchIsRepeatedMemoryEntities_TagOverlap(t *testing.T) {
	t.Parallel()

	sessionID := "batch-tag-overlap-test-" + uuid.New().String()
	defer cleanupDeduplicationTestData(t, sessionID)

	// 创建AI记忆系统（使用测试专用的创建函数）
	memory, err := createTestAIMemory(sessionID,
		WithInvoker(NewMockInvoker(context.Background())),
	)
	if err != nil {
		t.Fatalf("create AI memory failed: %v", err)
	}
	defer memory.Close()

	// 先保存一个记忆实体到数据库
	existingEntity := &MemoryEntity{
		Id:                 "existing-memory",
		Content:            "现有的Go语言记忆",
		Tags:               []string{"编程", "Go语言", "后端"},
		PotentialQuestions: []string{"Go语言怎么样？"},
		C_Score:            0.8,
		O_Score:            0.9,
		R_Score:            0.7,
		E_Score:            0.6,
		P_Score:            0.8,
		A_Score:            0.7,
		T_Score:            0.8,
		CorePactVector:     []float32{0.8, 0.9, 0.7, 0.6, 0.8, 0.7, 0.8},
	}

	// 保存到数据库
	err = memory.SaveMemoryEntities(existingEntity)
	if err != nil {
		t.Fatalf("save existing entity failed: %v", err)
	}

	// 测试批量标签重叠检查
	config := DefaultDeduplicationConfig()
	testEntities := []*MemoryEntity{
		{
			Id:                 "high-overlap-memory",
			Content:            "另一个Go语言记忆",
			Tags:               []string{"编程", "Go语言"}, // 2/4重叠，Jaccard = 2/4 = 0.5 < 0.8
			PotentialQuestions: []string{"Go语言好用吗？"},
			C_Score:            0.7,
			O_Score:            0.8,
			R_Score:            0.8,
			E_Score:            0.5,
			P_Score:            0.7,
			A_Score:            0.8,
			T_Score:            0.7,
			CorePactVector:     []float32{0.7, 0.8, 0.8, 0.5, 0.7, 0.8, 0.7},
		},
		{
			Id:                 "different-memory",
			Content:            "关于数据库的记忆",
			Tags:               []string{"数据库", "SQL", "存储"},
			PotentialQuestions: []string{"什么是数据库？"},
			C_Score:            0.6,
			O_Score:            0.7,
			R_Score:            0.8,
			E_Score:            0.4,
			P_Score:            0.6,
			A_Score:            0.7,
			T_Score:            0.6,
			CorePactVector:     []float32{0.6, 0.7, 0.8, 0.4, 0.6, 0.7, 0.6},
		},
	}

	nonRepeatedIndices, err := memory.BatchIsRepeatedMemoryEntities(testEntities, config)
	if err != nil {
		t.Fatalf("batch repetition check failed: %v", err)
	}

	t.Logf("Non-repeated indices: %v", nonRepeatedIndices)

	// 验证索引有效性
	for _, idx := range nonRepeatedIndices {
		if idx < 0 || idx >= len(testEntities) {
			t.Errorf("invalid index %d, should be in range [0, %d)", idx, len(testEntities))
		}
	}

	// 由于标签重叠度不高，预期两个记忆都不会被标记为重复
	if len(nonRepeatedIndices) == 0 {
		t.Errorf("expected some memories to pass repetition check")
	}

	t.Logf("Batch tag overlap test completed: %d/%d memories passed", len(nonRepeatedIndices), len(testEntities))
}

func cleanupDeduplicationTestData(t *testing.T, sessionID string) {
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
