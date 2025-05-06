package utils

import (
	"bytes"
	"fmt"
	"regexp"
	"unicode/utf8"
)

type TextSplitter struct {
	ChunkSize         int
	ChunkOverlap      int
	Separators        []string
	ProtectedPatterns []*regexp.Regexp
}

func NewTextSplitter() *TextSplitter {
	return &TextSplitter{
		ChunkSize:    700,
		ChunkOverlap: 50,
		Separators:   []string{"\n\n", "。", "！", "？", ";", "..."},
		ProtectedPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(\+-+){2,}`),         // ASCII 表格
			regexp.MustCompile(`<table.*?<\/table>`), // HTML 表格
		},
	}
}

// 核心分割方法
func (ts *TextSplitter) Split(text string) []string {
	// 保护特殊内容
	protected, placeholders := ts.protectContent(text)

	// 递归分割
	chunks := ts.recursiveSplit(protected)

	// 恢复被保护内容
	return ts.restoreContent(chunks, placeholders)
}

// 保护特殊内容不被分割
func (ts *TextSplitter) protectContent(text string) (string, map[string]string) {
	placeholders := make(map[string]string)
	i := 0

	for _, pattern := range ts.ProtectedPatterns {
		text = pattern.ReplaceAllStringFunc(text, func(m string) string {
			key := fmt.Sprintf("__PROTECTED_%d__", i)
			placeholders[key] = m
			i++
			return key
		})
	}
	return text, placeholders
}

// 递归分割核心逻辑
func (ts *TextSplitter) recursiveSplit(text string) []string {
	if utf8.RuneCountInString(text) <= ts.ChunkSize {
		return []string{text}
	}

	// 寻找最佳分割点
	splitPos := ts.findBestSplitPosition(text)
	if splitPos == -1 {
		splitPos = ts.ChunkSize
	}

	// 带重叠的分割
	chunk := text[:splitPos]
	remaining := text[splitPos:]

	return append([]string{chunk}, ts.recursiveSplit(remaining)...)
}

// 查找最佳分割位置
func (ts *TextSplitter) findBestSplitPosition(text string) int {
	runes := []rune(text)
	maxPos := Min(len(runes), ts.ChunkSize)

	// 优先查找分隔符
	for _, sep := range ts.Separators {
		pos := bytes.LastIndex([]byte(string(runes[:maxPos])), []byte(sep))
		if pos != -1 {
			return pos + len(sep)
		}
	}

	// 次找句子边界
	for i := maxPos - 1; i > 0; i-- {
		if isSentenceBoundary(runes[i]) {
			return i + 1
		}
	}

	return -1
}

// 恢复被保护内容
func (ts *TextSplitter) restoreContent(chunks []string, ph map[string]string) []string {
	for i := range chunks {
		for key, value := range ph {
			chunks[i] = regexp.MustCompile(regexp.QuoteMeta(key)).ReplaceAllString(chunks[i], value)
		}
	}
	return chunks
}

// 辅助函数
func isSentenceBoundary(r rune) bool {
	switch r {
	case '。', '！', '？', ';', '\n':
		return true
	}
	return false
}
