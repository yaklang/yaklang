package java

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestJavaFieldSensitiveTypeFallbackWithoutCallGraph(t *testing.T) {
	t.Run("positive cmd field", func(t *testing.T) {
		code := `
package com.example.utils;

class CmdBase {
    public String cmd;
    public String safe;
}

class CmdChild extends CmdBase {
}

class Runner {
    void run(CmdBase holder) throws Exception {
        Runtime.getRuntime().exec(holder.cmd);
    }
}

@RestController()
public class AstTaintCase001 {
    public void assign(@RequestParam String cmd) {
        CmdBase value = new CmdChild();
        value.cmd = cmd;
        value.safe = "safe";
    }
}
`
		ssatest.CheckSyntaxFlowEx(t, code, `Runtime.getRuntime().exec(* #-> * as $target)`, false, map[string][]string{
			"target": {"Parameter-cmd", "Undefined-Runtime"},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("negative safe field", func(t *testing.T) {
		code := `
package com.example.utils;

class CmdBase {
    public String cmd;
    public String safe;
}

class CmdChild extends CmdBase {
}

class Runner {
    void run(CmdBase holder) throws Exception {
        Runtime.getRuntime().exec(holder.safe);
    }
}

@RestController()
public class AstTaintCase001 {
    public void assign(@RequestParam String cmd) {
        CmdBase value = new CmdChild();
        value.cmd = cmd;
        value.safe = "safe";
    }
}
`
		ssatest.CheckSyntaxFlowEx(t, code, `Runtime.getRuntime().exec(* #-> * as $target)`, false, map[string][]string{
			"target": {`"safe"`, "Undefined-Runtime"},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}
