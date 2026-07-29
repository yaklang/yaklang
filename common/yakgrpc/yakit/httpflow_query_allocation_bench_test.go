package yakit

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var benchmarkQueriedHTTPFlows []*schema.HTTPFlow

func BenchmarkQueryHTTPFlowScan400Rows(b *testing.B) {
	db, err := gorm.Open("sqlite3", filepath.Join(b.TempDir(), "project.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	db.DB().SetMaxOpenConns(1)
	if err := db.AutoMigrate(&schema.HTTPFlow{}).Error; err != nil {
		b.Fatal(err)
	}

	flows := make([]*schema.HTTPFlow, 400)
	for index := range flows {
		path := fmt.Sprintf("/scan/%03d", index)
		flows[index] = &schema.HTTPFlow{
			Hash:       fmt.Sprintf("scan-benchmark-%03d", index),
			Url:        "http://scan.example" + path,
			Path:       path,
			Method:     "GET",
			SourceType: schema.HTTPFlow_SourceType_MITM,
			StatusCode: 200,
			Request:    "GET " + path + " HTTP/1.1\r\nHost: scan.example\r\n\r\n",
			Response:   "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n",
		}
	}
	if result := db.CreateInBatches(flows, 100); result.Error != nil {
		b.Fatal(result.Error)
	}

	request := &ypb.QueryHTTPFlowRequest{
		SourceType: schema.HTTPFlow_SourceType_MITM,
		AfterId:    0,
		SkipTotal:  true,
		Pagination: &ypb.Paging{
			Page:    1,
			Limit:   400,
			OrderBy: "id",
			Order:   "asc",
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		_, queried, queryErr := QueryHTTPFlow(db, request)
		if queryErr != nil {
			b.Fatal(queryErr)
		}
		if len(queried) != len(flows) {
			b.Fatalf("queried rows = %d, want %d", len(queried), len(flows))
		}
		benchmarkQueriedHTTPFlows = queried
	}
}
