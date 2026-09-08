package cvequeryops

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/cve/cveresources"
)

// 分阶段 profiling：JSON 解析 vs ToCVE 转换 vs DB 写入
func TestBenchBreakdown(t *testing.T) {
	filePath := generateBenchData(t, 20000)
	benchData, _ := ioutil.ReadFile(filePath)

	// 1. JSON 解析时间
	start := time.Now()
	var cveFileV2 cveresources.CVEYearFileV2
	err := json.Unmarshal(benchData, &cveFileV2)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	jsonTime := time.Since(start)
	t.Logf("JSON 解析: %v (%d 条)", jsonTime, len(cveFileV2.Vulnerabilities))

	// 2. ToCVE 转换时间（不含 DB）
	start = time.Now()
	cves := make([]*cveresources.CVE, 0, len(cveFileV2.Vulnerabilities))
	for _, vuln := range cveFileV2.Vulnerabilities {
		c, err := vuln.ToCVE(nil)
		if err != nil || c == nil {
			continue
		}
		cves = append(cves, c)
	}
	toCveTime := time.Since(start)
	t.Logf("ToCVE 转换: %v (%d 条)", toCveTime, len(cves))

	// 3. DB 写入时间（批量）
	db, _ := setupBenchDB(t)
	start = time.Now()
	for i := 0; i < len(cves); i += 500 {
		endIdx := i + 500
		if endIdx > len(cves) {
			endIdx = len(cves)
		}
		tx := db.Begin()
		for _, c := range cves[i:endIdx] {
			tx.Save(c)
		}
		tx.Commit()
	}
	batchWriteTime := time.Since(start)
	t.Logf("DB 批量写入: %v", batchWriteTime)
	db.Close()

	// 4. DB 写入时间（逐条，对比）
	db2, _ := setupBenchDB(t)
	start = time.Now()
	for _, c := range cves {
		db2.Save(c)
	}
	singleWriteTime := time.Since(start)
	t.Logf("DB 逐条写入: %v", singleWriteTime)
	db2.Close()

	// 5. ProductsTable 对比
	db3, _ := setupBenchDB(t)
	productSet := make(map[string]struct{})
	var products []cveresources.ProductsTable
	for _, c := range cves {
		for _, vendor := range cveresources.Set(splitStr(c.Vendor)) {
			for _, product := range cveresources.Set(splitStr(c.Product)) {
				if vendor == "" || product == "" {
					continue
				}
				key := vendor + "/" + product
				if _, exists := productSet[key]; !exists {
					productSet[key] = struct{}{}
					products = append(products, cveresources.ProductsTable{Product: product, Vendor: vendor})
				}
			}
		}
	}
	t.Logf("ProductsTable 去重后: %d 条 (原始约 %d 条)", len(products), len(cves)*3)

	start = time.Now()
	for i := 0; i < len(products); i += 500 {
		endIdx := i + 500
		if endIdx > len(products) {
			endIdx = len(products)
		}
		db3.CreateInBatches(products[i:endIdx], 500)
	}
	batchProductTime := time.Since(start)
	t.Logf("ProductsTable 批量写入: %v", batchProductTime)
	db3.Close()

	// 逐条 ProductsTable
	db4, _ := setupBenchDB(t)
	start = time.Now()
	for _, c := range cves {
		for _, vendor := range cveresources.Set(splitStr(c.Vendor)) {
			for _, product := range cveresources.Set(splitStr(c.Product)) {
				if vendor == "" || product == "" {
					continue
				}
				db4.Save(cveresources.ProductsTable{Product: product, Vendor: vendor})
			}
		}
	}
	singleProductTime := time.Since(start)
	t.Logf("ProductsTable 逐条写入: %v", singleProductTime)
	db4.Close()

	fmt.Printf("\n=== 分阶段 Profiling (%d CVEs) ===\n", len(cves))
	fmt.Printf("JSON 解析:          %v\n", jsonTime)
	fmt.Printf("ToCVE 转换:         %v\n", toCveTime)
	fmt.Printf("DB 批量写入:        %v\n", batchWriteTime)
	fmt.Printf("DB 逐条写入:        %v  (%.1fx slower)\n", singleWriteTime, float64(singleWriteTime)/float64(batchWriteTime))
	fmt.Printf("ProductsTable 批量: %v\n", batchProductTime)
	fmt.Printf("ProductsTable 逐条: %v  (%.1fx slower)\n", singleProductTime, float64(singleProductTime)/float64(batchProductTime))
	fmt.Printf("\n总时间(批量): ~%v\n", jsonTime+toCveTime+batchWriteTime+batchProductTime)
	fmt.Printf("总时间(逐条): ~%v\n", jsonTime+toCveTime+singleWriteTime+singleProductTime)
}

func splitStr(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	current := ""
	for _, c := range s {
		if c == ',' {
			if current != "" {
				result = append(result, current)
			}
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

// 确保引用 filepath 避免unused import
var _ = filepath.Join
