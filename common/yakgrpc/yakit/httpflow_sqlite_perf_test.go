package yakit

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	benchHTTPFlowRowsEnv   = "YAK_BENCH_HTTPFLOW_ROWS"
	benchHTTPFlowBatchEnv  = "YAK_BENCH_HTTPFLOW_BATCH"
	defaultBenchHTTPFlow   = 500000
	defaultBenchBatchSize  = 2000
	defaultBenchKeyword    = "yakbench"
	defaultBenchPageSize   = 30
	defaultBenchKeywordMod = 10
)

type httpFlowBenchFixture struct {
	path    string
	db      *gorm.DB
	rows    int
	keyword string
}

var (
	httpFlowBenchOnce         sync.Once
	httpFlowBenchFixtureValue *httpFlowBenchFixture
	httpFlowBenchErr          error
	projectDBSeq              uint64
)

// Run: CGO_ENABLED=1 go test ./common/yakgrpc/yakit -tags "sqlite_fts5" -bench BenchmarkHTTPFlowInsert -benchmem -run ^$ -count=1
func BenchmarkHTTPFlowInsert(b *testing.B) {
	_, db, err := createProjectTestDB(b)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		flow := &schema.HTTPFlow{
			HiddenIndex: fmt.Sprintf("bench-insert-%d", i),
			Url:         fmt.Sprintf("https://example.com/bench/%d", i%1000),
			Path:        fmt.Sprintf("/bench/%d", i%1000),
			Method:      "GET",
			Request:     "GET /bench HTTP/1.1\r\nHost: example.com\r\n\r\n",
			Response:    "HTTP/1.1 200 OK\r\n\r\n",
			SourceType:  schema.HTTPFlow_SourceType_MITM,
			RuntimeId:   "bench-runtime",
			Tags:        "bench",
			RemoteAddr:  "127.0.0.1",
			ContentType: "text/plain",
			BodyLength:  128,
			StatusCode:  200,
		}
		if err := InsertHTTPFlow(db, flow); err != nil {
			b.Fatal(err)
		}
	}
}

// Run: CGO_ENABLED=1 YAK_BENCH_HTTPFLOW_ROWS=500000 go test ./common/yakgrpc/yakit -tags "sqlite_fts5" -bench BenchmarkHTTPFlowQuery_SourceUpdatedAt -benchmem -run ^$ -count=1
func BenchmarkHTTPFlowQuery_SourceUpdatedAt(b *testing.B) {
	fixture := getHTTPFlowBenchFixture(b)
	params := &ypb.QueryHTTPFlowRequest{
		SourceType: schema.HTTPFlow_SourceType_MITM,
		Pagination: benchHTTPFlowPaging(),
	}

	_, flows, err := QueryHTTPFlow(fixture.db, params)
	if err != nil {
		b.Fatal(err)
	}
	if len(flows) == 0 {
		b.Fatal("seeded data missing")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := QueryHTTPFlow(fixture.db, params); err != nil {
			b.Fatal(err)
		}
	}
}

// Run: CGO_ENABLED=1 YAK_BENCH_HTTPFLOW_ROWS=500000 go test ./common/yakgrpc/yakit -tags "sqlite_fts5" -bench BenchmarkHTTPFlowQuery_RuntimeUpdatedAt -benchmem -run ^$ -count=1
func BenchmarkHTTPFlowQuery_RuntimeUpdatedAt(b *testing.B) {
	fixture := getHTTPFlowBenchFixture(b)
	params := &ypb.QueryHTTPFlowRequest{
		RuntimeId:  "rt-1",
		Pagination: benchHTTPFlowPaging(),
	}

	_, flows, err := QueryHTTPFlow(fixture.db, params)
	if err != nil {
		b.Fatal(err)
	}
	if len(flows) == 0 {
		b.Fatal("seeded data missing")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := QueryHTTPFlow(fixture.db, params); err != nil {
			b.Fatal(err)
		}
	}
}

