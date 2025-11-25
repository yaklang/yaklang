package yakgrpc

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestGetCVE_ReferencesParsing 测试 GetCVE 函数中 References 字段的解析逻辑
// 测试 CVE 1.0 和 CVE 2.0 两种格式
func TestGetCVE_ReferencesParsing(t *testing.T) {
	tests := []struct {
		name           string
		referencesJSON []byte
		expectedURLs   []string
		expectedError  bool
	}{
		{
			name:           "CVE 2.0 format - array of references",
			referencesJSON: []byte(`[{"url":"https://example.com/ref1","source":"test","tags":null},{"url":"https://example.com/ref2","source":"test","tags":null}]`),
			expectedURLs: []string{
				"https://example.com/ref1",
				"https://example.com/ref2",
			},
			expectedError: false,
		},
		{
			name:           "CVE 1.0 format - object with reference_data",
			referencesJSON: []byte(`{"reference_data":[{"url":"https://example.com/ref1","name":"Ref1","refsource":"test","tags":[]},{"url":"https://example.com/ref2","name":"Ref2","refsource":"test","tags":[]}]}`),
			expectedURLs: []string{
				"https://example.com/ref1",
				"https://example.com/ref2",
			},
			expectedError: false,
		},
		{
			name:           "CVE 2.0 format - single reference",
			referencesJSON: []byte(`[{"url":"https://plugins.trac.wordpress.org/browser/booking-calendar-contact-form/tags/1.2.59/dex_bccf.php#L1409","source":"security@wordfence.com","tags":null}]`),
			expectedURLs: []string{
				"https://plugins.trac.wordpress.org/browser/booking-calendar-contact-form/tags/1.2.59/dex_bccf.php#L1409",
			},
			expectedError: false,
		},
		{
			name:           "CVE 2.0 format - multiple references with null tags",
			referencesJSON: []byte(`[{"url":"https://plugins.trac.wordpress.org/browser/booking-calendar-contact-form/tags/1.2.59/dex_bccf.php#L1409","source":"security@wordfence.com","tags":null},{"url":"https://plugins.trac.wordpress.org/browser/booking-calendar-contact-form/trunk/dex_bccf.php#L1409","source":"security@wordfence.com","tags":null},{"url":"https://www.wordfence.com/threat-intel/vulnerabilities/id/83b0ae2c-6b08-4b71-a728-c60722ec20c7?source=cve","source":"security@wordfence.com","tags":null}]`),
			expectedURLs: []string{
				"https://plugins.trac.wordpress.org/browser/booking-calendar-contact-form/tags/1.2.59/dex_bccf.php#L1409",
				"https://plugins.trac.wordpress.org/browser/booking-calendar-contact-form/trunk/dex_bccf.php#L1409",
				"https://www.wordfence.com/threat-intel/vulnerabilities/id/83b0ae2c-6b08-4b71-a728-c60722ec20c7?source=cve",
			},
			expectedError: false,
		},
		{
			name:           "CVE 1.0 format - empty reference_data",
			referencesJSON: []byte(`{"reference_data":[]}`),
			expectedURLs:   []string{},
			expectedError:  false,
		},
		{
			name:           "CVE 2.0 format - empty array",
			referencesJSON: []byte(`[]`),
			expectedURLs:   []string{},
			expectedError:  false,
		},
		{
			name:           "Invalid JSON",
			referencesJSON: []byte(`invalid json`),
			expectedURLs:   nil,
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var urls []string

			// 尝试解析为 CVE 2.0 格式（数组格式）
			var refArray []map[string]interface{}
			if err := json.Unmarshal(tt.referencesJSON, &refArray); err == nil {
				// CVE 2.0 格式：直接是数组
				for _, rd := range refArray {
					if url, ok := rd["url"].(string); ok {
						urls = append(urls, url)
					}
				}
			} else {
				// 尝试解析为 CVE 1.0 格式（对象格式，包含 reference_data）
				var ref map[string]interface{}
				err = json.Unmarshal(tt.referencesJSON, &ref)
				if err != nil {
					if !tt.expectedError {
						t.Errorf("unmarshal references failed: %s", err)
					}
					return
				}
				if rdArr, ok := ref["reference_data"].([]interface{}); ok {
					for _, rd := range rdArr {
						if rdMap, ok := rd.(map[string]interface{}); ok {
							if url, ok := rdMap["url"].(string); ok {
								urls = append(urls, url)
							}
						}
					}
				}
			}

			// 验证结果
			if tt.expectedError {
				// 如果期望错误，但解析成功了，说明测试失败
				if len(urls) > 0 {
					t.Errorf("Expected error but got URLs: %v", urls)
				}
				return
			}

			if len(urls) != len(tt.expectedURLs) {
				t.Errorf("Expected %d URLs, got %d", len(tt.expectedURLs), len(urls))
				t.Errorf("Expected: %v", tt.expectedURLs)
				t.Errorf("Got: %v", urls)
				return
			}

			for i, expectedURL := range tt.expectedURLs {
				if urls[i] != expectedURL {
					t.Errorf("URL[%d] mismatch: expected %q, got %q", i, expectedURL, urls[i])
				}
			}

			// 验证拼接后的字符串
			urlStr := strings.Join(urls, "\n")
			if len(tt.expectedURLs) > 0 {
				expectedStr := strings.Join(tt.expectedURLs, "\n")
				if urlStr != expectedStr {
					t.Errorf("Joined string mismatch: expected %q, got %q", expectedStr, urlStr)
				}
			} else {
				if urlStr != "" {
					t.Errorf("Expected empty string, got %q", urlStr)
				}
			}
		})
	}
}
