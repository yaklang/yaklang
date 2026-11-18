package cliteral

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseInt(t *testing.T) {
	cases := []struct {
		text string
		want int64
	}{
		{"0", 0},
		{"42", 42},
		{"010", 8},
		{"0x1f", 31},
		{"0X1F", 31},
		{"0b1010", 10},
		{"42u", 42},
		{"42ULL", 42},
		{"42l", 42},
		{"1'000'000", 1000000},
		{"0xff'ff", 65535},
	}
	for _, tc := range cases {
		got, err := ParseInt(tc.text)
		require.NoError(t, err, tc.text)
		require.Equal(t, tc.want, got, tc.text)
	}
}

func TestIsInt(t *testing.T) {
	for _, text := range []string{"0", "42", "0x1f", "0b11", "42ULL", "1'000"} {
		require.True(t, IsInt(text), text)
	}
	for _, text := range []string{"", "a", "1.5", "0x", "x1"} {
		require.False(t, IsInt(text), text)
	}
}

func TestParseFloat(t *testing.T) {
	cases := []struct {
		text string
		want float64
	}{
		{"1.5", 1.5},
		{"1e3", 1000},
		{"1.5f", 1.5},
		{"1.5L", 1.5},
		{"1'000.5", 1000.5},
	}
	for _, tc := range cases {
		got, err := ParseFloat(tc.text)
		require.NoError(t, err, tc.text)
		require.Equal(t, tc.want, got, tc.text)
	}
}

func TestParseChar(t *testing.T) {
	cases := []struct {
		text string
		want int32
	}{
		{"'a'", 'a'},
		{"'\\n'", '\n'},
		{"'\\t'", '\t'},
		{"'\\0'", 0},
		{"'\\\\'", '\\'},
		{"'\\''", '\''},
		{"L'x'", 'x'},
		{"'\\x41'", 'A'},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, ParseChar(tc.text), tc.text)
	}
}

func TestParseString(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{`"hello"`, "hello"},
		{`"a\nb"`, "a\nb"},
		{`"quote\"inside"`, `quote"inside`},
		{`u8"utf8"`, "utf8"},
		{`R"raw(no\escape)"`, `no\escape`},
	}
	for _, tc := range cases {
		got, ok := ParseString(tc.text)
		require.True(t, ok, tc.text)
		require.Equal(t, tc.want, got, tc.text)
	}
	if _, ok := ParseString("identifier"); ok {
		t.Fatal("identifier must not parse as a string literal")
	}
}
