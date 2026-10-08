package yaklib

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func aiResourceTestDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "resource.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.AutoMigrate(models...).Error)
	return db
}

func TestMUSTPASS_QueryCybersecurityRiskStateAndEvidence(t *testing.T) {
	db := aiResourceTestDB(t, &schema.Risk{})
	first := &schema.Risk{Title: "SQL injection sentinel", Severity: "high", WaitingVerified: false, QuotedRequest: strconv.Quote("GET /?q=' HTTP/1.1\r\n"), Details: strconv.Quote("证据详情"), PacketPairs: schema.PacketPairList{&schema.PacketPair{HTTPFlowId: 42}}}
	second := &schema.Risk{Title: "SQL pending", Severity: "high", WaitingVerified: true}
	require.NoError(t, db.Create(first).Error)
	require.NoError(t, db.Create(second).Error)
	opts := []AIQueryOption{WithAIQueryDatabases(db, db)}
	result, err := QueryCybersecurityRisk(`{"Search":"SQL","Severity":"high"}`, opts...)
	require.NoError(t, err)
	require.Equal(t, 2, result["total"])
	result, err = QueryCybersecurityRisk(`{"WaitingVerified":true}`, opts...)
	require.NoError(t, err)
	require.Equal(t, 1, result["total"])
	result, err = QueryCybersecurityRisk(`{"WaitingVerified":false}`, opts...)
	require.NoError(t, err)
	require.Equal(t, 1, result["total"])
	result, err = QueryCybersecurityRisk(`{"Ids":[`+strconv.Itoa(int(first.ID))+`]}`, opts...)
	require.NoError(t, err)
	hit := result["hits"].([]map[string]any)[0]
	require.Equal(t, []int64{42}, hit["http_flow_ids"])
	var content map[string]any
	require.NoError(t, json.Unmarshal([]byte(hit["content"].(string)), &content))
	require.Equal(t, "证据详情", content["details"])
	require.Equal(t, "GET /?q=' HTTP/1.1\r\n", content["request"])
	var persisted schema.Risk
	require.NoError(t, db.First(&persisted, first.ID).Error)
	require.False(t, persisted.IsRead)
	for _, raw := range []string{`{"Token":"ignored"}`, `{"Pagination":{"Page":2}}`, `{"IsRead":"maybe"}`, `{"typo":true}`, `{} {}`, `null`} {
		_, err = QueryCybersecurityRisk(raw, opts...)
		require.Error(t, err)
	}
}

func TestMUSTPASS_QueryKnowledgeOriginalAndDocumentRecall(t *testing.T) {
	db := aiResourceTestDB(t, &schema.KnowledgeBaseInfo{}, &schema.KnowledgeBaseEntry{}, &schema.VectorStoreCollection{}, &schema.VectorStoreDocument{})
	kb := &schema.KnowledgeBaseInfo{KnowledgeBaseName: "web-security", KnowledgeBaseType: "security", RAGID: "rag-test"}
	require.NoError(t, db.Create(kb).Error)
	entry := &schema.KnowledgeBaseEntry{KnowledgeBaseID: int64(kb.ID), KnowledgeTitle: "CSRF验证", KnowledgeType: "guideline", KnowledgeDetails: "验证令牌\n校验来源\n保存证据", Summary: "cookie sentinel", HiddenIndex: "entry-source-uuid", SourcePage: 7}
	require.NoError(t, db.Create(entry).Error)
	collection := &schema.VectorStoreCollection{Name: "web-index", UUID: "collection-uuid", RAGID: kb.RAGID, Dimension: 3}
	require.NoError(t, db.Create(collection).Error)
	doc := &schema.VectorStoreDocument{CollectionID: collection.ID, CollectionUUID: collection.UUID, DocumentID: "question-uuid", Content: "How to verify cross site forgery?", Metadata: schema.MetadataMap{schema.META_Data_UUID: entry.HiddenIndex}}
	require.NoError(t, db.Create(doc).Error)
	opts := []AIQueryOption{WithAIQueryDatabases(db, db)}
	result, err := QueryKnowledge(`{"source":"collections"}`, opts...)
	require.NoError(t, err)
	require.Equal(t, 1, result["total"])
	result, err = QueryKnowledge(`{"source":"vector_collections"}`, opts...)
	require.NoError(t, err)
	require.Equal(t, 1, result["total"])
	require.Equal(t, "web-index", result["hits"].([]map[string]any)[0]["name"])
	result, err = QueryKnowledge(`{"query":"cookie","collection":"web-security"}`, opts...)
	require.NoError(t, err)
	require.Equal(t, 1, result["total"])
	result, err = QueryKnowledge(`{"source":"documents","query":"forgery","collection":"web-index"}`, opts...)
	require.NoError(t, err)
	hit := result["hits"].([]map[string]any)[0]
	require.Equal(t, entry.HiddenIndex, hit["entry_uuid"])
	require.True(t, result["literal_fallback"].(bool))
	require.NoError(t, yakit.EnsureVectorStoreDocumentFTS5(db))
	result, err = QueryKnowledge(`{"source":"documents","query":"forgery"}`, opts...)
	require.NoError(t, err)
	require.Len(t, result["hits"], 1)
	result, err = QueryKnowledge(`{"entry_uuid":"entry-source-uuid"}`, append(opts, WithAIContentLimit(3))...)
	require.NoError(t, err)
	hit = result["hits"].([]map[string]any)[0]
	require.Equal(t, "验证令", hit["content"])
	require.True(t, hit["truncated"].(bool))
	result, err = QueryKnowledge(`{"entry_uuid":"entry-source-uuid"}`, append(opts, WithAIContentLimit(3), WithAIContentOffset(3))...)
	require.NoError(t, err)
	require.Equal(t, "牌\n校", result["hits"].([]map[string]any)[0]["content"])
	_, err = QueryKnowledge(`{"source":"documents","entry_id":1}`, opts...)
	require.Error(t, err)
}

