package utils

import (
	"io"
	"strconv"
	"unicode/utf8"
)

// jsonStringReader 流式解码JSON字符串的reader
type jsonStringReader struct {
	r            io.Reader
	buffer       []byte // 输入缓冲区
	outputBuffer []byte // 输出缓冲区
	state        int    // 解析状态
	started      bool   // 是否已开始解析
	fallback     bool   // 是否进入fallback模式
	eof          bool   // 是否到达结尾
	totalRead    []byte // 已读取的所有数据，用于fallback
}

const (
	stateInit     = 0 // 初始状态，等待第一个非空字符
	stateInString = 1 // 在字符串内
	stateEscape   = 2 // 转义状态
	stateUnicode  = 3 // Unicode转义状态
	stateHex      = 4 // 十六进制转义状态
	stateFallback = 5 // 回退状态
)

func (r *jsonStringReader) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	// 如果进入fallback模式，先返回已缓存的数据，再透传
	if r.fallback {
		if len(r.totalRead) > 0 {
			// 安全边界检查
			if len(p) == 0 {
				return 0, nil
			}
			copied := copy(p, r.totalRead)
			if copied <= len(r.totalRead) {
				r.totalRead = r.totalRead[copied:]
			} else {
				// 防止越界，清空
				r.totalRead = r.totalRead[:0]
			}
			return copied, nil
		}
		return r.r.Read(p)
	}

	// 先从输出缓冲区读取
	if len(r.outputBuffer) > 0 {
		copied := copy(p, r.outputBuffer)
		r.outputBuffer = r.outputBuffer[copied:]
		return copied, nil
	}

	// 如果已到达末尾
	if r.eof {
		return 0, io.EOF
	}

	// 读取更多数据到输入缓冲区
	tempBuf := make([]byte, 1024)
	readCount, readErr := r.r.Read(tempBuf)
	if readCount > 0 {
		newData := tempBuf[:readCount]
		r.buffer = append(r.buffer, newData...)
		// 记录所有读取的数据，用于可能的fallback
		r.totalRead = append(r.totalRead, newData...)
	}

	// 处理缓冲区中的数据
	processed := r.processBuffer()

	// 如果没有处理任何数据且遇到EOF，设置eof标志
	if !processed && readErr == io.EOF {
		r.eof = true
		return 0, io.EOF
	}

	// 如果有输出数据，返回
	if len(r.outputBuffer) > 0 {
		copied := copy(p, r.outputBuffer)
		r.outputBuffer = r.outputBuffer[copied:]
		return copied, readErr
	}

	// 如果读取出错且没有数据可返回
	if readErr != nil && readErr != io.EOF {
		return 0, readErr
	}

	// 递归调用以处理更多数据
	if readErr != io.EOF {
		return r.Read(p)
	}

	return 0, readErr
}

// triggerFallback 触发fallback模式，重新构建原始数据
func (r *jsonStringReader) triggerFallback() {
	r.fallback = true
	// totalRead包含了所有从底层reader读取的原始数据
	// 我们不需要修改它，因为它已经包含了所有原始内容

	// 清空工作缓冲区
	r.outputBuffer = r.outputBuffer[:0]
	r.buffer = r.buffer[:0]
}

