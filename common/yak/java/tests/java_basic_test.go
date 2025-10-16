package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestJava_LocalType_Declaration(t *testing.T) {
	t.Run("test simple variable assign", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a;
		println(a);
		int a = 1;
		println(a);
		Boolen b= true;
		println(b);
		float c=3.14;
		println(c);
		string s ="aaa";
		println(s);`, []string{"Undefined-a", "1", "true", "3.14", "\"aaa\""}, t)
	})
	t.Run("test array declaration", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a[] = {};
		println(a);
		int a[] = {1,2,3};
		println(a);
	    string s[] = {"world","hello"};
		println(s[1]);
		println(a[2]);
		int c=a[1]+a[0];
		println(c);
		int[] numbers = {1,2,3};
		println(numbers);
		`, []string{"make([]number)", "make([]number)", "\"hello\"", "3", "3", "make([]number)"}, t)
	})
	t.Run("test two dim array declaration", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a[][] = {{1,2,3},{4,5,6}};
		println(a);
		println(a[1][2]);

		String a[][]={{"hello","world"},{"world","hello"}};
		println(a[1][1]);
		`,
			[]string{"make([][]number)",
				"6",
				"\"hello\""}, t)
	})
	t.Run("test array declaration", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		Object a[][][] = {{{1,2,3}}};
		println(a[0][0][0]);
		Object b[][] = {{1,2},3};
		println(b[0][1]);
		`,
			[]string{"1",
				"2",
			}, t)
	})

	// 测试 new 关键字的各种数组初始化形式
	t.Run("test array initialization with new keyword", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		// 指定长度，默认值为 0
		int[] arr1 = new int[5];
		println(arr1);
		println(arr1[0]);
		println(arr1[4]);
		
		// 不指定长度，直接用 {} 初始化
		int[] arr2 = new int[] {1, 2, 3, 4, 5};
		println(arr2[0]);
		println(arr2[4]);
		println(arr2[2]);
		
		// 简写形式（编译器自动推断）
		int[] arr3 = {10, 20, 30};
		println(arr3[0]);
		println(arr3[1]);
		println(arr3[2]);
		`, []string{
			"make([]number)",
			"0",  // arr1[0] 默认值
			"0",  // arr1[4] 默认值
			"1",  // arr2[0]
			"5",  // arr2[4]
			"3",  // arr2[2]
			"10", // arr3[0]
			"20", // arr3[1]
			"30", // arr3[2]
		}, t)
	})

	// 测试二维数组的各种初始化形式
	t.Run("test 2D array initialization with new keyword", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		// 指定两个维度的长度
		int[][] matrix1 = new int[3][4];
		println(matrix1[0][0]);
		println(matrix1[2][3]);
		
		// 只指定第一个维度，第二维待后续分配
		int[][] matrix2 = new int[3][];
		matrix2[0] = new int[2];
		matrix2[1] = new int[4];
		matrix2[2] = new int[3];
		println(matrix2[0][0]);
		println(matrix2[1][0]);
		
		// 使用数组初始化器直接赋值
		int[][] matrix3 = new int[][] {
			{1, 2, 3},
			{4, 5, 6}
		};
		println(matrix3[0][0]);
		println(matrix3[0][2]);
		println(matrix3[1][1]);
		println(matrix3[1][2]);
		
		// 简写初始化
		int[][] matrix4 = {
			{10, 20},
			{30, 40}
		};
		println(matrix4[0][0]);
		println(matrix4[0][1]);
		println(matrix4[1][0]);
		println(matrix4[1][1]);
		`, []string{
			"0",  // matrix1[0][0] 默认值
			"0",  // matrix1[2][3] 默认值
			"0",  // matrix2[0][0] 默认值
			"0",  // matrix2[1][0] 默认值
			"1",  // matrix3[0][0]
			"3",  // matrix3[0][2]
			"5",  // matrix3[1][1]
			"6",  // matrix3[1][2]
			"10", // matrix4[0][0]
			"20", // matrix4[0][1]
			"30", // matrix4[1][0]
			"40", // matrix4[1][1]
		}, t)
	})

	// 测试三维数组初始化
	t.Run("test 3D array initialization", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		// 三维完整声明
		int[][][] cube1 = new int[2][3][4];
		println(cube1[0][0][0]);
		println(cube1[1][2][3]);
		
		// 部分维度留空
		int[][][] cube2 = new int[2][][];
		cube2[0] = new int[2][3];
		cube2[1] = new int[3][4];
		println(cube2[0][0][0]);
		println(cube2[1][0][0]);
		
		// 使用初始化器
		int[][][] cube3 = new int[][][] {
			{{1, 2}, {3, 4}},
			{{5, 6}, {7, 8}}
		};
		println(cube3[0][0][0]);
		println(cube3[0][0][1]);
		println(cube3[0][1][0]);
		println(cube3[1][1][1]);
		`, []string{
			"0", // cube1[0][0][0] 默认值
			"0", // cube1[1][2][3] 默认值
			"0", // cube2[0][0][0] 默认值
			"0", // cube2[1][0][0] 默认值
			"1", // cube3[0][0][0]
			"2", // cube3[0][0][1]
			"3", // cube3[0][1][0]
			"8", // cube3[1][1][1]
		}, t)
	})

	// 测试引用类型数组（String）
	t.Run("test String array initialization", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		// String 数组 - 指定长度
		String[] names1 = new String[3];
		println(names1[0]);
		
		// String 数组 - 使用初始化器
		String[] names2 = new String[] {"Alice", "Bob", "Charlie"};
		println(names2[0]);
		println(names2[1]);
		println(names2[2]);
		
		// String 数组 - 简写形式
		String[] names3 = {"Tom", "Jerry"};
		println(names3[0]);
		println(names3[1]);
		
		// String 二维数组
		String[][] grid = new String[][] {
			{"A1", "A2"},
			{"B1", "B2"}
		};
		println(grid[0][0]);
		println(grid[0][1]);
		println(grid[1][0]);
		println(grid[1][1]);
		`, []string{
			"nil", // String 默认值为 null，这里可能显示为 Undefined
			"\"Alice\"",
			"\"Bob\"",
			"\"Charlie\"",
			"\"Tom\"",
			"\"Jerry\"",
			"\"A1\"",
			"\"A2\"",
			"\"B1\"",
			"\"B2\"",
		}, t)
	})

	// 测试动态赋值
	t.Run("test array dynamic assignment", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int[] arr = new int[5];
		arr[0] = 10;
		arr[1] = 20;
		arr[2] = 30;
		arr[3] = 40;
		arr[4] = 50;
		println(arr[0]);
		println(arr[2]);
		println(arr[4]);
		
		// 二维数组动态赋值
		int[][] matrix = new int[2][3];
		matrix[0][0] = 1;
		matrix[0][1] = 2;
		matrix[0][2] = 3;
		matrix[1][0] = 4;
		matrix[1][1] = 5;
		matrix[1][2] = 6;
		println(matrix[0][0]);
		println(matrix[0][2]);
		println(matrix[1][1]);
		println(matrix[1][2]);
		`, []string{
			"10",
			"30",
			"50",
			"1",
			"3",
			"5",
			"6",
		}, t)
	})

	// 测试混合初始化方式
	t.Run("test mixed array initialization", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		// 一维数组各种形式混合
		int[] a1 = new int[3];
		int[] a2 = new int[] {1, 2, 3};
		int[] a3 = {4, 5, 6};
		
		println(a1[0]);
		println(a2[1]);
		println(a3[2]);
		
		// 二维数组混合
		int[][] b1 = new int[2][2];
		int[][] b2 = new int[][] {{1, 2}, {3, 4}};
		int[][] b3 = {{5, 6}, {7, 8}};
		
		println(b1[0][0]);
		println(b2[0][1]);
		println(b3[1][0]);
		
		// 不同类型混合
		String[] s1 = new String[] {"hello", "world"};
		String[] s2 = {"foo", "bar"};
		
		println(s1[0]);
		println(s2[1]);
		`, []string{
			"0",
			"2",
			"6",
			"0",
			"2",
			"7",
			"\"hello\"",
			"\"bar\"",
		}, t)
	})

	t.Run("test return", func(t *testing.T) {
		CheckAllJavaCode(`
public class HelloWorld {
    public static void main(String[] args) {
        int result = a + b;
        return result;
        int a=2;
    }
}

`, t)
	})
	t.Run("test switch break", func(t *testing.T) {
		CheckJavaCode(`
		result= switch(e){
		default : break;
};
`, t)
	})
}

