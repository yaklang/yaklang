package standards

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewRuleMetadataEnricher(t *testing.T) {
	enricher, err := NewRuleMetadataEnricher()
	assert.NoError(t, err)
	assert.NotNil(t, enricher)
	assert.NotNil(t, enricher.mappings)
}

func TestEnrichGroupNames(t *testing.T) {
	enricher, err := NewRuleMetadataEnricher()
	assert.NoError(t, err)

	tests := []struct {
		name         string
		ruleName     string
		filePath     string
		cwes         []string
		expectGroups []string
	}{
		{
			name:     "SQL Injection - OWASP mapping",
			ruleName: "java-sql-injection",
			filePath: "buildin/java/cwe-89-sql-injection/java-sql-injection.sf",
			cwes:     []string{"CWE-89"},
			expectGroups: []string{
				"OWASP 2021 A03:Injection",
				"Language Library - Java",
			},
		},
		{
			name:     "XSS - Multiple OWASP categories",
			ruleName: "php-xss",
			filePath: "buildin/php/cwe-79-xss/php-xss.sf",
			cwes:     []string{"CWE-79"},
			expectGroups: []string{
				"OWASP 2021 A03:Injection",
				"Language Library - PHP",
			},
		},
		{
			name:     "Deserialization - OWASP A08",
			ruleName: "php-unserialize",
			filePath: "buildin/php/cwe-502-unserialize/php-unserialize.sf",
			cwes:     []string{"CWE-502"},
			expectGroups: []string{
				"OWASP 2021 A08:Software and Data Integrity Failures",
				"Language Library - PHP",
			},
		},
		{
			name:     "Framework specific - Shiro",
			ruleName: "shiro-deserialization",
			filePath: "buildin/java/components/shiro/shiro-deserialization.sf",
			cwes:     []string{"CWE-502"},
			expectGroups: []string{
				"OWASP 2021 A08:Software and Data Integrity Failures",
				"Framework - Apache Shiro",
			},
		},
		{
			name:     "CWE Top 25",
			ruleName: "csrf-detection",
			filePath: "buildin/java/cwe-352-csrf/csrf.sf",
			cwes:     []string{"CWE-352"},
			expectGroups: []string{
				"OWASP 2021 A01:Broken Access Control",
				"CWE Top 25 (2023)",
				"Language Library - Java",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups := enricher.EnrichGroupNames(tt.ruleName, tt.filePath, tt.cwes)
			
			// 检查预期的分组是否都存在
			for _, expected := range tt.expectGroups {
				assert.Contains(t, groups, expected, "Expected group %s not found", expected)
			}
		})
	}
}

func TestGetCWEName(t *testing.T) {
	enricher, err := NewRuleMetadataEnricher()
	assert.NoError(t, err)

	tests := []struct {
		cwe      string
		expected string
	}{
		{"CWE-89", "SQL Injection"},
		{"CWE-79", "Cross-site Scripting (XSS)"},
		{"CWE-502", "Deserialization of Untrusted Data"},
		{"cwe-89", "SQL Injection"}, // 测试格式化
		{"89", "SQL Injection"},     // 测试格式化
	}

	for _, tt := range tests {
		t.Run(tt.cwe, func(t *testing.T) {
			name := enricher.GetCWEName(tt.cwe)
			assert.Equal(t, tt.expected, name)
		})
	}
}

func TestGetOWASPByCWE(t *testing.T) {
	enricher, err := NewRuleMetadataEnricher()
	assert.NoError(t, err)

	tests := []struct {
		cwe      string
		expected []string
	}{
		{
			cwe:      "CWE-89",
			expected: []string{"OWASP 2021 A03:Injection"},
		},
		{
			cwe:      "CWE-502",
			expected: []string{"OWASP 2021 A08:Software and Data Integrity Failures"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.cwe, func(t *testing.T) {
			owasp := enricher.GetOWASPByCWE(tt.cwe)
			assert.ElementsMatch(t, tt.expected, owasp)
		})
	}
}

func TestFormatCWENumber(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"CWE-89", "CWE-89"},
		{"cwe-89", "CWE-89"},
		{"89", "CWE-89"},
		{"  CWE-89  ", "CWE-89"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := formatCWENumber(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatchPathPattern(t *testing.T) {
	tests := []struct {
		path     string
		pattern  string
		expected bool
	}{
		{
			path:     "buildin/java/cwe-89-sql/rule.sf",
			pattern:  "**/java/**",
			expected: true,
		},
		{
			path:     "buildin/php/components/shiro/rule.sf",
			pattern:  "**/components/shiro/**",
			expected: true,
		},
		{
			path:     "buildin/golang/sca/rule.sf",
			pattern:  "**/sca/**",
			expected: true,
		},
		{
			path:     "buildin/java/other/rule.sf",
			pattern:  "**/components/shiro/**",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.path+":"+tt.pattern, func(t *testing.T) {
			result := matchPathPattern(tt.path, tt.pattern)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractCWEFromTags(t *testing.T) {
	tests := []struct {
		tags     []string
		expected []string
	}{
		{
			tags:     []string{"CWE-89", "sql", "injection"},
			expected: []string{"CWE-89"},
		},
		{
			tags:     []string{"cwe-89", "CWE-564", "java"},
			expected: []string{"CWE-89", "CWE-564"},
		},
		{
			tags:     []string{"CWE-89", "CWE-89"}, // 重复
			expected: []string{"CWE-89"},
		},
		{
			tags:     []string{"java", "sql"},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := ExtractCWEFromTags(tt.tags)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}
