package aimem

import (
	"github.com/yaklang/yaklang/common/ai/aid/aimem/memory_type"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/schema"
)

func TestAIMemoryHNSWBackend_BasicOperations(t *testing.T) {
	sessionID := "test-hnsw-" + uuid.New().String()

	// 创建HNSW后端
	backend := NewTestAIMemoryHNSWBackend(t, sessionID)
	defer backend.Close()
	db := backend.db

	// 启用自动保存测试
	backend.autoSave = true

	// 测试数据
	entities := []*memory_type.MemoryEntity{
		{
			Id:        uuid.New().String(),
			CreatedAt: time.Now(),
			Content:   "测试记忆1：关于Go编程的知识",
			Tags:      []string{"编程", "Go"},
			PotentialQuestions: []string{
				"如何使用Go编程？",
				"Go语言有什么特点？",
			},
			C_Score:        0.8,
			O_Score:        0.9,
			R_Score:        0.7,
			E_Score:        0.6,
			P_Score:        0.5,
			A_Score:        0.8,
			T_Score:        0.9,
			CorePactVector: []float32{0.8, 0.9, 0.7, 0.6, 0.5, 0.8, 0.9},
		},
		{
			Id:        uuid.New().String(),
			CreatedAt: time.Now(),
			Content:   "测试记忆2：关于Python编程的知识",
			Tags:      []string{"编程", "Python"},
			PotentialQuestions: []string{
				"如何使用Python编程？",
				"Python语言有什么优势？",
			},
			C_Score:        0.7,
			O_Score:        0.8,
			R_Score:        0.8,
			E_Score:        0.7,
			P_Score:        0.6,
			A_Score:        0.7,
			T_Score:        0.8,
			CorePactVector: []float32{0.7, 0.8, 0.8, 0.7, 0.6, 0.7, 0.8},
		},
		{
			Id:        uuid.New().String(),
			CreatedAt: time.Now(),
			Content:   "测试记忆3：关于数据库设计的知识",
			Tags:      []string{"数据库", "设计"},
			PotentialQuestions: []string{
				"如何设计数据库？",
				"数据库设计的原则是什么？",
			},
			C_Score:        0.9,
			O_Score:        0.7,
			R_Score:        0.9,
			E_Score:        0.8,
			P_Score:        0.7,
			A_Score:        0.9,
			T_Score:        0.6,
			CorePactVector: []float32{0.9, 0.7, 0.9, 0.8, 0.7, 0.9, 0.6},
		},
	}

	// 测试添加
	t.Run("Add", func(t *testing.T) {
		for _, entity := range entities {
			// 先保存到数据库
			dbEntity := &schema.AIMemoryEntity{
				MemoryID:           entity.Id,
				SessionID:          sessionID,
				Content:            entity.Content,
				Tags:               schema.StringArray(entity.Tags),
				PotentialQuestions: schema.StringArray(entity.PotentialQuestions),
				C_Score:            entity.C_Score,
				O_Score:            entity.O_Score,
				R_Score:            entity.R_Score,
				E_Score:            entity.E_Score,
				P_Score:            entity.P_Score,
				A_Score:            entity.A_Score,
				T_Score:            entity.T_Score,
				CorePactVector:     schema.FloatArray(entity.CorePactVector),
			}
			if err := db.Create(dbEntity).Error; err != nil {
				t.Fatalf("save to database failed: %v", err)
			}

			// 添加到HNSW索引
			if err := backend.Add(entity); err != nil {
				t.Fatalf("add to HNSW failed: %v", err)
			}
		}
	})

	// 测试搜索
	t.Run("Search", func(t *testing.T) {
		// 搜索与第一个实体相似的记忆
		queryVector := entities[0].CorePactVector
		results, err := backend.Search(queryVector, 3)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}

		if len(results) == 0 {
			t.Fatal("no search results")
		}

		// 第一个结果应该是最相似的（自己）
		if results[0].Entity.Id != entities[0].Id {
			t.Errorf("expected first result to be %s, got %s", entities[0].Id, results[0].Entity.Id)
		}

		// 检查相似度分数
		if results[0].Score < 0.9 {
			t.Errorf("expected high similarity score, got %f", results[0].Score)
		}

		t.Logf("Search results:")
		for i, result := range results {
			t.Logf("  %d. %s (score: %.3f, distance: %.3f)",
				i+1, result.Entity.Content, result.Score, result.Distance)
		}
	})

	// 测试更新
	t.Run("Update", func(t *testing.T) {
		// 修改第一个实体
		updatedEntity := *entities[0]
		updatedEntity.Content = "更新后的测试记忆1：关于Go编程的高级知识"
		updatedEntity.C_Score = 0.9
		updatedEntity.CorePactVector = []float32{0.9, 0.9, 0.7, 0.6, 0.5, 0.8, 0.9}

		// 更新数据库 - 先查询现有记录再更新
		var existingEntity schema.AIMemoryEntity
		if err := db.Where("memory_id = ? AND session_id = ?", updatedEntity.Id, sessionID).
			First(&existingEntity).Error; err != nil {
			t.Fatalf("find existing entity failed: %v", err)
		}

		existingEntity.Content = updatedEntity.Content
		existingEntity.C_Score = updatedEntity.C_Score
		existingEntity.CorePactVector = schema.FloatArray(updatedEntity.CorePactVector)

		if err := db.Save(&existingEntity).Error; err != nil {
			t.Fatalf("update database failed: %v", err)
		}

		// 更新HNSW索引
		if err := backend.Update(&updatedEntity); err != nil {
			t.Fatalf("update HNSW failed: %v", err)
		}

		// 验证更新
		results, err := backend.Search(updatedEntity.CorePactVector, 1)
		if err != nil {
			t.Fatalf("search after update failed: %v", err)
		}

		if len(results) == 0 || results[0].Entity.Id != updatedEntity.Id {
			t.Fatal("updated entity not found in search results")
		}
	})

	// 测试删除
	t.Run("Delete", func(t *testing.T) {
		// 删除第一个实体
		entityToDelete := entities[0]

		// 从数据库删除
		if err := db.Where("memory_id = ? AND session_id = ?", entityToDelete.Id, sessionID).
			Delete(&schema.AIMemoryEntity{}).Error; err != nil {
			t.Fatalf("delete from database failed: %v", err)
		}

		// 从HNSW索引删除
		if err := backend.Delete(entityToDelete.Id); err != nil {
			t.Fatalf("delete from HNSW failed: %v", err)
		}

		// 验证删除
		results, err := backend.Search(entityToDelete.CorePactVector, 10)
		if err != nil {
			t.Fatalf("search after delete failed: %v", err)
		}

		// 确保删除的实体不在结果中
		for _, result := range results {
			if result.Entity.Id == entityToDelete.Id {
				t.Fatal("deleted entity still found in search results")
			}
		}
	})

	// 测试重建索引
	t.Run("RebuildIndex", func(t *testing.T) {
		if err := backend.RebuildIndex(); err != nil {
			t.Fatalf("rebuild index failed: %v", err)
		}

		// 验证重建后的索引
		results, err := backend.Search(entities[1].CorePactVector, 5)
		if err != nil {
			t.Fatalf("search after rebuild failed: %v", err)
		}

		if len(results) == 0 {
			t.Fatal("no results after rebuild")
		}

		t.Logf("Results after rebuild: %d", len(results))
	})

	// 测试统计信息
	t.Run("GetStats", func(t *testing.T) {
		stats := backend.GetStats()
		if stats["session_id"] != sessionID {
			t.Errorf("expected session_id %s, got %v", sessionID, stats["session_id"])
		}

		if stats["dimension"] != 7 {
			t.Errorf("expected dimension 7, got %v", stats["dimension"])
		}

		t.Logf("Stats: %+v", stats)
	})

	// 清理测试数据
	db.Where("session_id = ?", sessionID).Delete(&schema.AIMemoryEntity{})
	db.Where("session_id = ?", sessionID).Delete(&schema.AIMemoryCollection{})
}

