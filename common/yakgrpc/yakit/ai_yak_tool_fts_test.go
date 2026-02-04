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

	got, err := SearchAIYakToolBM25(db, &AIYakToolFilter{Keywords: "tcp"}, 10, 0)
	require.NoError(t, err)
	require.NotEmpty(t, got)
	require.Equal(t, "tcp_scan", got[0].Name)
}
