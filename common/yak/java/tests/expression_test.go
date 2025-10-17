package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestJava_Simple_Expression(t *testing.T) {
	t.Run("test PostfixExpression", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		a=1;
		println(a++);
		println(a--);`, []string{"2", "1"}, t)
	})
	t.Run("test PrefixUnaryExpression", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		a=11;
        println(+a);
		println(-a);
		int b = 5;
		println(~b);
		c=true;
		println(!c);`, []string{"11", "-11", "-6", "false"}, t)
	})
	t.Run("test PrefixBinaryExpression", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		a=11;
		b=++a;
		println(b);
		b=--a;
		println(b);`, []string{"12", "11"}, t)
	})
	t.Run("test MultiplicativeExpression", func(t *testing.T) {
		CheckJavaPrintlnValue(` 
		a = 2;
		b=4;
		println(a*b);
		println(b/a);
		println(b%a);`, []string{"8", "2", "0"}, t)
	})
	t.Run("test AdditiveExpression", func(t *testing.T) {
		CheckJavaPrintlnValue(` 
		a=2;
		b=4;
		println(a + b);
		println(a - b);`, []string{"6", "-2"}, t)
	})
	t.Run("test ShiftExpression", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		a=8;
		b=2;
         println(a << b);
         println(a >>>b); 
         println(a >> b); `, []string{"32", "2", "2"}, t)
	})
	t.Run("test RelationalExpression", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		a=11;
		b=22;
		println( a < b);
		println( a > b);
		println( a <= b);
		println( a >= b);`, []string{"false", "true", "false", "true"}, t)
	})
	t.Run("test EqualityExpression", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		 a=11;
		 b=22;
		 println(a == b);
		 println(b != a);`, []string{"false", "true"}, t)
	})
	t.Run("test AndE,xor,or,logicand,logicor expression", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		 println(12 & 5);
		 println(12 ^ 5);
		 println(12 | 5);
			`, []string{"4", "9", "13"}, t)
	})
	t.Run("test TernaryExpression", func(t *testing.T) {
		CheckJavaPrintlnValue(` int a = 1;
		int b = 0;
		int ret;
		ret = a > b ? a : b;
		println(ret);
		`, []string{"1"}, t)
	})
	t.Run("test AssignmentExpression", func(t *testing.T) {
		CheckJavaPrintlnValue(` 
		b=12;
		c=0;
		println(c=b);
		println(c+=b);
		println(c+=b);
		println(c-=b);
		println(c*=b);
		println(c/=b);
		println(c&=b);
		println(c|=b);
		println(c^=b);
		println(c>>=b);
		println(c>>>=b);
		println(c<<=b);
		println(c%=b);`, []string{"12", "24", "36", "24", "288", "24", "8", "12",
			"0", "0", "0", "0", "0"}, t)
	})

	t.Run("test instanceof expression ", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		String name = "James";
		println(name instanceof String);
		println(name instanceof int);
		if (name instanceof String s) {
           println(s);
        }
	`, []string{`true`, `false`, `"James"`}, t)
	})

	t.Run("test SliceCallExpression", func(t *testing.T) {
		CheckJavaPrintlnValue(` 
		int[] numbers = new int[10];
       numbers[0] = 1;
       numbers[1] = 2;
		println(numbers[0]);
		println(numbers[1]);
	`, []string{"1", "2"}, t)
	})

	t.Run("test FunctionCallExpression", func(t *testing.T) {
		CheckJavaCode(` a();
		a(b);
		a(b,c);
		a(1);
		a(1,"dog",true);
		a(b());
		`, t)
	})
	t.Run("test simple switch expression", func(t *testing.T) {
		CheckJavaPrintlnValue(` 
		int a = 1;
		int b = switch(a){
		case 1 -> 2;
		case 2 -> 10;
};
		println(b);
		`, []string{"2"}, t)
	})
	t.Run("test switch expression with muti-cases", func(t *testing.T) {
		CheckJavaPrintlnValue(` 
		int a = 1;
		int b = switch(a){
		case 5 -> 2;
		case 2 -> 10;
		case 1,2 -> 11;
};
		println(b);
		`, []string{"11"}, t)
	})
	t.Run("test switch expression with yield", func(t *testing.T) {
		CheckJavaPrintlnValue(` 
		int a = 2;
		int  b = switch(a){
		case 1 -> 11;
		case 2 -> {
			c = 33 ;
			println(c);
			yield 22;
}};
		println(b);
		`, []string{"33", "22"}, t)
	})
	t.Run("test switch expression ", func(t *testing.T) {
		CheckJavaPrintlnValue(` 
		int a = 2;
		int c =2;
		int  b = switch(a){
		case 1 -> {
			b=11;
			println(b);
			yield 111;
}
		case c,3 -> {
			b = 22 ;
			println(b);
			yield 222;
}};
		println(b);
		`, []string{"11", "22", "222"}, t)
	})

	t.Run("test switch expression with default  1", func(t *testing.T) {
		CheckJavaPrintlnValue(` 
		int a = 2;
		int c =2;
		int  b = switch(a){
		case 1 -> {
			b=11;
			println(b);
			yield 111;
}
		case 3 ->33  ;
		default -> 22 ;
};
		println(b);
		`, []string{"11", "22"}, t)
	})
	t.Run("test switch expression with default  2", func(t *testing.T) {
		CheckJavaPrintlnValue(` 
		int a = 2;
		int c =2;
		int  b = switch(a){
		case 1 -> {
			b=11;
			println(b);
			yield 111;
}
		case 3 ->33  ;
		default -> {
		b = 222 ;
		println(222);
		yield 22;
}
};
		println(b);
		`, []string{"11", "222", "22"}, t)
	})

	t.Run("test switch expression with default  3", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		var n = 1;
		 m = switch (n) {
            case 2:
                yield 2;
            default:
                yield 3;
        };
		`, []string{}, t)
	})

	t.Run("test assign expression", func(t *testing.T) {
		CheckJavaPrintlnValue(`int a =1 ;
		a=2;
		println(a);
		int b=4;
		a=b;
		println(a);
		`, []string{"2",
			"4"}, t)
	})

}

func TestJava_Literal(t *testing.T) {
	t.Run("test simple num", func(t *testing.T) {
		CheckJavaPrintlnValue(`int a = 1;	
				int b = 3;
				c= a+b;
				println(c);`,
			[]string{"4"}, t)
	})

	t.Run("test long int", func(t *testing.T) {
		CheckJavaPrintlnValue(`	
				FILE_MAX_SIZE = 100L*1024*1024;
				println(FILE_MAX_SIZE);`,
			[]string{"67108864"}, t)
	})

	t.Run("big number ", func(t *testing.T) {
		CheckJavaPrintlnValue(`
	long uid = -94044809860988047L;
	println(uid);
		`, []string{`neg("94044809860988047l")`}, t)
	})
}

func TestJava_TryWithSource(t *testing.T) {
	code := `package org.examle.A;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
class A {
	public static void main(String[] args) {
	try (InputStream in = new FileInputStream(src);
         OutputStream out = new FileOutputStream(dst)) {
        byte[] buf = new byte[BUFFER_SIZE];
        int n;
        while ((n = in.read(buf)) >= 0)
            out.write(buf, 0, n);
    	}
	}}
`
	t.Run("test in and out", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, code, `in<fullTypeName> as $in;out<fullTypeName> as $out;`,
			map[string][]string{
				"in":  {"\"java.io.FileInputStream\"", "\"java.io.InputStream\""},
				"out": {"\"java.io.FileOutputStream\"", "\"java.io.OutputStream\""},
			},
			ssaapi.WithLanguage(ssaapi.JAVA),
		)
	})

	t.Run("test close ", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, code, `
		.close as $closeInst 
		$closeInst<fullTypeName> as $close;
		`,
			map[string][]string{
				"close": []string{
					"\"java.io.FileInputStream\"",
					"\"java.io.FileOutputStream\"",
					"\"java.io.InputStream\"",
					"\"java.io.OutputStream\"",
				},
			},
			ssaapi.WithLanguage(ssaapi.JAVA),
		)
	})
}

func TestJava_Lambda(t *testing.T) {
	t.Run("test lambda expression SingleLambdaParameter", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		Calculator doubleNumber = number -> println(number * 2);
		println(doubleNumber);
	`, []string{"mul(Parameter-number, 2)", "Function-doubleNumber"}, t)
	})

	t.Run("test lambda expression FormalLambdaParameters", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		Calculator doubleNumber =(int number) ->{ println(number * 2);};
		println(doubleNumber);
	`, []string{"mul(Parameter-number, 2)", "Function-doubleNumber"}, t)
	})

	t.Run("test lambda expression MultiLambdaParameters", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		Calculator Mux =(number,times) ->{ println(number * times);};
		println(Mux);
	`, []string{"mul(Parameter-number, Parameter-times)", "Function-Mux"}, t)
	})

	t.Run("test lambda expression lambdaLVTIParameter", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		Calculator Mux =(var number,var times) ->{ println(number * times);};
		println(Mux);
`, []string{"mul(Parameter-number, Parameter-times)", "Function-Mux"}, t)
	})
}

func TestExpression_Extend(t *testing.T) {
	t.Run("test unquote string literal", func(t *testing.T) {
		code := `package org.example;

public class Main {
    public boolean sqlInjectLog(String username, String password) {
        String sql = "select * from user where username=\'" + username ;
        System.out.println("正在被尝试注入的 SQL 语句:" + sql);
        try {
            pstm = conn.prepareStatement(sql);
            rs = pstm.executeQuery();
            if (rs.next()) {
                //登陆成功
                return true;
            } else {
                return false;
            }
        } catch (SQLException e) {
            e.printStackTrace();
        }
        return false;
    }
}`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			prog.Show()
			vals, err := prog.SyntaxFlowWithError(`sql #-> as $a`)
			require.NoError(t, err)
			ret := vals.GetValues("a")
			ret.Show()
			require.Contains(t, ret.String(), "select * from user where username=")
			return nil
		}, ssaapi.WithLanguage(consts.JAVA))
	})
}

