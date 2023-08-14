package surigen

import (
	"crypto/rand"
	"math"
)

// MAXLEN 1MB
const MAXLEN = 1 << 20

type ByteMap struct {
	mp []byte
	// optimize with bitmap
	filled []bool

	lastPos int
	lastLen int
}

func NewByteMap(Len int) *ByteMap {
	if Len < 0 {
		return nil
	}
	if Len > MAXLEN {
		Len = MAXLEN
	}
	return &ByteMap{
		mp:     make([]byte, Len),
		filled: make([]bool, Len),
	}
}

// Resize resize the bytemap to size
// the original data will be copied
func (m *ByteMap) Resize(size int) {
	if size < 0 {
		return
	}
	mp := make([]byte, size)
	filled := make([]bool, size)
	copy(mp, m.mp)
	copy(filled, m.filled)
	m.mp = mp
	m.filled = filled
}

// Fill without check if filled
// if content is too large ByteMap will be resized to fit in
func (m *ByteMap) Fill(offset int, content []byte) {
	if len(content)+offset > len(m.mp) {
		m.Resize(1 << (math.Ilogb(float64(len(content)+offset)) + 1))
	}
	copy(m.mp[offset:], content)
	for i := 0; i < len(content); i++ {
		m.filled[offset+i] = true
	}

	m.lastPos = offset
	m.lastLen = len(content)
}

// FillLeftWithNoise fill the left empty space with noise
func (m *ByteMap) FillLeftWithNoise() {
	noise := make([]byte, len(m.mp))
	rand.Read(noise)
	for i := 0; i < len(m.mp); i++ {
		if m.filled[i] {
			continue
		}
		m.mp[i] = noise[i]
		m.filled[i] = true
	}
}

// FindFree find the first free space with Len
func (m *ByteMap) FindFree(Len int) int {
	emptyLen := 0
	for i := 0; i < len(m.mp); i++ {
		if m.filled[i] {
			emptyLen = 0
			continue
		}
		emptyLen++
		if emptyLen == Len {
			return i - Len + 1
		}
	}
	return -1
}

// FindFreeAfter find the first free space with Len after offset
func (m *ByteMap) FindFreeAfter(Len int, offset int) int {
	emptyLen := 0
	for i := offset + 1; i < len(m.mp); i++ {
		if m.filled[i] {
			emptyLen = 0
			continue
		}
		emptyLen++
		if emptyLen == Len {
			return i - Len + 1
		}
	}
	return -1
}

// FindFreeRange find all free spaces with Len in range [begin,end)
func (m *ByteMap) FindFreeRange(Len, begin, end int) []int {
	emptyLen := 0
	var res []int
	if len(m.mp) < end {
		end = len(m.mp)
	}
	if len(m.mp)-1 < begin {
		return res
	}
	for i := begin; i < end; i++ {
		if m.filled[i] {
			emptyLen = 0
			continue
		}
		emptyLen++
		if emptyLen >= Len {
			res = append(res, i-Len+1)
		}
	}
	return res
}

func (m *ByteMap) Bytes() []byte {
	return m.mp
}

func (m *ByteMap) Test(offset int) bool {
	return m.filled[offset]
}

func (m *ByteMap) Size() int {
	return len(m.mp)
}
