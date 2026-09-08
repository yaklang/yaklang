package cvequeryops

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cve/cveresources"
)

// generateBenchData 生成 benchmark 数据文件，返回文件路径
func generateBenchData(t testing.TB, count int) string {
	// 读取 demo JSON
	demoPath := filepath.Join("..", "cveresources", "cvedata", "cve_2_demo.json")
	demoData, err := ioutil.ReadFile(demoPath)
	if err != nil {
		t.Fatalf("read demo json failed: %v", err)
	}

	var demoFile cveresources.CVEYearFileV2
	if err := json.Unmarshal(demoData, &demoFile); err != nil {
		t.Fatalf("unmarshal demo failed: %v", err)
	}

	vulns := demoFile.Vulnerabilities
	expanded := make([]cveresources.CVEVulnerability, count)
	for i := 0; i < count; i++ {
		v := vulns[i%len(vulns)]
		v.Cve.ID = fmt.Sprintf("CVE-BENCH-%05d", i)
		expanded[i] = v
	}
	demoFile.Vulnerabilities = expanded

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "CVE-BENCH.json")
	data, _ := json.Marshal(demoFile)
	if err := ioutil.WriteFile(filePath, data, 0o644); err != nil {
		t.Fatalf("write bench file failed: %v", err)
	}
	return filePath
}

// setupBenchDB 创建临时 CVE 数据库用于 benchmark
func setupBenchDB(t testing.TB) (*gorm.DB, string) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "bench-cve.db")
	db, err := consts.CreateCVEDatabase(dbPath)
	if err != nil {
		t.Fatalf("create db failed: %v", err)
	}
	return db, dbPath
}

// BenchmarkLoadCVEByFileName 测试批量加载的性能
func BenchmarkLoadCVEByFileName(b *testing.B) {
	filePath := generateBenchData(b, 20000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		db, _ := setupBenchDB(b)
		manager := &cveresources.SqliteManager{DB: db}
		b.StartTimer()

		_, err := LoadCVEByFileName(filePath, manager)
		if err != nil {
			b.Fatalf("LoadCVEByFileName failed: %v", err)
		}

		b.StopTimer()
		db.Close()
	}
}

// BenchmarkSaveCVEVulnerabilitySingle 测试旧的逐条写入性能（作为对比基准）
func BenchmarkSaveCVEVulnerabilitySingle(b *testing.B) {
	filePath := generateBenchData(b, 20000)
	data, _ := ioutil.ReadFile(filePath)
	var cveFileV2 cveresources.CVEYearFileV2
	json.Unmarshal(data, &cveFileV2)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		db, _ := setupBenchDB(b)
		manager := &cveresources.SqliteManager{DB: db}
		b.StartTimer()

		for _, vuln := range cveFileV2.Vulnerabilities {
			manager.SaveCVEVulnerability(&vuln)
		}

		b.StopTimer()
		db.Close()
	}
}

// TestLoadBenchVerify 验证批量加载的数据正确性
func TestLoadBenchVerify(t *testing.T) {
	filePath := generateBenchData(t, 500)
	db, dbPath := setupBenchDB(t)
	defer os.Remove(dbPath)
	defer db.Close()

	manager := &cveresources.SqliteManager{DB: db}
	_, err := LoadCVEByFileName(filePath, manager)
	if err != nil {
		t.Fatalf("LoadCVEByFileName failed: %v", err)
	}

	var count int64
	db.Model(&cveresources.CVE{}).Count(&count)
	if count != 500 {
		t.Errorf("Expected 500 CVEs, got %d", count)
	}

	var productCount int64
	db.Model(&cveresources.ProductsTable{}).Count(&productCount)
	t.Logf("ProductsTable count: %d (deduplicated)", productCount)

	var cve cveresources.CVE
	db.Where("cve = ?", "CVE-BENCH-00000").First(&cve)
	if cve.CVE == "" {
		t.Error("Should find CVE-BENCH-00000")
	}
	t.Logf("Sample CVE: %s, vendor=%s, product=%s", cve.CVE, cve.Vendor, cve.Product)
}

// TestBenchComparison 对比测试：逐条 vs 批量
func TestBenchComparison(t *testing.T) {
	filePath := generateBenchData(t, 20000)
	data, _ := ioutil.ReadFile(filePath)
	var cveFileV2 cveresources.CVEYearFileV2
	json.Unmarshal(data, &cveFileV2)

	// 方式1：逐条写入（旧方式）
	db1, dbPath1 := setupBenchDB(t)
	manager1 := &cveresources.SqliteManager{DB: db1}
	start1 := time.Now()
	for _, vuln := range cveFileV2.Vulnerabilities {
		manager1.SaveCVEVulnerability(&vuln)
	}
	elapsed1 := time.Since(start1)
	var count1 int64
	db1.Model(&cveresources.CVE{}).Count(&count1)
	db1.Close()
	os.Remove(dbPath1)

	// 方式2：批量写入（新方式）
	db2, dbPath2 := setupBenchDB(t)
	manager2 := &cveresources.SqliteManager{DB: db2}
	start2 := time.Now()
	_, err := LoadCVEByFileName(filePath, manager2)
	if err != nil {
		t.Fatalf("LoadCVEByFileName failed: %v", err)
	}
	elapsed2 := time.Since(start2)
	var count2 int64
	db2.Model(&cveresources.CVE{}).Count(&count2)
	db2.Close()
	os.Remove(dbPath2)

	if count1 != count2 {
		t.Errorf("CVE count mismatch: single=%d, batch=%d", count1, count2)
	}

	fmt.Printf("\n=== Benchmark 结果 (%d CVEs) ===\n", count1)
	fmt.Printf("逐条写入 (旧): %v\n", elapsed1)
	fmt.Printf("批量写入 (新): %v\n", elapsed2)
	fmt.Printf("加速比: %.1fx\n", float64(elapsed1)/float64(elapsed2))
}
