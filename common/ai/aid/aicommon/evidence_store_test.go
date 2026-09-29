package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEvidenceStore_ApplyOperationsAtMetadataAndLegacyUnmarshal(t *testing.T) {
	store := NewEvidenceStore()
	store.ApplyOperationsAt([]EvidenceOperation{{Op: "add", ID: "a", Content: "first"}}, 100)
	require.Len(t, store.Items, 1)
	require.Equal(t, int64(100), store.Items[0].CreatedUnix)
	require.Equal(t, int64(100), store.Items[0].UpdatedUnix)

	store.ApplyOperationsAt([]EvidenceOperation{{Op: "update", ID: "a", Content: "second"}}, 120)
	require.Len(t, store.Items, 1)
	require.Equal(t, int64(100), store.Items[0].CreatedUnix)
	require.Equal(t, int64(120), store.Items[0].UpdatedUnix)
	require.Equal(t, "second", store.Items[0].Content)

	store.ApplyOperationsAt([]EvidenceOperation{{Op: "delete", ID: "a"}}, 130)
	require.Empty(t, store.Items)

	legacy := UnmarshalEvidenceStore(`{"items":[{"id":"legacy-id","content":"legacy content"}]}`)
	require.Len(t, legacy.Items, 1)
	require.Equal(t, int64(1), legacy.Items[0].CreatedUnix)
	require.Equal(t, int64(1), legacy.Items[0].UpdatedUnix)
}

func TestBuildSessionEvidenceUpsert_StableFallbackAndValidation(t *testing.T) {
	first, err := BuildSessionEvidenceUpsert("", "  confirmed result\r\nwith details  ")
	require.NoError(t, err)
	second, err := BuildSessionEvidenceUpsert("", "confirmed result\nwith details")
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	require.Regexp(t, `^saved_[0-9a-f]{16}$`, first.ID)
	require.Equal(t, "add", first.Op)
	require.Equal(t, "confirmed result\nwith details", first.Content)

	explicit, err := BuildSessionEvidenceUpsert("api-auth:v2", "401 confirmed")
	require.NoError(t, err)
	require.Equal(t, "api-auth:v2", explicit.ID)

	_, err = BuildSessionEvidenceUpsert("bad id with spaces", "content")
	require.ErrorContains(t, err, "invalid session evidence id")
	_, err = BuildSessionEvidenceUpsert("valid-id", "  ")
	require.ErrorContains(t, err, "content is required")
}

func TestSessionPromptState_EvidencePersistenceUsesLiveStoreOnly(t *testing.T) {
	c := evidenceConfig(t)
	s := c.GetSessionPromptState()
	c.ApplySessionEvidenceOps([]EvidenceOperation{
		{Op: "add", ID: "a", Content: "A"},
		{Op: "add", ID: "b", Content: "B"},
		{Op: "delete", ID: "a"},
	})
	store := UnmarshalEvidenceStore(s.GetSessionEvidence())
	require.Len(t, store.Items, 1)
	require.Equal(t, "b", store.Items[0].ID)
}

func TestConfigEvidenceWithoutTimelineKeepsMirror(t *testing.T) {
	c := &Config{SessionPromptState: NewSessionPromptState()}
	c.ApplySessionEvidenceOps([]EvidenceOperation{{Op: "add", ID: "read", Content: "observed first"}})
	require.Contains(t, c.GetSessionEvidenceRendered(), "observed first")
	c.ApplySessionEvidenceOps([]EvidenceOperation{{Op: "add", ID: "read", Content: "observed last"}})
	require.NotContains(t, c.GetSessionEvidenceRendered(), "observed first")
	require.Contains(t, c.GetSessionEvidenceRendered(), "observed last")
	require.Len(t, UnmarshalEvidenceStore(c.GetSessionPromptState().GetSessionEvidence()).Items, 1)
}
