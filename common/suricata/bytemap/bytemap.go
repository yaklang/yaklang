package bytemap

import (
	"math"
	"math/rand"
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
func (m *ByteMap) Fill(offsetList []int, content []byte) {
	offset := offsetList[rand.Intn(len(offsetList))]
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
func (m *ByteMap) FillLeftWithNoise(noise func() byte) {
	for i := 0; i < len(m.mp); i++ {
		if m.filled[i] {
			continue
		}
		m.mp[i] = noise()
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
		if i < 0 {
			println()
		}
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

func (m *ByteMap) Trim(trimleft bool, trimright bool, targetLen int) {
	// [lpos,rpos)
	lpos := 0
	rpos := len(m.mp)

	if trimright {
		for i := len(m.mp) - 1; i >= 0; i-- {
			if m.filled[i] {
				break
			}
			rpos--
		}
	}
	if trimleft {
		for i := 0; i < len(m.mp); i++ {
			if m.filled[i] {
				break
			}
			lpos++
		}
	}
	//
	//if rpos-lpos < targetLen {
	//	remain := targetLen - (lpos - rpos)
	//	lremain := lpos
	//	rremain := len(m.mp) - rpos
	//	minremain := utils.Min(lremain, rremain)
	//	if remain <= 2*minremain {
	//		lpos = lpos - remain/2
	//		rpos = rpos + remain - remain/2
	//	} else if lremain < rremain {
	//		lpos = 0
	//		rpos = rpos + (remain - lremain)
	//	} else if lremain >= rremain {
	//		lpos = lpos - (remain - rremain)
	//		rpos = len(m.mp)
	//	}
	//}

	m.filled = m.filled[lpos:rpos]
	m.mp = m.mp[lpos:rpos]
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

// Last is lastpos and lastLen
func (m *ByteMap) Last() (int, int) {
	return m.lastPos, m.lastLen
}
