package utils

import (
	"testing"
)

func TestIsBase64_Positive(t *testing.T) {
	for _, c := range []struct {
		name string
		args string
	}{
		{name: "urlencode base64", args: `eyJkZCI6MTI1fQ%3D%3D`},
		{name: "basic base64(utf8 valid)", args: "YWJjZGRkZA=="},
		{name: "basic base64(gzip)", args: "H4sIAAAAAAAA/0pMSgYAAAD//wEAAP//wkEkNQMAAAA="},
		{name: "utf8(中文)", args: `5L2g5aW9`},
		{name: "gb18030(中文)", args: `xOO6ww==`},
		{name: "zlib", args: `eJxSSgQBJQAAAAD//wEAAP//CKsCKg==`},
	} {
		t.Run(c.name, func(t *testing.T) {
			if !IsBase64(c.args) {
				t.Errorf("isBase64() = %v, want %v", false, true)
			}
		})
	}
}
