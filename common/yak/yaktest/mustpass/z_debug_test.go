package mustpass

import (
	"sort"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestMustPassDebug(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
		return
	}

	yakit.RegisterLowHTTPSaveCallback()

	debugName := "mixcaller2.yak"
	var debugCases [][]string
	for k, v := range files {
		if k == debugName {
			debugCases = append(debugCases, []string{k, v})
		}
	}

	sort.SliceStable(debugCases, func(i, j int) bool {
		return debugCases[i][0] < debugCases[j][0]
	})

	if vulinboxAddr == "" {
		panic("VULINBOX START ERROR")
	}

}
