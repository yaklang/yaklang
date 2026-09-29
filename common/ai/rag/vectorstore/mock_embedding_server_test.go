package vectorstore

import (
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
)

// Rapid calls must not share a wall-clock seed: repeated texts collapse
// independent documents and question indexes into one vector-store entry.
func TestMUSTPASS_MockEmbeddingRapidConcurrentTextGeneration(t *testing.T) {
	client := NewDefaultMockEmbedding()
	const count = 64
	texts := make(chan string, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); texts <- client.GenerateRandomText(32) }()
	}
	wg.Wait()
	close(texts)
	seen := make(map[string]bool)
	for text := range texts {
		require.NotEmpty(t, text)
		require.False(t, seen[text], "independent mock documents unexpectedly repeat")
		seen[text] = true
	}
	require.Len(t, seen, count)
}
