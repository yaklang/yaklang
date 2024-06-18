package memedit

import (
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"unicode/utf8"
)

type SafeString struct {
	utf8Valid bool

	runes []rune
	bytes []byte
}

func NewSafeString(i any) *SafeString {
	ss := &SafeString{}
	raw := codec.AnyToBytes(i)
	if utf8.Valid(raw) {
		ss.utf8Valid = true
		ss.runes = []rune(string(raw))
	} else {
		ss.bytes = raw
	}
	return ss
}

func (s *SafeString) Slice2(start, end int) string {
	if s.utf8Valid {
		return string(s.runes[start:end])
	}
	return string(s.bytes[start:end])
}

func (s *SafeString) SliceToEnd(start int) string {
	if s.utf8Valid {
		return string(s.runes[start:])
	}
	return string(s.bytes[start:])
}

func (s *SafeString) SliceBeforeStart(end int) string {
	if s.utf8Valid {
		return string(s.runes[:end])
	}
	return string(s.bytes[:end])
}

func (s *SafeString) Slice1(idx int) string {
	if s.utf8Valid {
		return string(s.runes[idx])
	}
	return string([]byte{s.bytes[idx]})
}

func (s *SafeString) String() string {
	if s.utf8Valid {
		return string(s.runes)
	}
	return string(s.bytes)
}

func (s *SafeString) Len() int {
	if s.utf8Valid {
		return len(s.runes)
	}
	return len(s.bytes)
}
