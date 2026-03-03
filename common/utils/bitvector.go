package utils

import "math/bits"

type BitVector struct {
	words []uint64
}

func NewBitVector() *BitVector {
	return &BitVector{}
}

func (b *BitVector) Clone() *BitVector {
	if b == nil {
		return nil
	}
	dup := &BitVector{
		words: make([]uint64, len(b.words)),
	}
	copy(dup.words, b.words)
	return dup
}

func (b *BitVector) ensure(index int) {
	if b == nil || index < 0 {
		return
	}
	word := index >> 6
	if word < len(b.words) {
		return
	}
	grow := make([]uint64, word+1)
	copy(grow, b.words)
	b.words = grow
}

func (b *BitVector) Set(index int) {
	if b == nil || index < 0 {
		return
	}
	b.ensure(index)
	word := index >> 6
	bit := uint(index & 63)
	b.words[word] |= 1 << bit
}

func (b *BitVector) Has(index int) bool {
	if b == nil || index < 0 {
		return false
	}
	word := index >> 6
	if word >= len(b.words) {
		return false
	}
	bit := uint(index & 63)
	return (b.words[word] & (1 << bit)) != 0
}

func (b *BitVector) Or(other *BitVector) {
	if b == nil || other == nil {
		return
	}
	if len(other.words) > len(b.words) {
		grow := make([]uint64, len(other.words))
		copy(grow, b.words)
		b.words = grow
	}
	for i, word := range other.words {
		b.words[i] |= word
	}
}

func (b *BitVector) IsEmpty() bool {
	if b == nil {
		return true
	}
	for _, word := range b.words {
		if word != 0 {
			return false
		}
	}
	return true
}

func (b *BitVector) ForEach(handler func(index int)) {
	if b == nil || handler == nil {
		return
	}
	for wordIndex, word := range b.words {
		for word != 0 {
			lsb := bits.TrailingZeros64(word)
			handler((wordIndex << 6) + lsb)
			word &= word - 1
		}
	}
}
