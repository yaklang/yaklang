package airaghttp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/utils"
)

// newMockEmbedder 构造确定性的离线 mock 嵌入器 (无网络)
func newMockEmbedder() vectorstore.EmbeddingClient {
	mock := vectorstore.NewDefaultMockEmbedding()
	return vectorstore.NewMockEmbedder(func(text string) ([]float32, error) {
		return mock.Embedding(text)
	})
}

// setupMockKnowledgeBase 在临时内存库中创建一个带文档的知识库 (全程离线)
// 返回 db / 嵌入器 / 集合名 / 清理函数
func setupMockKnowledgeBase(t *testing.T) (*gorm.DB, vectorstore.EmbeddingClient, string, func()) {
	t.Helper()

	db, err := utils.CreateTempTestDatabaseInMemory()
	if err != nil || db == nil {
		t.Skipf("temp database not available: %v", err)
	}

	embedder := newMockEmbedder()
	collectionName := "e2e_kb_" + utils.RandStringBytes(8)

	ragSystem, err := rag.NewRAGSystem(
		rag.WithDB(db),
		rag.WithName(collectionName),
		rag.WithDescription("airaghttp e2e knowledge base"),
		rag.WithEmbeddingModel("test"),
		rag.WithEmbeddingClient(embedder),
	)
	if err != nil {
		t.Skipf("create collection failed (embedding service may be unavailable): %v", err)
	}

	docs := map[string]string{
		"doc_sql": "SQL injection is a common web vulnerability. Use parameterized queries to defend against it.",
		"doc_xss": "Cross-site scripting allows attackers to run scripts in the victim browser. Use output encoding to mitigate.",
		"doc_go":  "Go is a programming language with goroutines and channels for concurrency.",
	}
	for id, content := range docs {
		if err := ragSystem.Add(id, content); err != nil {
			t.Fatalf("add document %s failed: %v", id, err)
		}
	}

	cleanup := func() {
		vectorstore.DeleteCollection(db, collectionName)
	}
	return db, embedder, collectionName, cleanup
}

// newE2EServer 基于注入 db + mock 嵌入器构造服务
func newE2EServer(t *testing.T, db *gorm.DB, embedder vectorstore.EmbeddingClient, collectionName, authToken string) *RAGHTTPServer {
	t.Helper()
	cfg := NewDefaultConfig()
	cfg.AuthToken = authToken
	cfg.Collections = []string{collectionName}
	cfg.Timeout = 15

	server, err := newRAGHTTPServerWithDeps(cfg, db, embedder)
	if err != nil {
		t.Fatalf("create server failed: %v", err)
	}
	return server
}

// TestE2E_HealthAndCollections 通过真实 httptest 回环验证 /health 与 /collections
func TestE2E_HealthAndCollections(t *testing.T) {
	db, embedder, collectionName, cleanup := setupMockKnowledgeBase(t)
	defer cleanup()

	server := newE2EServer(t, db, embedder, collectionName, "")
	ts := httptest.NewServer(server)
	defer ts.Close()

	base := ts.URL + "/api/rag-server"

	// /health
	resp, err := http.Get(base + "/health")
	if err != nil {
		t.Fatalf("GET /health failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/health status expected 200, got %d", resp.StatusCode)
	}
	var health map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatalf("decode /health failed: %v", err)
	}
	if health["collectionCount"].(float64) != 1 {
		t.Fatalf("collectionCount expected 1, got %v", health["collectionCount"])
	}

	// /collections
	cResp, err := http.Get(base + "/collections")
	if err != nil {
		t.Fatalf("GET /collections failed: %v", err)
	}
	defer cResp.Body.Close()
	body, _ := io.ReadAll(cResp.Body)
	if !strings.Contains(string(body), collectionName) {
		t.Fatalf("/collections expected to contain %s, got %s", collectionName, string(body))
	}
}

