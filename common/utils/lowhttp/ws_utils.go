package lowhttp

import (
	"unicode/utf8"
)

const (
	maskx = 0b00111111
	mask2 = 0b00011111
	mask3 = 0b00001111
	mask4 = 0b00000111

	// The default lowest and highest continuation byte.
	locb = 0b10000000
	hicb = 0b10111111

	// These names of these constants are chosen to give nice alignment in the
	// table below. The first nibble is an index into acceptRanges or F for
	// special one-byte cases. The second nibble is the Rune length or the
	// Status for the special one-byte case.
	xx = 0xF1 // invalid: size 1
	as = 0xF0 // ASCII: size 1
	s1 = 0x02 // accept 0, size 2
	s2 = 0x13 // accept 1, size 3
	s3 = 0x03 // accept 0, size 3
	s4 = 0x23 // accept 2, size 3
	s5 = 0x34 // accept 3, size 4
	s6 = 0x04 // accept 0, size 4
	s7 = 0x44 // accept 4, size 4
)

type acceptRange struct {
	lo uint8 // lowest value for second byte.
	hi uint8 // highest value for second byte.
}

var acceptRanges = [16]acceptRange{
	0: {locb, hicb},
	1: {0xA0, hicb},
	2: {locb, 0x9F},
	3: {0x90, hicb},
	4: {locb, 0x8F},
}

// first is information about the first byte in a UTF-8 sequence.
var first = [256]uint8{
	//   1   2   3   4   5   6   7   8   9   A   B   C   D   E   F
	as, as, as, as, as, as, as, as, as, as, as, as, as, as, as, as, // 0x00-0x0F
	as, as, as, as, as, as, as, as, as, as, as, as, as, as, as, as, // 0x10-0x1F
	as, as, as, as, as, as, as, as, as, as, as, as, as, as, as, as, // 0x20-0x2F
	as, as, as, as, as, as, as, as, as, as, as, as, as, as, as, as, // 0x30-0x3F
	as, as, as, as, as, as, as, as, as, as, as, as, as, as, as, as, // 0x40-0x4F
	as, as, as, as, as, as, as, as, as, as, as, as, as, as, as, as, // 0x50-0x5F
	as, as, as, as, as, as, as, as, as, as, as, as, as, as, as, as, // 0x60-0x6F
	as, as, as, as, as, as, as, as, as, as, as, as, as, as, as, as, // 0x70-0x7F
	//   1   2   3   4   5   6   7   8   9   A   B   C   D   E   F
	xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, // 0x80-0x8F
	xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, // 0x90-0x9F
	xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, // 0xA0-0xAF
	xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, // 0xB0-0xBF
	xx, xx, s1, s1, s1, s1, s1, s1, s1, s1, s1, s1, s1, s1, s1, s1, // 0xC0-0xCF
	s1, s1, s1, s1, s1, s1, s1, s1, s1, s1, s1, s1, s1, s1, s1, s1, // 0xD0-0xDF
	s2, s3, s3, s3, s3, s3, s3, s3, s3, s3, s3, s3, s3, s4, s3, s3, // 0xE0-0xEF
	s5, s6, s6, s6, s7, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, xx, // 0xF0-0xFF
}

func checkRune(p []byte) bool {
	p0 := p[0]
	x := first[p0]
	if x >= as {
		return true
	}
	sz := int(x & 7)
	lenOfP := len(p)
	if lenOfP > 1 && sz > 1 {
		accept := acceptRanges[x>>4]
		b1 := p[1]
		if b1 < accept.lo || accept.hi < b1 {
			return false
		}
	}
	if lenOfP > 2 && sz > 2 {
		b2 := p[2]
		if b2 < locb || hicb < b2 {
			return false
		}
	}
	if lenOfP > 3 && sz > 3 {
		b3 := p[3]
		if b3 < locb || hicb < b3 {
			return false
		}
	}
	return true
}

func IsValidUTF8WithRemind(p []byte) (valid bool, remindSize int) {
	if utf8.Valid(p) {
		return true, 0
	}
	end := len(p)
	if end == 0 {
		return false, -1
	}
	lastStart := end - 1

	for ; lastStart > 0; lastStart-- {
		if utf8.RuneStart(p[lastStart]) {
			break
		}
	}
	shouldSize := 0

	i := uint32(p[lastStart])
	if i&0b11111000 == 0b11110000 {
		shouldSize = 4
	} else if i&0b11110000 == 0b11100000 {
		shouldSize = 3
	} else if i&0b11100000 == 0b11000000 {
		shouldSize = 2
	} else {
		shouldSize = 1
	}

	remindSize = end - lastStart
	if remindSize > shouldSize {
		return false, remindSize
	}

	// check last rune
	return checkRune(p[lastStart:]), remindSize
}
