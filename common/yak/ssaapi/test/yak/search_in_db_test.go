package ssaapi

import (
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
