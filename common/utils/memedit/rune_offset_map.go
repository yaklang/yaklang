package memedit

import (
	"sort"
	"unicode/utf8"
)

// RuneOffsetMap 存储字符串的 rune 到字节偏移的映射关系。
//
// s 由调用方传入；热路径（MemEditor.GetRuneOffsetMap）传入的是
// GetSourceCodeUnsafe 的零拷贝别名（alias safeSourceCode.bytes），所以保留
// s 不会额外拷贝整文件源码，且 map memoize 在 editor 上、editor 持有
// safeSourceCode，生命周期一致。
type RuneOffsetMap struct {
	s string // 原始字符串（热路径下是 safeSourceCode.bytes 的零拷贝别名）
	// small 存储每个 rune 的起始字节偏移。绝大多数源码文件小于 4GiB，
	// uint32 足够表示所有偏移，内存占用是 []int 的一半（Hadoop 扫描期
	// 常驻 RuneOffsetMap 约 1.2GB，可省约 600MB）。
	small []uint32
	// wide 是 >4GiB 输入的兜底表示，避免 uint32 溢出。
	wide []int
}

// NewRuneOffsetMap 创建新的 RuneOffsetMap 并预计算偏移表。
// 注意：为避免大文件整串拷贝，热路径应传入零拷贝别名（如
// MemEditor.GetSourceCodeUnsafe 的返回值），并配合 MemEditor.GetRuneOffsetMap
// memoize，不要每次调用都重建。
func NewRuneOffsetMap(s string) *RuneOffsetMap {
	const maxUint32 = uint64(1<<32 - 1)
	runeCount := utf8.RuneCountInString(s)
	if uint64(len(s)) > maxUint32 {
		offsets := make([]int, 0, runeCount)
		bytePos := 0
		for _, r := range s {
			offsets = append(offsets, bytePos)
			bytePos += utf8.RuneLen(r)
		}
		return &RuneOffsetMap{s: s, wide: offsets}
	}

	offsets := make([]uint32, 0, runeCount)
	bytePos := 0
	for _, r := range s {
		offsets = append(offsets, uint32(bytePos))
		bytePos += utf8.RuneLen(r)
	}
	return &RuneOffsetMap{s: s, small: offsets}
}

// RuneIndexToByteOffset 将 rune 索引转换为字节偏移
func (m *RuneOffsetMap) RuneIndexToByteOffset(runeIndex int) (int, bool) {
	if m == nil || runeIndex < 0 || runeIndex >= m.RuneCount() {
		return 0, false
	}
	if m.small != nil {
		return int(m.small[runeIndex]), true
	}
	return m.wide[runeIndex], true
}

// ByteOffsetToRuneIndex 将字节偏移转换为 rune 索引
func (m *RuneOffsetMap) ByteOffsetToRuneIndex(byteOffset int) (int, bool) {
	// 检查偏移是否超出字符串范围
	if m == nil || byteOffset < 0 || byteOffset >= len(m.s) {
		return 0, false
	}

	// 二分查找第一个大于 byteOffset 的偏移位置
	count := m.RuneCount()
	index := sort.Search(count, func(i int) bool {
		if m.small != nil {
			return int(m.small[i]) > byteOffset
		}
		return m.wide[i] > byteOffset
	})

	if index == 0 {
		return 0, false // 偏移量小于第一个 rune 的起始位置
	}
	return index - 1, true
}

// RuneCount 返回字符串中的 rune 总数
func (m *RuneOffsetMap) RuneCount() int {
	if m == nil {
		return 0
	}
	if m.small != nil {
		return len(m.small)
	}
	return len(m.wide)
}

// String 返回原始字符串
func (m *RuneOffsetMap) String() string {
	return m.s
}
