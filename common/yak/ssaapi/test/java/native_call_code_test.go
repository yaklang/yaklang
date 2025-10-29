package java

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestNativeCallOpCode(t *testing.T) {
	code := `
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
	`
	// 获取a所在函数所有的opcode
	ssatest.CheckSyntaxFlowContain(t, code, `a<opcodes> as $result;`, map[string][]string{
		"result": {"Parameter", "ParameterMember", "Return", "Loop", "Function", "Call", "Phi", "Undefined", "ConstInst", "Jump", "If", "UnOp", "Switch", "ErrorHandler", "Make", "BinOp"},
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestGetSourceCode(t *testing.T) {
	code := `
	package com.org.B;
	class B {
		public static void main(String[] args) {
				b=6+6; 
				a = 2;
				if (c){
					bb1;
				}else {
					bb2;
				}
				prinln(a);
		}
	}
	`
	ssatest.CheckSyntaxFlow(t, code, `bb1<sourceCode> as $result;`,
		map[string][]string{
			"result": {"\"\\t\\t\\t\\t\\tbb1;\\n\""},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))

	ssatest.CheckSyntaxFlow(t, code, `bb2<sourceCode(context=3)> as $result;`,
		map[string][]string{
			"result": {"\"\\t\\t\\t\\tif (c){\\n\\t\\t\\t\\t\\tbb1;\\n\\t\\t\\t\\t}else {\\n\\t\\t\\t\\t\\tbb2;\\n\\t\\t\\t\\t}\\n\\t\\t\\t\\tprinln(a);\\n\\t\\t}\\n\""},
		}, ssaapi.WithLanguage(ssaconfig.JAVA))

}
