package java

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestGraphFrom_XXE(t *testing.T) {
	sfRule := `
desc("Description": 'checking setFeature/setXIncludeAware/setExpandEntityReferences in DocumentBuilderFactory.newInstance()')

DocumentBuilderFactory.newInstance()?{!((.setFeature) || (.setXIncludeAware) || (.setExpandEntityReferences))} as $entry;
$entry.*Builder().parse(* #-> as $source);

check $source then "XXE Attack" else "XXE Safe";
`
	t.Run("test graph in memory", func(t *testing.T) {
		ssatest.Check(t, XXE_Code, func(prog *ssaapi.Program) error {
			assert.Equal(t, prog.GetLanguage(), "java")
			results := prog.SyntaxFlowChain(sfRule).DotGraph()
			print(results)
			if !utils.MatchAllOfSubString(
				results,
				"fontcolor", "color",
				"step[",
				"penwidth=\"3.0\"",
				": call",
				"search-exact:parse", // "search parse",
				"all-actual-args",
			) {
				fmt.Println(results)
				t.Fatal("failed to match all of the substring, bad dot graph")
			}
			return nil
		}, ssaapi.WithLanguage(ssaapi.JAVA))
	})
	t.Run("test graph from database value", func(t *testing.T) {
		programName := uuid.NewString()
		prog, err := ssaapi.Parse(XXE_Code, ssaapi.WithProgramName(programName), ssaapi.WithLanguage(consts.JAVA))
		require.NoError(t, err)
		require.NotNil(t, prog)

		res, err := prog.SyntaxFlowWithError(sfRule)
		require.NoError(t, err)
		require.NotNil(t, res)

		resultID, err := res.Save()
		require.NoError(t, err)
		_ = resultID
		//defer func() {
		//	ssadb.DeleteProgram(ssadb.GetDB(), programName)
		//}()
		resValueID, err := ssadb.GetAllResultValue(ssadb.GetDB(), res.GetResultID())
		log.Infof("resValueID: %v", resValueID)
		auditValues, err := ssadb.GetAuditValuesByIds(ssadb.GetDB(), resValueID, programName)
		require.NoError(t, err)
		require.NotNil(t, auditValues)
		fmt.Println(ssaapi.NewSFGraphWithAuditValues(auditValues).DotGraph())
	})

}
