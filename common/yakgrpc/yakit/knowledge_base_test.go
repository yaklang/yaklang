package yakit

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestMUSTPASS_CreateKnowledgeBase 测试创建知识库
func TestMUSTPASS_CreateKnowledgeBase(t *testing.T) {
	// 创建临时测试数据库
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NoError(t, err)
	defer db.Close()

	// 自动迁移数据库表结构
	db.AutoMigrate(&schema.KnowledgeBaseInfo{})

	// 测试创建知识库
	knowledgeBase := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        "test_knowledge_base",
		KnowledgeBaseDescription: "这是一个测试知识库",
		KnowledgeBaseType:        "test",
	}

	err = CreateKnowledgeBase(db, knowledgeBase)
	assert.NoError(t, err)
	assert.NotZero(t, knowledgeBase.ID) // 确保ID已生成

	// 验证数据已保存到数据库
	var savedKnowledgeBase schema.KnowledgeBaseInfo
	err = db.Where("knowledge_base_name = ?", "test_knowledge_base").First(&savedKnowledgeBase).Error
	assert.NoError(t, err)
	assert.Equal(t, "test_knowledge_base", savedKnowledgeBase.KnowledgeBaseName)
	assert.Equal(t, "这是一个测试知识库", savedKnowledgeBase.KnowledgeBaseDescription)
	assert.Equal(t, "test", savedKnowledgeBase.KnowledgeBaseType)
}

// TestMUSTPASS_CreateKnowledgeBaseDuplicate 测试创建重复名称的知识库
func TestMUSTPASS_CreateKnowledgeBaseDuplicate(t *testing.T) {
	// 创建临时测试数据库
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NoError(t, err)
	defer db.Close()

	// 自动迁移数据库表结构
	db.AutoMigrate(&schema.KnowledgeBaseInfo{})

	// 创建第一个知识库
	knowledgeBase1 := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        "duplicate_test",
		KnowledgeBaseDescription: "第一个知识库",
		KnowledgeBaseType:        "test",
	}
	err = CreateKnowledgeBase(db, knowledgeBase1)
	assert.NoError(t, err)

	// 尝试创建同名的知识库（应该失败）
	knowledgeBase2 := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        "duplicate_test",
		KnowledgeBaseDescription: "第二个知识库",
		KnowledgeBaseType:        "test",
	}
	err = CreateKnowledgeBase(db, knowledgeBase2)
	assert.Error(t, err) // 应该返回错误
}

// TestMUSTPASS_GetKnowledgeBase 测试获取知识库
func TestMUSTPASS_GetKnowledgeBase(t *testing.T) {
	// 创建临时测试数据库
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NoError(t, err)
	defer db.Close()

	// 自动迁移数据库表结构
	db.AutoMigrate(&schema.KnowledgeBaseInfo{})

	// 创建测试知识库
	originalKB := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        "get_test_kb",
		KnowledgeBaseDescription: "用于测试获取功能的知识库",
		KnowledgeBaseType:        "test",
	}
	err = CreateKnowledgeBase(db, originalKB)
	assert.NoError(t, err)

	// 测试获取知识库
	retrievedKB, err := GetKnowledgeBase(db, int64(originalKB.ID))
	assert.NoError(t, err)
	assert.NotNil(t, retrievedKB)
	assert.Equal(t, originalKB.KnowledgeBaseName, retrievedKB.KnowledgeBaseName)
	assert.Equal(t, originalKB.KnowledgeBaseDescription, retrievedKB.KnowledgeBaseDescription)
	assert.Equal(t, originalKB.KnowledgeBaseType, retrievedKB.KnowledgeBaseType)

	// 测试获取不存在的知识库
	_, err = GetKnowledgeBase(db, 99999)
	assert.Error(t, err)
}

