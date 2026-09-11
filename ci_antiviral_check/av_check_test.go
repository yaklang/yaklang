// Package ci_antiviral_check 提供独立 CI 测试，扫描整个仓库的 Go 源码，
// 确保不会出现可能被杀软误报的明文恶意字符串。
//
// 本包不依赖任何 yak 内部包，不会被编译进 yak 二进制，仅通过 `go test` 在 CI 中运行。
//
// 运行方式:
//
//	go test ./ci_antiviral_check/...
package ci_antiviral_check

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// 工具函数
// ---------------------------------------------------------------------------

// repoRoot 返回仓库根目录（通过向上查找 go.mod 定位）。
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod (repo root)")
		}
		dir = parent
	}
}

// truncate 截断字符串用于报错显示。
func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

// ---------------------------------------------------------------------------
// 文件过滤：哪些文件不需要扫描
// ---------------------------------------------------------------------------

// shouldSkipFile 判断文件是否应该跳过检查。
func shouldSkipFile(path string) bool {
	slash := filepath.ToSlash(path)
	base := filepath.Base(path)

	// 跳过测试文件
	if strings.HasSuffix(base, "_test.go") {
		return true
	}
	// 跳过 protobuf 生成文件
	if strings.HasSuffix(base, ".pb.go") {
		return true
	}
	// 跳过 vendor
	if strings.Contains(slash, "/vendor/") {
		return true
	}
	// 跳过 /static/ 下的明文配置源文件（开发用，不是 Go 代码）
	if strings.Contains(slash, "/static/") && !strings.HasSuffix(base, ".go") {
		return true
	}
	// 跳过 .tar.gz 编码文件
	if strings.HasSuffix(base, ".tar.gz") {
		return true
	}
	// 跳过非 .go 文件
	if !strings.HasSuffix(base, ".go") {
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// 注释剥离：只检查实际代码，不检查注释
// ---------------------------------------------------------------------------

// stripComments 去掉 Go 源码中的注释，只保留实际代码字符串。
func stripComments(content string) string {
	var b strings.Builder
	lines := strings.Split(content, "\n")
	inBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 处理块注释
		if inBlock {
			if idx := strings.Index(trimmed, "*/"); idx >= 0 {
				inBlock = false
				trimmed = strings.TrimSpace(trimmed[idx+2:])
				if trimmed == "" {
					continue
				}
			} else {
				continue
			}
		}

		// 行注释
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		// 块注释开始
		if strings.HasPrefix(trimmed, "/*") {
			// 检查是否同行结束
			if !strings.Contains(trimmed[2:], "*/") {
				inBlock = true
			}
			continue
		}
		// 去掉行内注释（简单处理：确保 // 不在字符串内部）
		if idx := indexOfLineComment(line); idx >= 0 {
			line = line[:idx]
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

// indexOfLineComment 找到行注释 // 的位置（排除字符串内部的 //）。
func indexOfLineComment(line string) int {
	inStr := false
	strChar := byte(0)
	for i := 0; i < len(line)-1; i++ {
		c := line[i]
		if c == '\\' && inStr {
			i++ // 跳过转义字符
			continue
		}
		if !inStr && (c == '"' || c == '`' || c == '\'') {
			inStr = true
			strChar = c
		} else if inStr && c == strChar {
			inStr = false
		}
		if !inStr && c == '/' && line[i+1] == '/' {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// embed 加载行豁免：允许在加载逻辑中引用这些模式
// ---------------------------------------------------------------------------

var embedLoadKeywords = []string{
	"embeddata.Load",
	"templates.GetTemplates",
	"loadSignaturesFromEmbed",
	"loadSSTIPayloads",
	"loadPluginB64",
	"LoadCheckList",
	"LoadSerializedObjects",
	"//go:embed",
	"gzip_embed.New",
	"NewPreprocessingEmbed",
	"PreprocessingEmbed",
}

// isEmbedLoadLine 检查匹配内容所在的行是否是 embed 加载行。
func isEmbedLoadLine(raw, match string) bool {
	idx := strings.Index(raw, match)
	if idx < 0 {
		return false
	}
	lineStart := strings.LastIndex(raw[:idx], "\n") + 1
	lineEnd := strings.Index(raw[idx:], "\n")
	if lineEnd < 0 {
		lineEnd = len(raw) - idx
	}
	line := raw[lineStart : idx+lineEnd]
	for _, kw := range embedLoadKeywords {
		if strings.Contains(line, kw) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// 恶意模式定义：全局扫描规则
// ---------------------------------------------------------------------------

// maliciousPattern 定义一个禁用的明文模式和它的描述。
type maliciousPattern struct {
	pattern *regexp.Regexp
	desc    string
}

// globalMaliciousPatterns 是全仓库扫描的禁用明文模式列表。
// 这些模式如果出现在 Go 源码的非注释、非 embed 加载行中，说明应该被 embed + XOR 混淆。
var globalMaliciousPatterns = []maliciousPattern{
	// --- Java class base64 字节码 ---
	// yv66vg 是 Java .class 文件的 base64 magic header
	{
		regexp.MustCompile(`"yv66vg[A-Za-z0-9+/=]{100,}"`),
		"明文 Java class 字节码 base64 (yv66vg...)",
	},

	// --- Java 序列化对象 base64 ---
	// rO0AB 是 Java 序列化对象的 base64 magic header
	{
		regexp.MustCompile(`"rO0AB[A-Za-z0-9+/=]{20,}"`),
		"明文 Java 序列化对象 base64 (rO0AB...)",
	},

	// --- webshell 生成模板特征 ---
	// JSP
	{
		regexp.MustCompile(`Class\.forName\("java\.util\.Base64"\)`),
		"JSP webshell 模板: Class.forName Base64 反射调用",
	},
	{
		regexp.MustCompile(`Class\.forName\("sun\.misc\.BASE64Decoder"\)`),
		"JSP webshell 模板: Class.forName BASE64Decoder 反射调用",
	},
	{
		regexp.MustCompile(`super\.defineClass\(b,\s*0,\s*b\.length\)`),
		"JSP webshell 模板: defineClass 类加载",
	},
	// PHP
	{
		regexp.MustCompile(`eval\(base64_decode\(\$_POST\[`),
		"PHP webshell 模板: eval(base64_decode($_POST[...]))",
	},
	{
		regexp.MustCompile(`eval\(decrypt\(\$_SESSION\[\$payload\]`),
		"PHP webshell session 模板: eval(decrypt($_SESSION[...]))",
	},
	{
		regexp.MustCompile(`openssl_encrypt\(\$data,\s*'AES128'`),
		"Behinder PHP echo encoder: openssl_encrypt",
	},
	// ASP
	{
		regexp.MustCompile(`Function Encrypt\(data\)\nkey=Session\("k"\)`),
		"Behinder ASP echo encoder",
	},
	// ASPX
	{
		regexp.MustCompile(`Assembly\.Load\(Convert\.FromBase64String`),
		"ASPX webshell 模板: Assembly.Load + FromBase64String",
	},
	{
		regexp.MustCompile(`\.CreateInstance\("Payload"\)`),
		"ASPX webshell 模板: CreateInstance(\"Payload\")",
	},
	// assert|eval 拼接
	{
		regexp.MustCompile(`"assert\|eval\(base64_decode\('"\s*\+`),
		"Behinder PHP: assert|eval(base64_decode(...)) 拼接",
	},

	// --- SSTI payload 特征 ---
	{
		regexp.MustCompile(`ProcessBuilder;\s*x\.command\(\\"whoami\\"`),
		"SSTI payload: ProcessBuilder whoami",
	},
	{
		regexp.MustCompile(`getRuntime\(\)\.exec\('cat etc/passwd'\)`),
		"SSTI payload: Runtime.exec cat passwd",
	},

	// --- Java 反序列化 gadget 类名（以常量/字符串字面量形式硬编码）---
	{
		regexp.MustCompile(`"org\.apache\.commons\.collections[0-9]?\.functors\.ChainedTransformer"`),
		"Java 反序列化 gadget 类名: ChainedTransformer",
	},
	{
		regexp.MustCompile(`"com\.sun\.org\.apache\.bcel\.internal\.util\.ClassLoader"`),
		"Java 反序列化 gadget 类名: BCEL ClassLoader",
	},
	{
		regexp.MustCompile(`"org\.mozilla\.javascript\.DefiningClassLoader"`),
		"Java 反序列化 gadget 类名: DefiningClassLoader",
	},

	// --- 恶意文件特征库中的正则模式（硬编码 Pattern: `...` 形式）---
	{
		regexp.MustCompile(`Pattern:\s*\x60eval\\s\*\\\(`),
		"恶意特征库硬编码: eval\\s*\\(",
	},
	{
		regexp.MustCompile(`Pattern:\s*\x60shell_exec\\s\*\\\(`),
		"恶意特征库硬编码: shell_exec\\s*\\(",
	},
	{
		regexp.MustCompile(`Pattern:\s*\x60@eval\|@assert\|@system\x60`),
		"恶意特征库硬编码: @eval|@assert|@system",
	},
	{
		regexp.MustCompile(`Pattern:\s*\x60bash\\s\+\\-i\\s\+>`),
		"恶意特征库硬编码: bash -i 反弹 shell",
	},
}

// ---------------------------------------------------------------------------
// 测试入口
// ---------------------------------------------------------------------------

// TestNoPlaintextMaliciousStrings 扫描整个仓库的 Go 源码，
// 确保不会出现应该被 embed + XOR 混淆的明文恶意字符串。
//
// 扫描范围：仓库根目录下所有 .go 文件（排除 _test.go、.pb.go、vendor/、embed_data/ 等）。
// 豁免规则：注释行、embed 加载行（含 //go:embed、xorencoded.Load 等标识）不会被报错。
func TestNoPlaintextMaliciousStrings(t *testing.T) {
	root := repoRoot(t)

	// 扫描的顶层目录
	scanDirs := []string{"common", "embed", "scannode", "cmd"}

	violations := []string{}

	for _, scanDir := range scanDirs {
		absDir := filepath.Join(root, scanDir)
		if _, err := os.Stat(absDir); os.IsNotExist(err) {
			continue
		}
		filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if shouldSkipFile(path) {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			raw := string(content)
			code := stripComments(raw)
			relPath, _ := filepath.Rel(root, path)

			for _, mp := range globalMaliciousPatterns {
				loc := mp.pattern.FindStringIndex(code)
				if loc == nil {
					continue
				}
				matched := code[loc[0]:loc[1]]
				// 豁免：检查是否在 embed 加载行中
				if isEmbedLoadLine(raw, matched) {
					continue
				}
				violations = append(violations, relPath+" ["+mp.desc+"]: "+truncate(matched, 120))
			}
			return nil
		})
	}

	if len(violations) > 0 {
		t.Errorf("发现 %d 处明文恶意字符串，应通过 embed + XOR 方式加载：\n", len(violations))
		for _, v := range violations {
			t.Errorf("  - %s\n", v)
		}
		t.Errorf("\n请参考 common/utils/xorencoded/ 工具包将明文内容抽离为配置文件。\n已有示例：common/wsm/payloads/templates/embed.go")
	}
}
