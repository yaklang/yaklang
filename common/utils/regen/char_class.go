package regen

import (
	"fmt"
	"math/rand"

	"github.com/pkg/errors"
)

type tCharClass struct {
	Ranges    []*tCharClassRange
	TotalSize int32
}

type tCharClassRange struct {
	Start rune
	Size  int32
}

func newCharClass(start rune, end rune) (*tCharClass, error) {
	charRange, err := newCharClassRange(start, end)
	if err != nil {
		return nil, err
	}
	return &tCharClass{
		Ranges:    []*tCharClassRange{charRange},
		TotalSize: charRange.Size,
	}, nil
}

/*
ParseCharClass parses a character class as represented by syntax.Parse into a slice of CharClassRange structs.

Char classes are encoded as pairs of runes representing ranges:
[0-9] = 09, [a0] = aa00 (2 1-len ranges).

e.g.

"[a0-9]" -> "aa09" -> a, 0-9

"[^a-z]" -> "…" -> 0-(a-1), (z+1)-(max rune)
*/
func parseCharClass(runes []rune) (*tCharClass, error) {
	var totalSize int32
	numRanges := len(runes) / 2
	ranges := make([]*tCharClassRange, numRanges, numRanges)

	for i := 0; i < numRanges; i++ {
		start := runes[i*2]
		end := runes[i*2+1]

		if start == 0 {
			start = 1
		}

		r, err := newCharClassRange(start, end)
		if err != nil {
			return nil, err
		}

		ranges[i] = r
		totalSize += r.Size
	}

	return &tCharClass{ranges, totalSize}, nil
}

func (class *tCharClass) filterVisible() *tCharClass {
	var filteredRanges []*tCharClassRange
	var totalSize int32

	// Loop over each range in the character class
	for _, r := range class.Ranges {
		start := maxRune(r.Start, 32)               // Visible characters start from rune value 32
		end := minRune(r.Start+rune(r.Size-1), 126) // ... and end at rune value 126

		if start <= end { // Ensure the filtered range is valid
			size := end - start + 1
			filteredRanges = append(filteredRanges, &tCharClassRange{start, int32(size)})
			totalSize += int32(size)
		}
	}

	return &tCharClass{filteredRanges, totalSize}
}

// Helper functions
func minRune(a, b rune) rune {
	if a < b {
		return a
	}
	return b
}

func maxRune(a, b rune) rune {
	if a > b {
		return a
	}
	return b
}

func (class *tCharClass) GetAllRune() []rune {
	var runes []rune
	for _, r := range class.Ranges {
		for i := r.Start; i <= r.Start+rune(r.Size-1); i++ {
			runes = append(runes, i)
		}
	}
	return runes
}

func (class *tCharClass) GetAllRuneAsString() []string {
	var runes []string
	for _, r := range class.Ranges {
		for i := r.Start; i <= r.Start+rune(r.Size-1); i++ {
			runes = append(runes, string(i))
		}
	}
	return runes
}

func (class *tCharClass) GetOneRuneAsString() []string {
	var runes []string
	if len(class.Ranges) <= 0 {
		return nil
	}
	target := rand.Intn(len(class.Ranges))
	for rIndex, r := range class.Ranges {
		if target == rIndex {
			if r.Size-1 > 0 {
				randRune := rand.Intn(int(r.Size - 1))
				return []string{string(r.Start + rune(randRune))}
			} else {
				return []string{string(r.Start)}
			}
		}
	}
	return runes
}

func (class *tCharClass) GetVisibleOneRuneAsString() []string {
	visibleClass := class.filterVisible()
	return visibleClass.GetOneRuneAsString()
}

func (class *tCharClass) String() string {
	return fmt.Sprintf("%s", class.Ranges)
}

func newCharClassRange(start rune, end rune) (*tCharClassRange, error) {
	if start < 1 {
		return nil, errors.New("char class range cannot contain runes less than 1")
	}

	size := end - start + 1

	if size < 1 {
		return nil, errors.New("char class range size must be at least 1")
	}

	return &tCharClassRange{
		Start: start,
		Size:  size,
	}, nil
}

func (r tCharClassRange) String() string {
	if r.Size == 1 {
		return fmt.Sprintf("%s:1", runesToString(r.Start))
	}
	return fmt.Sprintf("%s-%s:%d", runesToString(r.Start), runesToString(r.Start+rune(r.Size-1)), r.Size)

}