// Run: CGO_ENABLED=1 YAK_BENCH_HTTPFLOW_ROWS=500000 go test ./common/yakgrpc/yakit -tags "sqlite_fts5" -bench BenchmarkHTTPFlowQuery_KeywordFTS -benchmem -run ^$ -count=1
func BenchmarkHTTPFlowQuery_KeywordFTS(b *testing.B) {
	fixture := getHTTPFlowBenchFixture(b)
	params := &ypb.QueryHTTPFlowRequest{
		Keyword:     fixture.keyword,
		KeywordType: "request",
		Pagination:  benchHTTPFlowPaging(),
	}

	_, flows, err := QueryHTTPFlow(fixture.db, params)
	if err != nil {
		b.Fatal(err)
	}
	if len(flows) == 0 {
		b.Fatal("seeded data missing")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := QueryHTTPFlow(fixture.db, params); err != nil {
			b.Fatal(err)
		}
	}
}

// Run: CGO_ENABLED=1 go test ./common/yakgrpc/yakit -tags "sqlite_fts5" -bench BenchmarkMITMV2_HTTPFlowInsertSync -benchmem -run ^$ -count=1
func BenchmarkMITMV2_HTTPFlowInsertSync(b *testing.B) {
	_, _, err := createProjectTestDBWithGlobal(b)
	if err != nil {
		b.Fatal(err)
	}
	prefix := benchMITMPrefix(b)
	if consts.IsHTTPFlowFTSAsyncEnabled() {
		warm := newBenchMITMFlow(prefix, -1, "")
		if err := InsertHTTPFlowEx(warm, true); err != nil {
			b.Fatal(err)
		}
		waitForAsyncQueueEmpty(b, 30*time.Second)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		flow := newBenchMITMFlow(prefix, i, defaultBenchKeyword)
		if err := InsertHTTPFlowEx(flow, true); err != nil {
			b.Fatal(err)
		}
	}
}

// Run: CGO_ENABLED=1 go test ./common/yakgrpc/yakit -tags "sqlite_fts5" -bench BenchmarkMITMV2_HTTPFlowInsertAsync -benchmem -run ^$ -count=1
func BenchmarkMITMV2_HTTPFlowInsertAsync(b *testing.B) {
	_, _, err := createProjectTestDBWithGlobal(b)
	if err != nil {
		b.Fatal(err)
	}
	prefix := benchMITMPrefix(b)
	prevSync := consts.GLOBAL_DB_SAVE_SYNC.IsSet()
	consts.GLOBAL_DB_SAVE_SYNC.SetTo(false)
	defer consts.GLOBAL_DB_SAVE_SYNC.SetTo(prevSync)
	waitForAsyncQueueEmpty(b, 30*time.Second)
	if consts.IsHTTPFlowFTSAsyncEnabled() {
		warm := newBenchMITMFlow(prefix, -1, "")
		if err := InsertHTTPFlowEx(warm, false); err != nil {
			b.Fatal(err)
		}
		waitForAsyncQueueEmpty(b, 30*time.Second)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		flow := newBenchMITMFlow(prefix, i, defaultBenchKeyword)
		if err := InsertHTTPFlowEx(flow, false); err != nil {
			b.Fatal(err)
		}
	}
	waitForAsyncQueueEmpty(b, 30*time.Second)
}

// Run: CGO_ENABLED=1 go test ./common/yakgrpc/yakit -tags "sqlite_fts5" -bench BenchmarkMITMV2_HTTPFlowUpdateTagsAsync -benchmem -run ^$ -count=1
func BenchmarkMITMV2_HTTPFlowUpdateTagsAsync(b *testing.B) {
	_, db, err := createProjectTestDBWithGlobal(b)
	if err != nil {
		b.Fatal(err)
	}
	prefix := benchMITMPrefix(b)
	prevSync := consts.GLOBAL_DB_SAVE_SYNC.IsSet()
	consts.GLOBAL_DB_SAVE_SYNC.SetTo(false)
	defer consts.GLOBAL_DB_SAVE_SYNC.SetTo(prevSync)

	flow := newBenchMITMFlow(prefix, 0, defaultBenchKeyword)
	if err := InsertHTTPFlow(db, flow); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		flow.Tags = fmt.Sprintf("tag-%d", i)
		if err := UpdateHTTPFlowTagsEx(flow); err != nil {
			b.Fatal(err)
		}
	}
	waitForAsyncQueueEmpty(b, 10*time.Second)
}

