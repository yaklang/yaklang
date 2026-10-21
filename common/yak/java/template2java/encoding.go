package template2java

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

// Template sources are not always UTF-8: legacy Java web applications are full of GBK/GB2312
// JSP pages (the Coremail case: `pageEncoding="GBK"`), and their bytes are not valid UTF-8.
// Feeding those bytes straight into the generated Java produced literals like "\xb0\xb2",
// which the Java parser rejects ("token recognition error at: '\'"), so the whole page failed
// to compile. Detect the declared charset, decode once, and emit Java-legal escapes.

var (
	templatePageEncodingRe = regexp.MustCompile(`(?i)pageEncoding\s*=\s*["']([A-Za-z0-9._-]+)["']`)
	templateCharsetRe      = regexp.MustCompile(`(?i)charset\s*=\s*["']?\s*([A-Za-z0-9._-]+)`)
)

// templateEncodingFromSource returns the charset declared in a template header, or nil for
// UTF-8 / unknown declarations. Detection is intentionally conservative: it only trusts an
// explicit declaration and never guesses from the body, so a UTF-8 file is never re-decoded.
func templateEncodingFromSource(src string) encoding.Encoding {
	// The header is ASCII; scanning a bounded prefix keeps this cheap on large pages.
	head := src
	if len(head) > 4096 {
		head = head[:4096]
	}
	name := ""
	if m := templatePageEncodingRe.FindStringSubmatch(head); len(m) > 1 {
		name = m[1]
	} else if m := templateCharsetRe.FindStringSubmatch(head); len(m) > 1 {
		name = m[1]
	}
	return templateEncodingByName(name)
}

func templateEncodingByName(name string) encoding.Encoding {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "utf-8", "utf8", "us-ascii", "ascii":
		return nil
	case "gbk", "cp936", "ms936", "windows-936":
		return simplifiedchinese.GBK
	case "gb2312", "gb-2312", "gb18030", "gb-18030":
		return simplifiedchinese.GB18030
	case "big5", "big-5", "cp950":
		return traditionalchinese.Big5
	case "shift_jis", "shift-jis", "sjis", "ms_kanji", "windows-31j", "cp932":
		return japanese.ShiftJIS
	case "euc-jp", "eucjp":
		return japanese.EUCJP
	case "euc-kr", "euckr", "cp949":
		return korean.EUCKR
	case "iso-8859-1", "iso8859-1", "latin1", "latin-1":
		return charmap.ISO8859_1
	case "windows-1252", "cp1252":
		return charmap.Windows1252
	default:
		return nil
	}
}

// decodeTemplateText converts text to UTF-8 using enc. A nil encoding is a passthrough; a
// decode error keeps the original text rather than dropping the page.
func decodeTemplateText(text string, enc encoding.Encoding) string {
	if enc == nil || text == "" {
		return text
	}
	if utf8.ValidString(text) && enc == nil {
		return text
	}
	decoded, err := enc.NewDecoder().String(text)
	if err != nil {
		return text
	}
	return decoded
}

// javaStringLiteral renders text as the *body* of a Java string literal (no surrounding
// quotes). It differs from Go's %q in one load-bearing way: Go emits \xNN for bytes that are
// not valid UTF-8, and Java has no \x escape, so the generated source would not parse. Here
// every non-ASCII or invalid byte becomes a legal Java escape.
func javaStringLiteral(text string) string {
	var b strings.Builder
	b.Grow(len(text) + 8)
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && size == 1 {
			// Undecodable byte (an undeclared legacy encoding, or a truncated sequence):
			// keep it as a Latin-1 code unit so the literal stays compilable.
			writeJavaUnicodeEscape(&b, rune(text[i]))
			i++
			continue
		}
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if r < 0x20 || r == 0x7f || r > 0x7e {
				writeJavaUnicodeEscape(&b, r)
			} else {
				b.WriteRune(r)
			}
		}
		i += size
	}
	return b.String()
}

func writeJavaUnicodeEscape(b *strings.Builder, r rune) {
	const hex = "0123456789abcdef"
	if r > 0xffff {
		// Java string literals carry UTF-16 code units: emit a surrogate pair.
		r -= 0x10000
		hi := 0xd800 + (r >> 10)
		lo := 0xdc00 + (r & 0x3ff)
		writeJavaUnicodeEscape(b, hi)
		writeJavaUnicodeEscape(b, lo)
		return
	}
	b.WriteString(`\u`)
	b.WriteByte(hex[(r>>12)&0xf])
	b.WriteByte(hex[(r>>8)&0xf])
	b.WriteByte(hex[(r>>4)&0xf])
	b.WriteByte(hex[r&0xf])
}