// TestMUSTPASS_UpdateKnowledgeBaseInfo 测试更新知识库信息
func TestMUSTPASS_UpdateKnowledgeBaseInfo(t *testing.T) {
	// 创建临时测试数据库
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NoError(t, err)
	defer db.Close()

	// 自动迁移数据库表结构
	db.AutoMigrate(&schema.KnowledgeBaseInfo{})

	// 创建测试知识库
	originalKB := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        "update_test_kb",
		KnowledgeBaseDescription: "原始描述",
		KnowledgeBaseType:        "test",
	}
	err = CreateKnowledgeBase(db, originalKB)
	assert.NoError(t, err)

	// 更新知识库信息（注意：不能更新KnowledgeBaseName，因为它有唯一索引）
	updatedKB := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        "update_test_kb", // 保持原名称不变
		KnowledgeBaseDescription: "更新后的描述",
		KnowledgeBaseType:        "updated_test",
	}
	err = UpdateKnowledgeBaseInfo(db, int64(originalKB.ID), updatedKB)
	assert.NoError(t, err)

	// 验证更新是否成功
	retrievedKB, err := GetKnowledgeBase(db, int64(originalKB.ID))
	assert.NoError(t, err)
	assert.Equal(t, "update_test_kb", retrievedKB.KnowledgeBaseName) // 名称保持不变
	assert.Equal(t, "更新后的描述", retrievedKB.KnowledgeBaseDescription)
	assert.Equal(t, "updated_test", retrievedKB.KnowledgeBaseType)

	// 测试更新不存在的知识库
	err = UpdateKnowledgeBaseInfo(db, 99999, updatedKB)
	assert.Error(t, err)
}

// TestMUSTPASS_DeleteKnowledgeBase 测试删除知识库
func TestMUSTPASS_DeleteKnowledgeBase(t *testing.T) {
	// 创建临时测试数据库
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NoError(t, err)
	defer db.Close()

	// 自动迁移数据库表结构
	db.AutoMigrate(&schema.KnowledgeBaseInfo{}, &schema.KnowledgeBaseEntry{})

	// 创建测试知识库
	testKB := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        "delete_test_kb",
		KnowledgeBaseDescription: "待删除的测试知识库",
		KnowledgeBaseType:        "test",
	}
	err = CreateKnowledgeBase(db, testKB)
	assert.NoError(t, err)

	// 创建相关的知识库条目
	testEntry := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:    int64(testKB.ID),
		KnowledgeTitle:     "测试条目",
		KnowledgeType:      "test",
		ImportanceScore:    5,
		KnowledgeDetails:   "这是一个测试条目",
		Summary:            "测试条目摘要",
		SourcePage:         1,
		Keywords:           schema.StringArray{"测试", "条目"},
		PotentialQuestions: schema.StringArray{"这是什么"},
	}
	err = CreateKnowledgeBaseEntry(db, testEntry)
	assert.NoError(t, err)

	// 删除知识库（应该连同条目一起删除）
	err = DeleteKnowledgeBase(db, int64(testKB.ID))
	assert.NoError(t, err)

	// 验证知识库已被删除
	_, err = GetKnowledgeBase(db, int64(testKB.ID))
	assert.Error(t, err)

	// 验证相关条目也已被删除
	var entryCount int64
	err = db.Model(&schema.KnowledgeBaseEntry{}).Where("knowledge_base_id = ?", testKB.ID).Count(&entryCount).Error
	assert.NoError(t, err)
	assert.Equal(t, int64(0), entryCount)
}

// TestMUSTPASS_GetKnowledgeBaseNameList 测试获取知识库名称列表
func TestMUSTPASS_GetKnowledgeBaseNameList(t *testing.T) {
	// 创建临时测试数据库
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NoError(t, err)
	defer db.Close()

	// 自动迁移数据库表结构
	db.AutoMigrate(&schema.KnowledgeBaseInfo{})

	// 创建多个测试知识库
	testKBs := []*schema.KnowledgeBaseInfo{
		{
			KnowledgeBaseName:        "kb_test_1",
			KnowledgeBaseDescription: "第一个测试知识库",
			KnowledgeBaseType:        "test",
		},
		{
			KnowledgeBaseName:        "kb_test_2",
			KnowledgeBaseDescription: "第二个测试知识库",
			KnowledgeBaseType:        "test",
		},
		{
			KnowledgeBaseName:        "kb_test_3",
			KnowledgeBaseDescription: "第三个测试知识库",
			KnowledgeBaseType:        "test",
		},
	}

	for _, kb := range testKBs {
		err = CreateKnowledgeBase(db, kb)
		assert.NoError(t, err)
	}

	// 获取知识库名称列表
	nameList, err := GetKnowledgeBaseNameList(db)
	assert.NoError(t, err)
	assert.Len(t, nameList, 3)
	assert.Contains(t, nameList, "kb_test_1")
	assert.Contains(t, nameList, "kb_test_2")
	assert.Contains(t, nameList, "kb_test_3")
}