func TestMUSTPASS_ManagePayloadsTransactionsAndExactContent(t *testing.T) {
	db := aiResourceTestDB(t, &schema.Payload{})
	opts := []AIQueryOption{WithAIQueryDatabases(db, db)}
	result, err := ManagePayloads(`{"operator":"add","group":"api_cases","folder":"web","contents":["a & b"," x\n\"\\ ","a & b"]}`, opts...)
	require.NoError(t, err)
	require.Equal(t, 2, result["affected"])
	require.Equal(t, 1, result["duplicates_skipped"])
	ids := result["payload_ids"].([]int64)
	require.NoError(t, db.Model(&schema.Payload{}).Where("id = ?", ids[0]).UpdateColumn("hit_count", 9).Error)
	result, err = ManagePayloads(`{"operator":"add","group":"api_cases","contents":["a & b"]}`, opts...)
	require.NoError(t, err)
	require.Zero(t, result["affected"])
	result, err = QueryPayloads(`{"group":"api_cases"}`, opts...)
	require.NoError(t, err)
	require.Equal(t, 2, result["total"])
	hits := result["hits"].([]map[string]any)
	require.Equal(t, "a & b", hits[0]["content"])
	require.Equal(t, " x\n\"\\ ", hits[1]["content"])
	require.Equal(t, int64(9), *hits[0]["hit_count"].(*int64))
	usage := result["usage"].(map[string]any)
	require.Equal(t, "{{payload(api_cases)}}", usage["fuzztag"])
	result, err = QueryPayloads(`{}`, opts...)
	require.NoError(t, err)
	require.Equal(t, 1, result["total"])
	result, err = QueryPayloads(`{"group":"api_cases","query":"\n\"\\"}`, opts...)
	require.NoError(t, err)
	require.Equal(t, 1, result["total"])
	_, err = ManagePayloads(`{"operator":"delete","group":"api_cases","payload_ids":[`+strconv.FormatInt(ids[0], 10)+`,999999]}`, opts...)
	require.Error(t, err)
	_, err = ManagePayloads(`{"operator":"change","group":"api_cases","payload_ids":[`+strconv.FormatInt(ids[1], 10)+`],"contents":["a & b"]}`, opts...)
	require.Error(t, err)
	result, err = QueryPayloads(`{"group":"api_cases"}`, opts...)
	require.NoError(t, err)
	require.Equal(t, 2, result["total"])
	require.Equal(t, " x\n\"\\ ", result["hits"].([]map[string]any)[1]["content"])
	result, err = ManagePayloads(`{"operator":"change","group":"api_cases","payload_ids":[`+strconv.FormatInt(ids[1], 10)+`],"contents":["replacement"]}`, opts...)
	require.NoError(t, err)
	require.Equal(t, 1, result["affected"])
	result, err = QueryPayloads(`{"payload_id":`+strconv.FormatInt(ids[1], 10)+`}`, opts...)
	require.NoError(t, err)
	require.Equal(t, "replacement", result["hits"].([]map[string]any)[0]["content"])
	_, err = ManagePayloads(`{"operator":"delete","group":"api_cases","payload_ids":[`+strconv.FormatInt(ids[1], 10)+`]}`, opts...)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ManagePayloads(`{"operator":"delete_group","group":"api_cases"}`, append(opts, WithAIQueryContext(ctx))...)
	require.ErrorIs(t, err, context.Canceled)
	result, err = QueryPayloads(`{"group":"api_cases"}`, opts...)
	require.NoError(t, err)
	require.Equal(t, 1, result["total"])
}

func TestMUSTPASS_QueryPayloadsFileSamplesAndRegistryDeletion(t *testing.T) {
	db := aiResourceTestDB(t, &schema.Payload{})
	opts := []AIQueryOption{WithAIQueryDatabases(db, db)}
	path := filepath.Join(t.TempDir(), "payloads.txt")
	require.NoError(t, os.WriteFile(path, []byte("first\n\"a & b\"\nthird\n"), 0600))
	require.NoError(t, yakit.CreatePayload(db, path, "file_cases", "", 0, true))
	result, err := QueryPayloads(`{"group":"file_cases"}`, append(opts, WithAIQueryLimit(1), WithAIQueryOffset(1))...)
	require.NoError(t, err)
	require.True(t, result["has_more"].(bool))
	hit := result["hits"].([]map[string]any)[0]
	require.Equal(t, "a & b", hit["content"])
	require.Equal(t, 2, hit["line_number"])
	require.False(t, hit["editable"].(bool))
	_, err = ManagePayloads(`{"operator":"add","group":"file_cases","contents":["bad"]}`, opts...)
	require.Error(t, err)
	_, err = ManagePayloads(`{"operator":"delete_group","group":"file_cases"}`, opts...)
	require.NoError(t, err)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "first\n\"a & b\"\nthird\n", string(raw))
}
