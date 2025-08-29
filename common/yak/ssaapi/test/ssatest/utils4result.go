package ssatest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/stretchr/testify/require"

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

		//
		if len(p) == 1 && p[0].GetProgramName() != "" {
			id, err := res.Save(schema.SFResultKindDebug)
			require.NoError(t, err)
			res, err = ssaapi.LoadResultByID(id)
			require.NoError(t, err)
			log.Infof("with database")
			handler(res)
		}
		return nil
	}, opt...)
}

func CheckResult(t *testing.T, code string, rule string, handler func(*ssaapi.SyntaxFlowResult), opt ...ssaapi.Option) {
	Check(t, code, func(p *ssaapi.Program) error {
		res, err := p.SyntaxFlowWithError(rule)
		require.NoError(t, err)

		// memory
		handler(res)

		// database
		if p.GetProgramName() != "" {
			id, err := res.Save(schema.SFResultKindDebug)
			require.NoError(t, err)
			res, err = ssaapi.LoadResultByID(id)
			require.NoError(t, err)
			log.Infof("with database")
			handler(res)
		}

		return nil
	}, opt...)
}

type GraphInTest struct {
	*ssaapi.DotGraph
	edgeStr []string
}

type PathInTest struct {
	From  string
	To    string
	Label string
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
	str += strings.Join(g.edgeStr, "\n")
	return str
}

func pathStr(from, to string) string {
	return fmt.Sprintf("[%s] -> [%s]", from, to)
}
func (g *GraphInTest) CreateEdge(edge ssaapi.Edge) error {
	err := g.DotGraph.CreateEdge(edge)
	if err != nil {
		return err
	}

	g.edgeStr = append(g.edgeStr,
		fmt.Sprintf("%s %v %v",
			pathStr(edge.From.GetRange().GetText(), edge.To.GetRange().GetText()),
			edge.Msg, edge.Kind,
		),
	)
	return nil
}

func (g *GraphInTest) Check(t *testing.T, from, to string, label ...string) {
	want := pathStr(from, to)
	checkEdge := func(edge string) (ret bool) {
		if !strings.Contains(edge, want) {
			log.Infof("edge not found: `%s` vs `%v`", edge, want)
			return false
		}

		if len(label) > 0 {
			for _, l := range label {
				if !strings.Contains(edge, l) {
					log.Infof("label not found: `%s` vs `%v`", edge, l)
					return false
				}
			}
		}
		return true
	}
	match := false
	for _, edge := range g.edgeStr {
		// check path
		if checkEdge(edge) {
			match = true
			break
		}
	}
	require.True(t, match)
}

func CheckSyntaxFlowGraphEdge(t *testing.T, code string, sfRule string, paths map[string][]PathInTest, opt ...ssaapi.Option) {
	checkMap := make(map[string]func(g *GraphInTest))
	for variable, path := range paths {
		checkMap[variable] = func(g *GraphInTest) {
			for _, path := range path {
				g.Check(t, path.From, path.To, path.Label)
			}
		}
	}
	CheckSyntaxFlowGraph(t, code, sfRule, checkMap, opt...)
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
			// dot.ShowDotGraphToAsciiArt(graph.DotGraph.String())
			checkFunc(graph)
		}
	}, opt...)
}
