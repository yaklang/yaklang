package crep

import (
	"testing"
)

func TestSNIResolver(t *testing.T) {
	tests := []struct {
		name         string
		mapping      map[string]string
		overwriteSNI bool
		forceSNI     string
		host         string
		expected     *string
	}{
		{
			name: "精确匹配",
			mapping: map[string]string{
				"example.com": "cdn.example.com",
			},
			host:     "example.com",
			expected: stringPtr("cdn.example.com"),
		},
		{
			name: "通配符匹配",
			mapping: map[string]string{
				"*.example.com": "cdn.example.com",
			},
			host:     "www.example.com",
			expected: stringPtr("cdn.example.com"),
		},
		{
			name: "未匹配返回nil",
			mapping: map[string]string{
				"example.com": "cdn.example.com",
			},
			host:     "other.com",
			expected: nil,
		},
		{
			name: "映射优先于强制模式",
			mapping: map[string]string{
				"example.com": "cdn.example.com",
			},
			overwriteSNI: true,
			forceSNI:     "forced.cdn.com",
			host:         "example.com",
			expected:     stringPtr("cdn.example.com"),
		},
		{
			name: "强制模式作为默认值",
			mapping: map[string]string{
				"example.com": "cdn.example.com",
			},
			overwriteSNI: true,
			forceSNI:     "forced.cdn.com",
			host:         "other.com",
			expected:     stringPtr("forced.cdn.com"),
		},
		{
			name: "Glob映射优先于强制模式",
			mapping: map[string]string{
				"*.example.com": "cdn.example.com",
			},
			overwriteSNI: true,
			forceSNI:     "forced.cdn.com",
			host:         "www.example.com",
			expected:     stringPtr("cdn.example.com"),
		},
		{
			name: "空SNI映射",
			mapping: map[string]string{
				"nosni.com": "",
			},
			host:     "nosni.com",
			expected: stringPtr(""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := NewSNIResolver(tt.mapping, tt.overwriteSNI, tt.forceSNI)
			result := resolver.Resolve(tt.host)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("期望 nil，但得到 %q", *result)
				}
			} else {
				if result == nil {
					t.Errorf("期望 %q，但得到 nil", *tt.expected)
				} else if *result != *tt.expected {
					t.Errorf("期望 %q，但得到 %q", *tt.expected, *result)
				}
			}
		})
	}
}

func stringPtr(s string) *string {
	return &s
}
