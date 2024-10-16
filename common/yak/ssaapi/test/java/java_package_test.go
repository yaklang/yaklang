package java

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestJava_Package_Simple(t *testing.T) {
	code := `
	package com.org.example;
	class A{}
`
	ssatest.CheckSyntaxFlow(t, code, `A.__pkg__ as $pkg`, map[string][]string{
		"pkg": {"\"com.org.example\""},
	}, ssaapi.WithLanguage(consts.JAVA))
}

func TestJavaPackage_Same_Name(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("a/src/main/java/com/org/example1/A.java", `
	package com.org.example1;
	class A{
	public void foo(){}
}
`)
	vf.AddFile("a/src/main/java/com/org/example2/A.java", `
	package com.org.example2;
	class A{
	public void bar(){}
}
`)
	ssatest.CheckWithFS(vf, t, func(progs ssaapi.Programs) error {
		prog := progs[0]
		prog.Show()
		ret := prog.SyntaxFlowChain(`A as $blueprint`)
		require.Equal(t, 4, ret.Show().Len())

		ret = prog.SyntaxFlowChain(`A?{<self>.__pkg__?{have: "com.org.example1"}}.foo as $fun1`)
		require.Contains(t, ret.Show().String(), "foo")

		ret = prog.SyntaxFlowChain(`A?{<self>.__pkg__?{have: "com.org.example2"}}.bar as $fun1`)
		require.Contains(t, ret.Show().String(), "bar")
		return nil
	}, ssaapi.WithLanguage(consts.JAVA))
}
