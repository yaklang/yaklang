package test

import "testing"

func TestIgnore(t *testing.T) {
	code := `

opts = []
opts.Push(risk.description(""))
opts.Push(risk.solution(""))

// @ssa-ignore
risk.NewRisk("", opts...)
	`

	check(t, code, []string{})
}
