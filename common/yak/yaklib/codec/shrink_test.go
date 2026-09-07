package codec

import (
	"strconv"
	"strings"
	"testing"
)

func shrinkStringReference(r any, size int, multiline bool) string {
	if size <= 6 {
		size = 10
	}

	half := size / 2
	verbose := strings.TrimSpace(AnyToString(r))
	runes := []rune(verbose)
	if len(runes) > size {
		runes = append(runes[:half], append([]rune("..."), runes[len(runes)-half:]...)...)
		verbose = string(runes)
	}
	if !multiline {
		verbose = strconv.Quote(verbose)
		verbose = verbose[1 : len(verbose)-1]
		verbose = strings.ReplaceAll(verbose, `\r`, " ")
		verbose = strings.ReplaceAll(verbose, `\n`, " ")
		verbose = strings.ReplaceAll(verbose, `\t`, " ")
		verbose = strings.ReplaceAll(verbose, `\"`, "\"")
	}
	return verbose
}

func TestShrinkStringMatchesReference(t *testing.T) {
	invalidShort := string([]byte{'a', 0xff, 'b', 0xc0, 'c'})
	invalidLong := string([]byte{0xff, 'a', 0xfe, 'b', 0xfd, 'c', 0xfc, 'd', 0xfb, 'e', 0xfa, 'f', 0xf9})
	tests := []struct {
		name  string
		input any
	}{
		{name: "empty", input: ""},
		{name: "spaces", input: "   \t\r\n  "},
		{name: "ascii under ten", input: "012345678"},
		{name: "ascii exact ten", input: "0123456789"},
		{name: "ascii over ten", input: "0123456789a"},
		{name: "ascii", input: "abcdefghijklmnopqrstuvwxyz"},
		{name: "chinese", input: "天地玄黄宇宙洪荒日月盈昃辰宿列张"},
		{name: "emoji", input: "😀😃😄😁😆😅😂🤣😊😇🙂🙃😉😌😍🥰"},
		{name: "combining", input: "e\u0301cole-A\u030angstro\u0308m-noe\u0308l"},
		{name: "invalid utf8 short", input: invalidShort},
		{name: "invalid utf8 long", input: invalidLong},
		{name: "invalid utf8 bytes", input: []byte(invalidLong)},
		{name: "newlines", input: "\nline one\r\nline two\nline three\n"},
		{name: "tabs and quotes", input: "\t\"alpha\"\t\"beta\"\t"},
		{name: "trimmed unicode spaces", input: "\u2003\u00a0content\u00a0\u2003"},
	}
	sizes := []int{-1, 0, 1, 6, 7, 8, 9, 10, 11, 15, 16, 17, 26, 27, 200}

	for _, test := range tests {
		for _, size := range sizes {
			for _, multiline := range []bool{false, true} {
				name := test.name + "/size=" + strconv.Itoa(size) + "/multiline=" + strconv.FormatBool(multiline)
				t.Run(name, func(t *testing.T) {
					want := shrinkStringReference(test.input, size, multiline)
					got := shrinkStringWithMultiLine(test.input, size, multiline)
					if got != want {
						t.Fatalf("shrink result mismatch\n got: %q\nwant: %q", got, want)
					}
				})
			}
		}
	}
}

var shrinkStringBenchmarkSink string

func BenchmarkShrinkStringBase64(b *testing.B) {
	for _, size := range []int{1 << 20, 8 << 20} {
		payload := strings.Repeat("QUJD", size/4)
		b.Run(strconv.Itoa(size>>20)+"MiB", func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(payload)))
			for i := 0; i < b.N; i++ {
				shrinkStringBenchmarkSink = ShrinkString(payload, 200)
			}
		})
	}
}
