package ssaapi

import (
	"fmt"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"testing"

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
	ssatest.CheckSyntaxFlow(t, code,
		`req* as $target`,
		map[string][]string{
			"target": {"Parameter-request"},
		})
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
	params := prog.SyntaxFlowChain(`req*.Get*() as $params`).Show()
	upSource := prog.SyntaxFlowChain("os.System(* as $sink)").Show()
	r := ssaapi.FindFlexibleDependsIntersection(upSource, params).Show().Len()
	if r <= 0 {
		t.Fatal("cannot find flexible depends intersection")
	}

	fmt.Println("-------------------------------------------------------")

	r = ssaapi.FindFlexibleCommonDepends(append(upSource, params...)).Show().Len()
	if r != 1 {
		t.Fatal("cannot find depends in common")
	}
}
