package tests

import "testing"

func TestJava_Expression(t *testing.T) {
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
}