// TestMUSTPASS_CreateKnowledgeBaseEntry 测试创建知识库条目
func TestMUSTPASS_CreateKnowledgeBaseEntry(t *testing.T) {
	// 创建临时测试数据库
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NoError(t, err)
	defer db.Close()

	// 自动迁移数据库表结构
	db.AutoMigrate(&schema.KnowledgeBaseInfo{}, &schema.KnowledgeBaseEntry{})

	// 创建测试知识库
	testKB := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        "entry_test_kb",
		KnowledgeBaseDescription: "用于测试条目的知识库",
		KnowledgeBaseType:        "test",
	}
	err = CreateKnowledgeBase(db, testKB)
	assert.NoError(t, err)

	// 创建知识库条目
	testEntry := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:    int64(testKB.ID),
		KnowledgeTitle:     "Golang并发编程",
		KnowledgeType:      "Programming",
		ImportanceScore:    9,
		Keywords:           schema.StringArray{"Go", "并发", "goroutine", "channel"},
		KnowledgeDetails:   "Go语言的并发模型基于goroutine和channel，提供了简洁而强大的并发编程能力。",
		Summary:            "Go语言并发编程基础",
		SourcePage:         42,
		PotentialQuestions: schema.StringArray{"什么是goroutine", "如何使用channel", "Go并发有什么优势"},
	}

	err = CreateKnowledgeBaseEntry(db, testEntry)
	assert.NoError(t, err)
	assert.NotZero(t, testEntry.ID) // 确保ID已生成

	// 验证数据已保存到数据库
	var savedEntry schema.KnowledgeBaseEntry
	err = db.Where("knowledge_title = ?", "Golang并发编程").First(&savedEntry).Error
	assert.NoError(t, err)
	assert.Equal(t, testEntry.KnowledgeTitle, savedEntry.KnowledgeTitle)
	assert.Equal(t, testEntry.KnowledgeType, savedEntry.KnowledgeType)
	assert.Equal(t, testEntry.ImportanceScore, savedEntry.ImportanceScore)
	assert.Equal(t, int64(testKB.ID), savedEntry.KnowledgeBaseID)
}

// TestMUSTPASS_GetKnowledgeBaseEntryById 测试根据ID获取知识库条目
func TestMUSTPASS_GetKnowledgeBaseEntryById(t *testing.T) {
	// 创建临时测试数据库
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NoError(t, err)
	defer db.Close()

	// 自动迁移数据库表结构
	db.AutoMigrate(&schema.KnowledgeBaseInfo{}, &schema.KnowledgeBaseEntry{})

	// 创建测试知识库
	testKB := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        "get_entry_test_kb",
		KnowledgeBaseDescription: "用于测试获取条目的知识库",
		KnowledgeBaseType:        "test",
	}
	err = CreateKnowledgeBase(db, testKB)
	assert.NoError(t, err)

	// 创建知识库条目
	originalEntry := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:    int64(testKB.ID),
		KnowledgeTitle:     "Python数据分析",
		KnowledgeType:      "DataScience",
		ImportanceScore:    8,
		Keywords:           schema.StringArray{"Python", "pandas", "numpy", "数据分析"},
		KnowledgeDetails:   "Python在数据分析领域有着丰富的生态系统，pandas和numpy是最常用的库。",
		Summary:            "Python数据分析工具介绍",
		SourcePage:         15,
		PotentialQuestions: schema.StringArray{"pandas怎么用", "numpy的优势", "Python数据分析流程"},
	}
	err = CreateKnowledgeBaseEntry(db, originalEntry)
	assert.NoError(t, err)

	// 测试获取条目
	retrievedEntry, err := GetKnowledgeBaseEntryByHiddenIndex(db, originalEntry.HiddenIndex)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedEntry)
	assert.Equal(t, originalEntry.KnowledgeTitle, retrievedEntry.KnowledgeTitle)
	assert.Equal(t, originalEntry.KnowledgeType, retrievedEntry.KnowledgeType)
	assert.Equal(t, originalEntry.ImportanceScore, retrievedEntry.ImportanceScore)

	// 测试获取不存在的条目
	_, err = GetKnowledgeBaseEntryByHiddenIndex(db, utils.RandStringBytes(10))
	assert.Error(t, err)
}

