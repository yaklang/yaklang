package loop_http_flow_analyze

import (
	"strings"

	"github.com/yaklang/yaklang/common/yak/httptpl"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// SimplifiedMatcher 简化的 matcher 结构（AI 友好）
type SimplifiedMatcher struct {
	Type     string   `json:"type"`                // word/regex/status/binary/dsl
	Patterns []string `json:"patterns"`            // 匹配模式列表
	Scope    string   `json:"scope,omitempty"`     // request/response/all (默认 all)
	MatchAll bool     `json:"match_all,omitempty"` // true=AND, false=OR (默认 false)
	Negative bool     `json:"negative,omitempty"`  // 反向匹配 (默认 false)
	Encoding string   `json:"encoding,omitempty"`  // hex/base64 (仅 binary 类型)
	ExprType string   `json:"expr_type,omitempty"` // nuclei-dsl (仅 dsl 类型)
}

// SecurityPattern 内置安全模式
type SecurityPattern struct {
	Name        string
	Description string
	Matchers    []SimplifiedMatcher
}

// builtinSecurityPatterns 内置安全检测模式
var builtinSecurityPatterns = map[string]*SecurityPattern{
	"sql_injection": {
		Name:        "SQL Injection",
		Description: "Detect SQL injection attempts",
		Matchers: []SimplifiedMatcher{
			{
				Type:     "word",
				Patterns: []string{"' or", "' and", "union select", "drop table", "-- ", "/**/", "' OR '", "' AND '"},
				Scope:    "all",
				MatchAll: false,
			},
			{
				Type:     "regex",
				Patterns: []string{`\bor\b.*=.*`, `union.*select`, `sleep\(\d+\)`, `benchmark\(`},
				Scope:    "all",
				MatchAll: false,
			},
		},
	},

	"xss": {
		Name:        "Cross-Site Scripting",
		Description: "Detect XSS attempts",
		Matchers: []SimplifiedMatcher{
			{
				Type:     "word",
				Patterns: []string{"<script", "javascript:", "onerror=", "onload=", "alert(", "prompt(", "confirm("},
				Scope:    "all",
				MatchAll: false,
			},
			{
				Type:     "regex",
				Patterns: []string{`<script.*?>`, `on\w+\s*=`, `javascript:\s*`},
				Scope:    "all",
				MatchAll: false,
			},
		},
	},

	"sensitive_data": {
		Name:        "Sensitive Data Exposure",
		Description: "Detect exposed sensitive information in responses",
		Matchers: []SimplifiedMatcher{
			{
				Type: "word",
				Patterns: []string{
					"password", "passwd", "pwd",
					"token", "api_key", "apikey", "api-key",
					"secret", "private_key", "privatekey",
					"authorization", "auth_token",
					"session_id", "sessionid",
					"credit_card", "creditcard",
					"ssn", "social_security",
				},
				Scope:    "response",
				MatchAll: false,
			},
		},
	},

	"error_response": {
		Name:        "Error Response",
		Description: "Detect error responses and stack traces",
		Matchers: []SimplifiedMatcher{
			{
				Type:     "status",
				Patterns: []string{"500", "501", "502", "503", "504"},
			},
			{
				Type: "word",
				Patterns: []string{
					"stack trace", "stacktrace", "exception",
					"error:", "fatal error", "syntax error",
					"at line", "traceback",
					"Internal Server Error",
				},
				Scope:    "response",
				MatchAll: false,
			},
		},
	},

	"command_injection": {
		Name:        "Command Injection",
		Description: "Detect command injection attempts",
		Matchers: []SimplifiedMatcher{
			{
				Type:     "word",
				Patterns: []string{"|", ";", "&&", "||", "`", "$(", "${"},
				Scope:    "request",
				MatchAll: false,
			},
			{
				Type:     "regex",
				Patterns: []string{`[\|;&]\s*(ls|cat|wget|curl|nc|bash|sh|python|perl|php)`},
				Scope:    "request",
				MatchAll: false,
			},
		},
	},

	"path_traversal": {
		Name:        "Path Traversal",
		Description: "Detect path traversal attempts",
		Matchers: []SimplifiedMatcher{
			{
				Type:     "word",
				Patterns: []string{"../", "..\\", "%2e%2e/", "%2e%2e\\", "....//", "....\\\\"},
				Scope:    "request",
				MatchAll: false,
			},
			{
				Type:     "regex",
				Patterns: []string{`\.\.[\\/]`, `%2e%2e[\\/]`},
				Scope:    "request",
				MatchAll: false,
			},
		},
	},

	"ssrf": {
		Name:        "Server-Side Request Forgery",
		Description: "Detect SSRF attempts",
		Matchers: []SimplifiedMatcher{
			{
				Type: "word",
				Patterns: []string{
					"localhost", "127.0.0.1", "0.0.0.0",
					"169.254.169.254", // AWS metadata
					"metadata.google.internal",
					"[::1]",
				},
				Scope:    "request",
				MatchAll: false,
			},
		},
	},

	"file_upload": {
		Name:        "File Upload",
		Description: "Detect file upload attempts with dangerous extensions",
		Matchers: []SimplifiedMatcher{
			{
				Type: "word",
				Patterns: []string{
					".php", ".jsp", ".asp", ".aspx",
					".sh", ".bash", ".py", ".pl",
					".exe", ".dll", ".so",
				},
				Scope:    "request",
				MatchAll: false,
			},
		},
	},
}

// getSecurityPattern 获取内置安全模式
func getSecurityPattern(name string) *SecurityPattern {
	return builtinSecurityPatterns[name]
}

// convertSimplifiedToYakMatcher 将简化的 matcher 转换为 YakMatcher
func convertSimplifiedToYakMatcher(simplified *SimplifiedMatcher) *httptpl.YakMatcher {
	if simplified == nil {
		return nil
	}

	// 设置默认值
	scope := simplified.Scope
	if scope == "" {
		scope = "all"
	}

	condition := "or"
	if simplified.MatchAll {
		condition = "and"
	}

	yakMatcher := &httptpl.YakMatcher{
		MatcherType:   simplified.Type,
		Scope:         scope,
		Condition:     condition,
		Group:         simplified.Patterns,
		GroupEncoding: simplified.Encoding,
		Negative:      simplified.Negative,
		ExprType:      simplified.ExprType,
	}

	return yakMatcher
}

// convertSimplifiedToGRPCMatcher 将简化的 matcher 转换为 gRPC HTTPResponseMatcher
func convertSimplifiedToGRPCMatcher(simplified *SimplifiedMatcher) *ypb.HTTPResponseMatcher {
	if simplified == nil {
		return nil
	}

	// 设置默认值
	scope := simplified.Scope
	if scope == "" {
		scope = "all"
	}

	condition := "or"
	if simplified.MatchAll {
		condition = "and"
	}

	grpcMatcher := &ypb.HTTPResponseMatcher{
		MatcherType:   simplified.Type,
		Scope:         scope,
		Condition:     condition,
		Group:         simplified.Patterns,
		GroupEncoding: simplified.Encoding,
		Negative:      simplified.Negative,
		ExprType:      simplified.ExprType,
	}

	return grpcMatcher
}

// describeSimplifiedMatchers 生成 matcher 的可读描述
func describeSimplifiedMatchers(matchers []SimplifiedMatcher) string {
	if len(matchers) == 0 {
		return "(none)"
	}

	parts := make([]string, 0, len(matchers))
	for _, m := range matchers {
		desc := m.Type
		if m.Scope != "" && m.Scope != "all" {
			desc += "/" + m.Scope
		}
		if len(m.Patterns) > 0 {
			groupPreview := strings.Join(m.Patterns, ", ")
			if len(groupPreview) > 80 {
				groupPreview = groupPreview[:80] + "..."
			}
			desc += " [" + groupPreview + "]"
		}
		if m.Negative {
			desc += " (negative)"
		}
		if m.MatchAll {
			desc += " (AND)"
		}
		parts = append(parts, desc)
	}

	return strings.Join(parts, "; ")
}
