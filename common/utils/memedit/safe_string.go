package memedit

import (
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type SafeString struct {
	utf8Valid bool
	bytes     []byte

	// 缓存，懒加载
	runes []rune
}

func NewSafeString(i any) *SafeString {
	ss := &SafeString{}
	raw := codec.AnyToBytes(i)

	ss.bytes = raw
	ss.utf8Valid = utf8.Valid(raw)

	return ss
}

// 懒加载 runes
func (s *SafeString) ensureRunes() {
	if s.runes == nil && s.utf8Valid {
		runeCount := utf8.RuneCount(s.bytes)
		s.runes = make([]rune, runeCount)
		i := 0
		tempBytes := s.bytes
		for len(tempBytes) > 0 {
			r, size := utf8.DecodeRune(tempBytes)
			s.runes[i] = r
			i++
			tempBytes = tempBytes[size:]
		}
	}
}

// SafeSlice 切片操作，end为可选参数
// 如果只传入start，则切片到末尾
// 如果传入start和end，则切片[start:end]
func (s *SafeString) SafeSlice(start int, end ...int) *SafeString {
	var endIdx int
	if len(end) > 0 {
		endIdx = end[0]
	} else {
		endIdx = s.Len()
	}

	if s.utf8Valid {
		s.ensureRunes()
		// 对于UTF-8有效的字符串，我们需要计算字节范围
		if len(s.runes) == 0 {
			return &SafeString{
				utf8Valid: true,
				bytes:     []byte{},
			}
		}

		// 计算字节切片范围
		var startByteIdx, endByteIdx int
		if start == 0 {
			startByteIdx = 0
		} else {
			startByteIdx = len([]byte(string(s.runes[:start])))
		}

		if endIdx >= len(s.runes) {
			endByteIdx = len(s.bytes)
		} else {
			endByteIdx = len([]byte(string(s.runes[:endIdx])))
		}

		return &SafeString{
			utf8Valid: s.utf8Valid,
			bytes:     s.bytes[startByteIdx:endByteIdx],
		}
	}
	return &SafeString{
		utf8Valid: s.utf8Valid,
		bytes:     s.bytes[start:endIdx],
	}
}

// Slice 返回字符串切片，end为可选参数
func (s *SafeString) Slice(start int, end ...int) string {
	var endIdx int
	if len(end) > 0 {
		endIdx = end[0]
	} else {
		endIdx = s.Len()
	}

	if s.utf8Valid {
		s.ensureRunes()
		return string(s.runes[start:endIdx])
	}
	return string(s.bytes[start:endIdx])
}

func (s *SafeString) SliceBeforeStart(end int) string {
	if s.utf8Valid {
		s.ensureRunes()
		return string(s.runes[:end])
	}
	return string(s.bytes[:end])
}

func (s *SafeString) Slice1(idx int) rune {
	if idx < 0 {
		return 0
	}

	if idx >= s.Len() {
		return 0
	}

	if s.utf8Valid {
		s.ensureRunes()
		return s.runes[idx]
	}
	return rune(s.bytes[idx])
}

func (s *SafeString) Runes() []rune {
	if s.utf8Valid {
		s.ensureRunes()
		return s.runes
	}
	return []rune(string(s.bytes))
}

func (s *SafeString) Bytes() []byte {
	if s.utf8Valid {
		return s.bytes
	}
	return s.bytes
}

func (s *SafeString) String() string {
	return string(s.bytes)
}

func (s *SafeString) Len() int {
	if s.utf8Valid {
		return utf8.RuneCount(s.bytes)
	}
	return len(s.bytes)
}

func (s *SafeString) IndexString(what string) int {
	if s.utf8Valid {
		return s.Index([]rune(what))
	} else {
		// 对于非UTF-8有效的字节，使用字节搜索
		whatBytes := []byte(what)
		if len(whatBytes) == 0 {
			return 0
		}
		if len(whatBytes) > len(s.bytes) {
			return -1
		}

		// 简单的字节搜索
		for i := 0; i <= len(s.bytes)-len(whatBytes); i++ {
			found := true
			for j := 0; j < len(whatBytes); j++ {
				if s.bytes[i+j] != whatBytes[j] {
					found = false
					break
				}
			}
			if found {
				return i
			}
		}
		return -1
	}
}

func (s *SafeString) Index(what []rune) int {
	if len(what) == 0 {
		return 0
	}

	if s.utf8Valid {
		s.ensureRunes()
		if len(what) > len(s.runes) {
			return -1
		}

		// 使用KMP算法匹配字符串
		// 构建next数组
		next := make([]int, len(what))
		next[0] = -1
		i, j := 0, -1
		for i < len(what)-1 {
			if j == -1 || what[i] == what[j] {
				i++
				j++
				next[i] = j
			} else {
				j = next[j]
			}
		}

		// 搜索
		i, j = 0, 0
		for i < len(s.runes) && j < len(what) {
			if j == -1 || s.runes[i] == what[j] {
				i++
				j++
			} else {
				j = next[j]
			}
		}
		if j == len(what) {
			return i - j
		}
		return -1
	} else {
		// 对于非UTF-8有效的字节，转换为字符串进行搜索
		whatStr := string(what)
		return s.IndexString(whatStr)
	}
}

// 向后兼容的方法别名
func (s *SafeString) SafeSlice2(start, end int) *SafeString {
	return s.SafeSlice(start, end)
}

func (s *SafeString) Slice2(start, end int) string {
	return s.Slice(start, end)
}

func (s *SafeString) SafeSliceToEnd(start int) *SafeString {
	return s.SafeSlice(start)
}

func (s *SafeString) SliceToEnd(start int) string {
	return s.Slice(start)
}
