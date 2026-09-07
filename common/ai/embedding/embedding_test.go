package embedding

import (
	"fmt"
	"os"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

// 生成 20 条示例文本，调用本地 11435 端口的 embedding 服务，
// 再生成 Quadrant Chart 代码并输出。
func TestGenerateQuadrantChartFromEmbeddingService(t *testing.T) {
	endpoint := os.Getenv("YAKLANG_EMBEDDING_TEST_URL")
	if endpoint == "" {
		t.Skip("set YAKLANG_EMBEDDING_TEST_URL to run the external embedding integration test")
	}
	type item struct {
		label string
		text  string
	}

	// 4 个聚类类别：DB / NET / ML / WEB，每类 5 条，共 20 条
	items := []item{
		{label: "DB-01 indexes", text: "Database indexing, B-tree, hash index, PostgreSQL, MySQL, query optimization"},
		{label: "DB-02 joins", text: "SQL joins, inner join, left join, normalization, ACID transactions"},
		{label: "DB-03 replication", text: "Replication, sharding, partitioning, consistency models, write-ahead logging"},
		{label: "DB-04 tuning", text: "Database tuning, query planner, vacuum analyze, slow queries"},
		{label: "DB-05 caching", text: "Caching strategies, materialized views, connection pooling"},

		{label: "NET-01 tcp", text: "TCP/IP sockets, UDP, latency, throughput, congestion control, packet capture"},
		{label: "NET-02 http", text: "HTTP/2, TLS handshake, keep-alive, proxies, CDN, load balancer"},
		{label: "NET-03 protocols", text: "DNS, QUIC, MTU, firewall rules, NAT traversal"},
		{label: "NET-04 routing", text: "Routing tables, BGP, traceroute, packet loss analysis"},
		{label: "NET-05 observability", text: "Network monitoring, pcap, metrics, latency histogram"},

		{label: "ML-01 nn", text: "Neural networks, gradient descent, backpropagation, activation functions"},
		{label: "ML-02 transformer", text: "Transformers, attention mechanism, embeddings, language modeling"},
		{label: "ML-03 classification", text: "Logistic regression, SVM, decision boundaries, ROC AUC"},
		{label: "ML-04 vector-search", text: "Vector search, ANN indexes, cosine similarity, HNSW"},
		{label: "ML-05 training", text: "Training loops, batch normalization, regularization, overfitting"},

		{label: "WEB-01 react", text: "React components, hooks, virtual DOM, state management"},
		{label: "WEB-02 vue", text: "Vue directives, reactive system, single file components"},
		{label: "WEB-03 bundling", text: "Webpack bundling, tree shaking, code splitting, performance"},
		{label: "WEB-04 typescript", text: "TypeScript types, interfaces, generics, strict mode"},
		{label: "WEB-05 ui", text: "CSS layouts, flexbox, grid, responsive design, accessibility"},
	}

	client := NewOpenaiEmbeddingClient(aispec.WithBaseURL(endpoint))

	data := make(map[string][][]float32)
	for _, it := range items {
		vec, err := client.Embedding(it.text)
		if err != nil {
			t.Fatalf("embedding service returned an error: %v", err)
		}
		data[it.label] = [][]float32{vec}
	}

	// 生成图表代码（可自定义标题与轴名称）
	code := GenerateQuadrantChartFromEmbeddingsWithOptions(data, &QuadrantChartOptions{
		Title:     "Embeddings Projection (PCA 2D)",
		XLeft:     "Low X",
		XRight:    "High X",
		YBottom:   "Low Y",
		YTop:      "High Y",
		Quadrant1: "",
	})

	fmt.Println("\n===== Quadrant Chart Code =====")
	fmt.Println(code)
}
