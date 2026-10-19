package aiknowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/embedding"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/schema"
)

func fixture(t *testing.T) (*gorm.DB, []Option, *atomic.Bool, *atomic.Int32) {
	t.Helper()
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "profile.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	db.DB().SetMaxOpenConns(1)
	failed, calls := &atomic.Bool{}, &atomic.Int32{}
	embed := vectorstore.NewMockEmbedder(func(string) ([]float32, error) {
		calls.Add(1)
		if failed.Load() {
			return nil, fmt.Errorf("EXPECTED_EMBEDDING_FAILURE")
		}
		return []float32{0.1, 0.2, 0.4, 0.8}, nil
	})
	return db, []Option{WithDatabase(db), WithEmbeddingClient(embed)}, failed, calls
}

func manage(t *testing.T, opts []Option, req map[string]any) map[string]any {
	t.Helper()
	result, err := Manage(req, opts...)
	require.NoError(t, err)
	return result
}

func create(t *testing.T, opts []Option, name string) {
	t.Helper()
	manage(t, opts, map[string]any{"op": "create", "name": name, "metadata": map[string]any{"description": "fixture", "tags": []string{"docs"}, "is_default": true}, "index_options": map[string]any{"dimension": 4, "m": 8, "ml": 0.3, "ef_search": 40, "ef_construct": 50}})
}

func TestKnowledgeLifecycleAndBoundedQueries(t *testing.T) {
	db, opts, failed, calls := fixture(t)
	create(t, opts, "docs")
	require.Zero(t, calls.Load(), "creating metadata must not contact embedding")
	_, err := Manage(map[string]any{"op": "create", "name": "docs"}, opts...)
	require.Error(t, err)
	create(t, opts, "other")
	listed := manage(t, opts, map[string]any{"op": "list", "limit": 1})
	require.Equal(t, 2, listed["total"])
	require.Equal(t, true, listed["has_more"])
	require.Equal(t, false, manage(t, opts, map[string]any{"op": "list", "limit": 1, "offset": 1})["has_more"])
	updated := manage(t, opts, map[string]any{"op": "update", "name": "docs", "metadata": map[string]any{"description": "", "tags": []string{}, "is_default": false}})["knowledge_base"].(baseInfo)
	require.Empty(t, updated.Description)
	require.Empty(t, updated.Tags)
	require.False(t, updated.IsDefault)
	collection := manage(t, opts, map[string]any{"op": "get", "name": "docs"})["collection"].(*vectorstore.CollectionInfo)
	require.Equal(t, 4, collection.Dimension)
	content := strings.Repeat("知识原文\n带引号\"和反斜杠\\😀 ", 100)
	added := manage(t, opts, map[string]any{"op": "add_entry", "name": "docs", "entry": map[string]any{"title": "启动", "content": content}, "index": false})
	id := added["entry_id"].(string)
	otherID := manage(t, opts, map[string]any{"op": "add_entry", "name": "other", "entry": map[string]any{"title": "foreign", "content": "FOREIGN_SECRET"}, "index": false})["entry_id"]
	_, err = Query(map[string]any{"name": "docs", "mode": "get", "entry_id": otherID, "token_limit": 512}, opts...)
	require.Error(t, err)
	_, err = Manage(map[string]any{"op": "delete_entry", "name": "docs", "entry_id": otherID}, opts...)
	require.Error(t, err)
	var reconstructed strings.Builder
	offset := 0
	for i := 0; i < 1000; i++ {
		result, err := Query(map[string]any{"name": "docs", "mode": "get", "entry_id": id, "token_limit": 256, "content_offset": offset}, opts...)
		require.NoError(t, err)
		require.Len(t, result.Results, 1)
		hit := result.Results[0]
		raw, err := json.Marshal(result)
		require.NoError(t, err)
		require.LessOrEqual(t, ytoken.CalcTokenCount(string(raw)), result.UsedTokens)
		require.LessOrEqual(t, result.UsedTokens, 256)
		reconstructed.WriteString(hit.Content)
		if !hit.Truncated {
			break
		}
		require.Greater(t, hit.NextContentOffset, offset, "continuation must make progress")
		offset = hit.NextContentOffset
	}
	require.Equal(t, content, reconstructed.String())
	result, err := Query(map[string]any{"name": "docs", "query": "FOREIGN_SECRET", "token_limit": 512}, opts...)
	require.NoError(t, err)
	require.Empty(t, result.Results)
	manage(t, opts, map[string]any{"op": "rebuild_index", "name": "docs"})
	result, err = Query(map[string]any{"name": "docs", "query": "启动", "mode": "semantic", "token_limit": 512}, opts...)
	require.NoError(t, err)
	require.NotEmpty(t, result.Results)
	require.Equal(t, id, result.Results[0].EntryID, "rebuilding must retain stable entry identity")
	failed.Store(true)
	_, err = Manage(map[string]any{"op": "update_entry", "name": "docs", "entry_id": id, "entry": map[string]any{"title": "new", "content": "should fail"}}, opts...)
	require.ErrorContains(t, err, "EXPECTED_EMBEDDING_FAILURE")
	var stored schema.KnowledgeBaseEntry
	require.NoError(t, db.Where("hidden_index = ?", id).First(&stored).Error)
	require.Equal(t, content, stored.KnowledgeDetails, "provider failure must preserve the old row")
	failed.Store(false)
	manage(t, opts, map[string]any{"op": "update_entry", "name": "docs", "entry_id": id, "entry": map[string]any{"title": "changed", "content": "NEW_CONTENT"}, "index": false})
	result, err = Query(map[string]any{"name": "docs", "mode": "get", "entry_id": id, "token_limit": 512}, opts...)
	require.NoError(t, err)
	require.Equal(t, "NEW_CONTENT", result.Results[0].Content)
	manage(t, opts, map[string]any{"op": "delete_entry", "name": "docs", "entry_id": id})
	manage(t, opts, map[string]any{"op": "delete", "name": "docs"})
	require.Equal(t, 1, manage(t, opts, map[string]any{"op": "list"})["total"])
}