// TestMUSTPASS_UpdateKnowledgeBaseEntry 测试更新知识库条目
func TestMUSTPASS_UpdateKnowledgeBaseEntry(t *testing.T) {
	// 创建临时测试数据库
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NoError(t, err)
	defer db.Close()

	// 自动迁移数据库表结构
	db.AutoMigrate(&schema.KnowledgeBaseInfo{}, &schema.KnowledgeBaseEntry{})

	// 创建测试知识库
	testKB := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        "update_entry_test_kb",
		KnowledgeBaseDescription: "用于测试更新条目的知识库",
		KnowledgeBaseType:        "test",
	}
	err = CreateKnowledgeBase(db, testKB)
	assert.NoError(t, err)

	// 创建知识库条目
	originalEntry := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:    int64(testKB.ID),
		KnowledgeTitle:     "JavaScript基础",
		KnowledgeType:      "WebDevelopment",
		ImportanceScore:    7,
		Keywords:           schema.StringArray{"JavaScript", "前端", "编程"},
		KnowledgeDetails:   "JavaScript是一种动态编程语言，主要用于网页开发。",
		Summary:            "JavaScript编程语言介绍",
		SourcePage:         8,
		PotentialQuestions: schema.StringArray{"JavaScript是什么", "如何学习JavaScript"},
	}
	err = CreateKnowledgeBaseEntry(db, originalEntry)
	assert.NoError(t, err)

	// 更新条目信息
	originalEntry.KnowledgeTitle = "JavaScript高级编程"
	originalEntry.ImportanceScore = 9
	originalEntry.KnowledgeDetails = "JavaScript是一种功能强大的动态编程语言，不仅用于前端开发，也广泛应用于后端开发（Node.js）。"
	originalEntry.Keywords = schema.StringArray{"JavaScript", "前端", "后端", "Node.js", "编程"}

	err = UpdateKnowledgeBaseEntryByHiddenIndex(db, originalEntry.HiddenIndex, originalEntry)
	assert.NoError(t, err)

	// 验证更新是否成功
	retrievedEntry, err := GetKnowledgeBaseEntryByHiddenIndex(db, originalEntry.HiddenIndex)
	assert.NoError(t, err)
	assert.Equal(t, "JavaScript高级编程", retrievedEntry.KnowledgeTitle)
	assert.Equal(t, 9, retrievedEntry.ImportanceScore)
	assert.Contains(t, retrievedEntry.KnowledgeDetails, "Node.js")
	assert.Contains(t, retrievedEntry.Keywords, "Node.js")
}

// TestMUSTPASS_DeleteKnowledgeBaseEntry 测试删除知识库条目
func TestMUSTPASS_DeleteKnowledgeBaseEntry(t *testing.T) {
	// 创建临时测试数据库
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NoError(t, err)
	defer db.Close()

	// 自动迁移数据库表结构
	db.AutoMigrate(&schema.KnowledgeBaseInfo{}, &schema.KnowledgeBaseEntry{})

	// 创建测试知识库
	testKB := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        "delete_entry_test_kb",
		KnowledgeBaseDescription: "用于测试删除条目的知识库",
		KnowledgeBaseType:        "test",
	}
	err = CreateKnowledgeBase(db, testKB)
	assert.NoError(t, err)

	// 创建知识库条目
	testEntry := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:    int64(testKB.ID),
		KnowledgeTitle:     "待删除的条目",
		KnowledgeType:      "Test",
		ImportanceScore:    5,
		Keywords:           schema.StringArray{"删除", "测试"},
		KnowledgeDetails:   "这是一个用于测试删除功能的条目",
		Summary:            "测试删除",
		SourcePage:         1,
		PotentialQuestions: schema.StringArray{"如何删除"},
	}
	err = CreateKnowledgeBaseEntry(db, testEntry)
	assert.NoError(t, err)

	// 删除条目
	err = DeleteKnowledgeBaseEntryByHiddenIndex(db, testEntry.HiddenIndex)
	assert.NoError(t, err)

	// 验证条目已被删除
	_, err = GetKnowledgeBaseEntryByHiddenIndex(db, testEntry.HiddenIndex)
	assert.Error(t, err)
}

