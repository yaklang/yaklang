package yaklib

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestMUSTPASS_ManageKnowledgeLifecycleAndScope(t *testing.T) {
	db := aiResourceTestDB(t, &schema.KnowledgeBaseInfo{}, &schema.KnowledgeBaseEntry{}, &schema.VectorStoreCollection{}, &schema.VectorStoreDocument{})
	opts := []AIQueryOption{WithAIQueryDatabases(db, db)}
	added, err := ManageKnowledge(`{"operator":"add","collection":"research notes","title":"认证规则","content":"old original sentinel","keywords":["a,b","认证"],"questions":["需要登录吗？"]}`, opts...)
	require.NoError(t, err)
	require.True(t, added["created_knowledge_base"].(bool))
	require.Equal(t, "keyword_ready", added["index_status"])
	require.Equal(t, "pending", added["semantic_index_status"])
	uuid := added["entry_uuid"].(string)
	kbID := added["knowledge_base_id"].(uint)
	original, err := QueryKnowledge(`{"query":"original sentinel"}`, opts...)
	require.NoError(t, err)
	require.Equal(t, 1, original["total"])
	foreign := &schema.KnowledgeBaseInfo{KnowledgeBaseName: "foreign", KnowledgeBaseType: "knowledge"}
	require.NoError(t, db.Create(foreign).Error)
	_, err = ManageKnowledge(fmt.Sprintf(`{"operator":"delete","knowledge_base_id":%d,"entry_uuid":%q}`, foreign.ID, uuid), opts...)
	require.Error(t, err)
	_, err = ManageKnowledge(fmt.Sprintf(`{"operator":"delete","knowledge_base_id":%d,"collection":"foreign","entry_uuid":%q}`, kbID, uuid), opts...)
	require.Error(t, err)
	require.NoError(t, yakit.EnsureVectorStoreDocumentFTS5(db))
	collection := &schema.VectorStoreCollection{Name: "original indexes", UUID: "managed-indexes", Dimension: 3, GraphBinary: []byte("old graph snapshot")}
	require.NoError(t, db.Create(collection).Error)
	for _, document := range []*schema.VectorStoreDocument{
		{CollectionID: collection.ID, CollectionUUID: collection.UUID, DocumentID: uuid, Content: "old original sentinel"},
		{CollectionID: collection.ID, CollectionUUID: collection.UUID, DocumentID: uuid + "_question_1", Content: "old question sentinel"},
		{CollectionID: collection.ID, CollectionUUID: collection.UUID, DocumentID: "metadata-index", Content: "metadata question sentinel", Metadata: schema.MetadataMap{schema.META_Data_UUID: uuid}},
		{CollectionID: collection.ID, CollectionUUID: collection.UUID, DocumentID: "foreign-document", Content: "foreign survives"},
	} {
		require.NoError(t, db.Create(document).Error)
	}
	changed, err := ManageKnowledge(fmt.Sprintf(`{"operator":"change","knowledge_base_id":%d,"entry_uuid":%q,"title":"认证更新","content":"new original sentinel"}`, kbID, uuid), opts...)
	require.NoError(t, err)
	require.NotEqual(t, uuid, changed["entry_uuid"])
	require.Equal(t, 3, changed["removed_index_count"])
	var count int
	require.NoError(t, db.Unscoped().Model(&schema.KnowledgeBaseEntry{}).Where("hidden_index = ?", uuid).Count(&count).Error)
	require.Zero(t, count)
	var storedCollection schema.VectorStoreCollection
	require.NoError(t, db.First(&storedCollection, collection.ID).Error)
	require.Empty(t, storedCollection.GraphBinary)
	original, err = QueryKnowledge(`{"source":"documents","query":"old"}`, opts...)
	require.NoError(t, err)
	require.Empty(t, original["hits"])
	original, err = QueryKnowledge(`{"query":"new original"}`, opts...)
	require.NoError(t, err)
	require.Equal(t, 1, original["total"])
	original, err = QueryKnowledge(`{"source":"documents","query":"foreign survives"}`, opts...)
	require.NoError(t, err)
	require.Len(t, original["hits"], 1)
	deleted, err := ManageKnowledge(fmt.Sprintf(`{"operator":"delete","knowledge_base_id":%d,"entry_uuid":%q}`, kbID, changed["entry_uuid"]), opts...)
	require.NoError(t, err)
	require.Equal(t, "deleted", deleted["index_status"])
	original, err = QueryKnowledge(fmt.Sprintf(`{"knowledge_base_id":%d}`, kbID), opts...)
	require.NoError(t, err)
	require.Zero(t, original["total"])
}

