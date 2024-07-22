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
		result := prog.SyntaxFlow(`C.println() as $entry;`)
		rets := result.GetValues("entry")
		if rets.Len() <= 0 {
			t.Fatal("no entry")
		}
		step1Args := rets[0].GetCallArgs()
		arg0 := step1Args[0]
		if arg0.GetConst() != nil {
			arg0.Show()
			t.Error("C.println(* as $params); params cannot be const")
		}
		//arg1 := step1Args[1]
		return nil
	}, ssaapi.WithLanguage(consts.JAVA))
}
