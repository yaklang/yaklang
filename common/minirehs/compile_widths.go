package minirehs

import "regexp/syntax"

// Tests select the reference before compilation; published DBs do not read it.
var compileWidthFactsEnabled = true

// concatWidthSides computes the same bounded sibling sums as sumWidthRange.
// -1 represents its (0,false) result. Nonnegative bounded widths and a checked
// total make addition associative here; otherwise retain the original path.
// Only width facts are shared, never consuming-position occurrences.
func concatWidthSides(subs []*syntax.Regexp, storage []int) (prefix, suffix []int, ok bool) {
	k := len(subs)
	maxInt := int(^uint(0) >> 1)
	if k > maxInt/2-1 {
		return nil, nil, false
	}
	n := 2 * (k + 1)
	if len(storage) < n {
		storage = make([]int, n)
	}
	prefix, suffix = storage[:k+1:k+1], storage[k+1:n:n]
	prefix[0], suffix[k] = 0, 0
	total := 0
	var previous *syntax.Regexp
	var value int
	for i, sub := range subs {
		if i == 0 || sub != previous {
			w, bounded := maxByteWidth(sub)
			if bounded {
				if w < 0 {
					return nil, nil, false
				}
				value = w
			} else {
				value = -1
			}
			previous = sub
		}
		if value >= 0 {
			if value > maxInt-total {
				return nil, nil, false
			}
			total += value
		}
		suffix[i] = value // Child facts until the backwards suffix pass.
		if prefix[i] < 0 || value < 0 {
			prefix[i+1] = -1
		} else {
			prefix[i+1] = prefix[i] + value
		}
	}
	for i := k - 1; i >= 0; i-- {
		if suffix[i] < 0 || suffix[i+1] < 0 {
			suffix[i] = -1
		} else {
			suffix[i] += suffix[i+1]
		}
	}
	return prefix, suffix, true
}

func widthSide(v int) (int, bool) {
	if v < 0 {
		return 0, false
	}
	return v, true
}

func (a *litWindowAcc) walkConcatFacts(re *syntax.Regexp, pre int, preB bool, suf int, sufB bool) bool {
	var local [34]int // Both sides for up to16 siblings; no pointer-bearing rows.
	left, right, ok := concatWidthSides(re.Sub, local[:])
	if !ok {
		return false
	}
	for i, sub := range re.Sub {
		lw, lb := widthSide(left[i])
		rw, rb := widthSide(right[i+1])
		a.walk(sub, addSat(pre, lw), preB && lb, addSat(suf, rw), sufB && rb)
	}
	return true
}

func (a *biCoverAcc) walkConcatFacts(re *syntax.Regexp, pre int, preB bool, suf int, sufB bool) bool {
	var local [34]int
	left, right, ok := concatWidthSides(re.Sub, local[:])
	if !ok {
		return false
	}
	for i, sub := range re.Sub {
		lw, lb := widthSide(left[i])
		rw, rb := widthSide(right[i+1])
		a.walk(sub, addSat(pre, lw), preB && lb, addSat(suf, rw), sufB && rb)
	}
	return true
}
