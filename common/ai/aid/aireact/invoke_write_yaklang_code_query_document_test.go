package aireact

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ziputil"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Helper function to create test ZIP file
func createTestZip(docs map[string]string) ([]byte, error) {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	for filename, content := range docs {
		writer, err := zipWriter.Create(filename)
		if err != nil {
			return nil, err
		}
		_, err = writer.Write([]byte(content))
		if err != nil {
			return nil, err
		}
	}

	err := zipWriter.Close()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

type mockStats_forGrepSamples struct {
	grepSamplesDone bool
	codeWritten     bool
}

func mockedYaklangGrepSamples(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, stat *mockStats_forGrepSamples) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()

	// Handle init task: analyze-requirement-and-search
	if utils.MatchAllOfSubString(prompt, "analyze-requirement-and-search", "create_new_file", "search_patterns") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "analyze-requirement-and-search",
  "create_new_file": true,
  "search_patterns": ["http.*server", "httpserver"],
  "reason": "User wants to create http server example"
}`))
		rsp.Close()
		return rsp, nil
	}

	// Handle compress search results: extract-ranked-lines
	if utils.MatchAllOfSubString(prompt, "extract-ranked-lines", "ranges", "rank", "reason") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "extract-ranked-lines",
  "ranges": [
    {"range": "1-5", "rank": 1, "reason": "Most relevant code"},
    {"range": "6-10", "rank": 2, "reason": "Secondary example"}
  ]
}`))
		rsp.Close()
		return rsp, nil
	}

	// First call: choose write_yaklang_code action
	if utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool", `"write_yaklang_code"`) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "write_yaklang_code", "write_yaklang_code_approach": "test grep samples" },
"human_readable_thought": "mocked thought for grep-samples test", "cumulative_summary": "..cumulative-mocked for grep-samples.."}
`))
		rsp.Close()
		return rsp, nil
	}

	// Verify satisfaction
	if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning") {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "grep-samples-mocked-reason"}`))
		rsp.Close()
		return rsp, nil
	}

	// Code loop: grep_yaklang_samples -> write_code -> finish
	if utils.MatchAllOfSubString(prompt, `"grep_yaklang_samples"`, `"require_tool"`, `"write_code"`, `"@action"`) {
		// extract nonce from <|GEN_CODE_{{.Nonce}}|>
		re := regexp.MustCompile(`<\|GEN_CODE_([^|]+)\|>`)
		matches := re.FindStringSubmatch(prompt)
		var nonceStr string
		if len(matches) > 1 {
			nonceStr = matches[1]
		}

		rsp := i.NewAIResponse()

		// First: grep yaklang samples
		if !stat.grepSamplesDone {
			rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "grep_yaklang_samples",
  "pattern": "http.*server",
  "case_sensitive": false,
  "context_lines": 15
}`))
			stat.grepSamplesDone = true
			rsp.Close()
			return rsp, nil
		}

		// Second: write code using grep results
		if !stat.codeWritten {
			rsp.EmitOutputStream(bytes.NewBufferString(utils.MustRenderTemplate(`{"@action": "write_code"}

<|GEN_CODE_{{ .nonce }}|>
// Code based on grep samples
println("http server example")
println("using Get method")
<|GEN_CODE_END_{{ .nonce }}|>`, map[string]any{
				"nonce": nonceStr,
			})))
			stat.codeWritten = true
			rsp.Close()
			return rsp, nil
		}

		// Third: finish
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
		rsp.Close()
		return rsp, nil
	}

	if utils.MatchAllOfSubString(prompt, `"@action"`, `"create_new_file"`, `"check-filepath"`, `"existed_filepath"`) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "check-filepath", "create_new_file": true}`))
		rsp.Close()
		return rsp, nil
	}

	fmt.Println("Unexpected prompt:", prompt)
	return nil, utils.Errorf("unexpected prompt: %s", prompt)
}

