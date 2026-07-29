package yakit

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func BenchmarkHTTPFlowSQLiteTextBinding(b *testing.B) {
	tests := []struct {
		name          string
		requestBytes  int
		responseBytes int
	}{
		{name: "small", requestBytes: 1024, responseBytes: 4096},
		{name: "medium", requestBytes: 16 * 1024, responseBytes: 16 * 1024},
		{name: "large", requestBytes: 64 * 1024, responseBytes: 256 * 1024},
	}

	for _, test := range tests {
		test := test
		request := strings.Repeat("r", test.requestBytes)
		response := strings.Repeat("s", test.responseBytes)
		for _, mode := range []string{"string", "text-bytes", "adaptive"} {
			mode := mode
			b.Run(test.name+"/"+mode, func(b *testing.B) {
				db, err := gorm.Open("sqlite3", filepath.Join(b.TempDir(), "project.db"))
				if err != nil {
					b.Fatal(err)
				}
				defer db.Close()
				if err := db.AutoMigrate(&schema.HTTPFlow{}).Error; err != nil {
					b.Fatal(err)
				}

				uniquePrefix := fmt.Sprintf("%s-%s-%d", test.name, mode, time.Now().UnixNano())
				flows := make([]*schema.HTTPFlow, b.N)
				for index := range flows {
					flows[index] = &schema.HTTPFlow{
						HiddenIndex: fmt.Sprintf("%s-%d", uniquePrefix, index),
						Request:     request,
						Response:    response,
						SourceType:  schema.HTTPFlow_SourceType_MITM,
					}
				}

				b.SetBytes(int64(len(request) + len(response)))
				b.ReportAllocs()
				b.ResetTimer()
				for _, flow := range flows {
					var result *gorm.DB
					switch mode {
					case "text-bytes":
						result = db.CreateWithColumnExpressions(flow, map[string]*gorm.SqlExpr{
							"request":  gorm.Expr("CAST(? AS TEXT)", utils.UnsafeStringToBytes(flow.Request)),
							"response": gorm.Expr("CAST(? AS TEXT)", utils.UnsafeStringToBytes(flow.Response)),
						})
					case "adaptive":
						result = createHTTPFlowRecord(db, flow)
					default:
						result = db.Create(flow)
					}
					if result.Error != nil {
						b.Fatal(result.Error)
					}
				}
			})
		}
	}
}
