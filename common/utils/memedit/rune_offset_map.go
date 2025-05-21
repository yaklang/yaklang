package memedit

import (
	"sort"
	"unicode/utf8"
)

// RuneOffsetMap 存储字符串的 rune 到字节偏移的映射关系
type RuneOffsetMap struct {
	s       string // 原始字符串
	offsets []int  // 每个 rune 的起始字节偏移
}

// NewRuneOffsetMap 创建新的 RuneOffsetMap 并预计算偏移表
func NewRuneOffsetMap(s string) *RuneOffsetMap {
	offsets := make([]int, 0, len(s))
	bytePos := 0
	for _, r := range s {
		offsets = append(offsets, bytePos)
		bytePos += utf8.RuneLen(r)
	}
	return &RuneOffsetMap{s: s, offsets: offsets}
}

// RuneIndexToByteOffset 将 rune 索引转换为字节偏移
func (m *RuneOffsetMap) RuneIndexToByteOffset(runeIndex int) (int, bool) {
	if runeIndex < 0 || runeIndex >= len(m.offsets) {
		return 0, false
	}
	return m.offsets[runeIndex], true
}

// ByteOffsetToRuneIndex 将字节偏移转换为 rune 索引
func (m *RuneOffsetMap) ByteOffsetToRuneIndex(byteOffset int) (int, bool) {
	// 检查偏移是否超出字符串范围
	if byteOffset < 0 || byteOffset >= len(m.s) {
		return 0, false
	}

	// 二分查找第一个大于 byteOffset 的偏移位置
	index := sort.Search(len(m.offsets), func(i int) bool {
		return m.offsets[i] > byteOffset
	})

	if index == 0 {
		return 0, false // 偏移量小于第一个 rune 的起始位置
	}
	return index - 1, true
}

// RuneCount 返回字符串中的 rune 总数
func (m *RuneOffsetMap) RuneCount() int {
	return len(m.offsets)
}

// String 返回原始字符串
func (m *RuneOffsetMap) String() string {
	return m.s
}