// Run: CGO_ENABLED=1 go test ./common/yakgrpc/yakit -tags "sqlite_fts5" -bench BenchmarkMITMV2_SaveBareRequestKV -benchmem -run ^$ -count=1
func BenchmarkMITMV2_SaveBareRequestKV(b *testing.B) {
	_, db, err := createProjectTestDB(b)
	if err != nil {
		b.Fatal(err)
	}
	payload := []byte("GET /bench HTTP/1.1\r\nHost: example.com\r\n\r\n")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("%d_request", i)
		if err := SetProjectKeyWithGroup(db, key, payload, BARE_REQUEST_GROUP); err != nil {
			b.Fatal(err)
		}
	}
}

// Run: CGO_ENABLED=1 go test ./common/yakgrpc/yakit -tags "sqlite_fts5" -bench BenchmarkMITMV2_SaveBareResponseKV -benchmem -run ^$ -count=1
func BenchmarkMITMV2_SaveBareResponseKV(b *testing.B) {
	_, db, err := createProjectTestDB(b)
	if err != nil {
		b.Fatal(err)
	}
	payload := []byte("HTTP/1.1 200 OK\r\n\r\n")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("%d_response", i)
		if err := SetProjectKeyWithGroup(db, key, payload, BARE_RESPONSE_GROUP); err != nil {
			b.Fatal(err)
		}
	}
}

// Run: CGO_ENABLED=1 go test ./common/yakgrpc/yakit -tags "sqlite_fts5" -bench BenchmarkMITMV2_SaveExtractedDataAsync -benchmem -run ^$ -count=1
func BenchmarkMITMV2_SaveExtractedDataAsync(b *testing.B) {
	_, _, err := createProjectTestDBWithGlobal(b)
	if err != nil {
		b.Fatal(err)
	}
	prevSync := consts.GLOBAL_DB_SAVE_SYNC.IsSet()
	consts.GLOBAL_DB_SAVE_SYNC.SetTo(false)
	defer consts.GLOBAL_DB_SAVE_SYNC.SetTo(prevSync)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data := &schema.ExtractedData{
			SourceType:     "httpflow",
			TraceId:        fmt.Sprintf("hid-%d", i),
			RuleVerbose:    "bench-rule",
			Data:           fmt.Sprintf("match-%d", i),
			DataIndex:      i % 128,
			Length:         16,
			IsMatchRequest: true,
		}
		if err := CreateOrUpdateExtractedDataEx(-1, data); err != nil {
			b.Fatal(err)
		}
	}
	waitForAsyncQueueEmpty(b, 10*time.Second)
}

// Run: CGO_ENABLED=1 YAK_BENCH_HTTPFLOW_ROWS=500000 go test ./common/yakgrpc/yakit -tags "sqlite_fts5" -bench BenchmarkMITMV2_MixedWriteRead -benchmem -run ^$ -count=1
func BenchmarkMITMV2_MixedWriteRead(b *testing.B) {
	fixture := getHTTPFlowBenchFixture(b)
	b.StopTimer()
	_, writeDB, err := cloneBenchmarkDatabase(b, fixture)
	if err != nil {
		b.Fatal(err)
	}
	readDB, err := openReadOnlyDB(fixture.path)
	if err != nil {
		b.Fatal(err)
	}
	prefix := benchMITMPrefix(b)
	b.StartTimer()

	const writeEvery = 10
	var seq uint64
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%writeEvery == 0 {
				id := int(atomic.AddUint64(&seq, 1))
				flow := newBenchMITMFlow(prefix, id, defaultBenchKeyword)
				if err := InsertHTTPFlow(writeDB, flow); err != nil {
					b.Fatal(err)
				}
			} else {
				params := &ypb.QueryHTTPFlowRequest{
					SourceType: schema.HTTPFlow_SourceType_MITM,
					Pagination: benchHTTPFlowPaging(),
				}
				if _, _, err := QueryHTTPFlow(readDB, params); err != nil {
					b.Fatal(err)
				}
			}
			i++
		}
	})
}

