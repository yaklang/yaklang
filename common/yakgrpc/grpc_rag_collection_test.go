package yakgrpc

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/entityrepos"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/consts"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestMUSTPASS_RAGCollectionSearch(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("skipping test in github actions")
	}

	client, err := NewLocalClient(true)
	require.NoError(t, err)

	db := consts.GetGormProfileDatabase()
	require.NotNil(t, db)

	collectionName := "test_rag_collection_" + utils.RandStringBytes(6)
	repository, err := entityrepos.GetOrCreateEntityRepository(db, collectionName, "test", entityrepos.WithDisableBulkProcess())
	if err != nil {
		return
	}
	reposInfo, _ := repository.GetInfo()

	entity1 := &schema.ERModelEntity{
		EntityName: "Go语言基础",
		EntityType: "ProgrammingLanguage",
	}

	err = repository.SaveEntity(entity1)
	require.NoError(t, err)

	time.Sleep(2 * time.Second) // 等待向量索引创建完成

	t.Cleanup(func() {
		yakit.DeleteEntities(db, &ypb.EntityFilter{
			BaseIndex: reposInfo.Uuid,
		})
	})

	// 3. 创建测试知识库
	knowledgeBase := &schema.KnowledgeBaseInfo{
		KnowledgeBaseName:        collectionName,
		KnowledgeBaseDescription: "测试单个条目的知识库",
		KnowledgeBaseType:        "test",
	}
	err = yakit.CreateKnowledgeBase(db, knowledgeBase)
	assert.NoError(t, err)

	// 获取创建的知识库ID

	kb, err := knowledgebase.NewKnowledgeBase(db, collectionName, "test", "test")
	require.NoError(t, err)

	t.Cleanup(func() {
		yakit.DeleteKnowledgeBase(db, int64(kb.GetID()))
	})

	// 4. 创建测试知识库条目
	testEntry := &schema.KnowledgeBaseEntry{
		KnowledgeBaseID:    kb.GetID(),
		KnowledgeTitle:     "Go语言基础",
		KnowledgeType:      "ProgrammingLanguage",
		ImportanceScore:    8,
		Keywords:           schema.StringArray{"Go", "Golang", "编程语言", "并发"},
		KnowledgeDetails:   "Go是Google开发的一种静态强类型、编译型语言。Go语言语法与C相近，但功能上有：内存安全，GC（垃圾回收），结构形态及CSP-style并发计算。",
		Summary:            "Go是Google开发的编程语言",
		SourcePage:         1,
		PotentialQuestions: schema.StringArray{"什么是Go语言", "Go语言有什么特点", "Go语言适用于什么场景"},
	}

	err = kb.AddKnowledgeEntry(testEntry)
	require.NoError(t, err)

	// 清理
	t.Cleanup(func() {
		yakit.DeleteKnowledgeBaseEntryByHiddenIndex(db, testEntry.HiddenIndex)
	})

	t.Cleanup(func() {
		rag.DeleteCollection(db, collectionName)
	})

	// 查询
	tests := []struct {
		name         string
		query        string
		docType      string
		expectEntity bool
		expectKnow   bool
	}{
		{
			name:         "entity",
			query:        "go语言",
			docType:      string(schema.RAGDocumentType_Entity),
			expectEntity: true,
		},
		{
			name:       "knowledge",
			query:      "go语言",
			docType:    string(schema.RAGDocumentType_Knowledge),
			expectKnow: true,
		},
	}

	for _, tc := range tests {
		stream, err := client.RAGCollectionSearch(context.Background(), &ypb.RAGCollectionSearchRequest{
			Query:          tc.query,
			CollectionName: collectionName,
			DocumentType:   []string{tc.docType},
			Limit:          3,
		})
		require.NoError(t, err)

		var gotEntity, gotKnow bool
		for {
			resp, err := stream.Recv()
			if err != nil {
				break
			}
			switch resp.Type {
			case string(schema.RAGDocumentType_Entity):
				gotEntity = gotEntity || (resp.Entity != nil && resp.Entity.GetHiddenIndex() == entity1.Uuid)
			case string(schema.RAGDocumentType_Knowledge):
				gotKnow = gotKnow || (resp.Knowledge != nil && resp.Knowledge.GetHiddenIndex() == testEntry.HiddenIndex)
			}
		}
		if tc.expectEntity {
			require.True(t, gotEntity)
		}
		if tc.expectKnow {
			require.True(t, gotKnow)
		}
	}
}
