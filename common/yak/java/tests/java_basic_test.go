package tests

import (
	"testing"
)

func TestJava_Expression(t *testing.T) {
	t.Run("test plusplus", func(t *testing.T) {
		CheckJavaCode(createJavaProgram(`a++;`), t)
	})
	t.Run("test subsub", func(t *testing.T) {
		CheckJavaCode(createJavaProgram(`a--;`), t)
	})
	t.Run("test the assignment of no-member variables", func(t *testing.T) {
		CheckJavaCode(createJavaProgram(` var a=1;
int b=1;
String c="a";
bool d=true;`), t)
	})
	t.Run("test the assignment of member variables", func(t *testing.T) {
		CheckJavaCode(`public class Person {
    // 成员变量
    private String name;
    private int age;
    private boolean isStudent;}`, t)
	})
	t.Run("test slice", func(t *testing.T) {
		CheckJavaCode(createJavaProgram(`List<Integer> numbers = new ArrayList<>();
        numbers.add(1);
        numbers.add(2);`), t)
	})
	t.Run("test MemberCallExpression", func(t *testing.T) {
		CheckJavaCode(`class Student<T> extends School {
    public String name="init";
    public T age;
    public boolean isStudent;

    public Student(String name, T age, boolean isStudent) {
        this.age = age;
        this.name = name;
        this.isStudent = isStudent;

    }

    public void setAge(T age) {
        this.age = age;
    }
    public <T> void eat(T food) { // 泛型方法
        System.out.println("Student is eating " + food);
    }

    public void  sayHello(){
        System.out.printf("Hello");
    }
    public static void main(String[] args) {
        Student  student = new Student("xiaoming",16,true);
        student.setAge("16");
        student.sayHello();

        System.out.println(super.name);
        student.<Integer>eat(1);
    }
}
`, t)
	})
	t.Run("test PrefixExpression", func(t *testing.T) {
		CheckJavaCode(createJavaProgram(`boolean a = true ;
        a=!a;
        int b = 1;
        b= +b;
        b= -b;
        b= ++b;
        b=--b;
        b=~b;`), t)
	})
	t.Run("test MultiplicativeExpression", func(t *testing.T) {
		CheckJavaCode(createJavaProgram(` int a = 2;
        int b = 3;
        int ret;
        ret = a * b;
        ret = b / a;
        ret = a % b;`), t)
	})
	t.Run("test AdditiveExpression", func(t *testing.T) {
		CheckJavaCode(createJavaProgram(` 
		a + b;
		b - a;`), t)
	})
	t.Run("test ShiftExpression", func(t *testing.T) {
		CheckJavaCode(createJavaProgram(`
        int a = -111;
        int b = 3;
        int ret;
        ret =  a << b;// -888
        ret =  a >>>b  ; //536870898
        ret =  a >> b  ;  //-14`), t)
	})
	t.Run("test RelationalExpression", func(t *testing.T) {
		CheckJavaCode(createJavaProgram(` int a = 2;
		int b = 3;
		boolean ret;
		ret = a < b;
		ret = b > a;
		ret = a <= b;
		ret = b >= a;`), t)
	})
	t.Run("test InstanceofExpression", func(t *testing.T) {
		CheckJavaCode(createJavaProgram(` Object a = new Object();
		boolean ret;
		ret = a instanceof Object;`), t)
	})
	t.Run("test EqualityExpression", func(t *testing.T) {
		CheckJavaCode(createJavaProgram(` int a = 2;
		int b = 3;
		boolean ret;
		ret = a == b;
		ret = b != a;`), t)
	})
	t.Run("test AndExpression", func(t *testing.T) {
		CheckJavaCode(createJavaProgram(` int a = 1;
		int b = 0;
		int ret;
		ret = a & b;`), t)
	})
	t.Run("test XorExpression", func(t *testing.T) {
		CheckJavaCode(createJavaProgram(` int a = 1;
		int b = 2;
		int ret;
		ret = a ^ b;`), t)
	})
	t.Run("test OrExpression", func(t *testing.T) {
		CheckJavaCode(createJavaProgram(` int a = 1;
		int b = 0;
		int ret;
		ret = a | b;`), t)
	})
	t.Run("test LogicalAndExpression", func(t *testing.T) {
		CheckJavaCode(createJavaProgram(` boolean a = true;
		boolean b = false;
		boolean ret;
		ret = a && b;`), t)
	})
	t.Run("test LogicalOrExpression", func(t *testing.T) {
		CheckJavaCode(createJavaProgram(` boolean a = true;
		boolean b = false;
		boolean ret;
		ret = a || b;`), t)
	})
	t.Run("test TernaryExpression", func(t *testing.T) {
		CheckJavaCode(createJavaProgram(` int a = 1;
		int b = 0;
		int ret;
		ret = a > b ? a : b;`), t)
	})
	t.Run("test AssignmentExpression", func(t *testing.T) {
		CheckJavaCode(createJavaProgram(` int a = 1;
		int b = 0;
		b = a;`), t)
	})

}
