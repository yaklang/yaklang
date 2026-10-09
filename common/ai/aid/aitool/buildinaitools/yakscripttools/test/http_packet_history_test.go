package test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	_ "github.com/yaklang/yaklang/common/ai/aid/liteforge/liteforgeapp"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// A complete offline investigation through the actual embedded tools and real
// SQLite databases. Each numbered step checks a distinct user-visible behavior.
func TestHTTPPacketHistoryTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	home := t.TempDir()
	local := filepath.Join(home, "yakit-projects")
	remote := filepath.Join(home, "Memfit workspace")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("YAKIT_HOME", local)
	t.Setenv("MEMFITAI_HOME", "") // Discover Memfit from its actual app config, not an environment shortcut.
	t.Setenv("SKIP_SYNC_BUILD_IN_AI_TOOL", "true")
	open := func(path string, models ...any) *gorm.DB {
		t.Helper()
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
		db, err := gorm.Open("sqlite3", path)
		require.NoError(t, err)
		db.DB().SetMaxOpenConns(1)
		require.NoError(t, db.AutoMigrate(models...).Error)
		t.Cleanup(func() { db.Close() })
		return db
	}
	db := open(filepath.Join(local, "projects", "auth.db"), &schema.HTTPFlow{})
	profile := open(filepath.Join(local, consts.YAK_PROFILE_PLUGIN_DB_NAME), &schema.Project{})
	foreign := open(filepath.Join(remote, "projects", "auth.db"), &schema.HTTPFlow{})
	foreignProfile := open(filepath.Join(remote, consts.YAK_PROFILE_PLUGIN_DB_NAME), &schema.Project{})
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".yakit", "memfit"), 0700))
	configJSON, err := json.Marshal(map[string]string{"YAKIT_HOME": remote})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".yakit", "memfit", "config.json"), configJSON, 0600))
	require.NoError(t, profile.Create(&schema.Project{ProjectName: "Yakit 认证调查", DatabasePath: dbPath(t, db), Type: "project"}).Error)
	// The two profiles intentionally reuse both project ID=1 and flow ID=1.
	require.NoError(t, foreignProfile.Create(&schema.Project{ProjectName: "Memfit 认证调查", DatabasePath: dbPath(t, foreign), Type: "project"}).Error)
	missing := filepath.Join(remote, "projects", "missing.db")
	require.NoError(t, foreignProfile.Create(&schema.Project{ProjectName: "Missing project", DatabasePath: missing, Type: "project"}).Error)
	require.NoError(t, foreignProfile.Create(&schema.Project{ProjectName: "SSA project", DatabasePath: dbPath(t, foreignProfile), Type: "ssa_project"}).Error)
	request := "POST /api/orders HTTP/1.1\r\nHost: auth.example\r\nCookie: sid=fixture\r\n\r\n{\"note\":\"保留引号\"}\nsecond line"
	response := "HTTP/1.1 403 Forbidden\r\nContent-Type: application/json\r\n\r\n{\"error\":\"CSRF token missing\"}\n第二行"
	save := func(target *gorm.DB, id uint, url, req, rsp string, status int64) *schema.HTTPFlow {
		t.Helper()
		flow := &schema.HTTPFlow{Model: gorm.Model{ID: id}, Hash: fmt.Sprintf("fixture-%d", id), Url: url, Method: "POST", SourceType: "mitm", StatusCode: status, ContentType: "application/json", Tags: "fixture"}
		flow.SetRequest(req)
		flow.SetResponse(rsp)
		require.NoError(t, target.Create(flow).Error)
		return flow
	}
	save(db, 1, "https://auth.example/api/orders", request, response, 403)
	save(db, 2, "https://auth.example/api/orders?page=2", request, "HTTP/1.1 401 Unauthorized\r\n\r\nexpired", 401)
	save(db, 3, "https://auth.example/other", request, "HTTP/1.1 200 OK\r\n\r\nok", 200)
	save(foreign, 1, "https://memfit.example/api/orders", "POST /api/orders HTTP/1.1\r\nHost: memfit.example\r\n\r\nforeign-request", "HTTP/1.1 200 OK\r\n\r\nforeign-response", 200)
	toolRuntime := &aitool.ToolRuntimeConfig{ProjectDatabase: db, ProfileDatabase: profile}
	tools := map[string]*aitool.Tool{}
	for _, name := range []string{"list_yak_projects", "query_http_packet_history", "fetch_http_packet_by_id", "grep"} {
		folder := "database"
		if name == "grep" {
			folder = "fs"
		}
		source, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/" + folder + "/" + name + ".yak")
		require.NoError(t, err)
		metadata := yakscripttools.LoadYakScriptToAiTools(name, string(source))
		require.NotNil(t, metadata)
		converted := yakscripttools.ConvertTools([]*schema.AIYakTool{metadata})
		require.Len(t, converted, 1)
		tools[name] = converted[0]
		if folder == "database" {
			require.True(t, aicommon.IsFixedInventoryTool(name))
		}
	}
	execute := func(name string, params map[string]any) (*aitool.ToolExecutionResult, error) {
		t.Helper()
		cfg := aitool.NewToolInvokeConfig()
		aitool.WithRuntimeConfig(toolRuntime)(cfg)
		return tools[name].ExecuteToolWithCapture(ctx, params, cfg)
	}
	invoke := func(name string, params map[string]any) string {
		t.Helper()
		out, err := execute(name, params)
		require.NoError(t, err)
		require.Nil(t, out.Result, "history must be raw stdout, not RESULT")
		return out.Stdout
	}
	// 1. Discover both installed apps, distinguish colliding project IDs, and
	// display real database size/date/path. Missing and SSA projects remain explicit.
	listed := invoke("list_yak_projects", map[string]any{"limit": 10})
	require.Contains(t, listed, "Yakit 认证调查")
	require.Contains(t, listed, "Memfit 认证调查")
	require.Contains(t, listed, "size_bytes=")
	require.Contains(t, listed, "last_operation_at=")
	require.Contains(t, listed, "available=false")
	require.Contains(t, listed, "supports_http=false")
	projects, err := yaklib.ListYakProjects(yaklib.WithDBHistoryRuntime(ctx, db, profile))
	require.NoError(t, err)
	ids := map[string]string{}
	for _, p := range projects {
		ids[p.Name] = p.DatabaseID
		if p.Name == "Yakit 认证调查" {
			require.True(t, p.Current)
			require.Contains(t, p.Source, "yakit")
			require.Positive(t, p.SizeBytes)
		}
		if p.Name == "Memfit 认证调查" {
			require.False(t, p.Current)
			require.Contains(t, p.Source, "memfit")
			require.EqualValues(t, 1, p.ProjectID)
		}
	}
	require.NotEqual(t, ids["Yakit 认证调查"], ids["Memfit 认证调查"])
	require.Contains(t, invoke("list_yak_projects", map[string]any{"query": "Memfit"}), "projects=1")
	require.Contains(t, invoke("list_yak_projects", map[string]any{"offset": 100}), "projects=0")
	t.Logf("list_yak_projects actual stdout:\n%s", listed)

	// 2. Query the runtime's current DB, reuse engine filters and stable ID paging;
	// small request/response quotes and line breaks must be actual raw text.
	queried := invoke("query_http_packet_history", map[string]any{"url": "/api/orders", "methods": "POST", "status_code": "400-499", "source_type": "mitm", "limit": 1})
	require.Contains(t, queried, "total=2 offset=0 next_offset=1 has_more=true hits=1")
	require.Contains(t, queried, "http_flow_id=2")
	require.NotContains(t, queried, "http_flow_id=1")
	require.Contains(t, queried, request)
	require.NotContains(t, queried, `\"note\"`)
	second := invoke("query_http_packet_history", map[string]any{"url": "/api/orders", "methods": "POST", "status_code": "400-499", "offset": 1, "limit": 1})
	require.Contains(t, second, "http_flow_id=1")
	require.Contains(t, second, response)
	require.Contains(t, second, "has_more=false")
	require.Contains(t, invoke("query_http_packet_history", map[string]any{"query": "CSRF token missing", "before_id": 2, "after_id": 0}), "hits=1")
	require.Contains(t, invoke("query_http_packet_history", map[string]any{"methods": "GET"}), "hits=0")
	t.Logf("query_http_packet_history actual stdout:\n%s", second)

	// 3. Cross-project lookup with the same flow ID returns the selected DB only.
	// No file mutation, global DB switch, or stale runtime binding is permitted.
	foreignBefore, err := os.ReadFile(dbPath(t, foreign))
	require.NoError(t, err)
	fetched := invoke("fetch_http_packet_by_id", map[string]any{"id": 1})
	require.Contains(t, fetched, request)
	require.Contains(t, fetched, response)
	t.Logf("fetch_http_packet_by_id actual stdout:\n%s", fetched)
	cross := invoke("query_http_packet_history", map[string]any{"database_id": ids["Memfit 认证调查"]})
	require.Contains(t, cross, "foreign-response")
	require.NotContains(t, cross, "CSRF token missing")
	cross = invoke("fetch_http_packet_by_id", map[string]any{"id": 1, "database_id": ids["Memfit 认证调查"], "export": true, "output_dir": t.TempDir()})
	require.Contains(t, cross, "foreign-request")
	require.Contains(t, cross, "request_file=")
	require.Contains(t, cross, "response_file=")
	require.Contains(t, invoke("fetch_http_packet_by_id", map[string]any{"id": 1}), "CSRF token missing")
	foreignAfter, err := os.ReadFile(dbPath(t, foreign))
	require.NoError(t, err)
	require.Equal(t, sha256.Sum256(foreignBefore), sha256.Sum256(foreignAfter))
	require.Equal(t, db, toolRuntime.ProjectDatabase)
	// Preserve the concrete HTTPFlow type as well as fields for existing exact-ID callers.
	legacy, err := yaklib.QueryHTTPFlowByID(1, yaklib.WithDBHistoryRuntime(ctx, db, profile))
	require.NoError(t, err)
	require.IsType(t, &schema.HTTPFlow{}, legacy)
	require.Equal(t, "application/json", legacy.ContentType)
	require.Equal(t, "fixture", legacy.Tags)

	// 4. Legacy large in-DB packets span multiple 64-KiB SQL chunks and contain
	// every binary byte, UTF-8, Go-only escapes, literal fuzztags and a late sentinel.
	// Lists stay bounded; fetch exports byte-identical complete packets for grep.
	binary := make([]byte, 256)
	for i := range binary {
		binary[i] = byte(i)
	}
	huge := append([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\n\r\n"), bytes.Repeat(binary, 900)...)
	huge = append(huge, []byte("\nLATE-EVIDENCE 证据 {{file(/should/not/execute)}}\n")...)
	save(db, 4, "https://auth.example/large", request, string(huge), 200)
	bounded := invoke("query_http_packet_history", map[string]any{"url": "/large", "packet_limit": 256})
	require.Less(t, len(bounded), 4096)
	require.Contains(t, bounded, "response omitted:")
	dir := t.TempDir()
	large := invoke("fetch_http_packet_by_id", map[string]any{"id": 4, "packet_limit": 256, "output_dir": dir})
	filePattern := regexp.MustCompile(`response_file=(.+) bytes=([0-9]+)`)
	match := filePattern.FindStringSubmatch(large)
	require.Len(t, match, 3)
	data, err := os.ReadFile(match[1])
	require.NoError(t, err)
	require.Equal(t, huge, data)
	stat, err := os.Stat(match[1])
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0600), stat.Mode().Perm())
	}
	grep := invoke("grep", map[string]any{"path": match[1], "pattern": "LATE-EVIDENCE", "pattern-mode": "substr", "context-buffer": 40, "limit": 1})
	require.Contains(t, grep, "LATE-EVIDENCE")
	require.Contains(t, grep, "证据")
	require.Contains(t, large, "Use grep or read_file")

	// 5. Complete flat/SSE and multipart packets are reconstructed from existing
	// engine sidecars. The stored multipart skeleton is itself over the inline limit.
	sideDir := t.TempDir()
	write := func(name string, data []byte) string {
		t.Helper()
		path := filepath.Join(sideDir, name)
		require.NoError(t, os.WriteFile(path, data, 0600))
		return path
	}
	largeRequest := bytes.Repeat([]byte("upload-raw\x00\n"), 10000)
	head := []byte(fmt.Sprintf("POST /upload HTTP/1.1\r\nHost: auth.example\r\nContent-Length: %d\r\n\r\n", len(largeRequest)))
	sseHead := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\n")
	sseBody := []byte("data: \"first\"\n\ndata: late-event\n\n")
	spill := save(db, 5, "https://auth.example/upload", "POST /upload HTTP/1.1\r\n\r\n[[truncated]]", "HTTP/1.1 200 OK\r\n\r\n[[truncated]]", 200)
	spill.IsTooLargeRequest = true
	spill.TooLargeRequestHeaderFile = write("request-head", head)
	spill.TooLargeRequestBodyFile = write("request-body", largeRequest)
	spill.IsReadTooSlowResponse = true
	spill.TooLargeResponseHeaderFile = write("response-head", sseHead)
	spill.TooLargeResponseBodyFile = write("response-body", sseBody)
	require.NoError(t, db.Save(spill).Error)
	out := invoke("fetch_http_packet_by_id", map[string]any{"id": 5, "output_dir": dir})
	for _, part := range []struct {
		name   string
		packet []byte
	}{{"request", append(head, largeRequest...)}, {"response", append(sseHead, sseBody...)}} {
		pattern := regexp.MustCompile(part.name + `_file=(.+) bytes=`)
		m := pattern.FindStringSubmatch(out)
		require.Len(t, m, 2)
		data, err := os.ReadFile(m[1])
		require.NoError(t, err)
		require.Equal(t, part.packet, data)
	}
	skeleton := "--fixture-boundary\r\nContent-Disposition: form-data; name=\"file\"; filename=\"test.bin\"\r\nContent-Type: application/octet-stream\r\n\r\n[[yakit: multipart file spilled, part=0, file=test.bin, size=110000]]\r\n--fixture-boundary--\r\n"
	multiHead := []byte("POST /multipart HTTP/1.1\r\nHost: auth.example\r\nContent-Type: multipart/form-data; boundary=fixture-boundary\r\n\r\n")
	partFile := write("part-0-data.txt", largeRequest)
	manifest := fmt.Sprintf(`[{"Index":0,"FieldName":"file","Filename":"test.bin","ContentType":"application/octet-stream","Size":%d,"File":"part-0-data.txt"}]`, len(largeRequest))
	write("manifest.json", []byte(manifest))
	multi := save(db, 6, "https://auth.example/multipart", string(multiHead)+skeleton, "HTTP/1.1 200 OK\r\n\r\nok", 200)
	multi.IsTooLargeRequest = true
	multi.TooLargeRequestBodyFile = partFile
	multi.TooLargeRequestHeaderFile = write("multipart-head", multiHead)
	require.True(t, yakit.FlowIsMultipartSpill(multi))
	require.NoError(t, db.Save(multi).Error)
	out = invoke("fetch_http_packet_by_id", map[string]any{"id": 6, "packet_limit": 128, "output_dir": dir})
	m := regexp.MustCompile(`request_file=(.+) bytes=`).FindStringSubmatch(out)
	require.Len(t, m, 2)
	data, err = os.ReadFile(m[1])
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(data, multiHead))
	mr := multipart.NewReader(bytes.NewReader(data[len(multiHead):]), "fixture-boundary")
	part, err := mr.NextPart()
	require.NoError(t, err)
	require.Equal(t, "test.bin", part.FileName())
	rebuilt, err := io.ReadAll(part)
	require.NoError(t, err)
	require.Equal(t, largeRequest, rebuilt)
	_, err = mr.NextPart()
	require.ErrorIs(t, err, io.EOF)

	// 6. Reject invalid/missing/SSA selections, absent IDs, excessive budgets and
	// missing sidecars. Cancellation is honored; no fake complete files are returned.
	for _, params := range []map[string]any{{"database_id": "arbitrary.db"}, {"database_id": ids["Missing project"]}, {"database_id": ids["SSA project"]}, {"limit": 100, "packet_limit": 32768}} {
		_, err := execute("query_http_packet_history", params)
		require.Error(t, err)
	}
	for _, params := range []map[string]any{{"id": 0}, {"id": 9999}, {"id": 1, "packet_limit": 0}} {
		_, err := execute("fetch_http_packet_by_id", params)
		require.Error(t, err)
	}
	require.NoError(t, os.Remove(spill.TooLargeRequestBodyFile))
	_, err = execute("fetch_http_packet_by_id", map[string]any{"id": 5, "output_dir": t.TempDir()})
	require.Error(t, err)
	require.NoError(t, os.WriteFile(spill.TooLargeRequestBodyFile, largeRequest, 0600))
	require.NoError(t, os.Remove(spill.TooLargeResponseBodyFile))
	// Request export succeeds, then response export fails: remove partial artifacts.
	failurePage, err := yaklib.QueryHTTPFlows(yaklib.WithDBHistoryRuntime(ctx, db, profile), yaklib.DatabaseExports["afterID"].(func(int64) yaklib.DBHistoryOption)(4), yaklib.DatabaseExports["beforeID"].(func(int64) yaklib.DBHistoryOption)(6))
	require.NoError(t, err)
	require.Len(t, failurePage.Items, 1)
	item := failurePage.Items[0]
	failedDir := t.TempDir()
	_, err = item.ExportPackets(failedDir, "both")
	require.Error(t, err)
	files, err := os.ReadDir(failedDir)
	require.NoError(t, err)
	require.Empty(t, files)
	canceled, cancelNow := context.WithCancel(ctx)
	cancelNow()
	_, err = yaklib.ListYakProjects(yaklib.WithDBHistoryRuntime(canceled, db, profile))
	require.ErrorIs(t, err, context.Canceled)
	// File output reports its complete byte count, not a quoted JSON length.
	length, err := strconv.Atoi(match[2])
	require.NoError(t, err)
	require.Equal(t, len(huge), length)
	require.False(t, strings.Contains(large, "LATE-EVIDENCE"), "large body should only be accessed via its file")
}

func dbPath(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var seq int
	var name, path string
	require.NoError(t, db.DB().QueryRow("PRAGMA database_list").Scan(&seq, &name, &path))
	return path
}
