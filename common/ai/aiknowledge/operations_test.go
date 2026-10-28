package aiknowledge

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/schema"
)

func TestKnowledgeListEntryCounts(t *testing.T) {
	db, opts, _, calls := fixture(t)
	for _, name := range []string{"count-a", "count-empty", "count-b"} {
		create(t, opts, name)
	}
	for _, name := range []string{"count-a", "count-a", "count-b"} {
		_, err := AddEntry(name, "entry", "content", append(opts, WithIndex(false))...)
		require.NoError(t, err)
	}
	deleted, err := AddEntry("count-a", "deleted", "excluded", append(opts, WithIndex(false))...)
	require.NoError(t, err)
	require.NoError(t, db.Where("hidden_index = ?", deleted["entry_id"]).Delete(&schema.KnowledgeBaseEntry{}).Error)
	listed, err := List(append(opts, WithKeyword("count-"), WithLimit(2))...)
	require.NoError(t, err)
	require.Equal(t, 3, listed["total"])
	require.Equal(t, true, listed["has_more"])
	page := listed["knowledge_bases"].([]listedBaseInfo)
	require.Len(t, page, 2)
	for i, want := range []int{2, 0} {
		details, err := Get(page[i].Name, opts...)
		require.NoError(t, err)
		require.Equal(t, want, page[i].EntryCount)
		require.Equal(t, details["entry_count"], page[i].EntryCount)
	}
	raw, err := json.Marshal(page)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"entry_count":0`, "empty bases must include their zero count")
	last, err := List(append(opts, WithKeyword("count-"), WithOffset(listed["next_offset"].(int)), WithLimit(2))...)
	require.NoError(t, err)
	require.Equal(t, false, last["has_more"])
	require.Equal(t, 1, last["knowledge_bases"].([]listedBaseInfo)[0].EntryCount)
	empty, err := List(append(opts, WithKeyword("missing"))...)
	require.NoError(t, err)
	require.Empty(t, empty["knowledge_bases"])
	require.Zero(t, calls.Load(), "listing/counting must not contact embedding")
}

func TestKnowledgeLegacyDescriptionReadIsConsistentAndReadOnly(t *testing.T) {
	db, opts, _, _ := fixture(t)
	_, err := Add("legacy", append(opts, WithDescription("canonical 描述"))...)
	require.NoError(t, err)
	require.NoError(t, db.Model(&schema.VectorStoreCollection{}).Where("name = ?", "legacy").Update("description", "").Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_description BEFORE UPDATE OF description ON `+(&schema.VectorStoreCollection{}).TableName()+` BEGIN SELECT RAISE(ABORT, 'READ_MUST_NOT_WRITE'); END`).Error)
	result, err := Get("legacy", opts...)
	require.NoError(t, err)
	require.Equal(t, "canonical 描述", result["knowledge_base"].(baseInfo).Description)
	require.Equal(t, "canonical 描述", result["collection"].(*vectorstore.CollectionInfo).Description)
	require.Equal(t, "", result["stored_collection_description"])
	stored, err := vectorstore.GetCollectionInfo(db, "legacy")
	require.NoError(t, err)
	require.Empty(t, stored.Description, "Get must leave historical storage untouched")
	require.NoError(t, db.Exec(`DROP TRIGGER reject_description`).Error)
	_, err = Update("legacy", append(opts, WithDescription("updated"))...)
	require.NoError(t, err)
	result, err = Get("legacy", opts...)
	require.NoError(t, err)
	require.NotContains(t, result, "stored_collection_description")
	stored, err = vectorstore.GetCollectionInfo(db, "legacy")
	require.NoError(t, err)
	require.Equal(t, "updated", stored.Description)
}

