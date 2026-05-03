package aiforge

import (
	"bytes"
	"strings"
	"text/template"

	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/log"
)

// queryPrompt 是 LiteForge 调用方常用的"动态内容"模板
// B 档改造：去掉 INPUT/EXTRA/OVERLAP 的内层 nonce
// 安全性说明：返回的字符串通常作为 LiteForge 的 Params 或 Prompt 字段被传入 dynamic 段，
// 外层 PROMPT_SECTION_dynamic_NONCE 已经屏蔽 prompt-injection，内层 nonce 是冗余防护
// 关键词: aicache, PROMPT_SECTION, queryPrompt, B 档, 去 nonce
var queryPrompt = `{{.PROMPT}}

{{ if .EXTRA }}
<extra>
{{.EXTRA}}
</extra>
{{ end }}

{{ if .OVERLAP }}
<overlap>
{{.OVERLAP}}
</overlap>
{{ end }}


<input>
{{.INPUT}}
</input>
`

// queryDynamicOnlyPrompt 是 B 档新增的"纯动态内容"模板（不含调用方稳定指令头部）
// 用于配合 BuildLiteForgeStaticAndDynamic 拆分 prompt 时使用
// 关键词: aicache, PROMPT_SECTION, queryDynamicOnlyPrompt, BuildLiteForgeStaticAndDynamic
var queryDynamicOnlyPrompt = `{{ if .EXTRA }}<extra>
{{.EXTRA}}
</extra>

{{ end }}{{ if .OVERLAP }}<overlap>
{{.OVERLAP}}
</overlap>

{{ end }}<input>
{{.INPUT}}
</input>
`

// LiteForgeQueryFromChunk 兼容函数：保留原签名，B 档改造仅去掉模板内层 nonce
// 旧调用方零代码变更，但 hash 复用能力不变（内容仍随 chunk 数据每次不同）
// 关键词: aicache, PROMPT_SECTION, LiteForgeQueryFromChunk, B 档兼容
func LiteForgeQueryFromChunk(prompt string, extraPrompt string, chunk chunkmaker.Chunk, overlapSize int) (string, error) {
	param := map[string]interface{}{
		"PROMPT": prompt,
		"INPUT":  string(chunk.Data()),
		"EXTRA":  extraPrompt,
	}

	if overlapSize > 0 || chunk.HaveLastChunk() {
		param["OVERLAP"] = string(chunk.PrevNBytes(overlapSize))
	}
	queryTemplate, err := template.New("query").Parse(queryPrompt)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = queryTemplate.ExecuteTemplate(&buf, "query", param)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// BuildLiteForgeStaticAndDynamic 是 B 档新增的辅助函数：把 chunk 的内容拆成两段
// 返回值 static：调用方稳定指令（适合作为 LiteForge.StaticInstruction 进 high-static 段）
// 返回值 dynamic：动态上下文（INPUT/EXTRA/OVERLAP，适合作为 LiteForge.Prompt 进 dynamic 段）
// 关键词: aicache, PROMPT_SECTION, BuildLiteForgeStaticAndDynamic, B 档拆分
func BuildLiteForgeStaticAndDynamic(prompt string, extraPrompt string, chunk chunkmaker.Chunk, overlapSize int) (static string, dynamic string, err error) {
	static = prompt

	param := map[string]interface{}{
		"INPUT": string(chunk.Data()),
		"EXTRA": extraPrompt,
	}
	if overlapSize > 0 || chunk.HaveLastChunk() {
		param["OVERLAP"] = string(chunk.PrevNBytes(overlapSize))
	}

	queryTemplate, parseErr := template.New("query-dynamic-only").Parse(queryDynamicOnlyPrompt)
	if parseErr != nil {
		return "", "", parseErr
	}
	var buf bytes.Buffer
	if execErr := queryTemplate.ExecuteTemplate(&buf, "query-dynamic-only", param); execErr != nil {
		return "", "", execErr
	}
	dynamic = buf.String()
	return static, dynamic, nil
}

// quickQueryBuild 兼容函数：B 档改造仅去掉模板内层 nonce
// 关键词: aicache, PROMPT_SECTION, quickQueryBuild, B 档兼容
func quickQueryBuild(prompt string, input ...string) string {
	param := map[string]interface{}{
		"PROMPT": prompt,
		"INPUT":  strings.Join(input, "\n"),
	}
	queryTemplate, err := template.New("query").Parse(queryPrompt)
	if err != nil {
		log.Errorf("parse query template failed: %s", err)
		return ""
	}
	var buf bytes.Buffer
	err = queryTemplate.ExecuteTemplate(&buf, "query", param)
	if err != nil {
		log.Errorf("execute query template failed: %s", err)
		return ""
	}
	return buf.String()
}
