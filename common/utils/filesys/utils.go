package filesys

import (
	"strings"
)

func SplitWithSeparator(path string, sep rune) (string, string) {
	if len(path) == 0 {
		return "", ""
	}
	idx := strings.LastIndex(path, string(sep))
	if idx == -1 {
		return "", path
	}
	return path[:idx], path[idx+1:]
}

func getExtension(path string) string {
	if len(path) == 0 {
		return ""
	}
	idx := strings.LastIndex(path, ".")
	if idx == -1 {
		return ""
	}
	return path[idx:]
}

func baseWithSeparators(path string, sep rune) string {
	if path == "" {
		return "."
	}
	for len(path) > 0 && path[len(path)-1] == byte(sep) {
		path = path[0 : len(path)-1]
	}
	if i := strings.LastIndex(path, string(sep)); i >= 0 {
		path = path[i+1:]
	}
	if path == "" {
		return string(sep)
	}
	return path
}

func joinWithSeparators(sep rune, elem ...string) string {
	size := 0
	for _, e := range elem {
		size += len(e)
	}
	if size == 0 {
		return ""
	}
	buf := make([]byte, 0, size+len(elem)-1)
	for _, e := range elem {
		if len(buf) > 0 || e != "" {
			if len(buf) > 0 {
				buf = append(buf, byte(sep))
			}
			buf = append(buf, e...)
		}
	}
	return cleanWithSeparators(string(buf), sep)
}

func cleanWithSeparators(path string, separators rune) string {
	if path == "" {
		return "."
	}

	sep := byte(separators)
	rooted := path[0] == sep
	n := len(path)

	out := lazybuf{s: path}
	r, dotdot := 0, 0
	if rooted {
		out.append(sep)
		r, dotdot = 1, 1
	}

	for r < n {
		switch {
		case path[r] == sep:
			// empty path element
			r++
		case path[r] == '.' && (r+1 == n || path[r+1] == sep):
			// . element
			r++
		case path[r] == '.' && path[r+1] == '.' && (r+2 == n || path[r+2] == sep):
			// .. element: remove to last /
			r += 2
			switch {
			case out.w > dotdot:
				// can backtrack
				out.w--
				for out.w > dotdot && out.index(out.w) != sep {
					out.w--
				}
			case !rooted:
				// cannot backtrack, but not rooted, so append .. element.
				if out.w > 0 {
					out.append(sep)
				}
				out.append('.')
				out.append('.')
				dotdot = out.w
			}
		default:
			// real path element.
			// add slash if needed
			if rooted && out.w != 1 || !rooted && out.w != 0 {
				out.append(sep)
			}
			// copy element
			for ; r < n && path[r] != sep; r++ {
				out.append(path[r])
			}
		}
	}

	// Turn empty string into "."
	if out.w == 0 {
		return "."
	}

	return out.string()
}

// A lazybuf is a lazily constructed path buffer.
// It supports append, reading previously appended bytes,
// and retrieving the final string. It does not allocate a buffer
// to hold the output until that output diverges from s.
type lazybuf struct {
	s   string
	buf []byte
	w   int
}

func (b *lazybuf) index(i int) byte {
	if b.buf != nil {
		return b.buf[i]
	}
	return b.s[i]
}

func (b *lazybuf) append(c byte) {
	if b.buf == nil {
		if b.w < len(b.s) && b.s[b.w] == c {
			b.w++
			return
		}
		b.buf = make([]byte, len(b.s))
		copy(b.buf, b.s[:b.w])
	}
	b.buf[b.w] = c
	b.w++
}

func (b *lazybuf) string() string {
	if b.buf == nil {
		return b.s[:b.w]
	}
	return string(b.buf[:b.w])
}
