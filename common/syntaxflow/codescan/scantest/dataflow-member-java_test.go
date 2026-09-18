package scantest

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

const dataflowMemberJavaRule = `Runtime.getRuntime().exec(* #-> * as $target)`

// TestDataflowMemberJava pins object/member dataflow behaviour for Java.
//
// Java entry points are open-ended (framework annotations, reflection), but a
// field value must still travel a real call/assignment path. The cases below
// assert that polymorphic arguments do propagate, while independent instances
// with no call path do not.
func TestDataflowMemberJava(t *testing.T) {
	opt := []ssaconfig.Option{ssaapi.WithLanguage(ssaconfig.JAVA)}

	t.Run("field write and read on same object", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
package com.example;

@RestController()
public class CaseSameObject {
    public void run(@RequestParam String cmd) throws Exception {
        CmdObject bean = new CmdObject();
        bean.setCmd(cmd);
        Runtime.getRuntime().exec(bean.getCmd());
    }
}

class CmdObject {
    private String cmd;
    public void setCmd(String c) { this.cmd = c; }
    public String getCmd() { return this.cmd; }
}
`, dataflowMemberJavaRule, map[string][]string{
			"target": {"Parameter-cmd", "Undefined-Runtime"},
		}, opt...)
	})

	t.Run("field flows through real cross-class call path", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
package com.example;

class CmdBase {
    public String cmd;
}

class CmdChild extends CmdBase {
}

class Runner {
    void run(CmdBase holder) throws Exception {
        Runtime.getRuntime().exec(holder.cmd);
    }
}

@RestController()
public class CaseFlow {
    public void assign(@RequestParam String cmd) {
        CmdBase value = new CmdChild();
        value.cmd = cmd;
        Runner runner = new Runner();
        runner.run(value);
    }
}
`, dataflowMemberJavaRule, map[string][]string{
			"target": {"Parameter-cmd", "Undefined-Runtime"},
		}, opt...)
	})

	// Independent instances with no call path: assign() never hands its object
	// to run(), so the field value must not leak into the exec() read site.
	t.Run("same-type instances without call path do not leak", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
package com.example;

class CmdBase {
    public String cmd;
}

class CmdChild extends CmdBase {
}

class Runner {
    void run(CmdBase holder) throws Exception {
        Runtime.getRuntime().exec(holder.cmd);
    }
}

@RestController()
public class CaseNoPath {
    public void assign(@RequestParam String cmd) {
        CmdBase value = new CmdChild();
        value.cmd = cmd;
    }
}
`, dataflowMemberJavaRule, map[string][]string{
			"target": {"Parameter-holder", "Undefined-Runtime"},
		}, opt...)
	})

	// Same field name on two unrelated classes must not cross-resolve: a shared
	// field name is not a type relation.
	t.Run("same field name on unrelated classes do not cross-resolve", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
package com.example;

class Alpha {
    public String cmd;
}

class Beta {
    public String cmd;
}

class Runner {
    void run(Alpha holder) throws Exception {
        Runtime.getRuntime().exec(holder.cmd);
    }
}

@RestController()
public class CaseDiff {
    public void assign(@RequestParam String cmd) {
        Beta other = new Beta();
        other.cmd = cmd;
    }
}
`, dataflowMemberJavaRule, map[string][]string{
			"target": {"Parameter-holder", "Undefined-Runtime"},
		}, opt...)
	})
}
