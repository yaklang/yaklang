package yakit

import (
	"strings"
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestSearchAIYakToolBM25_SQLiteFTS5(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, db.AutoMigrate(&schema.AIYakTool{}).Error)

	if err := EnsureAIYakToolFTS5(db); err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skipf("fts5 not available: %v", err)
		}
		require.NoError(t, err)
	}

	require.NoError(t, db.Create(&schema.AIYakTool{
		Name:        "tcp_scan",
		VerboseName: "TCP Scan",
		Description: "scan tcp ports",
		Keywords:    "tcp,port,scan",
		Path:        "tools/net/tcp_scan.yak",
		Content:     "print('hello')",
	}).Error)
	require.NoError(t, db.Create(&schema.AIYakTool{
		Name:        "http_probe",
		VerboseName: "HTTP Probe",
		Description: "probe http services",
		Keywords:    "http,probe",
		Path:        "tools/web/http_probe.yak",
		Content:     "print('ok')",
	}).Error)

	got, err := SearchAIYakToolBM25(db, &AIYakToolFilter{Keywords: []string{"tcp"}}, 10, 0)
	require.NoError(t, err)
	require.NotEmpty(t, got)
	require.Equal(t, "tcp_scan", got[0].Name)
}

func TestSearchAIYakToolBM25_ChineseScenarioQuery(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, db.AutoMigrate(&schema.AIYakTool{}).Error)

	if err := EnsureAIYakToolFTS5(db); err != nil {
		if strings.Contains(err.Error(), "no such module: fts5") {
			t.Skipf("fts5 not available: %v", err)
		}
		require.NoError(t, err)
	}

	require.NoError(t, db.Create(&schema.AIYakTool{
		Name:        "batch_do_http_request",
		VerboseName: "Batch HTTP Request Tool / 批量HTTP请求工具",
		Description: "批量HTTP请求工具，适用于接口批量验证、未授权访问排查、IDOR验证、越权验证、路径探测和请求重放。",
		Keywords:    "接口批量验证,未授权访问验证,IDOR验证,越权验证,批量验证接口,api endpoint validation,unauthorized access check,idor validation",
		Path:        "http/batch_do_http_request.yak",
		Content:     "print('hello')",
	}).Error)

	got, err := SearchAIYakToolBM25(db, &AIYakToolFilter{Keywords: []string{"测试/api/categories和/api/products/hot接口是否存在未授权访问和IDOR漏洞"}}, 10, 0)
	require.NoError(t, err)
	require.NotEmpty(t, got)
	require.Equal(t, "batch_do_http_request", got[0].Name)
}
