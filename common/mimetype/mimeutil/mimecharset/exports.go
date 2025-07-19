package mimecharset

import (
	"github.com/saintfish/chardet"
	"github.com/yaklang/yaklang/common/mimetype/mimeutil/basic_charset"
	"golang.org/x/net/html/charset"
	"strings"
)

func fallback(raw []byte, wrapper func([]byte) string) string {
	switch ret := wrapper(raw); ret {
	case "", "windows-1252", "iso-8859-1":
		results, err := defaultDetector.DetectAll(raw)
		if err != nil {
			// log.Warnf("unknown plain text charset: %v", raw)
			return ""
		}
		var (
			zhCharset       []chardet.Result
			fallbackCharset []chardet.Result
			otherCharset    []chardet.Result
		)
		for _, result := range results {
			charsetLower := strings.ToLower(result.Charset)
			if strings.ToLower(result.Language) == "zh" {
				zhCharset = append(zhCharset, result)
			} else if charsetLower == "iso-8859-1" || charsetLower == "windows-1252" {
				fallbackCharset = append(fallbackCharset, result)
			} else {
				otherCharset = append(otherCharset, result)
			}
		}

		if len(zhCharset) > 0 {
			enc, _ := charset.Lookup("gb18030")
			if enc != nil {
				_, err := enc.NewDecoder().Bytes(raw)
				if err == nil {
					return "gb18030"
				}
			}
		}

		for _, otherLang := range otherCharset {
			enc, _ := charset.Lookup(otherLang.Charset)
			if enc != nil {
				_, err := enc.NewDecoder().Bytes(raw)
				if err == nil {
					return otherLang.Charset
				}
			}
		}

		for _, fallbackLang := range fallbackCharset {
			enc, _ := charset.Lookup(fallbackLang.Charset)
			if enc != nil {
				_, err := enc.NewDecoder().Bytes(raw)
				if err == nil {
					return fallbackLang.Charset
				}
			}
		}

		return ret
	default:
		return ret
	}
}

// FromBOM is the same as basic_chardet
func FromBOM(raw []byte) string {
	return basic_charset.FromBOM(raw)
}

var defaultDetector = chardet.NewTextDetector()

func FromPlain(raw []byte) string {
	return fallback(raw, basic_charset.FromPlain)
}

func FromXML(raw []byte) string {
	return fallback(raw, basic_charset.FromXML)
}

func FromHTML(raw []byte) string {
	return fallback(raw, basic_charset.FromHTML)
}
