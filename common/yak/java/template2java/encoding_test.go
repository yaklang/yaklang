package template2java

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/yaklang/yaklang/common/yak/java/javasyntax"
)

// Legacy JSP pages declare GBK and contain non-UTF-8 bytes. Before the fix the generated Java
// carried Go-style \xNN escapes (Java has no \x escape), so the page failed to parse and the
// whole template degraded to a syntax error. This is the Coremail `webinst.war/*.jsp` case.
func TestJSPGBKContentProducesParsableJava(t *testing.T) {
	const page = `<html><head><title>服务重启失败</title></head><body>`
	encoded, err := simplifiedchinese.GBK.NewEncoder().String(page)
	require.NoError(t, err)
	require.False(t, isASCII(encoded), "fixture must contain non-ASCII bytes to be meaningful")

	source := `<%@ page contentType="text/html;charset=GBK" pageEncoding="GBK" %>` + encoded
	info, err := ConvertTemplateToJava(JSP, source, "webinst.war/restartServiceErrorInfo.jsp")
	require.NoError(t, err)

	code := info.GetContent()
	require.NotContains(t, code, `\x`, "generated Java must not contain Go-style \\x escapes")
	require.Contains(t, code, `\u670d\u52a1`, "Chinese text must be decoded and escaped as UTF-16 units")
	require.NoError(t, javasyntax.Validate(code), "generated Java must parse")
}

// A UTF-8 template must stay byte-for-byte readable: no decoding, no spurious escapes.
func TestJSPUTF8ContentStaysReadable(t *testing.T) {
	source := `<%@ page contentType="text/html;charset=UTF-8" %><p>hello 世界</p>`
	info, err := ConvertTemplateToJava(JSP, source, "web/hello.jsp")
	require.NoError(t, err)

	code := info.GetContent()
	require.Contains(t, code, "hello ")
	require.Contains(t, code, `\u4e16\u754c`)
	require.NoError(t, javasyntax.Validate(code))
}

// Undeclared non-UTF-8 bytes must still produce compilable Java: the byte becomes a Latin-1
// code unit escape instead of an invalid \x sequence.
func TestJSPUndeclaredLegacyBytesStayCompilable(t *testing.T) {
	raw := string([]byte{0xb0, 0xb2, 0x3c, 0x70, 0x3e}) // 0xb0 0xb2 are invalid UTF-8
	info, err := ConvertTemplateToJava(JSP, `<html><body>`+raw, "web/legacy.jsp")
	require.NoError(t, err)

	code := info.GetContent()
	require.NotContains(t, code, `\x`)
	require.Contains(t, code, `\u00b0`)
	require.NoError(t, javasyntax.Validate(code))
}

func TestJavaStringLiteralEscaping(t *testing.T) {
	cases := map[string]string{
		`plain`:              `plain`,
		`quote " and \`:      `quote \" and \\`,
		"tab\tnewline\n":     `tab\tnewline\n`,
		"世界":                 `\u4e16\u754c`,
		"\x01":               `\u0001`,
		string([]byte{0xc3}): `\u00c3`, // truncated UTF-8 sequence
	}
	for input, want := range cases {
		require.Equal(t, want, javaStringLiteral(input), "input=%q", input)
	}
	// Astral runes must be emitted as a surrogate pair, never as \x or a raw rune.
	require.Equal(t, `\ud83d\ude00`, javaStringLiteral("😀"))
	require.False(t, strings.Contains(javaStringLiteral("\xff\xfe"), `\x`))
}

func TestTemplateEncodingDetection(t *testing.T) {
	require.Nil(t, templateEncodingFromSource(`<html>`))
	require.Nil(t, templateEncodingFromSource(`<%@ page contentType="text/html;charset=UTF-8" %>`))
	require.NotNil(t, templateEncodingFromSource(`<%@ page pageEncoding="GBK" %>`))
	require.NotNil(t, templateEncodingFromSource(`<%@ page contentType="text/html; charset=gb2312" %>`))
	require.NotNil(t, templateEncodingFromSource(`<meta charset="big5">`))
	require.Nil(t, templateEncodingFromSource(`<meta charset="utf-8">`))
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 0x7f {
			return false
		}
	}
	return true
}
