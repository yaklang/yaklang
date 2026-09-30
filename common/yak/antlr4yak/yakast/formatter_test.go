package yakast

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakfmt"
)

func TestFormatterCacheResetsWithCompiler(t *testing.T) {
	compiler := NewYakCompiler()
	for _, source := range []string{"a=1", "a=2;f=func(x){return x+1}", "a=f\"汉字 ${1+2}\""} {
		if !compiler.Compiler(source) {
			t.Fatal(compiler.GetErrors())
		}
		expected, err := yakfmt.Format(source)
		if err != nil {
			t.Fatal(err)
		}
		expected = expected[:len(expected)-1]
		for i := 0; i < 3; i++ {
			if got := compiler.GetFormattedCode(); got != expected {
				t.Fatalf("stale format cache: %q, expected %q", got, expected)
			}
		}
	}
	if compiler.Compiler("a=") {
		t.Fatal("invalid source compiled")
	}
	if got := compiler.GetFormattedCode(); got != "" {
		t.Fatal("failed parse reused previous format", got)
	}
}
