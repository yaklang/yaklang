package cveresources

import (
	_ "embed"
	"encoding/json"
	"testing"
	"time"
)

//go:embed cvedata/cve_5_demo.json
var demoV5JSON []byte

//go:embed cvedata/cve_5_demo_old.json
var demoV5OldJSON []byte

// 测试 CVE 5.0 格式解析 - 新 CVE (有 title)
func TestCVE5FormatParsing(t *testing.T) {
	var record CVERecordV5
	err := json.Unmarshal(demoV5JSON, &record)
	if err != nil {
		t.Fatalf("Failed to parse CVE 5.0 format: %v", err)
	}

	// 验证基本字段
	if record.DataType != "CVE_RECORD" {
		t.Errorf("Expected dataType CVE_RECORD, got %s", record.DataType)
	}

	if record.CVEMetadata.CVEID != "CVE-2026-86544" {
		t.Errorf("Expected CVE-2026-86544, got %s", record.CVEMetadata.CVEID)
	}

	if record.CVEMetadata.State != "PUBLISHED" {
		t.Errorf("Expected state PUBLISHED, got %s", record.CVEMetadata.State)
	}

	// 验证 CNA 容器存在
	if record.Containers.CNA == nil {
		t.Fatal("CNA container should not be nil")
	}

	// 验证 title
	title := record.extractTitle()
	if title == "" {
		t.Error("Title should not be empty for CVE-2026-86544")
	}
	t.Logf("Title: %s", title)

	// 验证描述
	desc := record.DescriptionMain()
	if desc == "" {
		t.Error("Description should not be empty")
	}
	t.Logf("Description length: %d", len(desc))

	// 验证 CWE
	cwe := record.CWE()
	t.Logf("CWE: %s", cwe)
	if cwe == "" {
		t.Log("CWE is empty (some CVEs may not have CWE)")
	}

	// 验证 affected (vendor/product)
	vendors, products := record.extractVendorProduct()
	if len(vendors) == 0 {
		t.Error("Vendors should not be empty")
	}
	if len(products) == 0 {
		t.Error("Products should not be empty")
	}
	t.Logf("Vendors: %v", vendors)
	t.Logf("Products: %v", products)

	// 验证 CVSS
	cvssVersion, vectorString, cvss := record.extractCVSS()
	if cvss == nil {
		t.Error("CVSS data should not be nil")
	}
	t.Logf("CVSS version: %s, vector: %s, score: %.1f, severity: %s",
		cvssVersion, vectorString, cvss.baseScore, cvss.baseSeverity)

	// 验证日期
	pubDate := record.GetPublishedDate()
	if pubDate.IsZero() {
		t.Error("Published date should not be zero")
	}
	t.Logf("Published date: %s", pubDate)

	// 测试转换为数据库记录
	cveRecord, err := record.ToCVE(nil)
	if err != nil {
		t.Fatalf("Unexpected error converting to CVE record: %v", err)
	}
	if cveRecord == nil {
		t.Fatal("CVE record should not be nil")
	}

	// 验证转换后的字段
	if cveRecord.CVE != "CVE-2026-86544" {
		t.Errorf("Expected CVE-2026-86544, got %s", cveRecord.CVE)
	}
	if cveRecord.Title == "" {
		t.Error("Title should not be empty after ToCVE")
	}
	t.Logf("CVE record Title: %s", cveRecord.Title)
	t.Logf("CVE record Vendor: %s", cveRecord.Vendor)
	t.Logf("CVE record Product: %s", cveRecord.Product)
	t.Logf("CVE record Solution: %s", cveRecord.Solution)
	t.Logf("CVE record Severity: %s", cveRecord.Severity)
}

// 测试老 CVE (无 title) 的解析
func TestCVE5OldFormatParsing(t *testing.T) {
	var record CVERecordV5
	err := json.Unmarshal(demoV5OldJSON, &record)
	if err != nil {
		t.Fatalf("Failed to parse CVE 5.0 format: %v", err)
	}

	if record.CVEMetadata.CVEID != "CVE-2017-0144" {
		t.Errorf("Expected CVE-2017-0144, got %s", record.CVEMetadata.CVEID)
	}

	// 老 CVE 没有 title
	title := record.extractTitle()
	if title != "" {
		t.Errorf("Expected empty title for old CVE, got %s", title)
	}
	t.Log("Title is empty as expected for old CVE")

	// 老 CVE 应该有 affected
	vendors, products := record.extractVendorProduct()
	t.Logf("Vendors: %v", vendors)
	t.Logf("Products: %v", products)
	if len(vendors) == 0 {
		t.Error("Vendors should not be empty even for old CVE")
	}

	// 描述应该存在
	desc := record.DescriptionMain()
	if desc == "" {
		t.Error("Description should not be empty")
	}
	t.Logf("Description length: %d", len(desc))

	// 转换为数据库记录
	cveRecord, err := record.ToCVE(nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if cveRecord.Title != "" {
		t.Errorf("Expected empty title, got %s", cveRecord.Title)
	}
	t.Logf("CVE record (old) Vendor: %s, Product: %s", cveRecord.Vendor, cveRecord.Product)
}

// 测试时间格式解析
func TestCVE5TimeFormatParsing(t *testing.T) {
	record := &CVERecordV5{
		CVEMetadata: CVE5Metadata{
			DatePublished: "2026-09-07T23:03:21.458Z",
			DateUpdated:   "2026-09-07T23:03:21.458Z",
		},
	}

	pubDate := record.GetPublishedDate()
	expectedTime := time.Date(2026, 9, 7, 23, 3, 21, 458000000, time.UTC)
	if !pubDate.Equal(expectedTime) {
		t.Errorf("Expected %v, got %v", expectedTime, pubDate)
	}
}

// 测试 CWE 提取
func TestCVE5CWEExtraction(t *testing.T) {
	record := &CVERecordV5{
		Containers: CVE5Containers{
			CNA: &CVE5CNA{
				ProblemTypes: []CVE5ProblemType{
					{
						Descriptions: []CVE5ProblemTypeDesc{
							{Lang: "en", Description: "Incorrect Authorization", CWEId: "CWE-863", Type: "CWE"},
						},
					},
					{
						Descriptions: []CVE5ProblemTypeDesc{
							{Lang: "en", Description: "Some text description", Type: "text"},
						},
					},
				},
			},
		},
	}

	cwe := record.CWE()
	if cwe != "CWE-863" {
		t.Errorf("Expected 'CWE-863', got '%s'", cwe)
	}
}

// 测试 solution 提取
func TestCVE5SolutionExtraction(t *testing.T) {
	record := &CVERecordV5{
		Containers: CVE5Containers{
			CNA: &CVE5CNA{
				Solutions: []CVE5Solution{
					{Lang: "en", Value: "Upgrade to version 0.30.0 or later"},
					{Lang: "zh", Value: "升级到 0.30.0 或更高版本"},
				},
			},
		},
	}

	solution := record.extractSolution()
	if solution != "Upgrade to version 0.30.0 or later" {
		t.Errorf("Expected English solution, got '%s'", solution)
	}
}

// 测试无 solution 的情况
func TestCVE5NoSolution(t *testing.T) {
	record := &CVERecordV5{
		Containers: CVE5Containers{
			CNA: &CVE5CNA{
				Solutions: nil,
			},
		},
	}

	solution := record.extractSolution()
	if solution != "" {
		t.Errorf("Expected empty solution, got '%s'", solution)
	}
}