func TestKnowledgeBuildAndValidation(t *testing.T) {
	_, opts, _, _ := fixture(t)
	create(t, opts, "build")
	file := filepath.Join(t.TempDir(), "input.txt")
	require.NoError(t, os.WriteFile(file, []byte("SMOKE_BUILD_CONTENT\n启动时加载配置"), 0600))
	result := manage(t, opts, map[string]any{"op": "build", "name": "build", "source_file": file, "build_options": map[string]any{"mode": "chunks", "chunk_size": 512, "concurrency": 1}})
	require.Greater(t, result["entries_added"].(int), 0)
	query, err := Query(map[string]any{"name": "build", "query": "SMOKE_BUILD_CONTENT", "token_limit": 512}, opts...)
	require.NoError(t, err)
	require.NotEmpty(t, query.Results)
	callback := func(_ aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		response := aicommon.NewUnboundAIResponse()
		response.EmitOutputStream(strings.NewReader(`{"@action":"object","chunk_list":[{"title":"AI generated index","answer_location":{"start_line":1,"end_line":2},"question_list":["How to start the service?"]}]}`))
		response.Close()
		return response, nil
	}
	withAI := append(append([]Option{}, opts...), WithAIService(callback))
	manage(t, withAI, map[string]any{"op": "build", "name": "build", "content": "AI_INDEX_MARKER\nStart the service with yak", "build_options": map[string]any{"mode": "question_index", "concurrency": 1}})
	query, err = Query(map[string]any{"name": "build", "query": "AI_INDEX_MARKER", "token_limit": 512}, opts...)
	require.NoError(t, err)
	require.NotEmpty(t, query.Results)
	for _, req := range []map[string]any{
		{"op": "build", "name": "build"},
		{"op": "build", "name": "build", "content": "x", "source_file": file},
		{"op": "build", "name": "build", "content": "x", "build_options": map[string]any{"concurrency": 17}},
		{"op": "build", "name": "missing", "content": "x"},
		{"op": "create", "name": "bad", "index_options": map[string]any{"dimension": -1}},
		{"op": "list", "offset": -1},
		{"op": "update", "name": "build"},
		{"op": "get", "name": "build", "unknown_parameter": 1},
	} {
		_, err := Manage(req, opts...)
		require.Error(t, err, "%v", req)
	}
	for _, req := range []map[string]any{
		{"token_limit": 512}, {"name": "build"}, {"name": "missing", "token_limit": 512},
		{"name": "build", "token_limit": 127}, {"name": "build", "mode": "semantic", "token_limit": 512},
		{"name": "build", "token_limit": 512, "content_offset": 1},
	} {
		_, err := Query(req, opts...)
		require.Error(t, err, "%v", req)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Manage(map[string]any{"op": "delete", "name": "build"}, append(opts, WithContext(ctx))...)
	require.ErrorIs(t, err, context.Canceled)
}

func TestLegacyVectorIdentityAndDeletion(t *testing.T) {
	db, opts, _, _ := fixture(t)
	create(t, opts, "legacy")
	added := manage(t, opts, map[string]any{"op": "add_entry", "name": "legacy", "entry": map[string]any{"title": "legacy", "content": "LEGACY_CONTENT"}, "index": false})
	var entry schema.KnowledgeBaseEntry
	require.NoError(t, db.Where("hidden_index = ?", added["entry_id"]).First(&entry).Error)
	c, err := newConfig(opts)
	require.NoError(t, err)
	kb, err := c.load("legacy")
	require.NoError(t, err)
	require.NoError(t, kb.GetVectorStore().Add(&vectorstore.Document{ID: strconv.Itoa(int(entry.ID)), Content: "LEGACY_CONTENT", Metadata: map[string]any{"knowledge_base_id": entry.KnowledgeBaseID}}))
	result, err := Query(map[string]any{"name": "legacy", "mode": "semantic", "query": "LEGACY_CONTENT", "token_limit": 512}, opts...)
	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	require.Equal(t, entry.HiddenIndex, result.Results[0].EntryID)
	manage(t, opts, map[string]any{"op": "delete_entry", "name": "legacy", "entry_id": entry.HiddenIndex})
	result, err = Query(map[string]any{"name": "legacy", "mode": "semantic", "query": "LEGACY_CONTENT", "token_limit": 512}, opts...)
	require.NoError(t, err)
	require.Empty(t, result.Results)
}

func TestSplitEmbeddingDocumentsSurviveReplacement(t *testing.T) {
	_, opts, _, _ := fixture(t)
	client := vectorstore.NewMockEmbedder(func(text string) ([]float32, error) {
		if len(text) > 1024 {
			return nil, embedding.ErrInputTooLarge
		}
		return []float32{0.1, 0.2, 0.4, 0.8}, nil
	})
	opts = append(opts, WithEmbeddingClient(client))
	create(t, opts, "chunks")
	added := manage(t, opts, map[string]any{"op": "add_entry", "name": "chunks", "entry": map[string]any{"title": "split entry", "content": strings.Repeat("old content ", 500)}})
	id := added["entry_id"]
	for _, body := range []string{strings.Repeat("new content ", 220), "short replacement"} {
		manage(t, opts, map[string]any{"op": "update_entry", "name": "chunks", "entry_id": id, "entry": map[string]any{"title": "split entry", "content": body}})
		result, err := Query(map[string]any{"name": "chunks", "mode": "semantic", "query": "content", "token_limit": 4096, "per_entry_token_limit": 4096}, opts...)
		require.NoError(t, err)
		require.Len(t, result.Results, 1)
		require.Equal(t, body, result.Results[0].Content)
	}
	manage(t, opts, map[string]any{"op": "delete_entry", "name": "chunks", "entry_id": id})
	result, err := Query(map[string]any{"name": "chunks", "mode": "semantic", "query": "content", "token_limit": 512}, opts...)
	require.NoError(t, err)
	require.Empty(t, result.Results)
}

type dimensionedEmbedding struct{ vectorstore.EmbeddingClient }

func (*dimensionedEmbedding) GetModelDimension() int { return 4 }

func TestKnowledgeRejectsMismatchedEmbeddingDimension(t *testing.T) {
	db, opts, _, _ := fixture(t)
	client := vectorstore.NewMockEmbedder(func(string) ([]float32, error) { return []float32{1, 2, 3, 4}, nil })
	opts = append(opts, WithEmbeddingClient(&dimensionedEmbedding{client}))
	manage(t, opts, map[string]any{"op": "create", "name": "mismatch", "index_options": map[string]any{"dimension": 8}})
	_, err := Manage(map[string]any{"op": "add_entry", "name": "mismatch", "entry": map[string]any{"title": "test", "content": "must not persist"}}, opts...)
	require.ErrorContains(t, err, "dimension 8 does not match")
	var count int
	require.NoError(t, db.Model(&schema.KnowledgeBaseEntry{}).Count(&count).Error)
	require.Zero(t, count)
}
