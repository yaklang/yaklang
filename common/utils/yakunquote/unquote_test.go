package yakunquote

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnquoteInvalidUTF8(t *testing.T) {
	raw := "你好"
	hexStr := ""

	for _, r := range []byte(raw) {
		hexStr += fmt.Sprintf("\\x%x", r)
	}
	got, err := Unquote(fmt.Sprintf(`"%s"`, hexStr))
	if err != nil {
		t.Fatal(err)
	}
	if got != raw {
		t.Fatalf("want %s, got %s", raw, got)
	}
}

func TestCompUnquote(t *testing.T) {
	test := func(t *testing.T, input string, want string) {
		t.Helper()

		got, err := Unquote(input, true)
		require.NoError(t, err)
		require.Equal(t, want, got)
	}

	t.Run("empty", func(t *testing.T) {
		test(t, "", "")
	})

	t.Run("normal char", func(t *testing.T) {
		test(t, "abc", "abc")
	})

	t.Run("invalid control char", func(t *testing.T) {
		test(t, `\c`, `\c`)
	})

	t.Run("control char", func(t *testing.T) {
		test(t, `\a\b\f\n\r\t\v`, "\a\b\f\n\r\t\v")
	})

	t.Run("only backslash", func(t *testing.T) {
		test(t, `\`, `\`)
	})

	t.Run("escape backslash", func(t *testing.T) {
		test(t, `\\`, `\\`)
	})

	t.Run("hex not enough 1", func(t *testing.T) {
		test(t, `\x`, `\x`)
	})

	t.Run("invalid hex not enough 2", func(t *testing.T) {
		test(t, `\x6`, `\x6`)
	})

	t.Run("valid hex", func(t *testing.T) {
		test(t, `\x61`, `a`)
	})

	t.Run("invalid hex", func(t *testing.T) {
		test(t, `\xgg`, `\xgg`)
	})

	t.Run("invalid unicode not enough", func(t *testing.T) {
		test(t, `\u`, `\u`)
	})

	t.Run("valid unicode", func(t *testing.T) {
		test(t, `\u0061`, `a`)
	})
	t.Run("invalid eight unicode not enough", func(t *testing.T) {
		test(t, `\U`, `\U`)
	})

	t.Run("valid eight unicode", func(t *testing.T) {
		test(t, `\U00000061`, `a`)
	})

	t.Run("invalid oct not enough", func(t *testing.T) {
		test(t, `\0`, `\0`)
	})

	t.Run("invalid oct", func(t *testing.T) {
		test(t, `\088`, `\088`)
	})

	t.Run("valid oct", func(t *testing.T) {
		test(t, `\061`, `1`)
	})

	t.Run("invalid utf8", func(t *testing.T) {
		test(t, `中文`, `中文`)
	})

	t.Run("normal1", func(t *testing.T) {
		test(t, `dlkhalkdhlhsalkd"\x61`, `dlkhalkdhlhsalkd"a`)
	})
}
