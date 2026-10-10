package sfpattern_test

import (
	"testing"

	"github.com/yaklang/yaklang/common/syntaxflow/sfanalysis/sfanalysistest"
)

func TestBuiltinSourceRules_VerifyFilesystem(t *testing.T) {
	sfanalysistest.RunBuiltinRuleVerify(t, sfanalysistest.BuiltinVerifyFilter{Source: true})
}