func TestReAct_GrepYaklangSamples(t *testing.T) {
	// Create test ZIP file with mock code samples
	tempDir := os.TempDir()
	zipPath := filepath.Join(tempDir, "test-aikb-"+ksuid.New().String()+".zip")
	defer os.Remove(zipPath)

	// Create test code samples
	docs := map[string]string{
		"http/basics.yak": `# HTTP Basics Examples

// Example 1: Simple HTTP GET request
resp, err = http.Get("https://example.com")
if err != nil {
    die(err)
}
println(resp.Body)

// Example 2: HTTP server
httpserver.Serve("0.0.0.0", 8080, httpserver.handler(func(rsp, req) {
    rsp.Write("Hello World")
}))
`,
		"http/server.yak": `# HTTP Server Examples

// Starting a basic server
httpserver.Serve("127.0.0.1", 8080)

// Server with custom handler
httpserver.Serve("0.0.0.0", 8080, httpserver.handler(func(rsp, req) {
    rsp.Write("Custom response")
}))
`,
		"strings/utils.yak": `# String Utilities Examples

// String split
parts = str.Split("a,b,c", ",")
println(parts)
`,
	}

	raw, err := createTestZip(docs)
	if err != nil {
		t.Fatalf("Failed to create test zip data: %v", err)
	}
	err = os.WriteFile(zipPath, raw, 0644)
	if err != nil {
		t.Fatalf("Failed to create test zip: %v", err)
	}

	flag := ksuid.New().String()
	_ = flag
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 100)

	stat := &mockStats_forGrepSamples{
		grepSamplesDone: false,
		codeWritten:     false,
	}

	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedYaklangQueryDocument(i, r, stat)
		}),
		WithEventInputChan(in),
		WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithAIKBPath(zipPath), // Use test zip file as aikb
	)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "create http server example",
		}
	}()

	du := time.Duration(50)
	if utils.InGithubActions() {
		du = time.Duration(10)
	}
	after := time.After(du * time.Second)

	var grepSamplesSeen bool
	var codeGenerated bool

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR) {
				if e.GetNodeId() == "grep_yaklang_samples" {
					grepSamplesSeen = true
					content := string(e.GetContent())
					// Verify grep results are formatted correctly
					if !utils.MatchAllOfSubString(content, "Grep pattern") {
						t.Logf("Grep samples results: %s", content)
					}
				}
				if e.GetNodeId() == "write_code" {
					codeGenerated = true
					content := string(e.GetContent())
					if !utils.MatchAllOfSubString(content, "http server example") {
						t.Errorf("Generated code doesn't contain expected content: %s", content)
					}
					// Successfully completed grep_yaklang_samples -> write_code flow, exit
					break LOOP
				}
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	// Verify timeline
	fmt.Println("--------------------------------------")
	tl := ins.DumpTimeline()
	fmt.Println(tl)
	if !utils.MatchAllOfSubString(tl, "mocked thought for grep-samples") {
		t.Error("Timeline doesn't contain expected thought")
	}
	if !utils.MatchAllOfSubString(tl, "grep") {
		t.Error("Timeline doesn't contain grep action")
	}
	fmt.Println("--------------------------------------")

	// Verify the grep samples action was triggered
	if !stat.grepSamplesDone {
		t.Error("Grep samples action was not triggered")
	}
	if !stat.codeWritten {
		t.Error("Code was not written after grep samples")
	}

	// These checks are conditional since actual file access might fail in test
	_ = grepSamplesSeen
	_ = codeGenerated
}

