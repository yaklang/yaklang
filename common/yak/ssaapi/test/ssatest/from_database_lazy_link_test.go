package ssatest

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestFromDatabase_ParameterCanResolveFunction(t *testing.T) {
	code := `
#include <string.h>
void vulnerable_concatenation(const char *str1, const char *str2) {
    char buffer[128];
    strcpy(buffer, str1);
    strcat(buffer, str2);
}
`
	Check(t, code, func(prog *ssaapi.Program) error {
		result, err := prog.SyntaxFlowWithError("str2 as $p;", ssaapi.QueryWithInitInputVar(prog))
		require.NoError(t, err)
		require.NotEmpty(t, result.GetValues("p"))

		found := false
		for _, v := range result.GetValues("p") {
			if v.GetOpcode() != "Parameter" {
				continue
			}
			fn := v.GetFunction()
			if fn != nil && fn.String() == "Function-vulnerable_concatenation" {
				found = true
				break
			}
		}
		require.True(t, found, "parameter should resolve to its owner function")
		return nil
	}, ssaapi.WithLanguage(ssaconfig.C))
}

func TestFromDatabase_CrossFunctionTopDefsConsistency(t *testing.T) {
	code := `
#include <string.h>
#include <stdlib.h>

void vulnerable_concatenation(const char *str1, const char *str2) {
    char buffer[128];
    strcpy(buffer, str1);
    strcat(buffer, str2);
}

int main(int argc, char **argv) {
    char *env_str = getenv("TEST_STRING");
    if (env_str) {
        vulnerable_concatenation(argv[1], env_str);
    }
    return 0;
}
`
	rule := `
strcat(*<slice(index=1)> #-> as $unsafe);
getenv as $src;
$unsafe & $src as $hit;
alert $hit;
`
	CheckSyntaxFlowContain(t, code, rule, map[string][]string{
		"hit": {"Function-getenv"},
	}, ssaapi.WithLanguage(ssaconfig.C))
}

func TestFromDatabase_NativeCallGetFuncConsistency(t *testing.T) {
	code := `
#include <string.h>
void vulnerable_concatenation(const char *str1, const char *str2) {
    char buffer[128];
    strcpy(buffer, str1);
    strcat(buffer, str2);
}
`
	rule := `
str2 as $p;
$p <getFunc> as $f;
`
	CheckSyntaxFlowContain(t, code, rule, map[string][]string{
		"f": {"Function-vulnerable_concatenation"},
	}, ssaapi.WithLanguage(ssaconfig.C))
}
