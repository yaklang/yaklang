package java

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestBlueprint(t *testing.T) {
	code := `package main;
class B {
    public int b = 1;
}

class A {
    public B a;

    public void A() {
        System.out.println(this.a.b);
    }
}

public class Main {
    public static void main(String[] args) {
        A a = new A();
        a.a = new B();
        a.A();  // 显式调用方法
    }
}
`
	ssatest.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{"param": {"1", "Undefined-System"}}, ssaapi.WithLanguage(ssaapi.JAVA))
}
