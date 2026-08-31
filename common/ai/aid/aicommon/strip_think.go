package aicommon

import (
	"bytes"
	"io"
	"strings"
	"sync"
)

const (
	thinkTagOpen  = "<think>"
	thinkTagClose = "</think>"
)

// StripThinkTags removes <think>...</think> blocks (case-insensitive open/close)
// from model output so action parsers see the @action JSON that often follows.
func StripThinkTags(s string) string {
	if s == "" || !strings.Contains(strings.ToLower(s), "think>") {
		return s
	}
	var out strings.Builder
	out.Grow(len(s))
	lower := strings.ToLower(s)
	i := 0
	for i < len(s) {
		open := strings.Index(lower[i:], thinkTagOpen)
		if open < 0 {
			out.WriteString(s[i:])
			break
		}
		open += i
		out.WriteString(s[i:open])
		closeRel := strings.Index(lower[open+len(thinkTagOpen):], thinkTagClose)
		if closeRel < 0 {
			// Unclosed think block: drop the remainder.
			break
		}
		i = open + len(thinkTagOpen) + closeRel + len(thinkTagClose)
		// Skip a single trailing newline after </think>.
		if i < len(s) && (s[i] == '\n' || s[i] == '\r') {
			i++
			if i < len(s) && s[i-1] == '\r' && s[i] == '\n' {
				i++
			}
		}
	}
	return out.String()
}

// NewStripThinkTagsReader wraps r and filters out <think>...</think> spans while
// streaming. Safe for use before ExtractActionFromStream.
func NewStripThinkTagsReader(r io.Reader) io.Reader {
	if r == nil {
		return bytes.NewReader(nil)
	}
	return &stripThinkTagsReader{r: r}
}

type stripThinkTagsReader struct {
	r   io.Reader
	mu  sync.Mutex
	buf []byte
	// carry holds incomplete tag prefixes across Read calls.
	carry  []byte
	inThink bool
}

func (s *stripThinkTagsReader) Read(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for len(s.buf) == 0 {
		tmp := make([]byte, 4096)
		n, err := s.r.Read(tmp)
		if n > 0 {
			chunk := append(s.carry, tmp[:n]...)
			s.carry = nil
			filtered, carry, inThink := filterThinkChunk(chunk, s.inThink)
			s.inThink = inThink
			s.carry = carry
			s.buf = append(s.buf, filtered...)
		}
		if err != nil {
			if len(s.carry) > 0 && !s.inThink {
				// Flush incomplete prefix that is not a think-tag start.
				s.buf = append(s.buf, s.carry...)
				s.carry = nil
			}
			if len(s.buf) == 0 {
				return 0, err
			}
			break
		}
		if len(s.buf) == 0 && n == 0 {
			continue
		}
	}

	n := copy(p, s.buf)
	s.buf = s.buf[n:]
	return n, nil
}

func filterThinkChunk(chunk []byte, inThink bool) (out, carry []byte, stillInThink bool) {
	i := 0
	for i < len(chunk) {
		if inThink {
			closeIdx := indexFold(chunk[i:], thinkTagClose)
			if closeIdx < 0 {
				// Keep a suffix that might be a partial close tag.
				keep := partialSuffixLen(chunk, thinkTagClose)
				if keep > 0 {
					return out, chunk[len(chunk)-keep:], true
				}
				return out, nil, true
			}
			i += closeIdx + len(thinkTagClose)
			inThink = false
			// Skip one trailing newline.
			if i < len(chunk) && (chunk[i] == '\n' || chunk[i] == '\r') {
				i++
				if i < len(chunk) && chunk[i-1] == '\r' && chunk[i] == '\n' {
					i++
				}
			}
			continue
		}
		openIdx := indexFold(chunk[i:], thinkTagOpen)
		if openIdx < 0 {
			keep := partialSuffixLen(chunk[i:], thinkTagOpen)
			if keep > 0 {
				out = append(out, chunk[i:len(chunk)-keep]...)
				return out, chunk[len(chunk)-keep:], false
			}
			out = append(out, chunk[i:]...)
			return out, nil, false
		}
		out = append(out, chunk[i:i+openIdx]...)
		i += openIdx + len(thinkTagOpen)
		inThink = true
	}
	return out, nil, inThink
}

func indexFold(b []byte, needle string) int {
	if len(needle) == 0 || len(b) < len(needle) {
		return -1
	}
	n := []byte(strings.ToLower(needle))
	for i := 0; i+len(n) <= len(b); i++ {
		match := true
		for j := 0; j < len(n); j++ {
			c := b[i+j]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != n[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func partialSuffixLen(b []byte, needle string) int {
	max := len(needle) - 1
	if max > len(b) {
		max = len(b)
	}
	lowerNeedle := strings.ToLower(needle)
	for n := max; n > 0; n-- {
		suffix := b[len(b)-n:]
		prefix := lowerNeedle[:n]
		ok := true
		for i := 0; i < n; i++ {
			c := suffix[i]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != prefix[i] {
				ok = false
				break
			}
		}
		if ok {
			return n
		}
	}
	return 0
}
