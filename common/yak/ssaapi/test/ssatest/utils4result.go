package ssatest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/utils/dot"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func CheckResultWithFS(t *testing.T, fs filesys_interface.FileSystem, rule string, handler func(*ssaapi.SyntaxFlowResult), opt ...ssaapi.Option) {
	CheckWithFS(fs, t, func(p ssaapi.Programs) error {
		res, err := p.SyntaxFlowWithError(rule)
		require.NoError(t, err)

		// memory
		log.Infof("only in memory")
		handler(res)

		// database
		id, err := res.Save(schema.SFResultKindDebug)
		require.NoError(t, err)
		res, err = ssaapi.LoadResultByID(id)
		require.NoError(t, err)
		log.Infof("with database")
		handler(res)

		return nil
	}, opt...)
}

func CheckResult(t *testing.T, code string, rule string, handler func(*ssaapi.SyntaxFlowResult), opt ...ssaapi.Option) {
	Check(t, code, func(p *ssaapi.Program) error {
		res, err := p.SyntaxFlowWithError(rule)
		require.NoError(t, err)

		// memory
		log.Infof("only in memory")
		handler(res)

		// database
		id, err := res.Save(schema.SFResultKindDebug)
		require.NoError(t, err)
		res, err = ssaapi.LoadResultByID(id)
		require.NoError(t, err)
		log.Infof("with database")
		handler(res)

		return nil
	}, opt...)
}

type GraphInTest struct {
	*ssaapi.DotGraph
	edgeStr []string
}

func NewTestGraph() *GraphInTest {
	return &GraphInTest{
		DotGraph: ssaapi.NewDotGraph(),
	}
}

func (g *GraphInTest) String() string {
	str := ""
	dotStr := g.DotGraph.String()
	str += dotStr
	if graph, err := dot.DotGraphToAsciiArt(dotStr); err == nil {
		str += graph + "\n"
	} else {
		str += err.Error() + "\n"
	}
	str += strings.Join(g.edgeStr, "\n")
	return str
}

func pathStr(from, to string) string {
	return fmt.Sprintf("[%s] -> [%s]", from, to)
}
func (g *GraphInTest) CreateEdge(edge ssaapi.Edge) error {
	g.DotGraph.CreateEdge(edge)
	g.edgeStr = append(g.edgeStr,
		fmt.Sprintf("%s %v",
			pathStr(edge.From.GetRange().GetWordText(), edge.To.GetRange().GetWordText()),
			edge.Msg,
		),
	)
	return nil
}

func (g *GraphInTest) Check(t *testing.T, from, to string) {
	match := false
	for _, edge := range g.edgeStr {
		want := pathStr(from, to)
		if strings.Contains(edge, want) {
			match = true
			break
		}
	}
	require.True(t, match)
}

func CheckSyntaxFlowGraph(
	t *testing.T,
	code string,
	sfRule string,
	checkFuncs map[string]func(g *GraphInTest),
	opt ...ssaapi.Option,
) {

	CheckResult(t, code, sfRule, func(res *ssaapi.SyntaxFlowResult) {
		for variableName, checkFunc := range checkFuncs {
			graph := NewTestGraph()
			for _, v := range res.GetValues(variableName) {
				v.GenerateGraph(graph)
			}
			log.Infof("graph: \n%v", graph.String())
			checkFunc(graph)
		}
	}, opt...)
}
