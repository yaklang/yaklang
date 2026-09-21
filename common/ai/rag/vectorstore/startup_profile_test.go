package vectorstore

import (
	"bytes"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/schema"
)

// Only aggregate query counts/timings: never print memory contents or SQL args.
// GORM Rows timing excludes iteration; compare wall-clock restore time, not this
// logger duration, when benchmarking batched rows against First queries.
type startupQueryStats struct {
	sync.Mutex
	count int
	cost  time.Duration
}

func (s *startupQueryStats) Print(values ...interface{}) {
	if len(values) < 4 || values[0] != "sql" {
		return
	}
	s.Lock()
	defer s.Unlock()
	s.count++
	if elapsed, ok := values[2].(time.Duration); ok {
		s.cost += elapsed
	}
}

// Opt-in, read-only profiling of an existing collection. No migrations, graph
// recovery, embedding calls, or automatic graph persistence are performed.
func TestProfilePersistedRAGStartup(t *testing.T) {
	filename := os.Getenv("YAK_RAG_STARTUP_PROFILE_DB")
	if filename == "" {
		t.Skip("set YAK_RAG_STARTUP_PROFILE_DB for read-only profiling")
	}
	uri := &url.URL{Scheme: "file", Path: filename, RawQuery: "mode=ro"}
	db, err := gorm.Open("sqlite3", uri.String())
	require.NoError(t, err)
	defer db.Close()
	stats := &startupQueryStats{}
	db.SetLogger(stats)
	db.LogMode(true)
	name := os.Getenv("YAK_RAG_STARTUP_PROFILE_COLLECTION")
	if name == "" {
		name = "ai-memory-default"
	}
	var collection schema.VectorStoreCollection
	start := time.Now()
	require.NoError(t, db.Where("name = ?", name).First(&collection).Error)
	t.Logf("collection metadata: %s; graph bytes=%d", time.Since(start), len(collection.GraphBinary))
	start = time.Now()
	persistent, err := hnsw.LoadBinary[string](bytes.NewReader(collection.GraphBinary))
	require.NoError(t, err)
	t.Logf("binary decode: %s; nodes=%d layers=%d export_mode=%d", time.Since(start), persistent.Total, len(persistent.Layers), persistent.ExportMode)
	stats.count, stats.cost = 0, 0
	start = time.Now()
	graph, err := parseHNSWGraphFromBinary(db, &collection, LoadConfigFromCollectionInfo(&collection), bytes.NewReader(collection.GraphBinary))
	require.NoError(t, err)
	t.Logf("restore: %s; SQL queries=%d SQL dispatch time=%s nodes=%d", time.Since(start), stats.count, stats.cost, graph.Len())
	start = time.Now()
	require.Equal(t, collection.Dimension, graph.Dims())
	t.Logf("first lazy vector/dimension: %s", time.Since(start))
	GraphWrapperManager.ClearCache()
	t.Cleanup(GraphWrapperManager.ClearCache)
	start = time.Now()
	_, err = GetCollection(db, name, WithLazyLoadEmbeddingClient(), WithTryRebuildHNSWIndex(false), WithEnableAutoUpdateGraphInfos(false))
	require.NoError(t, err)
	t.Logf("complete collection load (embedding disabled): %s", time.Since(start))
}