func TestVisitMethodCallWithObjectRange(t *testing.T) {
	code := `
package com.example;

class Parent {
    public void parentMethod() {}
}

public class Test extends Parent {
    public void testMethod() {
		method();

        obj.method();

        obj.this();
        
        obj.super();
    }
}
`

	t.Run("test field range", func(t *testing.T) {
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			values, err := prog.SyntaxFlowWithError(`method as $method`)
			require.NoError(t, err)
			method := values.GetValues("method")
			require.NotEmpty(t, method)
			require.Greater(t, len(method), 1, "method should have only one value")
			require.NotNil(t, method[0].GetRange(), "method should have range")
			methodRange := method[0].GetRange().GetText()
			require.Equal(t, methodRange, "method",
				"method should have method name")

			values, err = prog.SyntaxFlowWithError(`obj.method as $objMethod`)
			require.NoError(t, err)
			objMethods := values.GetValues("objMethod")
			require.NotEmpty(t, objMethods)
			objMethodRange := objMethods[0].GetRange().GetText()
			require.Equal(t, objMethodRange, "method",
				"obj.method should have method name")

			values, err = prog.SyntaxFlowWithError(`obj.this as $objThis`)
			require.NoError(t, err)
			objThis := values.GetValues("objThis")
			require.NotEmpty(t, objThis)
			objThisRange := objThis[0].GetRange().GetText()
			require.Equal(t, objThisRange, "this",
				"obj.this should have method name")

			values, err = prog.SyntaxFlowWithError(`obj.super as $objSuper`)
			require.NoError(t, err)
			objSuper := values.GetValues("objSuper")
			require.NotEmpty(t, objSuper)
			objSuperRange := objSuper[0].GetRange().GetText()
			require.Equal(t, objSuperRange, "super",
				"obj.super should have method name")
			return nil
		}, ssaapi.WithLanguage(ssaapi.JAVA))
	})

	t.Run("test call range", func(t *testing.T) {
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			values, err := prog.SyntaxFlowWithError(`method() as $methodCall`)
			require.NoError(t, err)
			methodCalls := values.GetValues("methodCall")
			require.NotEmpty(t, methodCalls)
			methodCallRange := methodCalls[0].GetRange().GetText()
			require.Equal(t, methodCallRange, "method()",
				"this.method should have method name")

			values, err = prog.SyntaxFlowWithError(`obj.method() as $objMethodCall`)
			require.NoError(t, err)
			methodCalls = values.GetValues("objMethodCall")
			require.NotEmpty(t, methodCalls)
			methodCallRange = methodCalls[0].GetRange().GetText()
			require.Equal(t, methodCallRange, "method()",
				"obj.method should have method name")

			values, err = prog.SyntaxFlowWithError(`obj.this() as $objThisCall`)
			require.NoError(t, err)
			methodCalls = values.GetValues("objThisCall")
			require.NotEmpty(t, methodCalls)
			methodCallRange = methodCalls[0].GetRange().GetText()
			require.Equal(t, methodCallRange, "this()",
				"obj.this should have method name")

			values, err = prog.SyntaxFlowWithError(`obj.super() as $objSuperCall`)
			require.NoError(t, err)
			methodCalls = values.GetValues("objSuperCall")
			require.NotEmpty(t, methodCalls)
			methodCallRange = methodCalls[0].GetRange().GetText()
			require.Equal(t, methodCallRange, "super()",
				"obj.super should have method name")
			return nil
		}, ssaapi.WithLanguage(ssaapi.JAVA))
	})
}