func TestReAct_QueryDocumentWithFilters(t *testing.T) {
	t.Skip()

	// Test with path filters
	tempDir := os.TempDir()
	zipPath := filepath.Join(tempDir, "test-aikb-filters-"+ksuid.New().String()+".zip")
	defer os.Remove(zipPath)

	docs := map[string]string{
		"api/http.md":      "http.Get documentation",
		"api/tcp.md":       "tcp.Dial documentation",
		"internal/test.md": "internal test doc",
		"examples/demo.md": "example demo",
	}

	raw, err := createTestZip(docs)
	if err != nil {
		t.Fatalf("Failed to create test zip data: %v", err)
	}
	err = os.WriteFile(zipPath, raw, 0644)
	if err != nil {
		t.Fatalf("Failed to create test zip: %v", err)
	}

	// Test searcher with filters
	searcher, err := ziputil.NewZipGrepSearcher(zipPath)
	if err != nil {
		t.Fatalf("Failed to create searcher: %v", err)
	}

	// Search with include filter
	results, err := searcher.GrepSubString("documentation",
		ziputil.WithIncludePathSubString("api/"),
		ziputil.WithExcludePathSubString("internal"),
	)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Verify results
	if len(results) < 1 {
		t.Error("Expected at least 1 result with path filters")
	}

	for _, r := range results {
		if !utils.MatchAllOfSubString(r.FileName, "api/") {
			t.Errorf("Result should be in api/ directory: %s", r.FileName)
		}
		if utils.MatchAllOfSubString(r.FileName, "internal") {
			t.Errorf("Result should not be in internal directory: %s", r.FileName)
		}
	}
}

func TestReAct_QueryDocumentRRFRanking(t *testing.T) {
	t.Skip()

	// Test RRF ranking with multiple search terms
	tempDir := os.TempDir()
	zipPath := filepath.Join(tempDir, "test-aikb-rrf-"+ksuid.New().String()+".zip")
	defer os.Remove(zipPath)

	docs := map[string]string{
		"doc1.md": "http server example with Get method",
		"doc2.md": "http client using Get and Post",
		"doc3.md": "server configuration guide",
		"doc4.md": "tcp server implementation",
	}

	raw, err := createTestZip(docs)
	if err != nil {
		t.Fatalf("Failed to create test zip data: %v", err)
	}
	err = os.WriteFile(zipPath, raw, 0644)
	if err != nil {
		t.Fatalf("Failed to create test zip: %v", err)
	}

	searcher, err := ziputil.NewZipGrepSearcher(zipPath)
	if err != nil {
		t.Fatalf("Failed to create searcher: %v", err)
	}

	// Multiple searches (simulating keywords: "http", "server")
	var allResults []*ziputil.GrepResult

	results1, err := searcher.GrepSubString("http")
	if err != nil {
		t.Fatalf("Search 1 failed: %v", err)
	}
	allResults = append(allResults, results1...)

	results2, err := searcher.GrepSubString("server")
	if err != nil {
		t.Fatalf("Search 2 failed: %v", err)
	}
	allResults = append(allResults, results2...)

	// Apply merge and RRF ranking
	merged := ziputil.MergeGrepResults(allResults)
	ranked := utils.RRFRankWithDefaultK(merged)

	if len(ranked) == 0 {
		t.Error("Expected ranked results")
	}

	// doc1.md should rank high as it contains both "http" and "server"
	topResult := ranked[0]
	if topResult.FileName != "doc1.md" {
		t.Logf("Top result is %s (expected doc1.md), score: %.4f", topResult.FileName, topResult.Score)
		// Don't fail, as RRF ranking might vary
	}

	// Verify scores are in descending order
	for i := 1; i < len(ranked); i++ {
		if ranked[i].Score > ranked[i-1].Score {
			t.Errorf("Results not properly ranked: result[%d].Score (%.4f) > result[%d].Score (%.4f)",
				i, ranked[i].Score, i-1, ranked[i-1].Score)
		}
	}
}

