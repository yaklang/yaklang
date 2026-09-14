package sfpattern_test

import (
	"testing"

	"github.com/yaklang/yaklang/common/syntaxflow/sfanalysis"
)

func TestBuiltinSourceRules_VerifyFilesystem(t *testing.T) {
	sfanalysis.RunBuiltinRuleVerify(t, sfanalysis.BuiltinVerifyFilter{Source: true})
}
