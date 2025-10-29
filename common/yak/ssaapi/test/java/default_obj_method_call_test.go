package java

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
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
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestDefaultOBJMethodCall2(t *testing.T) {
	ssatest.Check(t, DefaultOBJMethodCall, func(prog *ssaapi.Program) error {
		prog.Show()
		result := prog.SyntaxFlow(`.toString(*?{!opcode: const} as $param,) as $sink; check $param`)
		if result.GetValues("param").Show().Len() <= 0 {
			t.Fatal("toString bind object not found")
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
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
		dump1(A.value); // 这个表达式 A.value 不可能是常量，他一定是个 phi;

		A obj = new A();  //  
		obj.hello();      // 在执行  A.hello() 或者 obj.hello() 之后，此时 A.value 为 "ddd";
		dump2(A.value);
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
		prog.Show()
		dump1 := prog.SyntaxFlow(`dump1(* as $param);`)
		d1 := dump1.GetValues("param")
		if d1.Show().Len() == 0 {
			t.Fatal("have not dump1")
		}
		if !strings.Contains(d1.String(), "abc") {
			t.Fatalf("dump1's parma error.want \"abc\",but got %s", d1.String())
		}
		// Todo：A.value的设置读取应该为phi
		//dump2 := prog.SyntaxFlow(`dump2(* as $param);`)
		//d2 := dump2.GetValues("param")
		//if d2.Show().Len() == 0 {
		//	t.Fatal("have not dump2")
		//}
		//if !strings.Contains(d2.String(), "\"ggg\"") {
		//	t.Fatalf("dump2's parma error,want \"ggg\",but got %s", d2.String())
		//}

		res1 := prog.SyntaxFlow(`result #-> as $result;`).GetValues("result")
		param1 := prog.SyntaxFlow(`obj.objMethod(* as $param1);`).GetValues("param1")
		if res1.Show().Len() == 0 {
			t.Fatal("result have no value")
		}
		if !strings.Contains(res1.String(), "world") {
			t.Fatalf("get result value error.want \"world\",but got %s", res1.String())
		}
		if param1.Show().Len() != 1 {
			t.Fatal("obj.ObjMethod() should have receiver as param")
		}

		res2 := prog.SyntaxFlow(`result2 #-> as $result;`).GetValues("result2")
		if res2.Show().Len() != 0 {
			t.Fatal("result2 get top def value do not equal 0")
		}
		param2 := prog.SyntaxFlow(` obj.objMethod(* as $param2);`).GetValues("param2")
		if param2.Show().Len() != 1 {
			t.Fatal("obj.objNoExistedMethod should have receiver as param")
		}
		// TODO: 跨过程，obj --> * 必须和 result2 ,需要修复ssaapi
		//target := prog.SyntaxFlow("obj-->* as $target;").GetValues("target")
		//target.Show()
		//if !strings.Contains(target.String(), "obj.objNoExistedMethod") {
		//	t.Fatal("obj-->* should contain obj.objNoExistedMethod")
		//}

		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}
func TestDefaultOBJFieldCall4(t *testing.T) {
	ssatest.CheckSyntaxFlow(t, `
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

`, `staticValue as $entry;`, map[string][]string{
		"entry": []string{"\"abc\"", "\"ddd\"", "\"eee\"", "phi(staticValue)[\"eee\",\"abc\"]"},
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestDefaultOBJParamAsCaller(t *testing.T) {
	ssatest.Check(t, `
class Cat{}
class A {
	public void A(Dog p){
		// 参数类型为null
		p.CallA("hello");
	}
	public void B(Cat p){
		// 参数类型存在
		p.CallB("hello");
	}
	public void C(int p){
		//参数类型为基础类型
		p.CallC("hello");
 }
}
`, func(prog *ssaapi.Program) error {
		callA := prog.SyntaxFlow(`p.CallA(* as $param,)`).GetValues("param")
		assert.Contains(t, callA.String(), "Parameter-p")
		callB := prog.SyntaxFlow(`p.CallB(* as $param,)`).GetValues("param")
		assert.Contains(t, callB.String(), "Parameter-p")
		callC := prog.SyntaxFlow(`p.CallC(* as $param,)`).GetValues("param")
		assert.Contains(t, callC.String(), "Parameter-p")
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestDefaultOBJ_Type_Transform(t *testing.T) {
	ssatest.Check(t, `
class cat{}
class A {
	public void A(Dog p){
		p += 2; // 转化为二元运算符
		p.CallA("hello");
	}
	public void B(Dog p){
		p > 2;
		p.CallB("hello");
	}
	public void C(Dog p){
		Cat p = (Cat) p;
		p.CallC("hello");
 }
	
}
`, func(prog *ssaapi.Program) error {
		prog.Show()
		callA := prog.SyntaxFlow(`p.CallA(* as $param,)`, ssaapi.QueryWithEnableDebug(true)).GetValues("param")
		callA.Show()
		assert.Contains(t, callA.String(), "add(Parameter-p, 2)")
		callB := prog.SyntaxFlow(`p.CallB(* as $param,)`).GetValues("param")
		assert.Contains(t, callB.String(), "Parameter-p")
		callC := prog.SyntaxFlow(`p.CallC(* as $param,)`).GetValues("param")
		callC.Show()
		assert.Contains(t, callC.String(), "Parameter-p")
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))

}

func TestDefaultOBJMemberCallAsCaller(t *testing.T) {
	ssatest.Check(t, `
class B{
	public string getBody(Dog p){
		return "hello";
	}
}
class A {
	public void A(Dog p){
		string body = foo.getBody(p);
		body.toString1(); 
	}
	public void B(Dog p){
		B b = new B();
		string body = b.getBody(p);
		body.toString2(); //应该有this
	}
}
`, func(prog *ssaapi.Program) error {
		prog.Show()
		callA := prog.SyntaxFlow(`.toString1(* as $param)`).GetValues("param")
		assert.Equal(t, 1, callA.Len())
		callB := prog.SyntaxFlow(`.toString2(* as $param)`).GetValues("param")
		assert.Equal(t, 1, callB.Len())
		assert.Contains(t, callB.String(), "Undefined-b.getBody(valid)(Undefined-B(Undefined-B),Parameter-p)")
		callB.Show()
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))

}

func TestFunctionCallSf(t *testing.T) {
	code := `public class A{
public void a(){}
}
public class B{
	public static void main(){
		A a = new A();
		a.a();
		a.a();
	}
}
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		result, err := prog.SyntaxFlowWithError(`a.a() as $call`)
		require.NoError(t, err)
		require.True(t, result.GetValues("call").Len() == 2)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}
func TestDefaultObjForPeekValue(t *testing.T) {
	ssatest.Check(t, `
public class A{
	public void a(){};
}

public class B{
	public static void main(){
		A a =new A();
		a.a(); // 有类型，前面没有出现过，会自动添加this
		a.b(); // 没有类型，前面没有出现过，不应该创建，应该添加this
		a.a(); // 前面已经出现过，所以直接拿并返回
		a.b(); // 前面已经出现过，直接拿并返回
	}
}
`, func(prog *ssaapi.Program) error {
		prog.Show()

		result1 := prog.SyntaxFlow(`a.a() as $a;`).GetValues("a")
		result1.Show()
		assert.Equal(t, "Undefined-a.a(valid)(Undefined-A(Undefined-A))", result1[0].String())
		assert.Equal(t, "Undefined-a.a(valid)(Undefined-A(Undefined-A))", result1[1].String())
		result2 := prog.SyntaxFlow(`a.b() as $b;`).GetValues("b")
		assert.Equal(t, "Undefined-a.b(Undefined-A(Undefined-A))", result2[0].String())
		assert.Equal(t, "Undefined-a.b(Undefined-A(Undefined-A))", result2[1].String())
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))

}

func TestDefaultObjForNormalMethod(t *testing.T) {
	ssatest.Check(t, `
public class A{
	public static int a(){
	return 666;
};
}

public class B{
	public static void main(){
		int c1=A.a(); //不应该有this
		int d1=A.b(); // 应该有this
		int c2=A.a(); // 前面已经出现过，直接拿并返回
		int d2=A.b(); // 前面已经出现过，直接拿并返回
	}
}
`, func(prog *ssaapi.Program) error {
		prog.Show()
		result1 := prog.SyntaxFlow(`c* as $c;`).GetValues("c").Show(false)
		assert.Equal(t, "Function-A.a()", result1[0].String())
		assert.Equal(t, "Function-A.a()", result1[1].String())
		result2 := prog.SyntaxFlow(`d* as $d;`).GetValues("d")
		assert.Equal(t, "Undefined-A.b(A)", result2[0].String())
		assert.Equal(t, "Undefined-A.b(A)", result2[1].String())
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))

}

func TestDefaultObjForStaticMethod(t *testing.T) {
	ssatest.Check(t, `
public class A{
	public static int a(){
	return 666;
};
}

public class B{
	public static void main(){
		A object =new A();
		int c1=object.a(); // 会直接拿到a并返回
		int d1=object.b(); // typ应该为nil,所以有this
		int c2=object.a(); // 前面已经出现过，直接拿并返回
		int d2=object.b(); // 前面已经出现过，直接拿并返回
	}
}
`, func(prog *ssaapi.Program) error {
		prog.Show()
		result1 := prog.SyntaxFlow(`c* as $c;`).GetValues("c")
		assert.Equal(t, "Function-A.a()", result1[0].String())
		assert.Equal(t, "Function-A.a()", result1[1].String())
		result2 := prog.SyntaxFlow(`d* as $d;`).GetValues("d")
		assert.Equal(t, "Undefined-object.b(Undefined-A(Undefined-A))", result2[0].String())
		assert.Equal(t, "Undefined-object.b(Undefined-A(Undefined-A))", result2[1].String())
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestDefaultObj_XX(t *testing.T) {
	code := `package com.example.filedownload;

import org.springframework.core.io.FileSystemResource;
import org.springframework.http.HttpHeaders;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.RestController;

import java.io.File;

@RestController
public class FileDownloadController {

    @GetMapping("/download/{filename}")
    public ResponseEntity<FileSystemResource> downloadFile(@PathVariable String filename) {
        File file = new File("path/to/your/files/" + filename);

        if (!file.exists()) {
            return ResponseEntity.status(HttpStatus.NOT_FOUND).build();
        }
		HttpHeaders headers = new HttpHeaders();
        headers.add(HttpHeaders.CONTENT_DISPOSITION, "attachment; filename=" + file.getName());

}
     
}`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		target := prog.SyntaxFlowChain(`file.getName(* as $param)`).Show()
		assert.Equal(t, 1, target.Len())

		target = prog.SyntaxFlowChain(`headers.add(* as $param)`).Show()
		assert.Equal(t, 3, target.Len())
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}
