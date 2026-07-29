package yakit

import (
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func benchmarkDurationP95(samples []time.Duration) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := (95*len(sorted) + 99) / 100
	if index < 1 {
		index = 1
	}
	return sorted[index-1]
}

// BenchmarkHTTPFlowSQLiteReadWriteContention models the live MITM workload:
// one serialized HTTPFlow writer and one projected, count-free incremental
// query begin together on every iteration. Use a fixed benchtime when comparing
// modes, for example: -bench BenchmarkHTTPFlowSQLiteReadWriteContention -benchtime=120x.
func BenchmarkHTTPFlowSQLiteReadWriteContention(b *testing.B) {
	modes := []struct {
		name               string
		writerMaxOpenConns int
		separateReader     bool
	}{
		{name: "max-open-1", writerMaxOpenConns: 1},
		{name: "max-open-2", writerMaxOpenConns: 2},
		{name: "single-writer-separate-reader", writerMaxOpenConns: 1, separateReader: true},
	}
	for _, mode := range modes {
		b.Run(mode.name, func(b *testing.B) {
			b.Setenv(consts.YakitSQLiteProjectMaxOpenConnsEnv, strconv.Itoa(mode.writerMaxOpenConns))
			if mode.separateReader {
				b.Setenv(consts.YakitSQLiteProjectReadPoolConnsEnv, "1")
			} else {
				b.Setenv(consts.YakitSQLiteProjectReadPoolConnsEnv, "0")
			}
			path := b.TempDir() + "/project.db"
			db, err := consts.CreateProjectDatabase(path)
			if err != nil {
				b.Fatal(err)
			}
			defer db.Close()
			readDB := db
			if mode.separateReader {
				readDB, err = consts.CreateProjectDatabaseReadOnly(path)
				if err != nil {
					b.Fatal(err)
				}
				defer readDB.Close()
			}

			request := strconv.Quote("POST /upload HTTP/1.1\r\nHost: example.test\r\n\r\n" + strings.Repeat("q", 32*1024))
			response := strconv.Quote("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\n\r\n" + strings.Repeat("s", 32*1024))
			writeJobs := make(chan int)
			readJobs := make(chan struct{})
			results := make(chan error, 2)
			writeDurations := make([]time.Duration, 0, b.N)
			readDurations := make([]time.Duration, 0, b.N)

			go func() {
				for index := range writeJobs {
					startedAt := time.Now()
					err := InsertHTTPFlow(db, &schema.HTTPFlow{
						Url:           fmt.Sprintf("http://example.test/upload/%d", index),
						Path:          fmt.Sprintf("/upload/%d", index),
						Method:        "POST",
						Request:       request,
						Response:      response,
						RequestLength: int64(len(request)),
						BodyLength:    int64(len(response)),
						SourceType:    schema.HTTPFlow_SourceType_MITM,
						ContentType:   "application/octet-stream",
						StatusCode:    200,
						HtmlTitle:     sql.NullString{Valid: true},
					})
					writeDurations = append(writeDurations, time.Since(startedAt))
					results <- err
				}
			}()

			go func() {
				var cursor int64
				for range readJobs {
					startedAt := time.Now()
					_, flows, err := QueryHTTPFlow(readDB, &ypb.QueryHTTPFlowRequest{
						AfterId:            cursor,
						AfterUpdatedAt:     time.Now().Add(-time.Minute).Unix(),
						SourceType:         schema.HTTPFlow_SourceType_MITM,
						ExcludeRequestRaw:  true,
						ExcludeResponseRaw: true,
						SkipTotal:          true,
						Pagination: &ypb.Paging{
							Page:    1,
							Limit:   300,
							OrderBy: "id",
							Order:   "asc",
						},
					})
					if len(flows) > 0 {
						cursor = int64(flows[len(flows)-1].ID)
					}
					readDurations = append(readDurations, time.Since(startedAt))
					results <- err
				}
			}()

			writerStatsBefore := db.DB().Stats()
			readerStatsBefore := readDB.DB().Stats()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				writeJobs <- index
				readJobs <- struct{}{}
				for completed := 0; completed < 2; completed++ {
					if err := <-results; err != nil {
						b.Fatal(err)
					}
				}
			}
			b.StopTimer()
			close(writeJobs)
			close(readJobs)

			writerStatsAfter := db.DB().Stats()
			readerStatsAfter := readDB.DB().Stats()
			operations := float64(max(1, 2*b.N))
			b.ReportMetric(float64(benchmarkDurationP95(writeDurations).Microseconds()), "write-p95-us")
			b.ReportMetric(float64(benchmarkDurationP95(readDurations).Microseconds()), "read-p95-us")
			poolWaits := writerStatsAfter.WaitCount - writerStatsBefore.WaitCount
			poolWaitDuration := writerStatsAfter.WaitDuration - writerStatsBefore.WaitDuration
			if readDB != db {
				poolWaits += readerStatsAfter.WaitCount - readerStatsBefore.WaitCount
				poolWaitDuration += readerStatsAfter.WaitDuration - readerStatsBefore.WaitDuration
			}
			b.ReportMetric(float64(poolWaits), "pool-waits")
			b.ReportMetric(float64(poolWaitDuration.Microseconds())/operations, "pool-wait-us/op")
		})
	}
}