func TestKnowledgeDescriptionUpdatesAreAtomic(t *testing.T) {
	db, opts, _, _ := fixture(t)
	_, err := Add("docs", append(opts, WithDescription("original"))...)
	require.NoError(t, err)
	for _, description := range []string{"updated 描述", ""} {
		_, err = Update("docs", append(opts, WithDescription(description))...)
		require.NoError(t, err)
		result, err := Get("docs", opts...)
		require.NoError(t, err)
		require.Equal(t, description, result["knowledge_base"].(baseInfo).Description)
		require.Equal(t, description, result["collection"].(*vectorstore.CollectionInfo).Description)
	}
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_description BEFORE UPDATE OF description ON `+(&schema.VectorStoreCollection{}).TableName()+` BEGIN SELECT RAISE(ABORT, 'COLLECTION_UPDATE_REFUSED'); END`).Error)
	_, err = Update("docs", append(opts, WithDescription("must roll back"), WithTags("must roll back"))...)
	require.ErrorContains(t, err, "COLLECTION_UPDATE_REFUSED")
	result, err := Get("docs", opts...)
	require.NoError(t, err)
	base := result["knowledge_base"].(baseInfo)
	require.Empty(t, base.Description)
	require.Empty(t, base.Tags)
	require.Empty(t, result["collection"].(*vectorstore.CollectionInfo).Description)
}

func TestKnowledgeNamedOperationsAndOptions(t *testing.T) {
	_, defaults, _, _ := fixture(t)
	options := func(extra ...Option) []Option { return append(append([]Option{}, defaults...), extra...) }
	_, err := Add("docs", options(WithDimension(4), WithDescription("old"), WithDescription("current"), WithTags("docs"), WithIsDefault(true))...)
	require.NoError(t, err)
	_, err = Add("other", options(WithDimension(4))...)
	require.NoError(t, err)
	updated, err := Update("docs", options(WithDescription(""), WithTags(), WithIsDefault(false))...)
	require.NoError(t, err)
	base := updated["knowledge_base"].(baseInfo)
	require.Empty(t, base.Description)
	require.Empty(t, base.Tags)
	require.False(t, base.IsDefault)
	entry, err := AddEntry("docs", "API", "MARKER original", options(WithEntrySummary("summary"), WithEntryKeywords("token"))...)
	require.NoError(t, err)
	id := entry["entry_id"].(string)
	_, err = UpdateEntry("docs", id, "API updated", "MARKER changed", options(WithIndex(false))...)
	require.NoError(t, err)
	_, err = Build("docs", "BUILD_MARKER configuration", options(WithBuildMode("chunks"), WithChunkSize(512), WithConcurrency(1))...)
	require.NoError(t, err)
	for _, scope := range []any{"docs", []string{"docs", "other"}, []any{"docs", "other"}} {
		result, err := Query(scope, "MARKER", 1024, options(WithLimit(1), WithLimit(2), WithPerEntryTokenLimit(256))...)
		require.NoError(t, err)
		require.Len(t, result.Results, 2)
		require.LessOrEqual(t, result.UsedTokens, 1024)
	}
	all, err := Query(nil, "MARKER", 1024, options(WithAll(true))...)
	require.NoError(t, err)
	require.True(t, all.All)
	require.Equal(t, 2, all.KnowledgeBaseCount)
	_, err = Query(nil, "MARKER", 1024, options(WithAll(false))...)
	require.ErrorContains(t, err, "manage_knowledge_base")
	_, err = Query("docs", "token", 1024, options(WithMode("semantic"), WithMinScore(math.NaN()))...)
	require.ErrorContains(t, err, "min_score")
	_, err = UpdateEntry("other", id, "foreign", "must not write", defaults...)
	require.Error(t, err)
	_, err = DeleteEntry("docs", id, defaults...)
	require.NoError(t, err)
	for _, name := range []string{"docs", "other"} {
		_, err = Delete(name, defaults...)
		require.NoError(t, err)
	}
	listed, err := List(defaults...)
	require.NoError(t, err)
	require.Zero(t, listed["total"])
	require.NotContains(t, Exports, "Manage", "Yak operations must have separate named functions")
}
