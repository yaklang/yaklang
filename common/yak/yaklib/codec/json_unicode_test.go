package codec

import (
	"testing"
)

func TestJsonUnicodeDecode(t *testing.T) {
	testCases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "lower u",
			in:   JsonUnicodeEncode("你好ab"),
			want: "你好ab",
		},
		{
			name: "upper U",
			in:   `\U00004f60\U0000597d`,
			want: "你好",
		},
		{
			name: "mixed",
			in:   `\u4f60\U0000597dab`,
			want: "你好ab",
		},
		{
			name: "double escaped lower u",
			in:   `\\u4f60\\u597d`,
			want: `\u4f60\u597d`,
		},
		{
			name: "double escaped upper U",
			in:   `\\U00004f60\\U0000597d`,
			want: `\U00004f60\U0000597d`,
		},
		{
			name: "json escaped quotes preserved",
			in:   `"{\"a\":\"\u0062\"}"`,
			want: `"{\"a\":\"b\"}"`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := JsonUnicodeDecode(tc.in); got != tc.want {
				t.Fatalf("JsonUnicodeDecode(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
