package java

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"testing"

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

	t.Run("draw dot graph in memory", func(t *testing.T) {
		ssatest.Check(t, XXE_Code, func(prog *ssaapi.Program) error {
			assert.Equal(t, prog.GetLanguage(), "java")
			results, err := prog.SyntaxFlowWithError(sfRule)
			require.NoError(t, err)
			entry := results.GetValues("entry")
			if !utils.MatchAllOfSubString(
				entry.DotGraph(),
				"newInstance",
				"DocumentBuilderFactory",
				"newInstance(DocumentBuilderFactory)",
				"search-exact:newInstance",
			) {
				fmt.Println(entry.DotGraph())
				t.Fatal("failed to match all of the substring, bad dot graph")
			}

			source := results.GetValues("source")
			//source.ShowDot()
			if !utils.MatchAllOfSubString(
				source.DotGraph(),
				"fontcolor", "color",
				"step[",
				"penwidth=\"3.0\"",
				": call",
				"search-exact:parse", // "search parse",
				"all-actual-args",
				"ByteArrayInputStream",
				"getBytes",
				"parse",
				"UTF-8",
				"xmlStr",
			) {
				fmt.Println(source.DotGraph())
				t.Fatal("failed to match all of the substring, bad dot graph")
			}
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

		resultID, err := res.Save()
		require.NoError(t, err)
		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), programName)
		}()

		result, err := ssaapi.CreateResultByID(resultID)
		require.NoError(t, err)
		require.NotNil(t, result)

		source := result.GetValues("source")
		require.NotNil(t, source)
		sourceDot := source.DotGraph()
		//source.ShowDot()
		entry := result.GetValues("entry")
		require.NotNil(t, entry)
		entryDot := entry.DotGraph()
		if !utils.MatchAllOfSubString(
			sourceDot,
			"newInstance",
			"DocumentBuilderFactory",
			"newInstance(DocumentBuilderFactory)",
			"search-exact:newInstance",
		) {
			fmt.Println(entryDot)
			t.Fatal("failed to match all of the substring, bad dot graph")
		}

		if !utils.MatchAllOfSubString(
			sourceDot,
			"fontcolor", "color",
			"step[",
			"penwidth=\"3.0\"",
			": call",
			"search-exact:parse", // "search parse",
			"all-actual-args",
			"ByteArrayInputStream",
			"getBytes",
			"parse",
			"UTF-8",
			"xmlStr",
		) {
			fmt.Println(sourceDot)
			t.Fatal("failed to match all of the substring, bad dot graph")
		}
	})
}
