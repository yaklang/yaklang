// Package markdownextractor provides functionality for extracting code blocks from markdown text.
// It supports both backtick and tilde delimited code blocks with language specifications.
package markdownextractor

import (
	"bytes"
	"errors"
	"strings"
)

// Common errors
var (
	ErrUnclosedCodeBlock = errors.New("unclosed code block")
)

// Minimum number of backticks or tildes required for a code block
const minDelimiters = 3

type state int

const (
	// stateNormal represents the state when parsing normal markdown text
	stateNormal state = iota
	// stateCodeBlockType represents the state when parsing the code block type/language
	stateCodeBlockType
	// stateCodeBlockContent represents the state when parsing the content within a code block
	stateCodeBlockContent
)

// codeBlock represents a markdown code block with its metadata
type codeBlock struct {
	typeName    string
	content     string
	startOffset int
	endOffset   int
	delimiter   byte
	backticks   int
}

// skipExtraDelimiters counts and skips consecutive delimiter characters (backticks or tildes)
// Returns the number of additional delimiters found
func skipExtraDelimiters(reader *bytes.Reader, delimiter byte) int {
	count := 0
	for {
		b, err := reader.ReadByte()
		if err != nil || b != delimiter {
			if err == nil {
				reader.UnreadByte()
			}
			break
		}
		count++
	}
	return count
}

// processCodeBlock handles the processing of a code block's content
func processCodeBlock(block codeBlock, callback func(typeName string, code string, startOffset int, endOffset int)) {
	code := block.content
	if block.typeName == "markdown" {
		callback(strings.TrimSpace(block.typeName), code, block.startOffset, block.endOffset)
		return
	}

	if strings.TrimSpace(code) == "" {
		code = strings.TrimRight(code, "\n")
	} else {
		code = strings.TrimSpace(code)
	}
	callback(strings.TrimSpace(block.typeName), code, block.startOffset, block.endOffset)
}

// ExtractMarkdownCode parses markdown text and extracts code blocks.
// For each code block found, it calls the callback function with:
// - typeName: the language/type specified for the code block
// - code: the content of the code block
// - startOffset: the starting position of the code content in the original text
// - endOffset: the ending position of the code content in the original text
// Returns the original markdown text and any error encountered during processing
func ExtractMarkdownCode(markdown string, callback func(typeName string, code string, startOffset int, endOffset int)) (string, error) {
	var (
		currentState state = stateNormal
		buffer       strings.Builder
		blockStack   []codeBlock
		inString     bool
	)

	reader := bytes.NewReader([]byte(markdown))
	pos := 0

	for {
		char, err := reader.ReadByte()
		if err != nil {
			break
		}

		switch currentState {
		case stateNormal:
			if !inString && (char == '`' || char == '~') {
				backticks := 1
				delimiter := char
				extraCount := skipExtraDelimiters(reader, delimiter)
				backticks += extraCount

				if backticks >= minDelimiters {
					currentState = stateCodeBlockType
					block := codeBlock{
						delimiter: delimiter,
						backticks: backticks,
					}
					blockStack = append(blockStack, block)
					buffer.Reset()
					pos += extraCount
					blockStack[len(blockStack)-1].startOffset = pos + 1
				} else {
					reader.Seek(-int64(extraCount)-1, 1)
				}
			}

		case stateCodeBlockType:
			if char == '\n' {
				blockStack[len(blockStack)-1].typeName = strings.TrimSpace(buffer.String())
				buffer.Reset()
				currentState = stateCodeBlockContent
				blockStack[len(blockStack)-1].startOffset = pos + 1
			} else {
				buffer.WriteByte(char)
			}

		case stateCodeBlockContent:
			if char == '"' {
				inString = !inString
			} else if !inString && (char == '`' || char == '~') {
				backticks := 1
				extraCount := skipExtraDelimiters(reader, char)
				backticks += extraCount

				if backticks >= minDelimiters {
					currentBlock := &blockStack[len(blockStack)-1]

					// 处理嵌套的代码块
					if char == currentBlock.delimiter && backticks == currentBlock.backticks {
						// 匹配当前代码块的结束
						currentBlock.endOffset = pos
						if currentBlock.endOffset > currentBlock.startOffset && currentBlock.startOffset < len(markdown) {
							currentBlock.content = markdown[currentBlock.startOffset:currentBlock.endOffset]
							processCodeBlock(*currentBlock, callback)
						}

						blockStack = blockStack[:len(blockStack)-1]
						if len(blockStack) == 0 {
							currentState = stateNormal
						} else {
							currentState = stateCodeBlockContent
						}
						pos += extraCount
					} else {
						// 处理新的代码块开始
						if currentBlock.typeName == "markdown" {
							block := codeBlock{
								delimiter: char,
								backticks: backticks,
							}
							blockStack = append(blockStack, block)
							buffer.Reset()
							pos += extraCount
							blockStack[len(blockStack)-1].startOffset = pos + 1
							currentState = stateCodeBlockType
						} else {
							pos += extraCount
						}
					}
					continue
				}
			}
		}
		pos++
	}

	// Check for unclosed code blocks
	if len(blockStack) > 0 {
		return markdown, ErrUnclosedCodeBlock
	}

	return markdown, nil
}
