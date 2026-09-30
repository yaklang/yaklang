package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestFrontendPredictionPreservesSSAValues(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		values       []string
	}{
		{"call result member", `<?php println(factory()->value = 1 + 2 * 3); println("tail");`, []string{"7", `"tail"`}},
		{"indexed result and nested assignment", `<?php println(factory()->data[0] = $x = 7); println($x); println("tail");`, []string{"7", "7", `"tail"`}},
		{"reference assignment", `<?php $x = 4; println(factory()->value = &$x); println("tail");`, []string{"4", `"tail"`}},
		{"arrow function parameter", `<?php $f = fn($x) => println($x + 3); $f(2); println("tail");`, []string{"add(Parameter-$x, 3)", `"tail"`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ssatest.CheckPrintlnValue(tc.source, tc.values, t)
		})
	}
}
