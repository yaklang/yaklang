package utils

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"strings"
	"unicode/utf8"
)

type TextSplitter struct {
	ChunkSize    int
	ChunkOverlap int
	Separators   []string
}

var DefaultTextSplitter = NewTextSplitter()

func NewTextSplitter() *TextSplitter {
	return &TextSplitter{
		ChunkSize:    700,
		ChunkOverlap: 50,
		Separators:   []string{"\n\n", "。", "！", "？", ";", "..."},
	}
}

// TextSplit 将文本按照分隔符与块大小递归切分为多个文本块
//
// 参数:
//   - ctx: 上下文，可用于取消操作
//   - text: 待切分的文本
//
// 返回值:
//   - 切分后的文本块列表
//
// Example:
// ```
// chunks = str.TextSplit(context.Background(), "段落一。段落二。段落三。")
// assert len(chunks) > 0, "TextSplit should produce chunks"
// ```
func (ts *TextSplitter) Split(ctx context.Context, text string) []string {
	var chunks []string
	reader := strings.NewReader(text)
	splitChan := ts.recursiveSplit(ctx, reader)
	for chunk := range splitChan {
		chunks = append(chunks, chunk)
	}
	return chunks
}

// TextReaderSplit 从 io.Reader 流式读取文本，并按分隔符与块大小递归切分，通过 channel 返回文本块
//
// 参数:
//   - ctx: 上下文，可用于取消操作
//   - reader: 提供文本数据的 io.Reader
//
// 返回值:
//   - 一个不断产出文本块的 channel
//
// Example:
// ```
// ch = str.TextReaderSplit(context.Background(), str.NewReader("段落一。段落二。"))
// for chunk = range ch { println(chunk) }
// ```
func (ts *TextSplitter) SplitReader(ctx context.Context, reader io.Reader) chan string {
	return ts.recursiveSplit(ctx, reader)
}

func RuneRead(r io.Reader, maxChars int) string {
	decoder := bufio.NewReader(r)
	var result []rune
	count := 0
	for count < maxChars {
		r, _, err := decoder.ReadRune()
		if err != nil {
			break
		}
		result = append(result, r)
		count++
	}
	return string(result)
}

// 递归分割核心逻辑
func (ts *TextSplitter) recursiveSplit(ctx context.Context, data io.Reader) chan string {
	result := make(chan string)
	go func() {
		defer close(result)
		var splitHandle func(reader io.Reader)
		splitHandle = func(textReader io.Reader) {
			select {
			case <-ctx.Done():
				return
			default:
			}

			text := RuneRead(textReader, ts.ChunkSize)
			if utf8.RuneCountInString(text) < ts.ChunkSize {
				result <- text
				return
			}

			// 寻找最佳分割点
			splitPos := ts.findBestSplitPosition(text)
			if splitPos == -1 {
				splitPos = ts.ChunkSize
			}

			// 分割文本
			currentChunk := text[:splitPos]
			result <- currentChunk
			remainingText := text[splitPos:]
			if len(remainingText) > 0 {
				newReader := io.MultiReader(
					bytes.NewReader([]byte(remainingText)),
					textReader,
				)
				splitHandle(newReader)
			} else {
				splitHandle(textReader)
			}
		}
		splitHandle(data)
	}()
	return result
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

// 辅助函数
func isSentenceBoundary(r rune) bool {
	switch r {
	case '。', '！', '？', ';', '\n':
		return true
	}
	return false
}