// Run: CGO_ENABLED=1 go test ./common/yakgrpc/yakit -tags "sqlite_fts5" -run TestHTTPFlowFTS5Enabled -count=1
func TestHTTPFlowFTS5Enabled(t *testing.T) {
	_, db, err := createProjectTestDB(t)
	require.NoError(t, err)
	require.NoError(t, seedHTTPFlows(db, 2000, defaultBenchKeyword))

	var createSQL string
	require.NoError(t, db.Raw(`SELECT sql FROM sqlite_master WHERE type='table' AND name='http_flows_fts';`).Row().Scan(&createSQL))
	require.NotEmpty(t, createSQL)
	lower := strings.ToLower(createSQL)
	require.Contains(t, lower, "fts5")
	require.Contains(t, lower, "tokenize='trigram'")

	var count int
	ftsQuery := buildHTTPFlowFTSQuery("request", defaultBenchKeyword, ftsSupportsPhraseQuery(createSQL))
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM http_flows_fts WHERE http_flows_fts MATCH ?`, ftsQuery).Row().Scan(&count))
	require.Greater(t, count, 0)
}

// Run: CGO_ENABLED=1 go test ./common/yakgrpc/yakit -tags "sqlite_fts5" -run TestHTTPFlowQueryPlan_UsesCompositeIndexes -count=1
func TestHTTPFlowQueryPlan_UsesCompositeIndexes(t *testing.T) {

	_, db, err := createProjectTestDB(t)
	require.NoError(t, err)
	require.NoError(t, seedHTTPFlows(db, 5000, defaultBenchKeyword))

	sourcePlan, err := explainQueryPlan(db, `SELECT id FROM http_flows WHERE source_type = ? ORDER BY updated_at DESC LIMIT ?`, schema.HTTPFlow_SourceType_MITM, defaultBenchPageSize)
	require.NoError(t, err)
	require.Contains(t, strings.Join(sourcePlan, "|"), "idx_http_flows_source_updated_at")

	runtimePlan, err := explainQueryPlan(db, `SELECT id FROM http_flows WHERE runtime_id = ? ORDER BY updated_at DESC LIMIT ?`, "rt-1", defaultBenchPageSize)
	require.NoError(t, err)
	require.Contains(t, strings.Join(runtimePlan, "|"), "idx_http_flows_runtime_id_updated_at")
}

func getHTTPFlowBenchFixture(tb testing.TB) *httpFlowBenchFixture {
	tb.Helper()
	httpFlowBenchOnce.Do(func() {
		var db *gorm.DB
		var path string
		path, db, httpFlowBenchErr = createBenchmarkDatabase(tb)
		if httpFlowBenchErr != nil {
			return
		}
		rows := benchHTTPFlowRows()
		if err := seedHTTPFlows(db, rows, defaultBenchKeyword); err != nil {
			httpFlowBenchErr = err
			return
		}
		httpFlowBenchFixtureValue = &httpFlowBenchFixture{
			path:    path,
			db:      db,
			rows:    rows,
			keyword: defaultBenchKeyword,
		}
	})
	if httpFlowBenchErr != nil {
		tb.Fatal(httpFlowBenchErr)
	}
	return httpFlowBenchFixtureValue
}

func benchHTTPFlowRows() int {
	if raw := os.Getenv(benchHTTPFlowRowsEnv); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			return v
		}
	}
	return defaultBenchHTTPFlow
}

func benchHTTPFlowBatchSize(rows int) int {
	if raw := os.Getenv(benchHTTPFlowBatchEnv); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			if v > rows {
				return rows
			}
			return v
		}
	}
	if rows < defaultBenchBatchSize {
		return rows
	}
	return defaultBenchBatchSize
}

func benchHTTPFlowPaging() *ypb.Paging {
	return &ypb.Paging{
		Page:    1,
		Limit:   defaultBenchPageSize,
		OrderBy: "updated_at",
		Order:   "desc",
	}
}

func createProjectTestDB(tb testing.TB) (string, *gorm.DB, error) {
	tb.Helper()
	dir := tb.TempDir()
	seq := atomic.AddUint64(&projectDBSeq, 1)
	dbPath := filepath.Join(dir, fmt.Sprintf("httpflow-%d.sqlite3", seq))
	db, err := consts.CreateProjectDatabase(dbPath)
	if err != nil {
		return "", nil, err
	}
	// Ensure WAL/PRAGMA are applied on the fresh connection.
	db.DB().SetMaxOpenConns(1)
	return dbPath, db, nil
}

func createBenchmarkDatabase(tb testing.TB) (string, *gorm.DB, error) {
	tb.Helper()
	dir, err := os.MkdirTemp("", "yakbench-httpflow-")
	if err != nil {
		return "", nil, err
	}
	dbPath := filepath.Join(dir, "httpflow.sqlite3")
	db, err := consts.CreateProjectDatabase(dbPath)
	if err != nil {
		return "", nil, err
	}
	// Keep the benchmark fixture directory around to allow reuse across benchmarks.
	db.DB().SetMaxOpenConns(1)
	return dbPath, db, nil
}

func cloneBenchmarkDatabase(tb testing.TB, fixture *httpFlowBenchFixture) (string, *gorm.DB, error) {
	tb.Helper()
	if fixture == nil || fixture.path == "" {
		return "", nil, fmt.Errorf("benchmark fixture is not initialized")
	}
	if fixture.db != nil {
		_ = fixture.db.Exec("PRAGMA wal_checkpoint(TRUNCATE);").Error
	}
	dir, err := os.MkdirTemp("", "yakbench-mixed-")
	if err != nil {
		return "", nil, err
	}
	destPath := filepath.Join(dir, "httpflow.sqlite3")
	if err := copyFile(fixture.path, destPath); err != nil {
		return "", nil, err
	}
	_ = copyFileOptional(fixture.path+"-wal", destPath+"-wal")
	_ = copyFileOptional(fixture.path+"-shm", destPath+"-shm")
	db, err := consts.CreateProjectDatabase(destPath)
	if err != nil {
		return "", nil, err
	}
	db.DB().SetMaxOpenConns(1)
	return destPath, db, nil
}

func copyFileOptional(src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		return nil
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o666)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func createProjectTestDBWithGlobal(tb testing.TB) (string, *gorm.DB, error) {
	tb.Helper()
	dbPath, _, err := createProjectTestDB(tb)
	if err != nil {
		return "", nil, err
	}
	if err := consts.SetGormProjectDatabase(dbPath); err != nil {
		return "", nil, err
	}
	return dbPath, consts.GetGormProjectDatabase(), nil
}

func openReadOnlyDB(path string) (*gorm.DB, error) {
	db, err := gorm.Open("sqlite3", path+"?mode=ro&cache=shared")
	if err != nil {
		return nil, err
	}
	maxOpen := runtime.NumCPU()
	if maxOpen < 4 {
		maxOpen = 4
	}
	db.DB().SetMaxIdleConns(maxOpen)
	db.DB().SetMaxOpenConns(maxOpen)
	_ = db.Exec("PRAGMA query_only = ON;").Error
	return db, nil
}

func waitForAsyncQueueEmpty(tb testing.TB, timeout time.Duration) {
	tb.Helper()
	flushHTTPFlowInsertQueue()
	done := make(chan struct{})
	select {
	case DBSaveAsyncChannel <- func(db *gorm.DB) error {
		close(done)
		return nil
	}:
	case <-time.After(timeout):
		tb.Fatalf("async db queue enqueue timed out after %s (len=%d)", timeout, len(DBSaveAsyncChannel))
	}
	select {
	case <-done:
		return
	case <-time.After(timeout):
		tb.Fatalf("async db queue not drained within %s (len=%d)", timeout, len(DBSaveAsyncChannel))
	}
}

func benchMITMPrefix(tb testing.TB) string {
	tb.Helper()
	return fmt.Sprintf("bench-%s-%d", tb.Name(), time.Now().UnixNano())
}

func newBenchMITMFlow(prefix string, i int, keyword string) *schema.HTTPFlow {
	path := fmt.Sprintf("/mitm/%d", i%1000)
	request := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: example.com\r\n\r\n", path)
	response := "HTTP/1.1 200 OK\r\n\r\n"
	if keyword != "" && i%defaultBenchKeywordMod == 0 {
		request += keyword
		response += keyword
	}
	flow := &schema.HTTPFlow{
		HiddenIndex: fmt.Sprintf("%s-mitm-%d", prefix, i),
		Url:         fmt.Sprintf("https://example.com%s", path),
		Path:        path,
		Method:      "GET",
		Request:     request,
		Response:    response,
		SourceType:  schema.HTTPFlow_SourceType_MITM,
		RuntimeId:   fmt.Sprintf("rt-%d", i%128),
		Tags:        "mitm",
		RemoteAddr:  "127.0.0.1",
		ContentType: "text/plain",
		BodyLength:  int64(128 + i%1024),
		StatusCode:  int64(200 + i%10),
	}
	return flow
}

func seedHTTPFlows(db *gorm.DB, rows int, keyword string) error {
	if rows <= 0 {
		return nil
	}
	columns, err := fetchHTTPFlowColumns(db)
	if err != nil {
		return err
	}
	specs := httpFlowBenchInsertSpecs(columns)
	if len(specs) == 0 {
		return fmt.Errorf("no insertable columns found for http_flows")
	}
	columnNames := make([]string, 0, len(specs))
	for _, spec := range specs {
		columnNames = append(columnNames, spec.name)
	}
	placeholders := make([]string, len(columnNames))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	insertSQL := fmt.Sprintf(
		"INSERT INTO http_flows (%s) VALUES (%s);",
		strings.Join(columnNames, ", "),
		strings.Join(placeholders, ", "),
	)

	sqlDB := db.DB()

	sourceTypes := []string{
		schema.HTTPFlow_SourceType_MITM,
		schema.HTTPFlow_SourceType_SCAN,
		schema.HTTPFlow_SourceType_CRAWLER,
	}
	methods := []string{"GET", "POST", "PUT"}
	tags := []string{"tag-a", "tag-b", "tag-c"}

	now := time.Now().UTC()
	batchSize := benchHTTPFlowBatchSize(rows)
	for start := 0; start < rows; start += batchSize {
		end := start + batchSize
		if end > rows {
			end = rows
		}

		tx, err := sqlDB.Begin()
		if err != nil {
			return err
		}
		stmt, err := tx.Prepare(insertSQL)
		if err != nil {
			_ = tx.Rollback()
			return err
		}

		for i := start; i < end; i++ {
			idx := i + 1
			sourceType := sourceTypes[idx%len(sourceTypes)]
			method := methods[idx%len(methods)]
			runtimeID := fmt.Sprintf("rt-%d", idx%128)
			url := fmt.Sprintf("https://example.com/%s/item/%d", sourceType, idx%1000)
			path := fmt.Sprintf("/%s/item/%d", sourceType, idx%1000)
			request := fmt.Sprintf("%s %s HTTP/1.1\r\nHost: example.com\r\n\r\n", method, path)
			response := "HTTP/1.1 200 OK\r\n\r\n"
			if keyword != "" && idx%defaultBenchKeywordMod == 0 {
				request += keyword
				response += keyword
			}
			bodyLen := int64(128 + idx%1024)
			reqLen := int64(len(request))
			respLen := int64(len(response))
			status := int64(200 + idx%10)
			createdAt := now.Add(time.Duration(idx) * time.Millisecond)
			updatedAt := createdAt.Add(time.Duration(idx%1000) * time.Millisecond)
			row := httpFlowBenchInsertRow{
				createdAt:   createdAt,
				updatedAt:   updatedAt,
				hiddenIndex: fmt.Sprintf("hid-%d", idx),
				hash:        fmt.Sprintf("hash-%d", idx),
				url:         url,
				path:        path,
				method:      method,
				request:     request,
				response:    response,
				sourceType:  sourceType,
				runtimeID:   runtimeID,
				tags:        tags[idx%len(tags)],
				remoteAddr:  "127.0.0.1",
				contentType: "text/plain",
				bodyLen:     bodyLen,
				status:      status,
				reqLen:      reqLen,
				respLen:     respLen,
				duration:    int64(idx % 500),
			}

			values := make([]any, 0, len(specs))
			for _, spec := range specs {
				values = append(values, spec.value(row))
			}
			if _, err := stmt.Exec(values...); err != nil {
				_ = stmt.Close()
				_ = tx.Rollback()
				return err
			}
		}

		if err := stmt.Close(); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return rebuildHTTPFlowFTS(db)
}

func rebuildHTTPFlowFTS(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	var tableName string
	if err := db.Raw(`SELECT name FROM sqlite_master WHERE type='table' AND name='http_flows_fts';`).Row().Scan(&tableName); err != nil {
		return err
	}
	if tableName == "" {
		return nil
	}
	return db.Exec(`INSERT INTO "http_flows_fts"("http_flows_fts") VALUES('rebuild');`).Error
}

type httpFlowBenchInsertRow struct {
	createdAt   time.Time
	updatedAt   time.Time
	hiddenIndex string
	hash        string
	url         string
	path        string
	method      string
	request     string
	response    string
	sourceType  string
	runtimeID   string
	tags        string
	remoteAddr  string
	contentType string
	bodyLen     int64
	status      int64
	reqLen      int64
	respLen     int64
	duration    int64
}

type httpFlowBenchInsertSpec struct {
	name  string
	value func(httpFlowBenchInsertRow) any
}

func fetchHTTPFlowColumns(db *gorm.DB) (map[string]struct{}, error) {
	rows, err := db.Raw(`PRAGMA table_info(http_flows);`).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make(map[string]struct{})
	for rows.Next() {
		var cid, notnull, pk int
		var name, colType string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		columns[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

func httpFlowBenchInsertSpecs(columns map[string]struct{}) []httpFlowBenchInsertSpec {
	candidates := []httpFlowBenchInsertSpec{
		{name: "created_at", value: func(r httpFlowBenchInsertRow) any { return r.createdAt }},
		{name: "updated_at", value: func(r httpFlowBenchInsertRow) any { return r.updatedAt }},
		{name: "hidden_index", value: func(r httpFlowBenchInsertRow) any { return r.hiddenIndex }},
		{name: "hash", value: func(r httpFlowBenchInsertRow) any { return r.hash }},
		{name: "url", value: func(r httpFlowBenchInsertRow) any { return r.url }},
		{name: "path", value: func(r httpFlowBenchInsertRow) any { return r.path }},
		{name: "method", value: func(r httpFlowBenchInsertRow) any { return r.method }},
		{name: "request", value: func(r httpFlowBenchInsertRow) any { return r.request }},
		{name: "response", value: func(r httpFlowBenchInsertRow) any { return r.response }},
		{name: "source_type", value: func(r httpFlowBenchInsertRow) any { return r.sourceType }},
		{name: "runtime_id", value: func(r httpFlowBenchInsertRow) any { return r.runtimeID }},
		{name: "tags", value: func(r httpFlowBenchInsertRow) any { return r.tags }},
		{name: "remote_addr", value: func(r httpFlowBenchInsertRow) any { return r.remoteAddr }},
		{name: "content_type", value: func(r httpFlowBenchInsertRow) any { return r.contentType }},
		{name: "body_length", value: func(r httpFlowBenchInsertRow) any { return r.bodyLen }},
		{name: "status_code", value: func(r httpFlowBenchInsertRow) any { return r.status }},
		{name: "request_length", value: func(r httpFlowBenchInsertRow) any { return r.reqLen }},
		{name: "response_length", value: func(r httpFlowBenchInsertRow) any { return r.respLen }},
		{name: "duration", value: func(r httpFlowBenchInsertRow) any { return r.duration }},
	}

	var specs []httpFlowBenchInsertSpec
	for _, spec := range candidates {
		if _, ok := columns[spec.name]; ok {
			specs = append(specs, spec)
		}
	}
	return specs
}

func explainQueryPlan(db *gorm.DB, query string, args ...any) ([]string, error) {
	rows, err := db.Raw("EXPLAIN QUERY PLAN "+query, args...).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var details []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			return nil, err
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return details, nil
}
