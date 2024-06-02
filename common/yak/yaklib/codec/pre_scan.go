package codec

import (
	"bytes"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding"
	"io"
	"regexp"
	"strings"
)

func charsetFromMetaElement(s string) string {
	for s != "" {
		csLoc := strings.Index(s, "charset")
		if csLoc == -1 {
			return ""
		}
		s = s[csLoc+len("charset"):]
		s = strings.TrimLeft(s, " \t\n\f\r")
		if !strings.HasPrefix(s, "=") {
			continue
		}
		s = s[1:]
		s = strings.TrimLeft(s, " \t\n\f\r")
		if s == "" {
			return ""
		}
		if q := s[0]; q == '"' || q == '\'' {
			s = s[1:]
			closeQuote := strings.IndexRune(s, rune(q))
			if closeQuote == -1 {
				return ""
			}
			return s[:closeQuote]
		}

		end := strings.IndexAny(s, "; \t\n\f\r")
		if end == -1 {
			end = len(s)
		}
		return s[:end]
	}
	return ""
}

type PrescanResult struct {
	Encoding encoding.Encoding
	Name     string
}

var metaCharset = regexp.MustCompile(`(?i)<\s*?meta[^>]*?charset\s*=\s*['"]?\s*([a-z-0-9]*)['"]?`)

func HtmlCharsetPrescan(content []byte, callback ...func(start, end int, matched PrescanResult)) (e encoding.Encoding, name string) {
	reader := bytes.NewReader(content)

	z := html.NewTokenizer(reader)
	var count = 0
	var tagCount = 0
	var startOffset int64 = 0
	var endOffset int64 = 0

	var finalResults []PrescanResult

	fallback := func() (encoding.Encoding, string) {
		if len(finalResults) == 0 {
			return nil, ""
		}
		last := finalResults[0]
		return last.Encoding, last.Name
	}

	for {
		count++
		if count > 800 {
			return fallback()
		}
		ret := z.Next()
		if tagCount > 200 {
			return fallback()
		}
		switch ret {
		case html.ErrorToken:
			return fallback()
		case html.EndTagToken:
			name, _ := z.TagName()
			if bytes.Equal(bytes.ToLower(name), []byte("head")) {
				return fallback()
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			tagCount++
			endOffset, _ = reader.Seek(0, io.SeekCurrent)
			endOffset -= int64(len(z.Buffered()))

			tagName, hasAttr := z.TagName()
			tagName = bytes.ToLower(tagName)
			if !bytes.Equal(tagName, []byte("meta")) {
				if bytes.Equal(tagName, []byte("head")) {
					startOffset, _ = reader.Seek(0, io.SeekCurrent)
					startOffset -= int64(len(z.Buffered()))
					endOffset = 0
				}
				continue
			}

			attrList := make(map[string]struct{})
			gotPragma := false

			const (
				dontKnow = iota
				doNeedPragma
				doNotNeedPragma
			)
			needPragma := dontKnow
			name = ""
			e = nil
			for hasAttr {
				var key, val []byte
				key, val, hasAttr = z.TagAttr()
				ks := string(key)
				if _, ok := attrList[ks]; ok {
					continue
				}

				if bytes.EqualFold(val, []byte("gb-18030")) {
					val = []byte("gb18030")
				}
				attrList[ks] = struct{}{}
				for i, c := range val {
					if 'A' <= c && c <= 'Z' {
						val[i] = c + 0x20
					}
				}

				switch ks {
				case "http-equiv":
					if bytes.Equal(bytes.ToLower(val), []byte("content-type")) {
						gotPragma = true
					}
				case "content":
					if e == nil {
						name = charsetFromMetaElement(string(val))
						if name != "" {
							e, name = charset.Lookup(name)
							if e != nil {
								needPragma = doNeedPragma
							}
						}
					}

				case "charset":
					valname := string(val)
					e, name = charset.Lookup(valname)
					needPragma = doNotNeedPragma
				}
			}

			if needPragma == dontKnow || needPragma == doNeedPragma && !gotPragma {
				continue
			}

			if strings.HasPrefix(name, "utf-16") {
				name = "utf-8"
				e = encoding.Nop
			}

			if e != nil {
				pRes := PrescanResult{
					Encoding: e, Name: name,
				}

				// gbk -> gb18030
				// gb2312 -> gb18030
				if strings.EqualFold(name, "gbk") || strings.EqualFold(name, "gb2312") {
					pRes.Encoding, pRes.Name = charset.Lookup("gb18030")
				}

				if endOffset > startOffset && startOffset >= 0 {
					for _, cb := range callback {
						result := metaCharset.FindSubmatchIndex(content[startOffset:endOffset])
						if len(result) > 3 {
							offset := int(startOffset)
							cb(offset+result[2], offset+result[3], pRes)
						}
					}
					startOffset = endOffset
				}
				finalResults = append(finalResults, pRes)
			}
		}
	}
}