func TestAIMemoryHNSWBackend_Persistence(t *testing.T) {
	// 测试HNSW图的持久化
	sessionID := "test-persistence-" + uuid.New().String()
	db, err := getTestDatabase()
	if db == nil || err != nil {
		t.Fatal("getTestDatabase failed")
	}

	db.AutoMigrate(&schema.AIMemoryEntity{}, &schema.AIMemoryCollection{})

	backend, err := NewAIMemoryHNSWBackend(WithHNSWSessionID(sessionID), WithHNSWDatabase(db))
	if err != nil {
		t.Fatalf("create HNSW backend failed: %v", err)
	}

	backend.autoSave = true // 启用自动保存测试并发安全性

	// 第一阶段：创建并保存数据
	t.Run("CreateAndSave", func(t *testing.T) {
		// 启用自动保存用于持久化测试
		backend.autoSave = true

		// 添加测试数据
		entity := &memory_type.MemoryEntity{
			Id:             uuid.New().String(),
			CreatedAt:      time.Now(),
			Content:        "持久化测试记忆",
			Tags:           []string{"测试"},
			C_Score:        0.8,
			O_Score:        0.7,
			R_Score:        0.9,
			E_Score:        0.6,
			P_Score:        0.5,
			A_Score:        0.8,
			T_Score:        0.7,
			CorePactVector: []float32{0.8, 0.7, 0.9, 0.6, 0.5, 0.8, 0.7},
		}

		// 保存到数据库
		dbEntity := &schema.AIMemoryEntity{
			MemoryID:       entity.Id,
			SessionID:      sessionID,
			Content:        entity.Content,
			Tags:           schema.StringArray(entity.Tags),
			C_Score:        entity.C_Score,
			O_Score:        entity.O_Score,
			R_Score:        entity.R_Score,
			E_Score:        entity.E_Score,
			P_Score:        entity.P_Score,
			A_Score:        entity.A_Score,
			T_Score:        entity.T_Score,
			CorePactVector: schema.FloatArray(entity.CorePactVector),
		}
		if err := db.Create(dbEntity).Error; err != nil {
			t.Fatalf("save to database failed: %v", err)
		}

		// 添加到HNSW
		if err := backend.Add(entity); err != nil {
			t.Fatalf("add to HNSW failed: %v", err)
		}

		// 保存图
		if err := backend.SaveGraph(); err != nil {
			t.Fatalf("save graph failed: %v", err)
		}

		backend.Close()
	})

	// 第二阶段：重新加载并验证
	t.Run("LoadAndVerify", func(t *testing.T) {
		loadBackEnd, err := NewAIMemoryHNSWBackend(WithHNSWSessionID(sessionID), WithHNSWDatabase(db))
		if err != nil {
			t.Fatalf("load HNSW backend failed: %v", err)
		}
		defer loadBackEnd.Close()

		// 验证数据是否正确加载
		queryVector := []float32{0.8, 0.7, 0.9, 0.6, 0.5, 0.8, 0.7}
		results, err := loadBackEnd.Search(queryVector, 1)
		if err != nil {
			t.Fatalf("search after reload failed: %v", err)
		}

		if len(results) == 0 {
			t.Fatal("no results after reload")
		}

		if results[0].Score < 0.9 {
			t.Errorf("expected high similarity after reload, got %f", results[0].Score)
		}

		t.Logf("Successfully loaded and verified persisted HNSW graph")
	})

	// 清理
	db.Where("session_id = ?", sessionID).Delete(&schema.AIMemoryEntity{})
	db.Where("session_id = ?", sessionID).Delete(&schema.AIMemoryCollection{})
}
