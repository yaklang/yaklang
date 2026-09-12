package stream_parser

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

func TestOperatorPanicDebugSuppressionPreservesDiagnostics(t *testing.T) {
	for _, source := range []string{
		`panic("expected rejection")`,
		`inner = func() { native() }; inner()`,
		`inner = func() { panic("nested rejection") }; inner()`,
		"// " + strings.Repeat("x", operatorProgramSourceLimit) + "\npanic(\"fallback rejection\")",
	} {
		var ordinary string
		for _, quiet := range []bool{false, true} {
			engine := antlr4yak.New()
			engine.GetVM().GetConfig().SetSuppressPanicDebugStack(quiet)
			engine.ImportLibs(map[string]any{"native": func() { panic("native rejection") }})
			err := evalOperatorProgram(context.Background(), engine, source)
			require.Error(t, err)
			require.Contains(t, err.Error(), "rejection")
			require.Contains(t, err.Error(), "Panic Stack:")
			require.Contains(t, err.Error(), "YakVM Panic:")
			if !quiet {
				ordinary = err.Error()
			} else {
				require.Equal(t, ordinary, err.Error())
			}
		}
	}
}
