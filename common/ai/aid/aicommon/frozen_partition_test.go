package aicommon

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewFrozenBlockPartition(t *testing.T) {
	_, ok := NewFrozenBlockPartition("static_facts", "Static Facts", "  \n", 100)
	require.False(t, ok)

	p, ok := NewFrozenBlockPartition("Static-Facts/One", " Static Facts ", " facts body ", 100)
	require.True(t, ok)
	require.Equal(t, "static_facts_one", p.ID)
	require.Equal(t, "Static Facts", p.Title)
	require.Equal(t, "facts body", p.Content)
	require.NotEmpty(t, p.Nonce)
	for _, r := range p.ID {
		require.True(t, (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_')
	}
}

func TestFrozenBlockPartitionNonceStable(t *testing.T) {
	a, ok := NewFrozenBlockPartition("static_facts", "Static Facts", "facts body", 100)
	require.True(t, ok)
	b, ok := NewFrozenBlockPartition("static_facts", "Static Facts", "facts body", 100)
	require.True(t, ok)
	c, ok := NewFrozenBlockPartition("static_facts", "Static Facts", "changed body", 100)
	require.True(t, ok)

	require.Equal(t, a.Nonce, b.Nonce)
	require.NotEqual(t, a.Nonce, c.Nonce)
}

func TestNormalizeFrozenBlockPartitionsSortAndDedup(t *testing.T) {
	partitions := []FrozenBlockPartition{
		{ID: "static_document", Title: "Old", Content: "old", Order: 110},
		{ID: "static_facts", Title: "Static Facts", Content: "facts", Order: 100},
		{ID: "static_document", Title: "Static Document", Content: "new", Order: 110},
		{ID: "empty", Content: "   ", Order: 90},
	}
	got := NormalizeFrozenBlockPartitions(partitions)
	require.Len(t, got, 2)
	require.Equal(t, "static_facts", got[0].ID)
	require.Equal(t, "static_document", got[1].ID)
	require.Equal(t, "new", got[1].Content, "later duplicate ID should overwrite earlier entry")
	require.NotEmpty(t, got[0].Nonce)
	require.NotEmpty(t, got[1].Nonce)
}

func TestFrozenBlockTemplateRendersPartitionTags(t *testing.T) {
	p, ok := NewFrozenBlockPartition("static_facts", "Static Facts", "facts body", 100)
	require.True(t, ok)
	materials := &PromptMaterials{FrozenPartitions: []FrozenBlockPartition{p}}
	rendered, err := RenderPromptTemplate("test-frozen", SharedFrozenBlockTemplate, materials.FrozenBlockData())
	require.NoError(t, err)

	start := "<|FROZEN_PARTITION_static_facts_" + p.Nonce + "|>"
	end := "<|FROZEN_PARTITION_END_static_facts_" + p.Nonce + "|>"
	require.Contains(t, rendered, "# Static Facts")
	require.Contains(t, rendered, start)
	require.Contains(t, rendered, end)
	require.Less(t, strings.Index(rendered, start), strings.Index(rendered, end))
}

func TestFrozenBlockPartitionsFromConfig(t *testing.T) {
	document, ok := NewFrozenBlockPartition("static_document", "Static Document", "document", 110)
	require.True(t, ok)
	facts, ok := NewFrozenBlockPartition("static_facts", "Static Facts", "facts", 100)
	require.True(t, ok)
	producer := NewFrozenBlockPartitionProducer()
	producer.AppendPartition(document)
	producer.AppendPartition(facts)
	cfg := NewConfig(nil, WithFrozenBlockPartitionProducer(producer))

	partitions := FrozenBlockPartitionsFromConfig(cfg)
	require.Len(t, partitions, 2)
	require.Equal(t, "static_facts", partitions[0].ID)
	require.Equal(t, "static_document", partitions[1].ID)
}