func TestMUSTPASS_ManageKnowledgeReplacementRollsBackIndexes(t *testing.T) {
	db := aiResourceTestDB(t, &schema.KnowledgeBaseInfo{}, &schema.KnowledgeBaseEntry{}, &schema.VectorStoreCollection{}, &schema.VectorStoreDocument{})
	kb := &schema.KnowledgeBaseInfo{KnowledgeBaseName: "rollback", KnowledgeBaseType: "knowledge"}
	require.NoError(t, db.Create(kb).Error)
	entry := &schema.KnowledgeBaseEntry{KnowledgeBaseID: int64(kb.ID), HiddenIndex: "import_%_uuid", KnowledgeTitle: "old", KnowledgeType: "note", KnowledgeDetails: "keep original"}
	require.NoError(t, db.Create(entry).Error)
	collection := &schema.VectorStoreCollection{Name: "rollback indexes", UUID: "rollback-uuid", Dimension: 3, GraphBinary: []byte("keep snapshot")}
	require.NoError(t, db.Create(collection).Error)
	for _, id := range []string{entry.HiddenIndex + "_question_0", "import_X_uuid_question_0"} {
		require.NoError(t, db.Create(&schema.VectorStoreDocument{DocumentID: id, CollectionID: collection.ID, Content: "keep index"}).Error)
	}
	db.Callback().Create().Before("gorm:create").Register("reject_replacement", func(scope *gorm.Scope) {
		if row, ok := scope.Value.(*schema.KnowledgeBaseEntry); ok && row.KnowledgeTitle == "reject" {
			scope.Err(fmt.Errorf("injected create failure"))
		}
	})
	defer db.Callback().Create().Remove("reject_replacement")
	opts := []AIQueryOption{WithAIQueryDatabases(db, db)}
	_, err := ManageKnowledge(fmt.Sprintf(`{"operator":"change","knowledge_base_id":%d,"entry_uuid":%q,"title":"reject","content":"replacement"}`, kb.ID, entry.HiddenIndex), opts...)
	require.Error(t, err)
	var old schema.KnowledgeBaseEntry
	require.NoError(t, db.First(&old, entry.ID).Error)
	require.Equal(t, "keep original", old.KnowledgeDetails)
	var count int
	require.NoError(t, db.Model(&schema.VectorStoreDocument{}).Count(&count).Error)
	require.Equal(t, 2, count)
	var stored schema.VectorStoreCollection
	require.NoError(t, db.First(&stored, collection.ID).Error)
	require.Equal(t, []byte("keep snapshot"), stored.GraphBinary)
	_, err = ManageKnowledge(fmt.Sprintf(`{"operator":"delete","knowledge_base_id":%d,"entry_uuid":%q}`, kb.ID, entry.HiddenIndex), opts...)
	require.NoError(t, err)
	require.NoError(t, db.Model(&schema.VectorStoreDocument{}).Count(&count).Error)
	require.Equal(t, 1, count, "UUID wildcards must not delete another entry's index")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ManageKnowledge(`{"operator":"add","collection":"canceled","title":"title","content":"content"}`, append(opts, WithAIQueryContext(ctx))...)
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, db.Model(&schema.KnowledgeBaseInfo{}).Where("knowledge_base_name = ?", "canceled").Count(&count).Error)
	require.Zero(t, count)
}

func TestMUSTPASS_ManageKnowledgeRejectsInvalidArguments(t *testing.T) {
	db := aiResourceTestDB(t, &schema.KnowledgeBaseInfo{}, &schema.KnowledgeBaseEntry{}, &schema.VectorStoreCollection{}, &schema.VectorStoreDocument{})
	for _, params := range []map[string]any{
		{"operator": "add", "title": "title", "content": "body"},
		{"operator": "change", "collection": "notes", "title": "title", "content": "body"},
		{"operator": "add", "collection": "notes", "entry_id": 1, "title": "title", "content": "body"},
		{"operator": "add", "collection": "notes", "title": "title", "content": "body", "importance": 11},
		{"operator": "delete", "collection": "notes", "entry_uuid": "exact", "content": "ignored"},
	} {
		raw, err := json.Marshal(params)
		require.NoError(t, err)
		_, err = ManageKnowledge(string(raw), WithAIQueryDatabases(db, db))
		require.Error(t, err)
	}
	var count int
	require.NoError(t, db.Model(&schema.KnowledgeBaseInfo{}).Count(&count).Error)
	require.Zero(t, count)
}