// TestMUSTPASS_SearchKnowledgeBaseEntry 测试搜索知识库条目
func TestMUSTPASS_SearchKnowledgeBaseEntry(t *testing.T) {
	// 创建临时测试数据库
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NoError(t, err)
	defer db.Close()

	// 自动迁移数据库表结构
	db.AutoMigrate(&schema.KnowledgeBaseInfo{}, &schema.KnowledgeBaseEntry{})

	// 创建测试知识库
	testKB := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        "search_test_kb",
		KnowledgeBaseDescription: "用于测试搜索的知识库",
		KnowledgeBaseType:        "test",
	}
	err = CreateKnowledgeBase(db, testKB)
	assert.NoError(t, err)

	// 创建多个测试条目
	testEntries := []*schema.KnowledgeBaseEntry{
		{
			KnowledgeBaseID:    int64(testKB.ID),
			KnowledgeTitle:     "Python机器学习",
			KnowledgeType:      "MachineLearning",
			ImportanceScore:    9,
			Keywords:           schema.StringArray{"Python", "机器学习", "scikit-learn"},
			KnowledgeDetails:   "Python是机器学习领域最受欢迎的编程语言，有丰富的库支持。",
			Summary:            "Python在机器学习中的应用",
			SourcePage:         20,
			PotentialQuestions: schema.StringArray{"Python机器学习库有哪些"},
		},
		{
			KnowledgeBaseID:    int64(testKB.ID),
			KnowledgeTitle:     "Java企业级开发",
			KnowledgeType:      "EnterpriseJava",
			ImportanceScore:    8,
			Keywords:           schema.StringArray{"Java", "企业级", "Spring"},
			KnowledgeDetails:   "Java在企业级应用开发中占据重要地位，Spring框架是最流行的选择。",
			Summary:            "Java企业级开发技术栈",
			SourcePage:         35,
			PotentialQuestions: schema.StringArray{"Spring框架怎么用"},
		},
		{
			KnowledgeBaseID:    int64(testKB.ID),
			KnowledgeTitle:     "Go语言微服务",
			KnowledgeType:      "Microservices",
			ImportanceScore:    8,
			Keywords:           schema.StringArray{"Go", "微服务", "Docker"},
			KnowledgeDetails:   "Go语言因其出色的并发性能，在微服务架构中越来越受欢迎。",
			Summary:            "Go语言在微服务中的优势",
			SourcePage:         50,
			PotentialQuestions: schema.StringArray{"Go微服务架构设计"},
		},
	}

	for _, entry := range testEntries {
		err = CreateKnowledgeBaseEntry(db, entry)
		assert.NoError(t, err)
	}

	// 测试搜索功能
	// 搜索包含"Python"的条目
	pythonResults, err := SearchKnowledgeBaseEntry(db, int64(testKB.ID), "Python")
	assert.NoError(t, err)
	assert.Len(t, pythonResults, 1)
	assert.Equal(t, "Python机器学习", pythonResults[0].KnowledgeTitle)

	// 搜索包含"微服务"的条目
	microserviceResults, err := SearchKnowledgeBaseEntry(db, int64(testKB.ID), "微服务")
	assert.NoError(t, err)
	assert.Len(t, microserviceResults, 1)
	assert.Equal(t, "Go语言微服务", microserviceResults[0].KnowledgeTitle)

	// 搜索包含"企业"的条目
	enterpriseResults, err := SearchKnowledgeBaseEntry(db, int64(testKB.ID), "企业")
	assert.NoError(t, err)
	assert.Len(t, enterpriseResults, 1)
	assert.Equal(t, "Java企业级开发", enterpriseResults[0].KnowledgeTitle)

	// 搜索不存在的关键词
	noResults, err := SearchKnowledgeBaseEntry(db, int64(testKB.ID), "不存在的关键词")
	assert.NoError(t, err)
	assert.Len(t, noResults, 0)

	// 空搜索（应该返回所有条目）
	allResults, err := SearchKnowledgeBaseEntry(db, int64(testKB.ID), "")
	assert.NoError(t, err)
	assert.Len(t, allResults, 3)
}

