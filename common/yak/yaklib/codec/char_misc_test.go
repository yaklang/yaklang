package codec

import (
	"bytes"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestZhEncoding(t *testing.T) {
	results, err := strconv.Unquote(`"\xe2(\xa1"`)
	if err != nil {
		t.Fatal(err)
	}
	resultRune, size := utf8.DecodeRuneInString(results)
	spew.Dump(resultRune, fmt.Sprintf("\\u%4x", resultRune), size)
	assert.Equal(t, size, 1)
	assert.Equal(t, int(resultRune), 65533)

	parseFailed, err := GB18030ToUtf8([]byte(results))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(parseFailed, []byte("\ufffd(\ufffd")) {
		t.Fatal("Failed to parse")
	}
	spew.Dump(parseFailed, string(parseFailed))
}

func TestCharMatch(t *testing.T) {
	tests := []struct {
		name         string
		input        func(*testing.T) string
		have         []string
		noHave       []string
		inputNotHave []string
	}{
		{
			name: "Simple Chinese",
			input: func(t *testing.T) string {
				data, err := Utf8ToGB18030([]byte("你好，我有一个帽衫"))
				if err != nil {
					t.Fatal("Conversion error:", err)
				}
				sample := `<html><head><meta charset="gb18030"><title>` + string(data) + `</title></html>`
				return sample
			},
			have:         []string{"帽衫"},
			inputNotHave: []string{"你好"},
		},

		{
			name: "Invalid UTF8",
			input: func(t *testing.T) string {
				raw, err := DecodeHex(`E228A1`)
				if err != nil {
					t.Fatal(err)
				}
				return string(raw)
			},
			noHave: []string{"\xef\xbf\xbd"},
		},

		{
			name: "Simple Chinese Dup Meat",
			input: func(t *testing.T) string {
				data, err := Utf8ToGB18030([]byte("你好，我有一个帽衫"))
				if err != nil {
					t.Fatal("Conversion error:", err)
				}
				sample := `<html><head><meta charset="gb18030"><meta charset="gb18030"/><title>` + string(data) + `</title></html>`
				return sample
			},
			have:         []string{"帽衫"},
			noHave:       []string{"gb18030"},
			inputNotHave: []string{"你好", "utf-8"},
		},

		{
			name: "Simple Chinese 2",
			input: func(t *testing.T) string {
				data, err := Utf8ToGB18030([]byte("你好，我有一个帽衫"))
				if err != nil {
					t.Fatal("Conversion error:", err)
				}
				sample := `<html><head><meta charset=gb18030><title>` + string(data) + `</title></html>`
				return sample
			},
			have:         []string{"帽衫"},
			inputNotHave: []string{"你好"},
		},

		{
			name: "Simple Chinese 2 Conflict",
			input: func(t *testing.T) string {
				data, err := Utf8ToGB18030([]byte("你好，我有一个帽衫"))
				if err != nil {
					t.Fatal("Conversion error:", err)
				}
				sample := `<html><head><meta charset=gb18030><meta charset=utf-8><title>` + string(data) + `</title></html>`
				return sample
			},
			have:         []string{"帽衫", "utf-8"},
			noHave:       []string{"gb18030"},
			inputNotHave: []string{"你好"},
		},

		{
			name: "Simple Chinese 2 JavaScript",
			input: func(t *testing.T) string {
				data, err := Utf8ToGB18030([]byte("你好，我有一个帽衫"))
				if err != nil {
					t.Fatal("Conversion error:", err)
				}
				sample := `(()=>{
	return () => "` + string(data) + `"
})()()`
				return sample
			},
			have:         []string{"帽衫"},
			inputNotHave: []string{"你好"},
		},

		// Additional test cases can be added here
	}

	for _, tc := range tests {
		for i := 0; i < 100; i++ {
			t.Run(tc.name, func(t *testing.T) {
				sample := tc.input(t)
				for _, a := range tc.inputNotHave {
					if strings.Contains(sample, a) {
						t.Fatal("Failed to find converted Chinese characters in converted HTML")
					}
				}
				result, err := MatchMIMEType(sample)
				if err != nil {
					t.Fatal("MIME matching error:", err)
				}

				res, _ := result.TryUTF8Convertor([]byte(sample))
				fmt.Println("Converted HTML:", string(res))

				for _, a := range tc.have {
					if !strings.Contains(string(res), a) {
						t.Fatal("Failed to find converted Chinese characters in converted HTML")
					}
				}

				for _, a := range tc.noHave {
					if strings.Contains(string(res), a) {
						t.Fatal("Failed to find converted Chinese characters in converted HTML")
					}
				}
			})
		}
	}
}

func TestHTMLCharsetFromMeta(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		expectedRaw string
		expectCheck bool
	}{
		{
			name:        "Basic",
			data:        []byte(`<html><head><meta charset="UTF-8"></html>`),
			expectedRaw: "UTF-8",
			expectCheck: true,
		},
		{
			name:        "Basic 2",
			data:        []byte(`<html><head><meta charset="UTF-8" /></html>`),
			expectedRaw: "UTF-8",
			expectCheck: true,
		},
		{
			name:        "Basic 3",
			data:        []byte(`<html><head><meta charset="UTF-8" /   ></html>`),
			expectedRaw: "UTF-8",
			expectCheck: true,
		},
		{
			name:        "Basic GB18030 Bad",
			data:        []byte(`<html><head><meta charset="GB-18030"></html>`),
			expectedRaw: "GB-18030",
			expectCheck: true,
		},
		{
			name:        "Basic GB18030",
			data:        []byte(`<html><head><meta charset="GB18030"></html>`),
			expectedRaw: "GB18030",
			expectCheck: true,
		},
		{
			name:        "Self-closing",
			data:        []byte(`<html><head><meta charset="UTF-8"  /></html>`),
			expectedRaw: "UTF-8",
			expectCheck: true,
		},
		{
			name:        "Space before self-closing",
			data:        []byte(`<html><head><meta charset="UTF-8"  / ></html>`),
			expectedRaw: "UTF-8",
			expectCheck: true,
		},
		{
			name:        "Meta with http-equiv",
			data:        []byte(`<html><head><meta http-equiv="content-type" content="text/html;charset=utf-8"></html>`),
			expectedRaw: "utf-8",
			expectCheck: true,
		},
		{
			name:        "Multiple meta tags",
			data:        []byte(`<html><head><meta charset="UTF-8"><meta http-equiv="content-type" content="text/html;charset=utf-8"></html>`),
			expectedRaw: "utf-8",
			expectCheck: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := false
			encoding, raw := HtmlCharsetPrescan(tt.data, func(start, end int, _ PrescanResult) {
				log.Infof("data: %#v", string(tt.data[start:end]))
				if strings.Contains(string(tt.data[start:end]), tt.expectedRaw) {
					check = true
				}
			})
			assert.Equal(t, tt.expectCheck, check)
			_ = encoding
			_ = raw
		})
	}
}

func TestHTMLCharsetFromMeta_Basic3(t *testing.T) {
	//charsetFromMetaElement("abc")
	count := 0
	data := []byte(`<html><head>
	<meta http-equiv="content-type" content="text/html;charset=utf-8">
	<meta http-equiv="content-type" content="text/html;charset=utf-8">
	<meta http-equiv="content-type" content="text/html;charset=utf-8">
</html>`)
	encoding, raw := HtmlCharsetPrescan(data, func(start, end int, _ PrescanResult) {
		log.Infof("data: %#v", string(data[start:end]))
		count++
	})
	assert.Equal(t, raw, "utf-8")
	assert.True(t, count == 3)
	_ = encoding
}
