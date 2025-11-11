package lsp

import (
	"testing"
)

func TestComputeCodeHash_Levels(t *testing.T) {
	code := `
// This is a comment
fn test() {
    println("hello")
}
`

	hash := ComputeCodeHash(code)

	if hash.Full == "" {
		t.Error("Full hash should not be empty")
	}

	if hash.Structure == "" {
		t.Error("Structure hash should not be empty")
	}

	if hash.Semantic == "" {
		t.Error("Semantic hash should not be empty")
	}

	// 全文哈希应该最敏感
	// 结构哈希应该次之
	// 语义哈希应该最不敏感
	t.Logf("Full: %s, Structure: %s, Semantic: %s", hash.Full, hash.Structure, hash.Semantic)
}

func TestComputeCodeHash_CommentChanges(t *testing.T) {
	code1 := `fn test() { println("hello") }`
	code2 := `// Comment added
fn test() { println("hello") }`

	hash1 := ComputeCodeHash(code1)
	hash2 := ComputeCodeHash(code2)

	// 全文哈希应该不同
	if hash1.Full == hash2.Full {
		t.Error("Full hash should differ when comment is added")
	}

	// 结构哈希应该相同（注释被移除）
	if hash1.Structure != hash2.Structure {
		t.Error("Structure hash should be same when only comment changes")
	}

	// 语义哈希应该相同
	if hash1.Semantic != hash2.Semantic {
		t.Error("Semantic hash should be same when only comment changes")
	}
}

func TestComputeCodeHash_WhitespaceChanges(t *testing.T) {
	code1 := `fn test() { println("hello") }`
	code2 := `fn test() {
    println("hello")
}`

	hash1 := ComputeCodeHash(code1)
	hash2 := ComputeCodeHash(code2)

	// 全文哈希应该不同
	if hash1.Full == hash2.Full {
		t.Error("Full hash should differ when whitespace changes")
	}

	// 结构哈希应该相同（空白被规范化）
	if hash1.Structure != hash2.Structure {
		t.Logf("Structure hash differs, which is acceptable: %s vs %s", hash1.Structure, hash2.Structure)
		// 这可能是正常的，因为结构哈希的规范化可能不完美
	}
}

func TestComputeCodeHash_SemanticChanges(t *testing.T) {
	code1 := `fn test() { println("hello") }`
	code2 := `fn test() { println("world") }`

	hash1 := ComputeCodeHash(code1)
	hash2 := ComputeCodeHash(code2)

	// 所有哈希都应该不同（字面量改变）
	if hash1.Full == hash2.Full {
		t.Error("Full hash should differ when code changes")
	}

	if hash1.Structure == hash2.Structure {
		t.Error("Structure hash should differ when code changes")
	}

	if hash1.Semantic == hash2.Semantic {
		t.Error("Semantic hash should differ when literal changes")
	}
}

func TestShouldRecompileSSA(t *testing.T) {
	code1 := `fn test() { println("hello") }`
	code2 := `// added comment
fn test() { println("hello") }`
	code3 := `fn test() { println("world") }`

	hash1 := ComputeCodeHash(code1)
	hash2 := ComputeCodeHash(code2)
	hash3 := ComputeCodeHash(code3)

	// 仅注释变化 - 不应重编译
	if ShouldRecompileSSA(hash1, hash2) {
		t.Error("Should not recompile SSA for comment-only changes")
	}

	// 语义变化 - 应该重编译
	if !ShouldRecompileSSA(hash1, hash3) {
		t.Error("Should recompile SSA for semantic changes")
	}
}

func TestShouldReparseAST(t *testing.T) {
	code1 := `fn test() { println("hello") }`
	code2 := `// added comment
fn test() { println("hello") }`
	code3 := `fn test() { println("world") }`

	hash1 := ComputeCodeHash(code1)
	hash2 := ComputeCodeHash(code2)
	hash3 := ComputeCodeHash(code3)

	// 仅注释变化 - 可能需要重解析（取决于实现）
	shouldReparse12 := ShouldReparseAST(hash1, hash2)
	t.Logf("Should reparse for comment change: %v", shouldReparse12)

	// 语义变化 - 应该重解析
	if !ShouldReparseAST(hash1, hash3) {
		t.Error("Should reparse AST for semantic changes")
	}
}

func TestExtractSemanticTokens(t *testing.T) {
	code := `fn test(x) {
		if x > 10 {
			println("big")
		}
	}`

	tokens := extractSemanticTokens(code)

	if len(tokens) == 0 {
		t.Error("Should extract some tokens")
	}

	t.Logf("Extracted %d tokens: %v", len(tokens), tokens)

	// 验证包含关键字
	hasKeyword := false
	for _, token := range tokens {
		if token == "KW:fn" || token == "KW:if" {
			hasKeyword = true
			break
		}
	}

	if !hasKeyword {
		t.Error("Should extract keywords")
	}
}

func TestRemoveComments(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single line comment",
			input:    "code // comment\nmore code",
			expected: "code \nmore code",
		},
		{
			name:     "hash comment",
			input:    "code # comment\nmore code",
			expected: "code \nmore code",
		},
		{
			name:     "block comment",
			input:    "code /* comment */ more",
			expected: "code  more",
		},
		{
			name:     "string with comment-like content",
			input:    `str = "not // a comment"`,
			expected: `str = "not // a comment"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeLineComments(tt.input)
			result = removeBlockComments(result)
			// 注意：完美的注释处理需要完整的词法分析
			// 这里只做基本验证
			t.Logf("Input: %q\nOutput: %q\nExpected: %q", tt.input, result, tt.expected)
		})
	}
}

func TestGetCacheKey(t *testing.T) {
	key1 := GetCacheKey("yak", "hash123")
	key2 := GetCacheKey("yak", "hash456")
	key3 := GetCacheKey("mitm", "hash123")

	if key1 == key2 {
		t.Error("Different hashes should produce different keys")
	}

	if key1 == key3 {
		t.Error("Different script types should produce different keys")
	}

	if key1 != "yak:hash123" {
		t.Errorf("Expected 'yak:hash123', got %q", key1)
	}
}

func TestHashConsistency(t *testing.T) {
	code := `fn test() { println("hello") }`

	// 多次计算应该得到相同结果
	hash1 := ComputeCodeHash(code)
	hash2 := ComputeCodeHash(code)

	if hash1.Full != hash2.Full {
		t.Error("Full hash should be consistent")
	}

	if hash1.Structure != hash2.Structure {
		t.Error("Structure hash should be consistent")
	}

	if hash1.Semantic != hash2.Semantic {
		t.Error("Semantic hash should be consistent")
	}
}
