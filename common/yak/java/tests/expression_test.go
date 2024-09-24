package tests

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestJava_Simple_Expression(t *testing.T) {
	t.Run("test PostfixExpression", func(t *testing.T) {
		CheckJavaCode(`
		a++;
		a--;`, t)
	})
	t.Run("test PrefixUnaryExpression", func(t *testing.T) {
		CheckJavaCode(`
        +a;
		-a;
		~a;
		!a;`, t)
	})
	t.Run("test PrefixBinaryExpression", func(t *testing.T) {
		CheckJavaCode(`
		++a;
		--a;`, t)
	})
	t.Run("test MultiplicativeExpression", func(t *testing.T) {
		CheckJavaCode(` 
         a * b;
         b / a;
         a % b;`, t)
	})
	t.Run("test AdditiveExpression", func(t *testing.T) {
		CheckJavaCode(` 
		a + b;
		b - a;`, t)
	})
	t.Run("test ShiftExpression", func(t *testing.T) {
		CheckJavaCode(`
         a << b;
         a >>>b  ; //无符号位移
         a >> b  ; //有符号位移`, t)
	})
	t.Run("test RelationalExpression", func(t *testing.T) {
		CheckJavaCode(`
		 a < b;
		 b > a;
		 a <= b;
		 b >= a;`, t)
	})
	t.Run("test EqualityExpression", func(t *testing.T) {
		CheckJavaCode(`
		 a == b;
		 b != a;`, t)
	})
	t.Run("test AndExpression", func(t *testing.T) {
		CheckJavaCode(`
		 a & b;`, t)
	})
	t.Run("test XorExpression", func(t *testing.T) {
		CheckJavaCode(` 
		 a ^ b;`, t)
	})
	t.Run("test OrExpression", func(t *testing.T) {
		CheckJavaCode(` 
		 a | b;`, t)
	})
	t.Run("test LogicalAndExpression", func(t *testing.T) {
		CheckJavaCode(` 
		a && b;`, t)
	})
	t.Run("test LogicalOrExpression", func(t *testing.T) {
		CheckJavaCode(` 	
		a||b;`, t)
	})
	t.Run("test TernaryExpression", func(t *testing.T) {
		CheckJavaCode(` int a = 1;
		int b = 0;
		int ret;
		ret = a > b ? a : b;`, t)
	})
	t.Run("test AssignmentExpression", func(t *testing.T) {
		CheckJavaCode(` 
		a=b;
		c+=b;
		a+=b;
		a-=b;
		a*=b;
		a/=b;
		a&=b;
		a|=b;
		a^=b;
		a>>=b;
		a>>>=b;
		a<<=b;
		a%=b;`, t)
	})
	t.Run("test SliceCallExpression", func(t *testing.T) {
		CheckJavaCode(` a[1];
	a[b];
	`, t)
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

func TestJavaFullType(t *testing.T) {
	code := `package main;
import java.io.FileInputStream;
import java.io.InputStream;
class A{
	public static void main(){
			InputStream in = new FileInputStream(src);
			while(in.read>0){}
	}
}
`
	ssatest.CheckSyntaxFlow(t, code, `in<fullTypeName> as $input`, map[string][]string{"input": {}}, ssaapi.WithLanguage(ssaapi.JAVA))
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
        while ((n =in.read( buf)) >= 0)
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
