package cvequeryops

import (
	"testing"
)

// 测试通过 cveawg API 实时获取 CVE 5.0 Record
func TestFetchCVEV5Record(t *testing.T) {
	record, err := FetchCVEV5Record("CVE-2026-86544")
	if err != nil {
		t.Fatalf("FetchCVEV5Record failed: %v", err)
	}

	if record.CVEId() != "CVE-2026-86544" {
		t.Errorf("Expected CVE-2026-86544, got %s", record.CVEId())
	}

	if record.Containers.CNA == nil {
		t.Fatal("CNA container should not be nil")
	}

	if record.Containers.CNA.Title == "" {
		t.Error("Title should not be empty for CVE-2026-86544")
	}
	t.Logf("Title: %s", record.Containers.CNA.Title)

	vendors, products := record.ExtractVendorProductExported()
	t.Logf("Vendors: %v, Products: %v", vendors, products)
}

// 测试获取老 CVE
func TestFetchCVEV5RecordOld(t *testing.T) {
	record, err := FetchCVEV5Record("CVE-2017-0144")
	if err != nil {
		t.Fatalf("FetchCVEV5Record failed: %v", err)
	}

	if record.CVEId() != "CVE-2017-0144" {
		t.Errorf("Expected CVE-2017-0144, got %s", record.CVEId())
	}

	// 老 CVE 没有 title
	if record.Containers.CNA != nil {
		t.Logf("Title (should be empty for old): '%s'", record.Containers.CNA.Title)
		vendors, products := record.ExtractVendorProductExported()
		t.Logf("Vendors: %v, Products: %v", vendors, products)
	}
}
