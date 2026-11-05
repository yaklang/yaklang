package java

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// Fixture mirrors the datart enum that produced ten risks on one code range:
// each constant is constructed with arguments, so every constant emits its own
// constructor call in the class body.
const sqlOperatorEnum = `
import java.util.regex.Pattern;

public enum SqlOperator {

    GT(Pattern.compile("[^\\s<]+(\\s*>\\s*){1}[^\\s=]+"), ">", Pattern.compile("\\s>\\s")),
    GTE(Pattern.compile("[^\\s]+(\\s*>\\s*=\\s*){1}[^\\s]+"), ">=", Pattern.compile("\\s*>=\\s*")),
    LT(Pattern.compile("[^\\s]+(\\s*<\\s*){1}[^\\s=>]+"), "<", Pattern.compile("\\s*<\\s*")),
    LTE(Pattern.compile("[^\\s]+(\\s*<\\s*=\\s*){1}[^\\s]+"), "<=", Pattern.compile("\\s*<=\\s*")),
    EQ(Pattern.compile("[^\\s=<]+(\\s*=\\s*){1}[^\\s]+"), "=", Pattern.compile("\\s*=\\s*")),
    NEQ(Pattern.compile("[^\\s]+(\\s*<\\s*>\\s*){1}[^\\s]+"), "<>", Pattern.compile("\\s*<>\\s*")),
    LIKE(Pattern.compile("[^\\s(not)]+(\\s+like\\s+){1}[^\\s]+", Pattern.CASE_INSENSITIVE), "like", Pattern.compile("\\s*like\\s*", Pattern.CASE_INSENSITIVE)),
    UNLIKE(Pattern.compile("[^\\s]+(\\s+not\\s+like\\s+){1}[^\\s]+", Pattern.CASE_INSENSITIVE), "not like", Pattern.compile("\\s*not\\s*like\\s*", Pattern.CASE_INSENSITIVE)),
    IN(Pattern.compile("[^\\s(not)]+(\\s+in\\s+){1}[^\\s]+", Pattern.CASE_INSENSITIVE), "in", Pattern.compile("\\s*in\\s*", Pattern.CASE_INSENSITIVE)),
    NIN(Pattern.compile("[^\\s]+(\\s+not\\s+in\\s+){1}[^\\s]+", Pattern.CASE_INSENSITIVE), "not in", Pattern.compile("\\s*not\\s*in\\s*", Pattern.CASE_INSENSITIVE));

    SqlOperator(Pattern pattern, String operator, Pattern replace) {
        this.pattern = pattern;
        this.operator = operator;
        this.replace = replace;
    }

    private final Pattern pattern;
    private final String operator;
    private final Pattern replace;
}
`

func TestEnumConstantConstructorRangeIsPerConstant(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("SqlOperator.java", sqlOperatorEnum)

	ssatest.CheckWithFS(vfs, t, func(programs ssaapi.Programs) error {
		// <getCall> on the enum blueprint is the constructor call of each
		// constant. These calls used to inherit the whole enum body range, so
		// ten distinct findings collapsed onto one code_range.
		result, err := programs.SyntaxFlowWithError(`SqlOperator<getCall> as $calls`)
		require.NoError(t, err)
		calls := result.GetValues("calls")
		require.Len(t, calls, 10, "every enum constant must keep its own constructor call")

		seen := make(map[string]int, len(calls))
		for _, call := range calls {
			rng := call.GetRange()
			require.NotNil(t, rng, "constructor call must carry a source range")
			start, end := rng.GetStart(), rng.GetEnd()
			require.NotNil(t, start)
			require.NotNil(t, end)
			require.Equal(t, start.GetLine(), end.GetLine(),
				"one constant's constructor call must not span the whole enum body: %s", rng.GetText())
			key := fmt.Sprintf("%d:%d-%d:%d", start.GetLine(), start.GetColumn(), end.GetLine(), end.GetColumn())
			seen[key]++
		}
		for key, count := range seen {
			require.Equal(t, 1, count, "constructor calls collapsed onto range %s", key)
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}
