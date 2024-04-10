package utils

import (
	"io"
	"unicode"
)

// TrimLeftReader wraps an io.Reader and trims leading white space from the input.
type TrimLeftReader struct {
	r        io.Reader
	trimmed  bool
	buf      []byte
	bufStart int
	bufEnd   int
}

func NewTrimLeftReader(r io.Reader) *TrimLeftReader {
	return &TrimLeftReader{r: r}
}

// Read implements the io.Reader interface for TrimLeftReader.
func (t *TrimLeftReader) Read(p []byte) (int, error) {
	if !t.trimmed {
		t.trimmed = true // Assume trimming is done after the first Read call

		// Initialize buffer if necessary
		if t.buf == nil {
			t.buf = make([]byte, 4096)
		}

		// Read data into buffer
		n, err := t.r.Read(t.buf)
		if err != nil {
			return n, err
		}

		// Trim leading white space
		start := 0
		for start < n && unicode.IsSpace(rune(t.buf[start])) {
			start++
		}
		t.bufStart = start
		t.bufEnd = n
	}

	// Calculate how much data we can copy to p
	toCopy := minInt(len(p), t.bufEnd-t.bufStart)
	copy(p, t.buf[t.bufStart:t.bufStart+toCopy])
	t.bufStart += toCopy

	// If buffer is exhausted, reset trim state
	if t.bufStart >= t.bufEnd {
		t.trimmed = false
	}

	return toCopy, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