func (r *jsonStringReader) processBuffer() bool {
	if len(r.buffer) == 0 {
		return false
	}

	processed := false
	i := 0

	for i < len(r.buffer) {
		// 安全边界检查 - 防止panic
		if i >= len(r.buffer) {
			break
		}
		char := r.buffer[i]

		switch r.state {
		case stateInit:
			// 跳过空白字符 - 支持多种空白字符
			if isWhitespace(char) {
				i++
				continue
			}
			// 如果第一个非空字符不是双引号，进入fallback模式
			if char != '"' {
				r.triggerFallback()
				return true
			}
			// 开始解析JSON字符串
			r.state = stateInString
			r.started = true
			i++ // 跳过开始的双引号
			processed = true

		case stateInString:
			if char == '"' {
				// 检查是否有剩余的非空白数据，如果有则表示是畸形JSON
				// 安全边界检查
				if i+1 > len(r.buffer) {
					// 索引越界，安全处理
					r.eof = true
					r.buffer = r.buffer[:0]
					return true
				}
				remaining := r.buffer[i+1:]
				hasNonWhitespace := false
				for _, b := range remaining {
					if !isWhitespace(b) {
						hasNonWhitespace = true
						break
					}
				}

				if hasNonWhitespace {
					// 发现畸形数据，触发fallback
					r.triggerFallback()
					return true
				}

				// 正常结束字符串
				r.eof = true
				r.buffer = r.buffer[i+1:] // 保留剩余数据
				return true
			} else if char == '\\' {
				r.state = stateEscape
				i++
				processed = true
			} else if char < 0x20 && char != '\t' && char != '\n' && char != '\r' {
				// 遇到不允许的控制字符，触发fallback
				r.triggerFallback()
				return true
			} else {
				// 普通字符，直接输出
				r.outputBuffer = append(r.outputBuffer, char)
				i++
				processed = true
			}

		case stateEscape:
			switch char {
			case '"':
				r.outputBuffer = append(r.outputBuffer, '"')
			case '\\':
				r.outputBuffer = append(r.outputBuffer, '\\')
			case '/':
				r.outputBuffer = append(r.outputBuffer, '/')
			case 'b':
				r.outputBuffer = append(r.outputBuffer, '\b')
			case 'f':
				r.outputBuffer = append(r.outputBuffer, '\f')
			case 'n':
				r.outputBuffer = append(r.outputBuffer, '\n')
			case 'r':
				r.outputBuffer = append(r.outputBuffer, '\r')
			case 't':
				r.outputBuffer = append(r.outputBuffer, '\t')
			case 'u':
				// Unicode转义 - 增强边界检查
				if i+4 >= len(r.buffer) {
					// 缓冲区不够，等待更多数据
					return processed
				}
				// 安全检查：确保不会越界
				if i+1 >= len(r.buffer) || i+4 >= len(r.buffer) {
					// 无效转义，回退为普通字符
					r.outputBuffer = append(r.outputBuffer, 'u')
				} else {
					// 检查后面4个字符是否都是十六进制
					hexStr := string(r.buffer[i+1 : i+5])
					if isHex(hexStr) {
						if codepoint, err := strconv.ParseUint(hexStr, 16, 16); err == nil {
							// 检查是否是代理对的高位 (0xD800-0xDBFF)
							if codepoint >= 0xD800 && codepoint <= 0xDBFF {
								// 这是代理对的高位，需要查找低位
								// 增强边界检查：确保有足够的字节来读取低位代理
								if i+10 >= len(r.buffer) {
									// 缓冲区不够，等待更多数据
									return processed
								}
								// 安全检查：确保所有访问都在边界内
								if i+5 < len(r.buffer) && i+6 < len(r.buffer) && i+10 < len(r.buffer) &&
									r.buffer[i+5] == '\\' && r.buffer[i+6] == 'u' {
									lowHexStr := string(r.buffer[i+7 : i+11])
									if isHex(lowHexStr) {
										if lowCodepoint, err := strconv.ParseUint(lowHexStr, 16, 16); err == nil {
											// 检查是否是代理对的低位 (0xDC00-0xDFFF)
											if lowCodepoint >= 0xDC00 && lowCodepoint <= 0xDFFF {
												// 计算实际的Unicode代码点
												actualCodepoint := 0x10000 + ((codepoint - 0xD800) << 10) + (lowCodepoint - 0xDC00)
												runeChar := rune(actualCodepoint)
												utf8Bytes := make([]byte, 4)
												n := utf8.EncodeRune(utf8Bytes, runeChar)
												r.outputBuffer = append(r.outputBuffer, utf8Bytes[:n]...)
												i += 10 // 跳过两个完整的Unicode转义序列
											} else {
												// 低位无效，只处理高位
												runeChar := rune(codepoint)
												utf8Bytes := make([]byte, 4)
												n := utf8.EncodeRune(utf8Bytes, runeChar)
												r.outputBuffer = append(r.outputBuffer, utf8Bytes[:n]...)
												i += 4
											}
										} else {
											// 低位解析失败，只处理高位
											runeChar := rune(codepoint)
											utf8Bytes := make([]byte, 4)
											n := utf8.EncodeRune(utf8Bytes, runeChar)
											r.outputBuffer = append(r.outputBuffer, utf8Bytes[:n]...)
											i += 4
										}
									} else {
										// 低位不是十六进制，只处理高位
										runeChar := rune(codepoint)
										utf8Bytes := make([]byte, 4)
										n := utf8.EncodeRune(utf8Bytes, runeChar)
										r.outputBuffer = append(r.outputBuffer, utf8Bytes[:n]...)
										i += 4
									}
								} else {
									// 没有找到低位代理，缓冲区不够或格式不对
									if i+10 >= len(r.buffer) {
										// 缓冲区不够，等待更多数据
										return processed
									} else {
										// 格式不对，只处理高位
										runeChar := rune(codepoint)
										utf8Bytes := make([]byte, 4)
										n := utf8.EncodeRune(utf8Bytes, runeChar)
										r.outputBuffer = append(r.outputBuffer, utf8Bytes[:n]...)
										i += 4
									}
								}
							} else {
								// 普通Unicode字符（不是代理对）
								runeChar := rune(codepoint)
								utf8Bytes := make([]byte, 4)
								n := utf8.EncodeRune(utf8Bytes, runeChar)
								r.outputBuffer = append(r.outputBuffer, utf8Bytes[:n]...)
								i += 4 // 跳过4个十六进制字符
							}
						} else {
							// 无效Unicode，回退为普通字符
							r.outputBuffer = append(r.outputBuffer, 'u')
						}
					} else {
						// 不是有效的十六进制，回退为普通字符
						r.outputBuffer = append(r.outputBuffer, 'u')
					}
				}
			case 'x':
				// 十六进制转义 \x20 - 增强边界检查
				if i+2 >= len(r.buffer) {
					// 缓冲区不够，等待更多数据
					return processed
				}
				// 安全检查：确保不会越界
				if i+1 >= len(r.buffer) || i+2 >= len(r.buffer) {
					// 无效转义，回退为普通字符
					r.outputBuffer = append(r.outputBuffer, 'x')
				} else {
					hexStr := string(r.buffer[i+1 : i+3])
					if isHex(hexStr) {
						if b, err := strconv.ParseUint(hexStr, 16, 8); err == nil {
							r.outputBuffer = append(r.outputBuffer, byte(b))
							i += 2 // 跳过2个十六进制字符
						} else {
							// 无效十六进制，回退为普通字符
							r.outputBuffer = append(r.outputBuffer, 'x')
						}
					} else {
						// 不是有效的十六进制，回退为普通字符
						r.outputBuffer = append(r.outputBuffer, 'x')
					}
				}
			default:
				// 检查是否是严重的无效转义，如果是则触发fallback
				if char < 0x20 || char > 0x7E {
					// 遇到不可打印字符作为转义字符，触发fallback
					r.triggerFallback()
					return true
				}
				// 一般的无效转义序列，保留原字符（向后兼容）
				r.outputBuffer = append(r.outputBuffer, char)
			}
			r.state = stateInString
			i++
			processed = true
		}
	}

	// 移除已处理的数据
	r.buffer = r.buffer[i:]
	return processed
}

// isWhitespace 检查字符是否为空白字符
func isWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n' ||
		b == '\v' || b == '\f' // 增加垂直制表符和换页符支持
}

// isHex 检查字符串是否都是十六进制字符
func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return len(s) > 0
}

func JSONStringReader(reader io.Reader) io.Reader {
	utf8reader := UTF8Reader(reader)
	return &jsonStringReader{
		r:            utf8reader,
		buffer:       make([]byte, 0),
		outputBuffer: make([]byte, 0),
		state:        stateInit,
		started:      false,
		fallback:     false,
		eof:          false,
		totalRead:    make([]byte, 0),
	}
}
