package java

import (
	_ "embed"
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestTraceConstructor(t *testing.T) {
	var checkGBK = false
	ssatest.Check(t, `public class RuntimeExecCrossFunction {
    public String crossFunction(String cmd) {
        return "echo 'Hello World'";
    }

    @RequestMapping("/runtime")
    public String RuntimeExecCross(String cmd, Model model) {
        StringBuilder sb = new StringBuilder();
        String line;

        try {
            Process proc = Runtime.getRuntime().exec(this.crossFunction(cmd));
            InputStream fis = proc.getInputStream();
            InputStreamReader isr = new InputStreamReader(fis, "GBK");
            BufferedReader br = new BufferedReader(isr);
            while ((line = br.readLine()) != null) {
                sb.append(line).append(System.lineSeparator());
            }

        } catch (IOException e) {
            e.printStackTrace();
            sb.append(e);
        }
        model.addAttribute("results", sb.toString());
        return "basevul/rce/runtime";
    }
}`, func(prog *ssaapi.Program) error {
		prog.Show()
		var results ssaapi.Values
		results = prog.SyntaxFlowChain(`br as $sink`).Show()
		if results.Len() <= 0 {
			t.Fatal("br not found")
		}

		checkGBK = false
		prog.SyntaxFlowChain(`br #-> * as $source`).Show().ForEach(func(value *ssaapi.Value) {
			if value.GetConstValue() == "GBK" {
				checkGBK = true
			}
		})
		if !checkGBK {
			t.Fatal("GBK not found")
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestCrossMethod(t *testing.T) {
	var check = false
	ssatest.Check(t, `public class RuntimeExecCrossFunction {
    public String crossFunction(String cmd) {
        return "echo 'Hello World'";
    }

    @RequestMapping("/runtime")
    public String RuntimeExecCross(String cmd, Model model) {
		Process finalCmd = this.crossFunction(cmd);
    }
}`, func(prog *ssaapi.Program) error {
		prog.Show()
		var results ssaapi.Values
		results = prog.SyntaxFlowChain(`finalCmd as $sink`).Show()
		if results.Len() <= 0 {
			t.Fatal("finalCmd not found")
		}

		check = false
		prog.SyntaxFlowChain(`finalCmd #-> * as $source`).Show().ForEach(func(value *ssaapi.Value) {
			if ret := value.GetConstValue(); strings.Contains(fmt.Sprint(ret), "Hello World") {
				check = true
			}
		})
		if !check {
			t.Fatal("Cross Method failed not found")
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestCrossMethod_UseMethodBeforeDecl(t *testing.T) {
	var check = false
	ssatest.Check(t, `public class RuntimeExecCrossFunction {
    @RequestMapping("/runtime")
    public String RuntimeExecCross(String cmd, Model model) {
		Process finalCmd = this.crossFunction(cmd);
    }

	public String crossFunction(String cmd) {
        return "echo 'Hello World'";
    }
}`, func(prog *ssaapi.Program) error {
		prog.Show()
		var results ssaapi.Values
		results = prog.SyntaxFlowChain(`finalCmd as $sink`).Show()
		if results.Len() <= 0 {
			t.Fatal("finalCmd not found")
		}

		check = false
		prog.SyntaxFlowChain(`finalCmd #-> * as $source`).Show().ForEach(func(value *ssaapi.Value) {
			if ret := value.GetConstValue(); strings.Contains(fmt.Sprint(ret), "Hello World") {
				check = true
			}
		})
		if !check {
			t.Fatal("Cross Method failed not found")
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestCrossMethod_UseMethodBeforeDecl_NoThis(t *testing.T) {
	var check = false
	ssatest.Check(t, `public class RuntimeExecCrossFunction {
    @RequestMapping("/runtime")
    public String RuntimeExecCross(String cmd, Model model) {
		Process finalCmd = crossFunction(cmd);
    }

	public String crossFunction(String cmd) {
        return "echo 'Hello World'";
    }
}`, func(prog *ssaapi.Program) error {
		prog.Show()
		var results ssaapi.Values
		results = prog.SyntaxFlowChain(`finalCmd as $sink`).Show()
		if results.Len() <= 0 {
			t.Fatal("finalCmd not found")
		}

		check = false
		prog.SyntaxFlowChain(`finalCmd #-> * as $source`).Show().ForEach(func(value *ssaapi.Value) {
			if ret := value.GetConstValue(); strings.Contains(fmt.Sprint(ret), "Hello World") {
				check = true
			}
		})
		if !check {
			t.Fatal("Cross Method failed not found")
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}
