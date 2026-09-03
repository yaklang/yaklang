package tests

import (
	"testing"

	"github.com/samber/lo"
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
	requireCSharpCompileErrorFree(t, progs[0])
	got := lo.Map(
		progs[0].Ref("println").GetUsers().Flat(func(v *ssaapi.Value) ssaapi.Values {
			return ssaapi.Values{v.GetOperand(1)}
		}),
		func(v *ssaapi.Value, _ int) string { return v.String() },
	)
	require.Equal(t, []string{"1"}, got)
}

func TestCSharp_ClassMainStaticMain_NoObjectError(t *testing.T) {
	src := `public class Main { public static void Main(string[] args) { int a = 1; println(a); } }`
	prog, err := ssaapi.Parse(src, ssaapi.WithLanguage("csharp"))
	require.NoError(t, err)
	requireCSharpCompileErrorFree(t, prog)
	got := lo.Map(
		prog.Ref("println").GetUsers().Flat(func(v *ssaapi.Value) ssaapi.Values {
			return ssaapi.Values{v.GetOperand(1)}
		}),
		func(v *ssaapi.Value, _ int) string { return v.String() },
	)
	require.Equal(t, []string{"1"}, got)
}
