package java

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestNativeCallOpCode(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("A.java", `
	package com.org.A;
	class A {
		 int num=1;
		public  int methodA(int a) {//Function,Parameter
			try {
				println(this.num);//ParameterMember
				b=6+6; //BinOp
				a = 2;//ConstInst
				if (c){//Undefined,If,Jump
					a=22;
					if(d){
						bb = !true;//UnOp
						int[] myArray = {1, 2, 3, 4, 5};//Make
					}
				}else {
					a=33;
				}
				prinln(a);//Call,Phi
				for (int i=0;i<10;i++){//Loop
					a+=i;
				}
				switch(a){}//Switch
				return a;//Return
			}catch (Exception e){//ErrorHandler
				return 0;
			}
		}
	}	
	`)
	ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
		prog := programs[0]
		prog.Show()
		
		result := prog.SyntaxFlowChain("a<opcodes> as $result").Show()
		assert.Equal(t,16,result.Len()) 
		result =prog.SyntaxFlowChain("b<getFunc><opcodes> as $result",sfvm.WithEnableDebug(true))
		assert.Equal(t,16,result.Len()) 
		return nil
	})
}

func TestGetSourceCode(t *testing.T){
	vf := filesys.NewVirtualFs()
	vf.AddFile("B.java", `
	package com.org.B;
	class B {
		public static void main(String[] args) {
				b=6+6; 
				a = 2;
				if (c){
					a=22;
				}else {
					a=33;
				}
				prinln(a);
		}
	}
	`)
	ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
		prog := programs[0]
		prog.Show()
		result := prog.SyntaxFlowChain("a<sourceCode> as $result").Show(false)
		assert.Equal(t,4,result.Len())

		result = prog.SyntaxFlowChain("b<sourceCode(context=2)>?{have:'class B'&&have:'if (c)'} as $result;").Show(false)
		assert.Equal(t,1,result.Len())

		result = prog.SyntaxFlowChain("b<sourceCode(context=666666666)>?{have:'package com.org.B'} as $result;").Show(false)
		assert.Equal(t,1,result.Len())
		return nil
	})
}

func TestNativeCall_FreeMaker_GetSourceCode(t *testing.T) {
	vf := filesys.NewVirtualFs()

	vf.AddFile("com/example/demo/controller/freemakerdemo/FreeMakerDemo.java",`package com.example.demo.controller.freemakerdemo;
    
@Controller
@RequestMapping("/freemarker")
public class FreeMakerDemo {
    

    @GetMapping("/welcome")
    public String welcome(@RequestParam String name, Model model) {
        if (name == null || name.isEmpty()) {
            model.addAttribute("name", "Welcome to Safe FreeMarker Demo, try <code>/freemarker/safe/welcome?name=Hacker<>");
        } else {
            model.addAttribute("name", name);
        }
        return "welcome";
    }

}`)
	vf.AddFile("src/main/resources/application.properties",`spring.application.name=demo
# freemaker
spring.freemarker.template-loader-path=classpath:/templates/
spring.freemarker.suffix=.ftl
`)
	vf.AddFile("welcome.ftl",`<!DOCTYPE html>
<html>
<head>
    <title>Welcome</title>
</head>
<body>
<h1>Welcome ${name1}!</h1>
</body>
</html>
`)
ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
		prog := programs[0]
		sink := prog.SyntaxFlowChain(`*Mapping.__ref__<getFunc><getReturns>?{<typeName>?{have:'string'}}<freeMarkerSink><sourceCode>as $result;`,sfvm.WithEnableDebug(true)).Show()
        assert.Equal(t, 1, sink.Len())
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}
