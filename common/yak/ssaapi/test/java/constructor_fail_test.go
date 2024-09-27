package java

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_Creator_function(t *testing.T) {
	/*
		TODO: implement creator function side-effect
		should add side-effect kind: call-self: modify call.key = member  for this case
		and in analyzer:
			TopDef: use Object-Member trace this $a->a, and could get `1`.
			BottomUse: no test
	*/
	ssatest.CheckSyntaxFlowContain(t, `<?php
	class A {
		var $a;
	}

	function creator() {
		$a = new A;
		$a->a = 1;
		return $a;
	}

	$a = creator(); 
	println($a->a);
	`, `println(* as $member)`, map[string][]string{
		"member": {"side-effect"},
	}, ssaapi.WithLanguage(ssaapi.PHP))
}

func Test_Constructor(t *testing.T) {
	/*
		should implement Test_Creator_function first
		in this case :
			```java
			public class A {
				int a;
				public  A(int pa) {
					this.a = pa;
				}

				public static void main(){
					var a = new A(1);
					println(a.a);
				}
			}
			```
		// FIXME: this case in java2ssa
			func void A-A(A this, int pa) { // this function type should be class A,
				this.a = pa;
			}

			// FIXME: this case in all language2ssa
			func void main-main() {
				t0 = undefined/make
				t1 = call A-A(t0, 1)
				t0 = side-effect this.a = 1 by t1
				println(Undefined-t1.a)
			}

		// we want constructor be this:
			func A A-A(int pa) {
				t0 = undefined/make
				t0.a = pa
				return t0  // create side-effect for call-self
			}

			func void main-main() {
				t0 = call A-A(1)
				t0.a = side-effect t0.a=1 by t0
				println(side-... ) // t0.a
			}
	*/

	ssatest.CheckSyntaxFlow(t, `
public class A {
	int a;
	public  A(int pa) {
		this.a = pa;
	}

	public static void main(){
		var a = new A(1);
		println(a.a);
	}
	
}
	`, `println(* #-> as $member)`, map[string][]string{
		"member": {"1"},
	}, ssaapi.WithLanguage(ssaapi.JAVA),
	)
}

func Test_Runtime_Example(t *testing.T) {
	vfs := filesys.NewVirtualFs()

	vfs.AddFile("runtime.java", `
public class RuntimeBean {
	private String command;
	public RuntimeBean(String command) {
		this.command = command;
	}
	public void doCommand() {
		try {
			Runtime.getRuntime().exec(this.command);
		}catch (IOException e) {
			e.printStackTrace();
		}
	}
}
	`)

	vfs.AddFile("main.java", `
	public class main {
		public static void main(String[] args) {
			var decodeString = "ZGVjb2RlIHN0YXRpYw==";
			new RuntimeBean(decodeString).doCommand();
		}
	}
	`)

	// Compile the code
	prog, err := ssaapi.ParseProject(vfs, ssaapi.WithLanguage(ssaapi.JAVA))
	require.NoError(t, err)
	for _, p := range prog {
		p.Show()
	}

	res, err := prog.SyntaxFlowWithError(`
	Runtime.getRuntime().exec( * #-> as $command);
	`)
	require.NoError(t, err)
	require.Contains(t, res.GetValues("command").String(), "ZGVjb2RlIHN0YXRpYw==")
	res.Show()
}
