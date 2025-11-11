package lsp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yakgrpc"
)

// TestCacheEffectiveness 测试缓存效果
func TestCacheEffectiveness(t *testing.T) {
	// 创建 gRPC 服务器
	server, err := yakgrpc.NewServer(
		yakgrpc.WithInitFacadeServer(false),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 创建 HTTP LSP 服务器
	lspServer := NewYakLSPHTTPServer(server, "127.0.0.1:0")

	// 创建 HTTP 测试服务器
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lspServer.handleLSP(w, r)
	}))
	defer httpServer.Close()

	// 读取真实的 code-helper.yak 文件
	codeHelperPath := "/Users/v1ll4n/Projects/yaklang-ai-training-materials/apps/code-helper/code-helper.yak"
	realCode, err := os.ReadFile(codeHelperPath)
	if err != nil {
		t.Skipf("Skipping test: cannot read code-helper.yak: %v", err)
		return
	}

	baseCode := string(realCode)

	t.Logf("\n=== Testing Cache Effectiveness ===")
	t.Logf("Base code size: %d bytes", len(baseCode))

	// 第一次请求：应该慢（需要解析 SSA）
	testCode1 := baseCode + "\nrag."
	duration1 := testCompletionWithResult(t, httpServer.URL, testCode1, "first")
	t.Logf("1️⃣  First request (cold cache): %v", duration1)

	// 等待一小段时间确保缓存已保存
	time.Sleep(100 * time.Millisecond)

	// 第二次请求：相同代码，应该快（命中缓存）
	duration2 := testCompletionWithResult(t, httpServer.URL, testCode1, "second")
	t.Logf("2️⃣  Second request (same code, should hit cache): %v", duration2)

	// 第三次请求：稍微不同的代码（只改变位置）
	testCode3 := baseCode + "\nrag."
	duration3 := testCompletionWithResult(t, httpServer.URL, testCode3, "third")
	t.Logf("3️⃣  Third request (same code again): %v", duration3)

	// 第四次请求：增加一个字符
	testCode4 := baseCode + "\nrags"
	duration4 := testCompletionWithResult(t, httpServer.URL, testCode4, "fourth")
	t.Logf("4️⃣  Fourth request (slightly different code): %v", duration4)

	t.Logf("\n=== Cache Performance Analysis ===")

	// 分析缓存效果
	if duration2 < duration1/2 {
		t.Logf("✅ Cache is VERY effective: 2nd request is %.1fx faster than 1st", float64(duration1)/float64(duration2))
	} else if float64(duration2) < float64(duration1)*0.8 {
		t.Logf("✅ Cache is effective: 2nd request is %.1f%% faster than 1st", (1-float64(duration2)/float64(duration1))*100)
	} else {
		t.Logf("⚠️  Cache may not be working: 2nd request (%v) is similar to 1st (%v)", duration2, duration1)
	}

	if duration3 < duration1/2 {
		t.Logf("✅ Cache persists: 3rd request is %.1fx faster than 1st", float64(duration1)/float64(duration3))
	}

	t.Logf("\nSummary:")
	t.Logf("  Cold cache (1st):  %v", duration1)
	t.Logf("  Hot cache (2nd):   %v (%.1f%% of 1st)", duration2, float64(duration2)/float64(duration1)*100)
	t.Logf("  Hot cache (3rd):   %v (%.1f%% of 1st)", duration3, float64(duration3)/float64(duration1)*100)
	t.Logf("  New code (4th):    %v (%.1f%% of 1st)", duration4, float64(duration4)/float64(duration1)*100)
}

// TestDeduplication 测试去重效果
func TestDeduplication(t *testing.T) {
	// 创建 gRPC 服务器
	server, err := yakgrpc.NewServer(
		yakgrpc.WithInitFacadeServer(false),
	)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// 创建 HTTP LSP 服务器
	lspServer := NewYakLSPHTTPServer(server, "127.0.0.1:0")

	// 创建 HTTP 测试服务器
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lspServer.handleLSP(w, r)
	}))
	defer httpServer.Close()

	// 使用简单的测试代码
	testCode := `r`

	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "textDocument/completion",
		"params": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri":  "file:///test.yak",
				"text": testCode,
			},
			"position": map[string]interface{}{
				"line":      0,
				"character": 1,
			},
		},
	}

	jsonData, _ := json.Marshal(reqBody)
	resp, err := http.Post(httpServer.URL+"/lsp", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		Result []map[string]interface{} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	t.Logf("\n=== Deduplication Test ===")
	t.Logf("Total suggestions: %d", len(result.Result))

	// 检查是否有重复
	labels := make(map[string]int)
	for _, item := range result.Result {
		if label, ok := item["label"].(string); ok {
			labels[label]++
		}
	}

	duplicates := 0
	for label, count := range labels {
		if count > 1 {
			t.Logf("⚠️  Duplicate: '%s' appears %d times", label, count)
			duplicates++
		}
	}

	if duplicates == 0 {
		t.Logf("✅ No duplicates found! Deduplication is working")
	} else {
		t.Errorf("❌ Found %d duplicate labels", duplicates)
	}
}

// 辅助函数
func testCompletionWithResult(t *testing.T, serverURL, code, name string) time.Duration {
	lines := bytes.Split([]byte(code), []byte("\n"))
	lastLine := len(lines) - 1
	lastColumn := len(lines[lastLine])

	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "textDocument/completion",
		"params": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri":  "file:///" + name + ".yak",
				"text": code,
			},
			"position": map[string]interface{}{
				"line":      lastLine,
				"character": lastColumn,
			},
		},
	}

	jsonData, _ := json.Marshal(reqBody)

	startTime := time.Now()
	resp, err := http.Post(serverURL+"/lsp", "application/json", bytes.NewBuffer(jsonData))
	duration := time.Since(startTime)

	if err != nil {
		t.Logf("Request %s failed: %v", name, err)
		return duration
	}
	defer resp.Body.Close()

	return duration
}
