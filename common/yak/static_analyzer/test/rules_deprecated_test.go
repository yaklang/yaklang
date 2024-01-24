package test

import (
	"testing"
)

func TestDeprecated(t *testing.T) {
	check(t,
		`
		env.Set("a", "21")
		`,
		[]string{
			"! 已弃用，可以使用 `os.Setenv` 代替",
		},
	)
}
