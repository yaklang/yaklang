package yakunquote

import (
	"strconv"
	"unicode/utf8"
)

func unhex(b byte) (v rune, ok bool) {
	c := rune(b)
	switch {
	case '0' <= c && c <= '9':
		return c - '0', true
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10, true
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10, true
	}
	return
}

// UnquoteChar decodes the first character or byte in the escaped string
// or character literal represented by the string s.
// It returns four values:
//
//  1. value, the decoded Unicode code point or byte value;
//  2. multibyte, a boolean indicating whether the decoded character requires a multibyte UTF-8 representation;
//  3. tail, the remainder of the string after the character; and
//  4. an error that will be nil if the character is syntactically valid.
//
// The second argument, quote, specifies the type of literal being parsed
// and therefore which escaped quote character is permitted.
// If set to a single quote, it permits the sequence \' and disallows unescaped '.
// If set to a double quote, it permits \" and disallows unescaped ".
// If set to zero, it does not permit either escape and allows both quote characters to appear unescaped.
func UnquoteChar(s string, quote byte) (value rune, multibyte bool, tail string, err error) {
	// easy cases
	if len(s) == 0 {
		err = strconv.ErrSyntax
		return
	}
	switch c := s[0]; {
	case c == quote && (quote == '\'' || quote == '"'):
		err = strconv.ErrSyntax
		return
	case c >= utf8.RuneSelf:
		r, size := utf8.DecodeRuneInString(s)
		return r, true, s[size:], nil
	case c != '\\':
		return rune(s[0]), false, s[1:], nil
	}

	// hard case: c is backslash
	if len(s) <= 1 {
		err = strconv.ErrSyntax
		return
	}
	c := s[1]
	s = s[2:]

	switch c {
	case 'a':
		value = '\a'
	case 'b':
		value = '\b'
	case 'f':
		value = '\f'
	case 'n':
		value = '\n'
	case 'r':
		value = '\r'
	case 't':
		value = '\t'
	case 'v':
		value = '\v'
	case 'x', 'u', 'U':
		n := 0
		switch c {
		case 'x':
			n = 2
		case 'u':
			n = 4
		case 'U':
			n = 8
		}
		var v rune
		if len(s) < n {
			err = strconv.ErrSyntax
			return
		}
		for j := 0; j < n; j++ {
			x, ok := unhex(s[j])
			if !ok {
				err = strconv.ErrSyntax
				return
			}
			v = v<<4 | x
		}
		s = s[n:]
		if c == 'x' {
			// single-byte string, possibly not UTF-8
			value = v
			break
		}
		if !utf8.ValidRune(v) {
			err = strconv.ErrSyntax
			return
		}
		value = v
		multibyte = true
	case '0', '1', '2', '3', '4', '5', '6', '7':
		v := rune(c) - '0'
		if len(s) < 2 {
			err = strconv.ErrSyntax
			return
		}
		for j := 0; j < 2; j++ { // one digit already; two more
			x := rune(s[j]) - '0'
			if x < 0 || x > 7 {
				err = strconv.ErrSyntax
				return
			}
			v = (v << 3) | x
		}
		s = s[2:]
		if v > 255 {
			err = strconv.ErrSyntax
			return
		}
		value = v
	case '\\':
		value = '\\'
	case '\'', '"', '`':
		value = rune(c)
	default:
		err = strconv.ErrSyntax
		return
	}
	tail = s
	return
}

func Unquote(str string) (string, error) {
	if len(str) < 2 {
		return "", strconv.ErrSyntax
	}
	quote := str[0]
	if quote != str[len(str)-1] {
		return "", strconv.ErrSyntax
	}

	return UnquoteInner(str[1:len(str)-1], quote)
}

func UnquoteInner(str string, quote byte) (string, error) {
	res := ""
	for {
		if len(str) <= 0 {
			break
		}
		c, _, rs, err := UnquoteChar(str, quote)
		str = rs
		if err != nil {
			return "", err
		}
		res += string(c)
	}
	return res, nil
}