// TestMUSTPASS_GetKnowledgeBaseEntryByFilter 测试分页获取知识库条目
func TestMUSTPASS_GetKnowledgeBaseEntryByFilter(t *testing.T) {
	// 创建临时测试数据库
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NoError(t, err)
	defer db.Close()

	// 自动迁移数据库表结构
	db.AutoMigrate(&schema.KnowledgeBaseInfo{}, &schema.KnowledgeBaseEntry{})

	// 创建测试知识库
	testKB := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        "filter_test_kb",
		KnowledgeBaseDescription: "用于测试分页过滤的知识库",
		KnowledgeBaseType:        "test",
	}
	err = CreateKnowledgeBase(db, testKB)
	assert.NoError(t, err)

	// 创建多个测试条目（总共10个）
	for i := 1; i <= 10; i++ {
		testEntry := &schema.KnowledgeBaseEntry{
			KnowledgeBaseID:    int64(testKB.ID),
			KnowledgeTitle:     fmt.Sprintf("测试条目 %d", i),
			KnowledgeType:      "Test",
			ImportanceScore:    i % 10,
			Keywords:           schema.StringArray{fmt.Sprintf("关键词%d", i), "测试"},
			KnowledgeDetails:   fmt.Sprintf("这是第%d个测试条目的详细信息", i),
			Summary:            fmt.Sprintf("测试条目%d摘要", i),
			SourcePage:         i,
			PotentialQuestions: schema.StringArray{fmt.Sprintf("关于条目%d的问题", i)},
		}
		err = CreateKnowledgeBaseEntry(db, testEntry)
		assert.NoError(t, err)
	}

	// 测试分页功能
	// 第一页，每页5条
	paging1 := &ypb.Paging{
		Page:  1,
		Limit: 5,
	}
	paginator1, entries1, err := GetKnowledgeBaseEntryByFilter(db, int64(testKB.ID), "", paging1)
	assert.NoError(t, err)
	assert.NotNil(t, paginator1)
	assert.Len(t, entries1, 5)
	assert.Equal(t, int(10), paginator1.TotalRecord) // 修正类型，TotalRecord是int类型

	// 第二页，每页5条
	paging2 := &ypb.Paging{
		Page:  2,
		Limit: 5,
	}
	paginator2, entries2, err := GetKnowledgeBaseEntryByFilter(db, int64(testKB.ID), "", paging2)
	assert.NoError(t, err)
	assert.NotNil(t, paginator2)
	assert.Len(t, entries2, 5)
	assert.Equal(t, int(10), paginator2.TotalRecord)

	// 测试关键词过滤 + 分页
	paging3 := &ypb.Paging{
		Page:  1,
		Limit: 3,
	}
	paginator3, entries3, err := GetKnowledgeBaseEntryByFilter(db, int64(testKB.ID), "测试", paging3)
	assert.NoError(t, err)
	assert.NotNil(t, paginator3)
	assert.Len(t, entries3, 3)                       // 第一页只取3条
	assert.Equal(t, int(10), paginator3.TotalRecord) // 但总数应该是10（所有条目都包含"测试"）

	// 测试超出范围的页数
	pagingOverLimit := &ypb.Paging{
		Page:  10,
		Limit: 5,
	}
	paginatorOverLimit, entriesOverLimit, err := GetKnowledgeBaseEntryByFilter(db, int64(testKB.ID), "", pagingOverLimit)
	assert.NoError(t, err)
	assert.NotNil(t, paginatorOverLimit)
	assert.Len(t, entriesOverLimit, 0) // 超出范围，应该返回空数组

	// 测试 AfterId - 获取ID大于指定值的记录
	// 先获取第一个条目的ID
	pagingFirst := &ypb.Paging{
		Page:  1,
		Limit: 1,
	}
	_, firstEntries, err := GetKnowledgeBaseEntryByFilter(db, int64(testKB.ID), "", pagingFirst)
	assert.NoError(t, err)
	assert.Len(t, firstEntries, 1)
	firstEntryID := firstEntries[0].ID

	// 测试获取ID大于第一个条目的所有记录
	pagingAfter := &ypb.Paging{
		Page:    1,
		Limit:   20, // 足够大以获取所有后续记录
		AfterId: int64(firstEntryID),
	}
	paginatorAfter, entriesAfter, err := GetKnowledgeBaseEntryByFilter(db, int64(testKB.ID), "", pagingAfter)
	assert.NoError(t, err)
	assert.NotNil(t, paginatorAfter)
	assert.Len(t, entriesAfter, 9) // 应该有9条记录（总共10条，排除第一条）
	// 验证所有返回的记录ID都大于firstEntryID
	for _, entry := range entriesAfter {
		assert.Greater(t, entry.ID, firstEntryID, "AfterID过滤后的记录ID应该大于指定ID")
	}

	// 测试 BeforeId - 获取ID小于指定值的记录
	// 先获取最后一个条目的ID
	pagingLast := &ypb.Paging{
		Page:  2,
		Limit: 5,
	}
	_, lastPageEntries, err := GetKnowledgeBaseEntryByFilter(db, int64(testKB.ID), "", pagingLast)
	assert.NoError(t, err)
	assert.Len(t, lastPageEntries, 5)
	lastEntryID := lastPageEntries[len(lastPageEntries)-1].ID

	// 测试获取ID小于最后一个条目的所有记录
	pagingBefore := &ypb.Paging{
		Page:     1,
		Limit:    20, // 足够大以获取所有之前的记录
		BeforeId: int64(lastEntryID),
	}
	paginatorBefore, entriesBefore, err := GetKnowledgeBaseEntryByFilter(db, int64(testKB.ID), "", pagingBefore)
	assert.NoError(t, err)
	assert.NotNil(t, paginatorBefore)
	assert.Len(t, entriesBefore, 9) // 应该有9条记录（总共10条，排除最后一条）
	// 验证所有返回的记录ID都小于lastEntryID
	for _, entry := range entriesBefore {
		assert.Less(t, entry.ID, lastEntryID, "BeforeID过滤后的记录ID应该小于指定ID")
	}

	// 测试同时使用 AfterId 和 BeforeId - 获取指定范围内的记录
	// 获取中间范围的记录（第3到第7条）
	pagingRange := &ypb.Paging{
		Page:  1,
		Limit: 3,
	}
	_, rangeEntries, err := GetKnowledgeBaseEntryByFilter(db, int64(testKB.ID), "", pagingRange)
	assert.NoError(t, err)
	assert.Len(t, rangeEntries, 3)
	thirdEntryID := rangeEntries[len(rangeEntries)-1].ID

	pagingRange2 := &ypb.Paging{
		Page:  3,
		Limit: 3,
	}
	_, rangeEntries2, err := GetKnowledgeBaseEntryByFilter(db, int64(testKB.ID), "", pagingRange2)
	assert.NoError(t, err)
	assert.Greater(t, len(rangeEntries2), 0)
	eighthEntryID := rangeEntries2[0].ID

	// 使用 AfterId 和 BeforeId 获取第4到第7条记录
	pagingBetween := &ypb.Paging{
		Page:     1,
		Limit:    10,
		AfterId:  int64(thirdEntryID),
		BeforeId: int64(eighthEntryID),
	}
	paginatorBetween, entriesBetween, err := GetKnowledgeBaseEntryByFilter(db, int64(testKB.ID), "", pagingBetween)
	assert.NoError(t, err)
	assert.NotNil(t, paginatorBetween)
	// 验证所有返回的记录ID在指定范围内
	for _, entry := range entriesBetween {
		assert.Greater(t, entry.ID, thirdEntryID, "记录ID应该大于AfterId")
		assert.Less(t, entry.ID, eighthEntryID, "记录ID应该小于BeforeId")
	}

	// 测试 AfterId 配合关键词过滤
	pagingAfterWithKeyword := &ypb.Paging{
		Page:    1,
		Limit:   10,
		AfterId: int64(firstEntryID),
	}
	paginatorAfterKeyword, entriesAfterKeyword, err := GetKnowledgeBaseEntryByFilter(db, int64(testKB.ID), "测试", pagingAfterWithKeyword)
	assert.NoError(t, err)
	assert.NotNil(t, paginatorAfterKeyword)
	assert.Len(t, entriesAfterKeyword, 9) // 所有记录都包含"测试"，排除第一条
	// 验证过滤条件都满足
	for _, entry := range entriesAfterKeyword {
		assert.Greater(t, entry.ID, firstEntryID)
	}
}

