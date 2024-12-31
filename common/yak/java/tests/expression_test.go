package tests

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
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
		`, []string{"0"}, t)
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
	try (InputStream   in = new FileInputStream(src);
         OutputStream out = new FileOutputStream(dst)) {
        byte[] buf = new byte[BUFFER_SIZE];
        int n;
        while ((n = in.read(buf)) >= 0)
            out.write(buf, 0, n);
    	}
	}}
`
	ssatest.CheckSyntaxFlow(t, code, `in<fullTypeName> as $in;out<fullTypeName> as $out;`,
		map[string][]string{
			"in":  []string{"\"java.io.FileInputStream\"", "\"java.io.InputStream\""},
			"out": []string{"\"java.io.FileOutputStream\"", "\"java.io.OutputStream\""},
		}, ssaapi.WithLanguage(ssaapi.JAVA))
	ssatest.CheckSyntaxFlow(t, code, `.close<fullTypeName> as $close;`,
		map[string][]string{
			"close": []string{"\"java.io.FileInputStream\"", "\"java.io.FileOutputStream\"", "\"java.io.InputStream\"", "\"java.io.OutputStream\""},
		}, ssaapi.WithLanguage(ssaapi.JAVA))
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
