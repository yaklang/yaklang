package vectorstore

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type beirRecord struct {
	ID    string `json:"_id"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

func TestSciFactRemoteEmbeddingEval(t *testing.T) {
	if os.Getenv("YAK_RAG_REMOTE_EMBED") == "" {
		t.Skip("set YAK_RAG_REMOTE_EMBED=1 to call the free embedding service")
	}
	root := os.Getenv("YAK_RAG_SCIFACT")
	if root == "" {
		root = filepath.Join(os.TempDir(), "yak-rag-eval", "scifact", "scifact")
	}
	corpusPath := filepath.Join(root, "corpus.jsonl")
	queryPath := filepath.Join(root, "queries.jsonl")
	qrelPath := filepath.Join(root, "qrels", "test.tsv")
	for _, path := range []string{corpusPath, queryPath, qrelPath} {
		if _, err := os.Stat(path); err != nil {
			t.Skipf("SciFact file missing (%s); download the BEIR scifact zip first", path)
		}
	}

	corpus, err := readBEIR(corpusPath, true)
	require.NoError(t, err)
	queries, err := readBEIR(queryPath, false)
	require.NoError(t, err)
	qrels, err := readQrels(qrelPath)
	require.NoError(t, err)
	evalQueries := make([]beirRecord, 0, len(qrels))
	for _, query := range queries {
		if len(qrels[query.ID]) > 0 {
			evalQueries = append(evalQueries, query)
		}
	}
	require.Greater(t, len(corpus), 1000)
	require.Greater(t, len(evalQueries), 100)
	fmt.Fprintf(os.Stderr, "SCIFACT corpus=%d labeled_queries=%d\n", len(corpus), len(evalQueries))

	embedder, err := NewAIBalanceFreeEmbedder()
	require.NoError(t, err)
	require.True(t, embedder.IsAvailable(), "free embedding service is not available")

	cachePath := filepath.Join(root, "qwen3-1024.embedcache")
	cache, err := loadEmbedCache(cachePath)
	require.NoError(t, err)
	jobs := make([]embedJob, 0, len(corpus)+len(evalQueries))
	for _, doc := range corpus {
		jobs = append(jobs, embedJob{key: "d:" + doc.ID, text: doc.Text})
	}
	for _, query := range evalQueries {
		jobs = append(jobs, embedJob{key: "q:" + query.ID, text: query.Text})
	}
	embedStarted := time.Now()
	require.NoError(t, cache.fillMissing(embedder, jobs, 4))
	fmt.Fprintf(os.Stderr, "SCIFACT embed_done cached=%d elapsed=%s\n", len(cache.vectors), time.Since(embedStarted).Round(time.Second))

	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.VectorStoreCollection{}, &schema.VectorStoreDocument{}).Error)
	byText := make(map[string][]float32, len(corpus))
	for _, doc := range corpus {
		byText[doc.Text] = cache.vectors["d:"+doc.ID]
	}
	lookup := &cachedEmbedder{byText: byText}
	store, err := NewSQLiteVectorStoreHNSW(
		t.Name(), "scifact", embedder.GetModelName(), embedder.GetModelDimension(), lookup, db,
		WithDisableEmbedCollectionInfo(true),
	)
	require.NoError(t, err)
	t.Cleanup(func() { GraphWrapperManager.RemoveCollectionFromCache(db, store.collection) })

	buildStarted := time.Now()
	for i, doc := range corpus {
		require.NoError(t, store.AddWithOptions(doc.ID, doc.Text))
		if (i+1)%1000 == 0 {
			fmt.Fprintf(os.Stderr, "SCIFACT indexed %d/%d\n", i+1, len(corpus))
		}
	}
	require.NoError(t, store.PersistGraph(context.Background()))
	fmt.Fprintf(os.Stderr, "SCIFACT index_build=%s\n", time.Since(buildStarted).Round(time.Second))

	docVectors := make(map[string][]float32, len(corpus))
	for _, doc := range corpus {
		docVectors[doc.ID] = cache.vectors["d:"+doc.ID]
	}

	const k = 10
	var (
		hnswRecall float64
		exactRecall float64
		hnswMRR float64
		exactMRR float64
		hnswNDCG float64
		exactNDCG float64
		annRecall float64
		indexTime time.Duration
	)
	for i, query := range evalQueries {
		vector := cache.vectors["q:"+query.ID]
		relevant := qrels[query.ID]
		started := time.Now()
		hits := store.hnsw.SearchWithDistanceAndFilter(vector, k, func(key string, _ hnsw.Vector) bool {
			return key != DocumentTypeCollectionInfo
		})
		indexTime += time.Since(started)
		hnswIDs := make([]string, 0, len(hits))
		for _, hit := range hits {
			hnswIDs = append(hnswIDs, hit.Key)
		}
		exactIDs := exactTopIDs(vector, docVectors, k)
		hnswRecall += recallAt(hnswIDs, relevant, k)
		exactRecall += recallAt(exactIDs, relevant, k)
		hnswMRR += reciprocalRank(hnswIDs, relevant)
		exactMRR += reciprocalRank(exactIDs, relevant)
		hnswNDCG += ndcgAt(hnswIDs, relevant, k)
		exactNDCG += ndcgAt(exactIDs, relevant, k)
		annRecall += recallAt(hnswIDs, idSet(exactIDs), k)
		if i < 8 || recallAt(hnswIDs, relevant, k) == 0 {
			fmt.Fprintf(os.Stderr, "SCIFACT q=%s hnsw=%v exact=%v relevant=%v\n", query.ID, hnswIDs[:min(3, len(hnswIDs))], exactIDs[:min(3, len(exactIDs))], relevantKeys(relevant))
		}
	}
	n := float64(len(evalQueries))
	fmt.Fprintf(os.Stderr, "SCIFACT RESULT queries=%d k=%d hnsw_recall@10=%.4f exact_recall@10=%.4f hnsw_mrr=%.4f exact_mrr=%.4f hnsw_ndcg@10=%.4f exact_ndcg@10=%.4f ann_recall@10=%.4f index_avg=%s\n",
		len(evalQueries), k,
		hnswRecall/n, exactRecall/n,
		hnswMRR/n, exactMRR/n,
		hnswNDCG/n, exactNDCG/n,
		annRecall/n, (indexTime / time.Duration(len(evalQueries))).Round(time.Microsecond))

	require.GreaterOrEqual(t, annRecall/n, 0.90, "HNSW drifted from exact search")
	require.InDelta(t, exactRecall/n, hnswRecall/n, 0.03)
}

type embedJob struct {
	key  string
	text string
}

type vectorCache struct {
	mu      sync.Mutex
	path    string
	vectors map[string][]float32
	dirty   int
}

func loadEmbedCache(path string) (*vectorCache, error) {
	cache := &vectorCache{path: path, vectors: map[string][]float32{}}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cache, nil
		}
		return nil, err
	}
	defer file.Close()
	reader := bufio.NewReaderSize(file, 1<<20)
	var count uint32
	if err := binary.Read(reader, binary.LittleEndian, &count); err != nil {
		return nil, err
	}
	for i := uint32(0); i < count; i++ {
		var keyLen uint32
		if err := binary.Read(reader, binary.LittleEndian, &keyLen); err != nil {
			return nil, err
		}
		key := make([]byte, keyLen)
		if _, err := io.ReadFull(reader, key); err != nil {
			return nil, err
		}
		vector := make([]float32, 1024)
		if err := binary.Read(reader, binary.LittleEndian, vector); err != nil {
			return nil, err
		}
		cache.vectors[string(key)] = vector
	}
	return cache, nil
}

func (c *vectorCache) fillMissing(embedder *AIBalanceFreeEmbedding, jobs []embedJob, workers int) error {
	missing := make([]embedJob, 0)
	c.mu.Lock()
	for _, job := range jobs {
		if _, ok := c.vectors[job.key]; !ok {
			missing = append(missing, job)
		}
	}
	c.mu.Unlock()
	if len(missing) == 0 {
		return nil
	}
	fmt.Fprintf(os.Stderr, "SCIFACT embedding %d missing texts with %d workers\n", len(missing), workers)
	work := make(chan embedJob)
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error
	var doneCount int
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range work {
				vector, err := embedWithRetry(embedder, job.text)
				if err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("embed %s: %w", job.key, err)
					}
					errMu.Unlock()
					continue
				}
				c.mu.Lock()
				c.vectors[job.key] = vector
				c.dirty++
				doneCount++
				needFlush := c.dirty >= 100
				finished := doneCount
				c.mu.Unlock()
				if finished%100 == 0 {
					fmt.Fprintf(os.Stderr, "SCIFACT embedded %d/%d\n", finished, len(missing))
				}
				if needFlush {
					if err := c.flush(); err != nil {
						errMu.Lock()
						if firstErr == nil {
							firstErr = err
						}
						errMu.Unlock()
					}
				}
			}
		}()
	}
	for _, job := range missing {
		work <- job
	}
	close(work)
	wg.Wait()
	if firstErr != nil {
		_ = c.flush()
		return firstErr
	}
	return c.flush()
}

func (c *vectorCache) flush() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dirty == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	temp := c.path + ".tmp"
	file, err := os.Create(temp)
	if err != nil {
		return err
	}
	writer := bufio.NewWriterSize(file, 1<<20)
	if err := binary.Write(writer, binary.LittleEndian, uint32(len(c.vectors))); err != nil {
		file.Close()
		return err
	}
	for key, vector := range c.vectors {
		if len(vector) != 1024 {
			file.Close()
			return fmt.Errorf("cached vector %s has dimension %d", key, len(vector))
		}
		if err := binary.Write(writer, binary.LittleEndian, uint32(len(key))); err != nil {
			file.Close()
			return err
		}
		if _, err := writer.WriteString(key); err != nil {
			file.Close()
			return err
		}
		if err := binary.Write(writer, binary.LittleEndian, vector); err != nil {
			file.Close()
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temp, c.path); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "SCIFACT cache_flushed vectors=%d\n", len(c.vectors))
	c.dirty = 0
	return nil
}

type cachedEmbedder struct {
	byText map[string][]float32
}

func (e *cachedEmbedder) Embedding(text string) ([]float32, error) {
	vector, ok := e.byText[text]
	if !ok || len(vector) != 1024 {
		return nil, fmt.Errorf("no cached embedding for text length %d", len(text))
	}
	return vector, nil
}

func (e *cachedEmbedder) EmbeddingRaw(text string) ([][]float32, error) {
	vector, err := e.Embedding(text)
	if err != nil {
		return nil, err
	}
	return [][]float32{vector}, nil
}

func readBEIR(path string, withTitle bool) ([]beirRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	records := make([]beirRecord, 0, 5183)
	for scanner.Scan() {
		var record beirRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, err
		}
		if withTitle && record.Title != "" {
			record.Text = record.Title + "\n" + record.Text
		}
		records = append(records, record)
	}
	return records, scanner.Err()
}

func readQrels(path string) (map[string]map[string]struct{}, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	qrels := map[string]map[string]struct{}{}
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if first {
			first = false
			if strings.HasPrefix(line, "query-id") {
				continue
			}
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		if qrels[parts[0]] == nil {
			qrels[parts[0]] = map[string]struct{}{}
		}
		qrels[parts[0]][parts[1]] = struct{}{}
	}
	return qrels, scanner.Err()
}

func exactTopIDs(query []float32, docs map[string][]float32, k int) []string {
	type pair struct {
		id    string
		score float64
	}
	ranked := make([]pair, 0, len(docs))
	queryNorm := vectorNorm(query)
	for id, doc := range docs {
		ranked = append(ranked, pair{id: id, score: dotProduct(query, doc) / (queryNorm * vectorNorm(doc))})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].id < ranked[j].id
		}
		return ranked[i].score > ranked[j].score
	})
	if len(ranked) > k {
		ranked = ranked[:k]
	}
	ids := make([]string, len(ranked))
	for i := range ranked {
		ids[i] = ranked[i].id
	}
	return ids
}

func dotProduct(a, b []float32) float64 {
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}

func vectorNorm(values []float32) float64 {
	return math.Sqrt(dotProduct(values, values))
}

func recallAt(ranked []string, relevant map[string]struct{}, k int) float64 {
	if len(relevant) == 0 {
		return 0
	}
	if len(ranked) > k {
		ranked = ranked[:k]
	}
	hit := 0
	for _, id := range ranked {
		if _, ok := relevant[id]; ok {
			hit++
		}
	}
	return float64(hit) / float64(len(relevant))
}

func reciprocalRank(ranked []string, relevant map[string]struct{}) float64 {
	for i, id := range ranked {
		if _, ok := relevant[id]; ok {
			return 1 / float64(i+1)
		}
	}
	return 0
}

func ndcgAt(ranked []string, relevant map[string]struct{}, k int) float64 {
	if len(ranked) > k {
		ranked = ranked[:k]
	}
	var dcg float64
	for i, id := range ranked {
		if _, ok := relevant[id]; ok {
			dcg += 1 / math.Log2(float64(i+2))
		}
	}
	ideal := len(relevant)
	if ideal > k {
		ideal = k
	}
	var idcg float64
	for i := 0; i < ideal; i++ {
		idcg += 1 / math.Log2(float64(i+2))
	}
	if idcg == 0 {
		return 0
	}
	return dcg / idcg
}

func idSet(ids []string) map[string]struct{} {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}

func relevantKeys(relevant map[string]struct{}) []string {
	keys := make([]string, 0, len(relevant))
	for key := range relevant {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