// TestMUSTPASS_KnowledgeBaseCompleteWorkflow 测试知识库完整工作流程
func TestMUSTPASS_KnowledgeBaseCompleteWorkflow(t *testing.T) {
	// 创建临时测试数据库
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NoError(t, err)
	defer db.Close()

	// 自动迁移数据库表结构
	db.AutoMigrate(&schema.KnowledgeBaseInfo{}, &schema.KnowledgeBaseEntry{})

	// 1. 创建知识库
	kb := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        "complete_workflow_test",
		KnowledgeBaseDescription: "完整工作流程测试知识库",
		KnowledgeBaseType:        "workflow_test",
	}
	err = CreateKnowledgeBase(db, kb)
	assert.NoError(t, err)

	// 2. 创建多个条目
	entries := []*schema.KnowledgeBaseEntry{
		{
			KnowledgeBaseID:    int64(kb.ID),
			KnowledgeTitle:     "云计算基础",
			KnowledgeType:      "CloudComputing",
			ImportanceScore:    9,
			Keywords:           schema.StringArray{"云计算", "AWS", "Azure", "Docker"},
			KnowledgeDetails:   "云计算是一种提供可配置的计算资源共享池的模式，包括网络、服务器、存储、应用和服务。",
			Summary:            "云计算技术概述",
			SourcePage:         1,
			PotentialQuestions: schema.StringArray{"什么是云计算", "云计算有什么优势"},
		},
		{
			KnowledgeBaseID:    int64(kb.ID),
			KnowledgeTitle:     "容器技术",
			KnowledgeType:      "ContainerTech",
			ImportanceScore:    8,
			Keywords:           schema.StringArray{"Docker", "Kubernetes", "容器", "微服务"},
			KnowledgeDetails:   "容器技术提供了轻量级的虚拟化解决方案，Docker和Kubernetes是最流行的容器技术。",
			Summary:            "容器技术和编排",
			SourcePage:         2,
			PotentialQuestions: schema.StringArray{"Docker怎么用", "Kubernetes的作用"},
		},
	}

	for _, entry := range entries {
		err = CreateKnowledgeBaseEntry(db, entry)
		assert.NoError(t, err)
	}

	// 3. 测试搜索功能
	searchResults, err := SearchKnowledgeBaseEntry(db, int64(kb.ID), "Docker")
	assert.NoError(t, err)
	assert.Len(t, searchResults, 2) // 两个条目都包含Docker

	// 4. 测试分页获取
	paging := &ypb.Paging{Page: 1, Limit: 10}
	paginator, allEntries, err := GetKnowledgeBaseEntryByFilter(db, int64(kb.ID), "", paging)
	assert.NoError(t, err)
	assert.Equal(t, int(2), paginator.TotalRecord)
	assert.Len(t, allEntries, 2)

	// 5. 更新第一个条目
	firstEntry := allEntries[0]
	firstEntry.ImportanceScore = 10
	firstEntry.KnowledgeDetails += " 云计算已成为现代IT基础设施的核心。"
	err = UpdateKnowledgeBaseEntryByHiddenIndex(db, firstEntry.HiddenIndex, firstEntry)
	assert.NoError(t, err)

	// 验证更新
	updatedEntry, err := GetKnowledgeBaseEntryByHiddenIndex(db, firstEntry.HiddenIndex)
	assert.NoError(t, err)
	assert.Equal(t, 10, updatedEntry.ImportanceScore)
	assert.Contains(t, updatedEntry.KnowledgeDetails, "云计算已成为现代IT基础设施的核心")

	// 6. 删除一个条目
	err = DeleteKnowledgeBaseEntryByHiddenIndex(db, allEntries[1].HiddenIndex)
	assert.NoError(t, err)

	// 验证删除
	_, err = GetKnowledgeBaseEntryByHiddenIndex(db, allEntries[1].HiddenIndex)
	assert.Error(t, err)

	// 7. 获取知识库名称列表
	nameList, err := GetKnowledgeBaseNameList(db)
	assert.NoError(t, err)
	assert.Contains(t, nameList, "complete_workflow_test")

	// 8. 最后删除整个知识库
	err = DeleteKnowledgeBase(db, int64(kb.ID))
	assert.NoError(t, err)

	// 验证知识库和剩余条目都被删除
	_, err = GetKnowledgeBase(db, int64(kb.ID))
	assert.Error(t, err)

	var remainingEntryCount int64
	err = db.Model(&schema.KnowledgeBaseEntry{}).Where("knowledge_base_id = ?", kb.ID).Count(&remainingEntryCount).Error
	assert.NoError(t, err)
	assert.Equal(t, int64(0), remainingEntryCount)
}
