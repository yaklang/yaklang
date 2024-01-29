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

func TestDeprecatedRange(t *testing.T) {
	check(t,
		`
	c = 0
	for _, url := range [] {
		http.RequestFaviconHash("faviconUrl")~
	}
	rsp = http.Get(
		"",
	)~
	if rsp == 0 {
		b = c // 
	}
	rspRaw = http.dump(rsp)~
		`,
		[]string{
			"! 已弃用，使用 poc.Get 代替",
		},
	)
}
