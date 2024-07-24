package java

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

const DefaultOBJMethodCall = `package com.example.demo.controller.deepcross;
@RestController
public class DeepCrossController {
    @GetMapping({"/xss/direct/6"})
    public ResponseEntity<String> noDeepCross6(@RequestParam(required = false) String body) {
        if (body == null) {
            return ResponseEntity.ok("No input, try <a href='/xss/no-cross?body=hello-world'>here</a>");
        }
		body += "abc";
        body = body.replaceAll("Hello", "---Hello---");
        body += "\n\nSigned by DeepCrossController";
        body = DummyUtil.filterXSS(body);
        ResponseEntity<String> resp = new ResponseEntity(body, HttpStatus.OK);
        return resp;
    }

	public String test(String body) {
		body += "abc";
		body = body.toString();
return body;
	}
}

`

func TestDefaultOBJMethodCall(t *testing.T) {
	ssatest.Check(t, DefaultOBJMethodCall, func(prog *ssaapi.Program) error {
		prog.Show()
		result := prog.SyntaxFlow(`.replaceAll(*?{!opcode: const} as $param,) as $sink; check $param`)
		if result.GetValues("param").Len() <= 0 {
			t.Fatal("replaceAll bind object not found")
		}
		return nil
	}, ssaapi.WithLanguage(consts.JAVA))
}

func TestDefaultOBJMethodCall2(t *testing.T) {
	ssatest.Check(t, DefaultOBJMethodCall, func(prog *ssaapi.Program) error {
		prog.Show()
		result := prog.SyntaxFlow(`.toString(*?{!opcode: const} as $param,) as $sink; check $param`)
		if result.GetValues("param").Show().Len() <= 0 {
			t.Fatal("toString bind object not found")
		}
		return nil
	}, ssaapi.WithLanguage(consts.JAVA))
}

func TestDefaultOBJMethodCall3(t *testing.T) {
	ssatest.Check(t, `
class A {
	static String value = "abc";

	static {
		if (outterCondition) {
			value = "eee";
        }
	}

	public static String hello() {

		value = "ddd";
		A.value = "ggg";

		return "world";
	}

	public String objMethod() {
		return "world";
	}
}

class B {
	public static void main() {
		dump(A.value); // 这个表达式 A.value 不可能是常量，他一定是个 phi;

		A obj = new A();  //  
		obj.hello();      // 在执行  A.hello() 或者 obj.hello() 之后，此时 A.value 为 "ddd";

		// 这个语句的审计要求是：
        //   1. objMethod 必须有一个参数，就是 obj 自身
        //   2. 跨过程必须成功，obj 和 result 数据流上没有任何关系，obj --> * 不可能经过 result;
		String result = obj.objMethod();  

		// 这个语句的审计要求是：
        //   1. objMethod 必须有一个参数，就是 obj 自身
        //   2. 跨过程必须失败，因为压根没有 objNoExistedMethod，
        //         因为无法掌握 objNoExistedMethod 的实现细节，所以
		//         obj 和 result 数据流上是有关系的，obj --> * 必须和 result2 有关;
		String result2 = obj.objNoExistedMethod();
	}
}


`, func(prog *ssaapi.Program) error {
		result := prog.SyntaxFlow(`dump(* as $param);`)
		rets := result.GetValues("param")
		rets.Show()
		if rets.Len() <= 0 {
			t.Fatal("no entry")
		}

		return nil
	}, ssaapi.WithLanguage(consts.JAVA))
}
func TestDefaultOBJFieldCall4(t *testing.T) {
	ssatest.Check(t, `
class A {
	static String staticValue = "abc";

	static {
		if (outterCondition) {
			staticValue = "eee";
        }
	}

	public static String hello() {
		staticValue = "ddd";
	}
}

`, func(prog *ssaapi.Program) error {
		result := prog.SyntaxFlow(`staticValue as $entry;`)
		rets := result.GetValues("entry")
		if rets.Len() <= 0 {
			t.Fatal("no entry")
		}
		if rets.Len() != 4 {
			t.Fatal("staticValue should be 4")
		}
		return nil
	}, ssaapi.WithLanguage(consts.JAVA))
}

func TestDefaultOBJNoExistedMethodCall5(t *testing.T) {
	ssatest.Check(t, `
class A {
	public static String hello() {
		noExisted();
	}
}

`, func(prog *ssaapi.Program) error {
		prog.Show()
		result := prog.SyntaxFlow(`noExisted(* as $entry)`)
		rets := result.GetValues("entry")
		if rets.Len() <= 0 {
			t.Fatal("no entry")
		}
		rets.Show()
		return nil
	}, ssaapi.WithLanguage(consts.JAVA))
}

func TestDefaultOBJParamAsCaller(t *testing.T) {
	ssatest.Check(t, `
class A {
	public void A(int p){
	p.Call("hello");
	}
}
`, func(prog *ssaapi.Program) error {
		result := prog.SyntaxFlow(`.Call(*?{!opcode: const} as $param,);`)
		rets := result.GetValues("param")
		if rets.Len() <= 0 {
			t.Fatal("no param")
		}
		if rets.Len() != 0 {
			t.Fatal("param should be 1 ")
		}
		rets.Show()
		return nil
	}, ssaapi.WithLanguage(consts.JAVA))

}

func TestDefaultOBJWithTypeTransform(t *testing.T) {
	ssatest.Check(t, `
class A {
	public void A(){
	input = "ls";
	cmd = (String) method.invoke(clazz.newInstance(), ls);
	}
}
`, func(prog *ssaapi.Program) error {
		result := prog.SyntaxFlow(`method.invoke(* #-> as $target);`)
		rets := result.GetValues("target")
		if rets.Len() <= 0 {
			t.Fatal("no target")
		}
		rets.Show()
		return nil
	}, ssaapi.WithLanguage(consts.JAVA))

}

func TestAAA3(t *testing.T) {
	code := `
public class A{
	public int num;
	public void setNum(int n){
	this.num=n;
}
	public int getNum(){
		return this.num;
}

}

public class B{
	public static void main(){
		A a =new A();
		a.setNum(22);
		int target = a.getNum();
	}
}


`
	prog, err := ssaapi.Parse(code, ssaapi.WithLanguage("java"))
	if err != nil {
		t.Fatal(err)
	}
	prog.Show()
	result := prog.SyntaxFlow(`target #-> as $a;`)

	result.Show()

}
