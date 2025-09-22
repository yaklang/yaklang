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

	// 如果进入fallback模式，直接透传
	if r.fallback {
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
		r.buffer = append(r.buffer, tempBuf[:readCount]...)
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

func (r *jsonStringReader) processBuffer() bool {
	if len(r.buffer) == 0 {
		return false
	}

	processed := false
	i := 0

	for i < len(r.buffer) {
		char := r.buffer[i]

		switch r.state {
		case stateInit:
			// 跳过空白字符
			if char == ' ' || char == '\t' || char == '\r' || char == '\n' {
				i++
				continue
			}
			// 如果第一个非空字符不是双引号，进入fallback模式
			if char != '"' {
				r.fallback = true
				r.outputBuffer = append(r.outputBuffer, r.buffer...)
				r.buffer = r.buffer[:0]
				return true
			}
			// 开始解析JSON字符串
			r.state = stateInString
			r.started = true
			i++ // 跳过开始的双引号
			processed = true

		case stateInString:
			if char == '"' {
				// 结束字符串
				r.eof = true
				r.buffer = r.buffer[i+1:] // 保留剩余数据
				return true
			} else if char == '\\' {
				r.state = stateEscape
				i++
				processed = true
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
				// Unicode转义
				if i+4 < len(r.buffer) {
					// 检查后面4个字符是否都是十六进制
					hexStr := string(r.buffer[i+1 : i+5])
					if isHex(hexStr) {
						if codepoint, err := strconv.ParseUint(hexStr, 16, 16); err == nil {
							// 检查是否是代理对的高位 (0xD800-0xDBFF)
							if codepoint >= 0xD800 && codepoint <= 0xDBFF {
								// 这是代理对的高位，需要查找低位
								if i+10 < len(r.buffer) && r.buffer[i+5] == '\\' && r.buffer[i+6] == 'u' {
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
				} else {
					// 缓冲区不够，等待更多数据
					return processed
				}
			case 'x':
				// 十六进制转义 \x20
				if i+2 < len(r.buffer) {
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
				} else {
					// 缓冲区不够，等待更多数据
					return processed
				}
			default:
				// 无效转义序列，保留原字符
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
	}
}
