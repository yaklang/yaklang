package cve

import (
	"embed"
	"io"
	"os"
	"testing"

	"github.com/yaklang/yaklang/common/cve/cvequeryops"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed CIDate.db
var DbFs embed.FS

func TestMigrate(t *testing.T) {
	_migrateTable()
}

type productWithVersion struct {
	name    string
	version string
	target  string
}

func openEmbeddedCVEDatabase(t *testing.T) *cveresources.SqliteManager {
	t.Helper()

	DbFp, err := DbFs.Open("CIDate.db")
	if err != nil {
		t.Fatalf("open embedded cve db failed: %v", err)
	}
	defer DbFp.Close()

	tempFp, err := os.CreateTemp("", "Date.db")
	if err != nil {
		t.Fatalf("create temp cve db failed: %v", err)
	}
	t.Cleanup(func() {
		tempFp.Close()
		os.Remove(tempFp.Name())
	})

	if _, err = io.Copy(tempFp, DbFp); err != nil {
		t.Fatalf("copy embedded cve db failed: %v", err)
	}

	return cveresources.GetManager(tempFp.Name(), true)
}

func TestQueryCVEWithoutPagination(t *testing.T) {
	manager := openEmbeddedCVEDatabase(t)

	// MCP query_cve often omits pagination; this must not panic the engine.
	paging, data, err := cveresources.QueryCVE(manager.DB, &ypb.QueryCVERequest{
		Keywords: "apache",
	})
	if err != nil {
		t.Fatalf("QueryCVE without pagination failed: %v", err)
	}
	if paging == nil {
		t.Fatal("expected non-nil paginator")
	}
	if len(data) == 0 {
		t.Fatal("expected at least one CVE result")
	}
}

func TestQueryCVEWithFixName(t *testing.T) {
	data := []productWithVersion{
		{
			name:    "httpd", // 硬编码修复测试
			version: "2.4.49",
			target:  "CVE-2021-42013",
		},
		{
			name:    "apt2", // 产品名冗杂修复
			version: "0.7.5",
			target:  "CVE-2009-1358",
		},
		{
			name:    "python3-e", // 产品名冗杂修复
			version: "2.2",
			target:  "CVE-2006-1542",
		},
		{
			name:    "linux-2019",
			version: "9.0",
			target:  "CVE-2003-0780",
		},
	}

	M := openEmbeddedCVEDatabase(t)
	for _, datum := range data {
		cve := cvequeryops.QueryCVEYields(M.DB, cvequeryops.ProductWithVersion(datum.name, datum.version))
		count := 0
		for {
			flag := false
			select {
			case item, ok := <-cve:
				if !ok {
					flag = true
					break
				}
				if item.CVE != datum.target {
					panic("Mismatch: Redundant data: " + datum.name)
				} else {
					count++
				}
			}
			if flag {
				break
			}
		}
		if count < 1 {
			panic("Mismatch: Lack of data: " + datum.name)
		}
	}
}
