package cveresources

import (
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
)

//go:embed cvedata/cve_5_demo.json
var demoV5JSONForDB []byte

// 测试 5.0 Record 写入数据库并读取
func TestCVEV5DatabaseRoundTrip(t *testing.T) {
	// 创建内存数据库
	db, err := gorm.Open(consts.SQLite, ":memory:")
	if err != nil {
		t.Fatalf("open db failed: %v", err)
	}
	db.AutoMigrate(&CVE{})

	// 解析 5.0 record
	var record CVERecordV5
	err = json.Unmarshal(demoV5JSONForDB, &record)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// 转换并保存
	cve, err := record.ToCVE(db)
	if err != nil {
		t.Fatalf("ToCVE failed: %v", err)
	}

	// 通过 MergeCVEV5Fields 保存
	err = MergeCVEV5Fields(db, cve)
	if err != nil {
		t.Fatalf("MergeCVEV5Fields failed: %v", err)
	}

	// 从数据库读取
	var result CVE
	db.Where("cve = ?", "CVE-2026-86544").First(&result)

	if result.CVE != "CVE-2026-86544" {
		t.Errorf("Expected CVE-2026-86544, got %s", result.CVE)
	}
	if result.Title == "" {
		t.Error("Title should not be empty in DB")
	}
	t.Logf("DB Title: %s", result.Title)
	t.Logf("DB Vendor: %s", result.Vendor)
	t.Logf("DB Product: %s", result.Product)
	t.Logf("DB Severity: %s", result.Severity)

	// 验证 ToGPRCModel 包含 TitleOrigin
	grpcModel := result.ToGPRCModel()
	if grpcModel.TitleOrigin == "" {
		t.Error("TitleOrigin should not be empty in gRPC model")
	}
	t.Logf("gRPC TitleOrigin: %s", grpcModel.TitleOrigin)
}

// 测试 MergeCVEV5Fields 不覆盖已有数据
func TestMergeCVEV5FieldsNoOverwrite(t *testing.T) {
	db, err := gorm.Open(consts.SQLite, ":memory:")
	if err != nil {
		t.Fatalf("open db failed: %v", err)
	}
	db.AutoMigrate(&CVE{})

	// 先插入一条 NVD 数据，有 CPE/CVSS 但没有 title
	existing := &CVE{
		CVE:               "CVE-2026-86544",
		DescriptionMain:   "NVD description",
		Vendor:            "nvd-vendor",
		Product:           "nvd-product",
		CVSSVersion:       "3.1",
		BaseCVSSv2Score:   9.8,
		Severity:          "CRITICAL",
		CPEConfigurations: []byte(`{"nodes":[]}`),
	}
	db.Save(existing)

	// 解析 5.0 record
	var record CVERecordV5
	json.Unmarshal(demoV5JSONForDB, &record)
	v5cve, _ := record.ToCVE(db)

	// 合并
	err = MergeCVEV5Fields(db, v5cve)
	if err != nil {
		t.Fatalf("MergeCVEV5Fields failed: %v", err)
	}

	// 读取并验证
	var result CVE
	db.Where("cve = ?", "CVE-2026-86544").First(&result)

	// title 应该被 5.0 补充
	if result.Title == "" {
		t.Error("Title should be filled by V5")
	}
	t.Logf("Title (from V5): %s", result.Title)

	// vendor/product 不应该被覆盖（NVD 已有值）
	if result.Vendor != "nvd-vendor" {
		t.Errorf("Vendor should not be overwritten, expected 'nvd-vendor', got '%s'", result.Vendor)
	}
	if result.Product != "nvd-product" {
		t.Errorf("Product should not be overwritten, expected 'nvd-product', got '%s'", result.Product)
	}

	// CVSS 不应该被覆盖
	if result.Severity != "CRITICAL" {
		t.Errorf("Severity should not be overwritten, expected 'CRITICAL', got '%s'", result.Severity)
	}
	if result.BaseCVSSv2Score != 9.8 {
		t.Errorf("Score should not be overwritten, expected 9.8, got %f", result.BaseCVSSv2Score)
	}
}