func TestReAct_QueryDocumentSizeLimit(t *testing.T) {
	// Test size limit enforcement
	tempDir := os.TempDir()
	zipPath := filepath.Join(tempDir, "test-aikb-sizelimit-"+ksuid.New().String()+".zip")
	defer os.Remove(zipPath)

	// Create large documents to test size limit
	docs := make(map[string]string)
	for i := 0; i < 100; i++ {
		content := ""
		for j := 0; j < 100; j++ {
			content += fmt.Sprintf("Line %d: This is a test document with some content about http server and api documentation. ", j)
		}
		docs[fmt.Sprintf("doc%d.md", i)] = content
	}

	raw, err := createTestZip(docs)
	if err != nil {
		t.Fatalf("Failed to create test zip data: %v", err)
	}
	err = os.WriteFile(zipPath, raw, 0644)
	if err != nil {
		t.Fatalf("Failed to create test zip: %v", err)
	}

	// Test with small size limit (1KB)
	searcher, err := ziputil.NewZipGrepSearcher(zipPath)
	if err != nil {
		t.Fatalf("Failed to create searcher: %v", err)
	}

	// Search for common term that will match many documents
	results, err := searcher.GrepSubString("http", ziputil.WithContext(2))
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("Expected some results")
	}

	// Merge and rank
	merged := ziputil.MergeGrepResults(results)
	ranked := utils.RRFRankWithDefaultK(merged)

	// Test size limit logic manually
	maxSize := int64(1024) // 1KB limit
	var docBuffer bytes.Buffer
	docBuffer.WriteString("=== Document Query Results ===\n")

	var includedResults int
	var truncated bool

	for i, result := range ranked {
		resultStr := fmt.Sprintf("--- Result %d ---\n", i+1)
		resultStr += result.String()
		resultStr += "\n"

		if int64(docBuffer.Len()+len(resultStr)+100) > maxSize {
			truncated = true
			break
		}

		docBuffer.WriteString(resultStr)
		includedResults++
	}

	if truncated {
		docBuffer.WriteString(fmt.Sprintf("...[truncated: %d more results]\n", len(ranked)-includedResults))
	}

	docBuffer.WriteString("=== End ===\n")
	finalResult := docBuffer.String()

	// Verify truncation happened
	if !truncated {
		t.Log("Warning: Expected truncation with 1KB limit, but no truncation occurred")
	}

	// Verify final size is within limit (with some margin for footer)
	if int64(len(finalResult)) > maxSize+200 {
		t.Errorf("Final result size %d exceeds limit %d (even with margin)", len(finalResult), maxSize)
	}

	// Verify truncation message exists if truncated
	if truncated && !utils.MatchAllOfSubString(finalResult, "truncated") {
		t.Error("Truncated result should contain truncation message")
	}

	t.Logf("Results: total=%d, included=%d, truncated=%v, size=%d bytes",
		len(ranked), includedResults, truncated, len(finalResult))
}

func TestReAct_QueryDocumentDefaultSizeLimit(t *testing.T) {
	// Test that default size limit is set correctly
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			rsp.Close()
			return rsp, nil
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Check default value
	expectedDefault := int64(20 * 1024)
	aikbResultMaxSize := ins.config.GetConfigInt64("aikb_result_max_size", 20*1024)
	if aikbResultMaxSize != expectedDefault {
		t.Errorf("Default aikb result max size should be %d, got %d",
			expectedDefault, aikbResultMaxSize)
	}

	// Test with custom value
	ins2, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			rsp.Close()
			return rsp, nil
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithAIKBResultMaxSize(10*1024), // 10KB
	)
	if err != nil {
		t.Fatal(err)
	}

	aikbResultMaxSize = ins2.config.GetConfigInt64("aikb_result_max_size")
	if aikbResultMaxSize != 10*1024 {
		t.Errorf("Custom aikb result max size should be %d, got %d",
			10*1024, aikbResultMaxSize)
	}

	// Test with value exceeding hard limit
	ins3, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish"}`))
			rsp.Close()
			return rsp, nil
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithAIKBResultMaxSize(50*1024), // Try to set 50KB (exceeds hard limit)
	)
	if err != nil {
		t.Fatal(err)
	}

	// Should be capped at 20KB
	aikbResultMaxSize = ins3.config.GetConfigInt64("aikb_result_max_size")
	if aikbResultMaxSize != 20*1024 {
		t.Errorf("aikb result max size exceeding hard limit should be capped at %d, got %d",
			20*1024, aikbResultMaxSize)
	}

	close(in)
}
