package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCFieldSensitiveTypeFallbackWithoutCallGraph(t *testing.T) {
	t.Run("positive cmd field", func(t *testing.T) {
		code := `
#include <stdio.h>

struct Box {
    char *cmd;
    char *safe;
};

void run(struct Box *holder) {
    printf("%s", holder->cmd);
}

void assign(char *cmd) {
    struct Box value;
    value.cmd = cmd;
    value.safe = "safe";
}
`
		ssatest.CheckSyntaxFlowContain(t, code, `printf(* #-> * as $target)`, map[string][]string{
			"target": {"Parameter-cmd"},
		}, ssaapi.WithLanguage(ssaconfig.C))
	})

	t.Run("negative safe field", func(t *testing.T) {
		code := `
#include <stdio.h>

struct Box {
    char *cmd;
    char *safe;
};

void run(struct Box *holder) {
    printf("%s", holder->safe);
}

void assign(char *cmd) {
    struct Box value;
    value.cmd = cmd;
    value.safe = "safe";
}
`
		ssatest.CheckSyntaxFlowContain(t, code, `printf(* #-> * as $target)`, map[string][]string{
			"target": {`"safe"`},
		}, ssaapi.WithLanguage(ssaconfig.C))
	})
}