func TestJavaSyntaxBlock(t *testing.T) {
	t.Run("test simple block", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
class A{
	public static void main(String[] args){
		{
			int a=2;
			println(a); // 2 
		}
		println(a); //
	}
}
	`, []string{"2", "Undefined-a"}, t)
	})

	t.Run("test synchronized block", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`
	class A{
		public static void main(String[] args){
			synchronized(this){
				println("hello");
			}
		}
	}`, []string{`"hello"`}, t)
	})
}

func TestJavaTernaryExpression(t *testing.T) {
	t.Run("test basic ternary", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a = 5;
		String result = (a > 3) ? "greater" : "smaller";
		println(result);
		`, []string{`"greater"`}, t)
	})

	t.Run("test nested ternary", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a = 5;
		String result = (a > 10) ? "large" : (a > 3) ? "medium" : "small";
		println(result);
		`, []string{`"medium"`}, t)
	})

	t.Run("test ternary with expressions", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a = 5;
		int b = 10;
		int result = (c) ? (a + b) : (b - a);
		println(result);
		`, []string{`phi(result)[15,5]`}, t)
	})

	t.Run("test ternary with boolean result", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a = 5;
		int b = 3;
		boolean result = (c) ? true : false;
		println(result);
		boolean result2 = (c) ? true : false;
		println(result2);
		`, []string{`phi(result)[true,false]`, `phi(result2)[true,false]`}, t)
	})

	t.Run("test ternary with method calls", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a = 5;
		String str = "test";
		String result = (c) ? str.toUpperCase() : str.toLowerCase();
		println(result);
		`, []string{`phi(result)[Undefined-str.toUpperCase("test"),Undefined-str.toLowerCase("test")]`}, t)
	})

	// Test conditional branches in ternary expressions
	t.Run("test ternary condition evaluation", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a = 5;
		String result = (c) ? "condition true" : "condition false";
		println(result);
		`, []string{`phi(result)["condition true","condition false"]`}, t)
	})
}
