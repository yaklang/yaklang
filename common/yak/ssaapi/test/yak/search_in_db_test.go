package ssaapi

import (
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"testing"
)

func TestSearchInDatabase(t *testing.T) {
	uid := uuid.New().String()
	_, err := ssaapi.Parse(`
handler = (request, response) => {
	cmd, err = request.GetParams("cmd")
	die(err)
	if cmd.Contains("system") {
		cmd = cmd.Replace("system", "bad-bad")
	}
	response.Write(os.System(cmd))
}
register("/route1", handler)
`, ssaapi.WithDatabaseProgramName(uid))
	if err != nil {
		t.Fatal(err)
	}
	prog, err := ssaapi.FromDatabase(uid)
	if err != nil {
		t.Fatal(err)
	}
	prog.GlobRefRaw(`req*`).Show()
}
