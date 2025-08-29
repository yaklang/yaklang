package java

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestGraphFrom_XXE(t *testing.T) {
	sfRule := `
desc("Description": 'checking setFeature/setXIncludeAware/setExpandEntityReferences in DocumentBuilderFactory.newInstance()')

DocumentBuilderFactory.newInstance()?{!((.setFeature) || (.setXIncludeAware) || (.setExpandEntityReferences))} as $entry;
$entry.*Builder().parse(* #-> as $source);

check $source then "XXE Attack" else "XXE Safe";
`
	check := func(t *testing.T, result *ssaapi.SyntaxFlowResult) {
		source := result.GetValues("source").Show()
		fmt.Println(source.DotGraph())
		if !utils.MatchAllOfSubString(
			source.DotGraph(),
			"fontcolor", "color",
			"penwidth=\"3.0\"",
			"call",
			"actual-args",
			"search-exact:newInstance",
			"search-exact:parse",
			"search-glob:*Builder",
			"newInstance",
		) {
			t.Fatal("failed to match all of the substring, bad dot graph")
		}

		entry := result.GetValues("entry").Show()
		require.NotNil(t, entry)
		entryDot := entry.DotGraph()
		fmt.Println(entryDot)
		if !utils.MatchAllOfSubString(
			entryDot,
			"newInstance",
			"DocumentBuilderFactory",
			"newInstance",
			"search-exact:newInstance",
		) {
			t.Fatal("failed to match all of the substring, bad dot graph")
		}
	}

	t.Run("draw dot graph in memory", func(t *testing.T) {
		ssatest.Check(t, XXE_Code, func(prog *ssaapi.Program) error {
			assert.Equal(t, prog.GetLanguage(), "java")
			results, err := prog.SyntaxFlowWithError(sfRule)
			require.NoError(t, err)
			check(t, results)

			return nil
		}, ssaapi.WithLanguage(ssaapi.JAVA))
	})
	t.Run("draw dot which its value from database", func(t *testing.T) {
		programName := uuid.NewString()
		prog, err := ssaapi.Parse(XXE_Code, ssaapi.WithProgramName(programName), ssaapi.WithLanguage(consts.JAVA))
		require.NoError(t, err)
		require.NotNil(t, prog)

		res, err := prog.SyntaxFlowWithError(sfRule)
		require.NoError(t, err)
		require.NotNil(t, res)

		resultID, err := res.Save(schema.SFResultKindDebug)
		require.NoError(t, err)
		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), programName)
		}()

		result, err := ssaapi.LoadResultByID(resultID)
		require.NoError(t, err)
		require.NotNil(t, result)

		check(t, result)
	})
}
