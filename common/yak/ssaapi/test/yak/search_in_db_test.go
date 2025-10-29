package ssaapi

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestSearchInDatabase(t *testing.T) {
	code := `
handler = (request, response) => {
	cmd, err = request.GetParams("cmd")
	die(err)
	if cmd.Contains("system") {
		cmd = cmd.Replace("system", "bad-bad")
	}
	response.Write(os.System(cmd))
}
register("/route1", handler)
`
	ssatest.CheckSyntaxFlowContain(t, code, `req*.GetParams() --> * as $target`, map[string][]string{
		"target": {"ParameterMember-parameter[1].Write(ParameterMember-freeValue-os"},
	}, ssaapi.WithLanguage(ssaconfig.Yak))
}

func TestSearchAndFind(t *testing.T) {
	code := `
cmd = request.GetParam("cmd")
cmd.ReplaceAll("hacked", "foo-bar")
if cmd.Contains("ca") {
	cmd = str.Replace("foo", "bar")
}
os.System(cmd)
`
	prog, err := ssaapi.Parse(code)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("----------PARAMS------------")
	params := prog.SyntaxFlowChain(`req*.Get*() as $params`).Show()
	fmt.Println("-----------SINK-----------")
	upSource := prog.SyntaxFlowChain("os.System(* as $sink)").Show()
	fmt.Println("-----------Intersection---------------")
	r := ssaapi.FindFlexibleDependsIntersection(upSource, params).Show().Len()
	if r <= 0 {
		t.Fatal("cannot find flexible depends intersection")
	}

	fmt.Println("-------------CommonDepends---------------------------------")

	r = ssaapi.FindFlexibleCommonDepends(append(upSource, params...)).Show().Len()
	if r != 1 {
		t.Fatal("cannot find depends in common")
	}
}