// TestE2E_SearchReturnsResults 真实向量检索 (离线 mock 嵌入)
func TestE2E_SearchReturnsResults(t *testing.T) {
	db, embedder, collectionName, cleanup := setupMockKnowledgeBase(t)
	defer cleanup()

	server := newE2EServer(t, db, embedder, collectionName, "")
	ts := httptest.NewServer(server)
	defer ts.Close()

	base := ts.URL + "/api/rag-server"

	// 用文档原文作为 query, 保证 mock 嵌入命中
	payload := `{"query":"SQL injection is a common web vulnerability. Use parameterized queries to defend against it.","limit":5}`
	resp, err := http.Post(base+"/search", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /search failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("/search status expected 200, got %d, body=%s", resp.StatusCode, string(b))
	}

	var result struct {
		OK      bool `json:"ok"`
		Total   int  `json:"total"`
		Results []struct {
			Content string  `json:"content"`
			Score   float64 `json:"score"`
			Source  string  `json:"source"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode /search response failed: %v", err)
	}
	if !result.OK {
		t.Fatalf("/search ok expected true")
	}
	if result.Total < 1 || len(result.Results) < 1 {
		t.Fatalf("/search expected at least 1 result, got total=%d len=%d", result.Total, len(result.Results))
	}
	t.Logf("/search returned %d results, top source=%s score=%.3f",
		result.Total, result.Results[0].Source, result.Results[0].Score)
}

// TestE2E_AuthEnforced 真实回环下的 Bearer 鉴权
func TestE2E_AuthEnforced(t *testing.T) {
	db, embedder, collectionName, cleanup := setupMockKnowledgeBase(t)
	defer cleanup()

	server := newE2EServer(t, db, embedder, collectionName, "topsecret")
	ts := httptest.NewServer(server)
	defer ts.Close()

	base := ts.URL + "/api/rag-server"

	// 无 token -> 401
	resp, err := http.Get(base + "/health")
	if err != nil {
		t.Fatalf("GET /health failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token expected 401, got %d", resp.StatusCode)
	}

	// 正确 token -> 200
	req, _ := http.NewRequest(http.MethodGet, base+"/health", nil)
	req.Header.Set("Authorization", "Bearer topsecret")
	okResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("authed GET /health failed: %v", err)
	}
	okResp.Body.Close()
	if okResp.StatusCode != http.StatusOK {
		t.Fatalf("correct token expected 200, got %d", okResp.StatusCode)
	}
}

// TestE2E_StartupRejectsWhenNoKnowledgeBase 无可用知识库时拒绝启动
func TestE2E_StartupRejectsWhenNoKnowledgeBase(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	if err != nil || db == nil {
		t.Skipf("temp database not available: %v", err)
	}
	cfg := NewDefaultConfig()
	// 显式指定一个不存在的集合, 且空库中没有任何集合 -> 必须报错
	cfg.Collections = []string{"non_existent_collection"}
	_, err = newRAGHTTPServerWithDeps(cfg, db, newMockEmbedder())
	if err == nil {
		t.Fatal("expected error when no knowledge base available, got nil")
	}
	if !strings.Contains(err.Error(), "no available knowledge base") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestE2E_ChatStreamRealAI 端到端流式问答 (默认跳过, 需真实 AI)
// 设置环境变量 AIRAGHTTP_E2E_AI=1 且本机配置好 aiconfig 时运行.
func TestE2E_ChatStreamRealAI(t *testing.T) {
	if os.Getenv("AIRAGHTTP_E2E_AI") != "1" {
		t.Skip("skip real-AI chat e2e; set AIRAGHTTP_E2E_AI=1 to enable")
	}

	db, embedder, collectionName, cleanup := setupMockKnowledgeBase(t)
	defer cleanup()

	server := newE2EServer(t, db, embedder, collectionName, "")
	ts := httptest.NewServer(server)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		ts.URL+"/api/rag-server/chat?q=what+is+sql+injection", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /chat failed: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("chat content-type expected event-stream, got %q", ct)
	}

	gotStart, gotEnd := false, false
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "event: start" {
			gotStart = true
		}
		if line == "event: end" {
			gotEnd = true
			break
		}
	}
	if !gotStart || !gotEnd {
		t.Fatalf("chat SSE expected start and end events, got start=%v end=%v", gotStart, gotEnd)
	}
}
