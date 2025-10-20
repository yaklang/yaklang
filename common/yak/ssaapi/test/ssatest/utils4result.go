package ssatest

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
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

func CheckResult(t *testing.T, code string, rule string, handler func(*ssaapi.SyntaxFlowResult), sfOption []ssaapi.QueryOption, opt []ssaapi.Option) {
	Check(t, code, func(p *ssaapi.Program) error {
		sfOption = append(sfOption, ssaapi.QueryWithEnableDebug())
		res, err := p.SyntaxFlowWithError(rule, sfOption...)
		require.NoError(t, err)

		// memory
		log.Infof("memory result")
		handler(res)

		// database
		if p.GetProgramName() != "" {
			id, err := res.Save(schema.SFResultKindDebug)
			require.NoError(t, err)
			res, err = ssaapi.LoadResultByID(id)
			require.NoError(t, err)
			log.Infof("database result ")
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
	}, nil, opt)
}

func CheckSyntaxFlow(t *testing.T, code string, sf string, wants map[string][]string, opt ...ssaapi.Option) {
	checkSyntaxFlowEx(t, code, sf, false, wants, opt, nil)
}
func CheckSyntaxFlowPrintWithPhp(t *testing.T, code string, wants []string) {
	checkSyntaxFlowEx(t, code, `println(* #-> * as $param)`, true, map[string][]string{"param": wants}, []ssaapi.Option{ssaapi.WithLanguage(ssaapi.PHP)}, nil)
}
func CheckSyntaxFlowContain(t *testing.T, code string, sf string, wants map[string][]string, opt ...ssaapi.Option) {
	checkSyntaxFlowEx(t, code, sf, true, wants, opt, nil)
}

func CheckSyntaxFlowWithFS(t *testing.T, fs fi.FileSystem, sf string, wants map[string][]string, contain bool, opt ...ssaapi.Option) {
	CheckResultWithFS(t, fs, sf, func(sfr *ssaapi.SyntaxFlowResult) {
		CompareResult(t, contain, sfr, wants)
	}, opt...)
}

func CheckSyntaxFlowSource(t *testing.T, code string, sf string, want map[string][]string, opt ...ssaapi.Option) {
	CheckResult(t, code, sf, func(results *ssaapi.SyntaxFlowResult) {
		results.Show(sfvm.WithShowCode())
		require.NotNil(t, results)
		for name, want := range want {
			log.Infof("name:%v want: %v", name, want)
			gotVs := results.GetValues(name)
			require.GreaterOrEqual(t, len(gotVs), len(want), "key[%s] not found", name)
			got := lo.Map(gotVs, func(v *ssaapi.Value, _ int) string { return v.GetRange().GetText() })
			log.Infof("got: %v", got)
			require.Equal(t, len(gotVs), len(want))
			require.Equal(t, want, got, "key[%s] not match", name)
		}
	}, nil, opt)
}

func CheckSyntaxFlowEx(t *testing.T, code string, sf string, contain bool, wants map[string][]string, opt ...ssaapi.Option) {
	checkSyntaxFlowEx(t, code, sf, contain, wants, opt, nil)
}

func CheckSyntaxFlowWithSFOption(t *testing.T, code string, sf string, wants map[string][]string, opt ...ssaapi.QueryOption) {
	checkSyntaxFlowEx(t, code, sf, false, wants, nil, opt)
}

func checkSyntaxFlowEx(t *testing.T, code string, sf string, contain bool, wants map[string][]string, ssaOpt []ssaapi.Option, sfOpt []ssaapi.QueryOption) {
	CheckResult(t, code, sf, func(sfr *ssaapi.SyntaxFlowResult) {
		CompareResult(t, contain, sfr, wants)
	}, sfOpt, ssaOpt)
}

func CheckBottomUser(t *testing.T, code, variable string, want []string, contain bool, opt ...ssaapi.Option) {
	rule := fmt.Sprintf("%s as $start; $start --> as $target", variable)
	CheckResult(t, code, rule, func(result *ssaapi.SyntaxFlowResult) {
		CompareResult(t, contain, result, map[string][]string{
			"target": want,
		})
	}, nil, opt)
}

func CheckTopDef(t *testing.T, code, variable string, want []string, contain bool, opt ...ssaapi.Option) {
	rule := fmt.Sprintf("%s as $start; $start #-> as $target", variable)
	CheckResult(t, code, rule, func(result *ssaapi.SyntaxFlowResult) {
		// result.GetValues("target").ShowDot()
		CompareResult(t, contain, result, map[string][]string{
			"target": want,
		})
	}, nil, opt)
}

func CompareResult(t *testing.T, contain bool, results *ssaapi.SyntaxFlowResult, wants map[string][]string) {
	results.Show(sfvm.WithShowAll())
	for name, want := range wants {
		gotVs := results.GetValues(name)
		// gotVs.ShowDot()
		if contain {
			require.GreaterOrEqual(t, len(gotVs), len(want), "key[%s] not found", name)
		} else {
			require.Equal(t, len(gotVs), len(want), "key[%s] not found", name)
		}
		got := lo.Map(gotVs, func(v *ssaapi.Value, _ int) string { return v.String() })
		sort.Strings(got)
		sort.Strings(want)
		if contain {
			// every want should be found in got
			for _, containSubStr := range want {
				match := false
				// should contain at least one
				for _, g := range got {
					if strings.Contains(g, containSubStr) {
						match = true
					}
				}
				if !match {
					t.Errorf("key: %s want[%s] not found in got[%v]", name, want, got)
					t.FailNow()
				}
			}
		} else {
			require.Equal(t, len(want), len(gotVs))
			require.Equal(t, want, got, "key[%s] not match", name)
		}
	}
}
