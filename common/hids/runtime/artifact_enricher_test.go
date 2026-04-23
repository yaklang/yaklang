//go:build hids && linux

package runtime

import (
	"testing"
	"time"
)

func TestArtifactEnricherPrunesExpiredAndOverCapacityEntries(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	enricher := &artifactEnricher{
		window:     time.Minute,
		maxEntries: 2,
		cache: map[string]artifactCacheEntry{
			"expired": {lastUsed: now.Add(-2 * time.Minute)},
			"older":   {lastUsed: now.Add(-40 * time.Second)},
			"fresh-a": {lastUsed: now.Add(-20 * time.Second)},
			"fresh-b": {lastUsed: now.Add(-10 * time.Second)},
		},
	}

	enricher.prune(now)

	if _, exists := enricher.cache["expired"]; exists {
		t.Fatal("expected expired artifact cache entry to be evicted")
	}
	if len(enricher.cache) != 2 {
		t.Fatalf("expected cache to be capacity bounded to 2 entries, got %d", len(enricher.cache))
	}
	if _, exists := enricher.cache["older"]; exists {
		t.Fatal("expected oldest fresh artifact cache entry to be evicted when over capacity")
	}
	if _, exists := enricher.cache["fresh-a"]; !exists {
		t.Fatal("expected newer artifact cache entry to remain")
	}
	if _, exists := enricher.cache["fresh-b"]; !exists {
		t.Fatal("expected newest artifact cache entry to remain")
	}
}
