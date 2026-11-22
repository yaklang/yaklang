package match

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
)

// applyBufferTransforms applies Suricata sticky-buffer transforms in source order.
// When legacy callers only set dotPrefix without populating transforms, dotprefix
// is still applied for compatibility.
func applyBufferTransforms(buffer []byte, transforms []string, dotPrefix bool) []byte {
	if len(transforms) == 0 {
		if dotPrefix {
			return dotPrefixBuffer(buffer)
		}
		return buffer
	}
	hasDotPrefix := false
	cur := buffer
	for _, tr := range transforms {
		switch tr {
		case "dotprefix":
			hasDotPrefix = true
			cur = dotPrefixBuffer(cur)
		case "to_lowercase":
			cur = asciiLowerBytes(cur)
		case "to_uppercase":
			cur = asciiUpperBytes(cur)
		case "header_lowercase":
			cur = headerLowercaseBytes(cur)
		case "url_decode":
			cur = urlDecodeTransformBytes(cur)
		case "strip_whitespace":
			cur = stripWhitespaceBytes(cur)
		case "strip_pseudo_headers":
			cur = stripPseudoHeadersBytes(cur)
		case "to_sha1":
			if cur != nil {
				sum := sha1.Sum(cur)
				cur = sum[:]
			}
		case "to_sha256":
			if cur != nil {
				sum := sha256.Sum256(cur)
				cur = sum[:]
			}
		case "to_md5":
			if cur != nil {
				sum := md5.Sum(cur)
				cur = sum[:]
			}
		}
	}
	if dotPrefix && !hasDotPrefix {
		cur = dotPrefixBuffer(cur)
	}
	return cur
}

func asciiLowerBytes(src []byte) []byte {
	if len(src) == 0 {
		return src
	}
	needsCopy := false
	for _, b := range src {
		if b >= 'A' && b <= 'Z' {
			needsCopy = true
			break
		}
	}
	if !needsCopy {
		return src
	}
	dst := make([]byte, len(src))
	for i, b := range src {
		if b >= 'A' && b <= 'Z' {
			b += 'a' - 'A'
		}
		dst[i] = b
	}
	return dst
}

func asciiUpperBytes(src []byte) []byte {
	if len(src) == 0 {
		return src
	}
	needsCopy := false
	for _, b := range src {
		if b >= 'a' && b <= 'z' {
			needsCopy = true
			break
		}
	}
	if !needsCopy {
		return src
	}
	dst := make([]byte, len(src))
	for i, b := range src {
		if b >= 'a' && b <= 'z' {
			b -= 'a' - 'A'
		}
		dst[i] = b
	}
	return dst
}

// headerLowercaseBytes implements Suricata's header_lowercase transform:
// lowercases ASCII bytes in each header name (before the first ':') while
// preserving header values and line endings verbatim.
func headerLowercaseBytes(src []byte) []byte {
	if len(src) == 0 {
		return src
	}
	dst := make([]byte, len(src))
	copy(dst, src)
	inName := true
	for i, b := range dst {
		if b == '\n' {
			inName = true
			continue
		}
		if inName {
			if b == ':' {
				inName = false
				continue
			}
			if b >= 'A' && b <= 'Z' {
				dst[i] = b + ('a' - 'A')
			}
		}
	}
	return dst
}

func hexNibble(b byte) (byte, bool) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', true
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, true
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, true
	default:
		return 0, false
	}
}

// urlDecodeTransformBytes implements Suricata's url_decode transform:
// decodes valid %XX hex escapes and '+' -> ' ', keeping invalid '%' verbatim.
func urlDecodeTransformBytes(src []byte) []byte {
	if len(src) == 0 {
		return src
	}
	if bytes.IndexByte(src, '%') < 0 && bytes.IndexByte(src, '+') < 0 {
		return src
	}
	dst := make([]byte, 0, len(src))
	for i := 0; i < len(src); i++ {
		b := src[i]
		if b == '+' {
			dst = append(dst, ' ')
			continue
		}
		if b == '%' && i+2 < len(src) {
			hi, ok1 := hexNibble(src[i+1])
			lo, ok2 := hexNibble(src[i+2])
			if ok1 && ok2 {
				dst = append(dst, (hi<<4)|lo)
				i += 2
				continue
			}
		}
		dst = append(dst, b)
	}
	return dst
}

// stripWhitespaceBytes removes ASCII whitespace (space, \t, \r, \n, \v, \f).
func stripWhitespaceBytes(src []byte) []byte {
	if len(src) == 0 {
		return src
	}
	hasWS := false
	for _, b := range src {
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' || b == '\v' || b == '\f' {
			hasWS = true
			break
		}
	}
	if !hasWS {
		return src
	}
	dst := make([]byte, 0, len(src))
	for _, b := range src {
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' || b == '\v' || b == '\f' {
			continue
		}
		dst = append(dst, b)
	}
	return dst
}

// stripPseudoHeadersBytes removes HTTP/2 pseudo-header lines starting with ':'.
func stripPseudoHeadersBytes(src []byte) []byte {
	if len(src) == 0 {
		return src
	}
	// Fast path: if buffer contains no line starting with ':', return unchanged.
	if src[0] != ':' && !bytes.Contains(src, []byte("\n:")) {
		return src
	}
	dst := make([]byte, 0, len(src))
	for pos := 0; pos < len(src); {
		nl := bytes.IndexByte(src[pos:], '\n')
		var line []byte
		if nl < 0 {
			line = src[pos:]
			pos = len(src)
		} else {
			line = src[pos : pos+nl+1]
			pos += nl + 1
		}
		if len(line) > 0 && line[0] == ':' {
			continue
		}
		dst = append(dst, line...)
	}
	return dst
}
