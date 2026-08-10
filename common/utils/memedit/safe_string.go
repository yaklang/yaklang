package memedit

import (
	"bytes"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type SafeString struct {
	utf8Valid bool
	bytes     []byte

	// 缓存，懒加载。SafeString 可能被多个扫描 goroutine 共享（同一个
	// MemEditor 的 safeSourceCode），缓存写入必须并发安全，否则 -race 会报
	// data race（旧实现 isASCII/Len 直接写字段）。
	asciiState atomic.Int32           // 0=未检查, 1=ASCII, 2=非ASCII
	runeLen    atomic.Int64           // -1=未计算
	runes      atomic.Pointer[[]rune] // 非 ASCII 时惰性物化的 rune 切片
	runesOnce  sync.Once
}

func NewSafeString(i any) *SafeString {
	ss := &SafeString{}
	ss.runeLen.Store(-1)
	raw := codec.AnyToBytes(i)

	ss.bytes = raw
	ss.utf8Valid = utf8.Valid(raw)

	return ss
}

func (s *SafeString) isASCII() bool {
	if !s.utf8Valid {
		return false
	}
	if st := s.asciiState.Load(); st != 0 {
		return st == 1
	}
	only := true
	for _, b := range s.bytes {
		if b >= utf8.RuneSelf {
			only = false
			break
		}
	}
	st := int32(2)
	if only {
		st = 1
		s.runeLen.CompareAndSwap(-1, int64(len(s.bytes)))
	}
	s.asciiState.Store(st)
	return only
}

func (s *SafeString) runeIndexToByteOffset(idx int) int {
	if idx <= 0 {
		return 0
	}
	if idx >= s.Len() {
		return len(s.bytes)
	}
	if s.isASCII() {
		return idx
	}

	byteOffset := 0
	for i := 0; i < idx && byteOffset < len(s.bytes); i++ {
		_, size := utf8.DecodeRune(s.bytes[byteOffset:])
		byteOffset += size
	}
	return byteOffset
}

// 懒加载 runes
func (s *SafeString) ensureRunes() {
	if !s.utf8Valid || s.isASCII() {
		return
	}
	s.runesOnce.Do(func() {
		runeCount := s.Len()
		runes := make([]rune, runeCount)
		i := 0
		tempBytes := s.bytes
		for len(tempBytes) > 0 {
			r, size := utf8.DecodeRune(tempBytes)
			runes[i] = r
			i++
			tempBytes = tempBytes[size:]
		}
		s.runes.Store(&runes)
	})
}

func newSafeString(utf8Valid bool, bytes []byte, runeLen int, asciiState int32) *SafeString {
	ss := &SafeString{utf8Valid: utf8Valid, bytes: bytes}
	ss.runeLen.Store(int64(runeLen))
	ss.asciiState.Store(asciiState)
	return ss
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
		if s.isASCII() {
			return newSafeString(true, s.bytes[start:endIdx], endIdx-start, 1)
		}
		if endIdx == 0 {
			return newSafeString(true, []byte{}, 0, 0)
		}
		startByteIdx := s.runeIndexToByteOffset(start)
		endByteIdx := s.runeIndexToByteOffset(endIdx)

		return newSafeString(s.utf8Valid, s.bytes[startByteIdx:endByteIdx], endIdx-start, 0)
	}
	return newSafeString(s.utf8Valid, s.bytes[start:endIdx], -1, 0)
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
		startByteIdx := s.runeIndexToByteOffset(start)
		endByteIdx := s.runeIndexToByteOffset(endIdx)
		return string(s.bytes[startByteIdx:endByteIdx])
	}
	return string(s.bytes[start:endIdx])
}

func (s *SafeString) SliceBeforeStart(end int) string {
	if s.utf8Valid {
		if end > s.Len() {
			end = s.Len()
		}
		return string(s.bytes[:s.runeIndexToByteOffset(end)])
	}
	// 对于非 UTF-8，也需要边界检查
	if end > len(s.bytes) {
		end = len(s.bytes)
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
		if s.isASCII() {
			return rune(s.bytes[idx])
		}
		s.ensureRunes()
		if runes := s.runes.Load(); runes != nil {
			return (*runes)[idx]
		}
		return 0
	}
	return rune(s.bytes[idx])
}

func (s *SafeString) Runes() []rune {
	if s.utf8Valid {
		if s.isASCII() {
			return []rune(string(s.bytes))
		}
		s.ensureRunes()
		if runes := s.runes.Load(); runes != nil {
			return *runes
		}
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
		if n := s.runeLen.Load(); n >= 0 {
			return int(n)
		}
		if s.isASCII() {
			return int(s.runeLen.Load())
		}
		s.runeLen.CompareAndSwap(-1, int64(utf8.RuneCount(s.bytes)))
		return int(s.runeLen.Load())
	}
	return len(s.bytes)
}

func (s *SafeString) IndexString(what string) int {
	if s.utf8Valid {
		if s.isASCII() {
			return bytes.Index(s.bytes, []byte(what))
		}
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
		if s.isASCII() {
			buf := make([]byte, len(what))
			for i, r := range what {
				if r >= utf8.RuneSelf {
					return -1
				}
				buf[i] = byte(r)
			}
			return bytes.Index(s.bytes, buf)
		}
		s.ensureRunes()
		runes := s.runes.Load()
		if runes == nil {
			return -1
		}
		if len(what) > len(*runes) {
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
		for i < len(*runes) && j < len(what) {
			if j == -1 || (*runes)[i] == what[j] {
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
