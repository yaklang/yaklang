package cveresources

import (
	_ "embed"
	"encoding/json"
	"testing"
	"time"
)

//go:embed cvedata/cve_2_demo.json
var demoJSON []byte

// 测试CVE 2.0格式解析
func TestCVE2FormatParsing(t *testing.T) {
	// 使用嵌入的demo.json文件进行真实数据测试
	// 测试解析真实的CVE 2.0数据
	var cveData CVEYearFileV2
	err := json.Unmarshal(demoJSON, &cveData)
	if err != nil {
		t.Fatalf("Failed to parse CVE 2.0 format: %v", err)
	}

	// 验证基本字段
	if cveData.Version != "2.0" {
		t.Errorf("Expected version 2.0, got %s", cveData.Version)
	}

	if cveData.Format != "NVD_CVE" {
		t.Errorf("Expected format NVD_CVE, got %s", cveData.Format)
	}

	if len(cveData.Vulnerabilities) == 0 {
		t.Fatalf("Expected at least 1 vulnerability, got %d", len(cveData.Vulnerabilities))
	}

	// 测试第一个CVE
	vuln := cveData.Vulnerabilities[0]

	// 测试CVE ID提取
	cveId := vuln.CVEId()
	if cveId == "" {
		t.Error("CVE ID should not be empty")
	}
	t.Logf("CVE ID: %s", cveId)

	// 测试CWE提取
	cwe := vuln.CWE()
	t.Logf("CWE: %s", cwe)

	// 测试描述提取
	desc := vuln.DescriptionMain()
	if desc == "" {
		t.Error("Description should not be empty")
	}
	t.Logf("Description length: %d", len(desc))

	// 测试日期解析
	publishedDate := vuln.GetPublishedDate()
	if publishedDate.IsZero() {
		t.Error("Published date should not be zero")
	}
	t.Logf("Published date: %s", publishedDate)

	lastModifiedDate := vuln.GetLastModifiedDate()
	if lastModifiedDate.IsZero() {
		t.Error("Last modified date should not be zero")
	}
	t.Logf("Last modified date: %s", lastModifiedDate)

	// 测试CVE Tags（应该是数组）
	if vuln.Cve.CveTags == nil {
		t.Error("CVE Tags should not be nil")
	}
	t.Logf("CVE Tags count: %d", len(vuln.Cve.CveTags))

	// 测试CVSS评分系统
	metrics := vuln.Cve.Metrics
	hasAnyMetric := false

	if len(metrics.CvssMetricV40) > 0 {
		hasAnyMetric = true
		t.Logf("CVSS v4.0 score: %.1f (%s)",
			metrics.CvssMetricV40[0].CvssData.BaseScore,
			metrics.CvssMetricV40[0].CvssData.BaseSeverity)
	}

	if len(metrics.CvssMetricV31) > 0 {
		hasAnyMetric = true
		t.Logf("CVSS v3.1 score: %.1f (%s)",
			metrics.CvssMetricV31[0].CvssData.BaseScore,
			metrics.CvssMetricV31[0].CvssData.BaseSeverity)
	}

	if len(metrics.CvssMetricV30) > 0 {
		hasAnyMetric = true
		t.Logf("CVSS v3.0 score: %.1f (%s)",
			metrics.CvssMetricV30[0].CvssData.BaseScore,
			metrics.CvssMetricV30[0].CvssData.BaseSeverity)
	}

	if len(metrics.CvssMetricV2) > 0 {
		hasAnyMetric = true
		t.Logf("CVSS v2 score: %.1f (%s)",
			metrics.CvssMetricV2[0].CvssData.BaseScore,
			metrics.CvssMetricV2[0].BaseSeverity)
	}

	if !hasAnyMetric {
		t.Log("No CVSS metrics found for this CVE")
	}

	// 测试转换为数据库记录
	cveRecord, err := vuln.ToCVE(nil)
	if err != nil {
		// 只有"REJECT"CVE才应该返回错误
		if err.Error() != "REJECT" {
			t.Errorf("Unexpected error converting to CVE record: %v", err)
		}
		t.Log("CVE was rejected (expected for some CVEs)")
	} else {
		if cveRecord == nil {
			t.Error("CVE record should not be nil")
		} else {
			t.Logf("Successfully converted to CVE record: %s", cveRecord.CVE)
		}
	}

	// 统计不同CVSS版本的使用情况
	var v40Count, v31Count, v30Count, v2Count int
	for _, v := range cveData.Vulnerabilities {
		if len(v.Cve.Metrics.CvssMetricV40) > 0 {
			v40Count++
		}
		if len(v.Cve.Metrics.CvssMetricV31) > 0 {
			v31Count++
		}
		if len(v.Cve.Metrics.CvssMetricV30) > 0 {
			v30Count++
		}
		if len(v.Cve.Metrics.CvssMetricV2) > 0 {
			v2Count++
		}
	}

	t.Logf("CVSS version usage - v4.0: %d, v3.1: %d, v3.0: %d, v2.0: %d",
		v40Count, v31Count, v30Count, v2Count)
}

// 测试时间格式解析
func TestTimeFormatParsing(t *testing.T) {
	vuln := &CVEVulnerability{
		Cve: CVE2Data{
			Published:    "2023-01-01T12:34:56.000",
			LastModified: "2023-01-02T12:34:56.000",
		},
	}

	publishedDate := vuln.GetPublishedDate()
	expectedTime := time.Date(2023, 1, 1, 12, 34, 56, 0, time.UTC)

	if !publishedDate.Equal(expectedTime) {
		t.Errorf("Expected %v, got %v", expectedTime, publishedDate)
	}
}

// 测试CWE提取逻辑
func TestCWEExtraction(t *testing.T) {
	vuln := &CVEVulnerability{
		Cve: CVE2Data{
			Weaknesses: []CVE2Weakness{
				{
					Description: []CVE2Description{
						{Lang: "en", Value: "CWE-79"},
						{Lang: "en", Value: "CWE-89"},
					},
				},
				{
					Description: []CVE2Description{
						{Lang: "en", Value: "NVD-CWE-Other"},
					},
				},
			},
		},
	}

	cwe := vuln.CWE()
	if cwe != "CWE-79 | CWE-89" {
		t.Errorf("Expected 'CWE-79 | CWE-89', got '%s'", cwe)
	}
}
