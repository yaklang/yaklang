package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestCSharp_LocalAssignPrintln(t *testing.T) {
	CheckCSharpPrintlnValue(`
int a = 1;
println(a);
string s = "aaa";
println(s);
bool b = true;
println(b);
`, []string{"1", "\"aaa\"", "true"}, t)
}

func TestCSharp_IfPrintln(t *testing.T) {
	CheckCSharpPrintlnValue(`
int a = 1;
if (true) {
    a = 2;
} else {
    a = 3;
}
println(a);
`, []string{"phi(a)[2,3]"}, t)
}

func TestCSharp_CallAndMember(t *testing.T) {
	CheckCSharpPrintlnValue(`
int a = 1;
int b = a + 2;
println(b);
Foo f = new Foo();
println(f);
`, []string{"3", "Undefined-Foo(Undefined-Foo)"}, t)
}

func TestCSharp_ParseProjectWithFS(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("Demo.cs", `
using System;
namespace Demo {
    public class Box {
        public int W = 1;
        public int Area() { return W * 2; }
    }
    public class Program {
        public static void Main(string[] args) {
            Box b = new Box();
            println(b.W);
        }
    }
}
`)
	progs, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.CSHARP), ssaapi.WithMemory())
	require.NoError(t, err)
	require.NotEmpty(t, progs)
	require.NotNil(t, progs[0])
}
