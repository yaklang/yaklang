package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCSharp_Expression_Arithmetic(t *testing.T) {
	t.Run("const fold", func(t *testing.T) {
		CheckCSharpPrintlnValue(`
int a = 1 + 2 * 3;
println(a);
int b = (10 - 4) / 2;
println(b);
int c = 7 % 3;
println(c);
`, []string{"7", "3", "1"}, t)
	})
	t.Run("binary with variable", func(t *testing.T) {
		CheckCSharpPrintlnValue(`
int a = foo();
int b = a + 1;
println(b);
int c = a << 2;
println(c);
int d = a & 0xff;
println(d);
`, []string{"add(Undefined-foo(), 1)", "shl(Undefined-foo(), 2)", "and(Undefined-foo(), 255)"}, t)
	})
	t.Run("compare and logic", func(t *testing.T) {
		CheckCSharpPrintlnValue(`
int a = foo();
bool x = a > 1;
println(x);
bool y = a == 2;
println(y);
bool z = a != 3;
println(z);
`, []string{"gt(Undefined-foo(), 1)", "eq(Undefined-foo(), 2)", "neq(Undefined-foo(), 3)"}, t)
	})
	t.Run("unary", func(t *testing.T) {
		CheckCSharpPrintlnValue(`
int a = foo();
int b = -a;
println(b);
bool c = !flag();
println(c);
int d = ~a;
println(d);
`, []string{"neg(Undefined-foo())", "not(Undefined-flag())", "bitwise-not(Undefined-foo())"}, t)
	})
}

func TestCSharp_Expression_Assignment(t *testing.T) {
	t.Run("compound assign", func(t *testing.T) {
		CheckCSharpPrintlnValue(`
int a = 1;
a += 2;
println(a);
a *= foo();
println(a);
`, []string{"3", "mul(3, Undefined-foo())"}, t)
	})
	t.Run("inc dec", func(t *testing.T) {
		CheckCSharpPrintlnValue(`
int a = 1;
a++;
println(a);
--a;
println(a);
int b = a++;
println(b);
println(a);
`, []string{"2", "1", "1", "2"}, t)
	})
	t.Run("chain assign", func(t *testing.T) {
		CheckCSharpPrintlnValue(`
int a, b;
a = b = 5;
println(a);
println(b);
`, []string{"5", "5"}, t)
	})
	t.Run("null coalesce assign", func(t *testing.T) {
		CheckCSharpPrintlnValue(`
string s = foo();
s ??= "default";
println(s);
`, []string{"phi(s)[Undefined-foo(),\"default\"]"}, t)
	})
}

func TestCSharp_Expression_AssignmentEvaluationOrder(t *testing.T) {
	t.Run("null coalescing assignment evaluates rhs only on the null path", func(t *testing.T) {
		prog := parseCSharpSemantics(t, `
public class AssignmentOrder {
    public static string Run(string value) {
        value ??= rhsSource();
        return value;
    }
}
`)

		orders := explicitReturnCallOrders(t, prog, "Run")
		require.Contains(t, orders, []string{"return"}, "the non-null path must not evaluate the RHS")
		require.Contains(t, orders, []string{"rhsSource", "return"}, "the null path must evaluate the RHS")
	})

	t.Run("compound property reads before rhs and writes after it", func(t *testing.T) {
		prog := parseCSharpSemantics(t, `
public class OrderedProperty {
    public int Value {
        get { return getterSource(); }
        set { setterSink(value); }
    }
}

public class AssignmentOrder {
    public static void Run(OrderedProperty box) {
        box.Value += rhsSource();
        doneSink();
    }
}
`)

		requireReachableCallOrder(t, prog, "Run", "box.get_Value", "rhsSource", "box.set_Value", "doneSink")
	})

	t.Run("null coalescing indexer evaluates lhs once and sets only on null path", func(t *testing.T) {
		prog := parseCSharpSemantics(t, `
public class OrderedIndexer {
    public string this[int index] {
        get { return indexerGetterSource(index); }
        set { indexerSetterSink(index, value); }
    }
}

public class AssignmentOrder {
    public static OrderedIndexer receiverSource() { return new OrderedIndexer(); }

    public static string Run() {
        return receiverSource()[indexSource()] ??= rhsSource();
    }
}
`)

		matches := func(actual, expected string) bool {
			return actual == expected || strings.HasSuffix(actual, "."+expected)
		}
		count := func(order []string, name string) int {
			total := 0
			for _, call := range order {
				if matches(call, name) {
					total++
				}
			}
			return total
		}
		position := func(order []string, name string) int {
			for index, call := range order {
				if matches(call, name) {
					return index
				}
			}
			return -1
		}

		orders := explicitReturnCallOrders(t, prog, "Run")
		require.NotEmpty(t, orders)
		sawNonNullPath := false
		sawNullPath := false
		for _, order := range orders {
			require.Equal(t, 1, count(order, "receiverSource"), "receiver must be evaluated once on path %v", order)
			require.Equal(t, 1, count(order, "indexSource"), "index must be evaluated once on path %v", order)
			require.Equal(t, 1, count(order, "get_Item"), "getter must be evaluated once on path %v", order)
			require.Less(t, position(order, "receiverSource"), position(order, "indexSource"), "receiver precedes index on path %v", order)
			require.Less(t, position(order, "indexSource"), position(order, "get_Item"), "index precedes getter on path %v", order)

			rhsCount := count(order, "rhsSource")
			setterCount := count(order, "set_Item")
			if rhsCount == 0 {
				sawNonNullPath = true
				require.Zero(t, setterCount, "non-null path must not invoke the setter: %v", order)
				continue
			}
			sawNullPath = true
			require.Equal(t, 1, rhsCount, "RHS must be evaluated once on the null path: %v", order)
			require.Equal(t, 1, setterCount, "setter must be evaluated once on the null path: %v", order)
			require.Less(t, position(order, "get_Item"), position(order, "rhsSource"), "getter precedes RHS on path %v", order)
			require.Less(t, position(order, "rhsSource"), position(order, "set_Item"), "RHS precedes setter on path %v", order)
		}
		require.True(t, sawNonNullPath, "expected a non-null path without RHS/setter: %v", orders)
		require.True(t, sawNullPath, "expected a null path with RHS/setter: %v", orders)

		// The expression result is a phi, not a second unconditional write to
		// receiver[index]. Only the null branch above may carry that relation.
		mergedReturns := 0
		prog.Program.EachFunction(func(function *ssa.Function) {
			if function.GetMethodName() != "Run" {
				return
			}
			for _, blockID := range function.Blocks {
				block, exists := function.GetBasicBlockByID(blockID)
				if !exists || block == nil {
					continue
				}
				for _, instructionID := range block.Insts {
					instruction, exists := function.GetInstructionById(instructionID)
					if !exists || instruction == nil {
						continue
					}
					ret, isReturn := ssa.ToReturn(instruction)
					if !isReturn {
						continue
					}
					for _, value := range ret.GetValues() {
						if _, isPhi := ssa.ToPhi(value); !isPhi {
							continue
						}
						mergedReturns++
						require.Empty(t, ssa.GetObjectKeyPairs(value),
							"merged ??= result must not be recorded as an unconditional member store")
					}
				}
			}
		})
		require.Equal(t, 1, mergedReturns, "expected the returned ??= value to be phi-merged")
	})
}

func TestCSharp_Expression_Conditional(t *testing.T) {
	t.Run("ternary", func(t *testing.T) {
		CheckCSharpPrintlnValue(`
int a = foo();
int b = a > 0 ? 1 : 2;
println(b);
`, []string{"phi(b)[1,2]"}, t)
	})
	t.Run("null coalesce", func(t *testing.T) {
		CheckCSharpPrintlnValue(`
string s = foo();
string r = s ?? "x";
println(r);
`, []string{"phi(r)[Undefined-foo(),\"x\"]"}, t)
	})
	t.Run("logic and or", func(t *testing.T) {
		CheckCSharpPrintlnValue(`
bool a = f1();
bool b = f2();
bool c = a && b;
println(c);
bool d = a || b;
println(d);
`, []string{"phi(c)[Undefined-f2(),false]", "phi(d)[true,Undefined-f2()]"}, t)
	})
}

func TestCSharp_Expression_Literal(t *testing.T) {
	CheckCSharpPrintlnValue(`
int a = 0x10;
println(a);
long b = 100L;
println(b);
double c = 1.5;
println(c);
string s = "hello";
println(s);
string v = @"raw\n";
println(v);
char ch = 'x';
println(ch);
bool t1 = true;
println(t1);
object n = null;
println(n);
int bin = 0b1010;
println(bin);
int under = 1_000;
println(under);
`, []string{"16", "100", "1.5", "\"hello\"", "\"raw\\\\n\"", "\"x\"", "true", "nil", "10", "1000"}, t)
}

func TestCSharp_Expression_InterpolatedString(t *testing.T) {
	CheckCSharpPrintlnValue(`
string name = foo();
string s = $"Hello {name}!";
println(s);
`, []string{"add(add(\"Hello \", Undefined-foo()), \"!\")"}, t)
}

func TestCSharp_Expression_MemberAndCall(t *testing.T) {
	t.Run("static call and member chain", func(t *testing.T) {
		code := CreateCSharpProgram(`
var x = Console.ReadLine();
Console.WriteLine(x);
var y = System.IO.File.ReadAllText("a.txt");
`)
		ssatest.CheckSyntaxFlow(t, code, `
Console.ReadLine() as $read;
Console.WriteLine(* as $arg);
File.ReadAllText(* as $path);
`, map[string][]string{
			"read": {"Undefined-Console.ReadLine(Undefined-Console)"},
			"arg":  {"Undefined-Console.ReadLine(Undefined-Console)"},
			"path": {"\"a.txt\""},
		}, ssaapi.WithLanguage(ssaconfig.CSHARP))
	})
	t.Run("instance method call", func(t *testing.T) {
		code := `
public class Svc {
    public string Run(string s) { return s + "!"; }
}
public class Main {
    public static void Main(string[] args) {
        var svc = new Svc();
        var r = svc.Run("a");
        println(r);
    }
}`
		ssatest.CheckSyntaxFlow(t, code, `println(* as $r); $r #-> as $src`, map[string][]string{
			"src": {"\"a\"", "\"!\""},
		}, ssaapi.WithLanguage(ssaconfig.CSHARP))
	})
	t.Run("null conditional", func(t *testing.T) {
		code := CreateCSharpProgram(`
var s = foo();
var l = s?.Length;
println(l);
var r = s?.Trim();
println(r);
`)
		ssatest.CheckSyntaxFlow(t, code, `println(* as $v)`, map[string][]string{
			"v": {"phi(l)[Undefined-s.Length(valid),nil]", "phi(r)[Undefined-s.Trim(valid)(Undefined-foo()),nil]"},
		}, ssaapi.WithLanguage(ssaconfig.CSHARP))
	})
	t.Run("named and default arguments", func(t *testing.T) {
		code := CreateCSharpProgram(`
Foo.Bar(1, name: "x", flag: true);
`)
		ssatest.CheckSyntaxFlow(t, code, `Foo.Bar(* as $args)`, map[string][]string{
			"args": {"1", "\"x\"", "true"},
		}, ssaapi.WithLanguage(ssaconfig.CSHARP))
	})
}

func TestCSharp_Expression_MemberCalleeEvaluatesPropertyReceiverOnce(t *testing.T) {
	prog := parseCSharpSemantics(t, `
public class Executor {
    public void Execute(object value) { executeSink(value); }
}
public class ReceiverHolder {
    public Executor Current { get { getterBodySource(); return new Executor(); } }
    public object Run() {
        Current.Execute(argumentSource());
        return done();
    }
}
public class Program { public static void Main() { new ReceiverHolder().Run(); } }
`)

	orders := explicitReturnCallOrders(t, prog, "Run")
	require.NotEmpty(t, orders)
	for _, order := range orders {
		countSuffix := func(suffix string) int {
			count := 0
			for _, name := range order {
				if name == suffix || strings.HasSuffix(name, "."+suffix) {
					count++
				}
			}
			return count
		}
		positionSuffix := func(suffix string) int {
			for index, name := range order {
				if name == suffix || strings.HasSuffix(name, "."+suffix) {
					return index
				}
			}
			return -1
		}
		require.Equal(t, 1, countSuffix("get_Current"), "property receiver must be evaluated once: %v", order)
		getter := positionSuffix("get_Current")
		argument := positionSuffix("argumentSource")
		execute := positionSuffix("Execute")
		require.NotEqual(t, -1, getter, "missing getter call: %v", order)
		require.NotEqual(t, -1, argument, "missing argument call: %v", order)
		require.NotEqual(t, -1, execute, "missing Execute call: %v", order)
		require.Less(t, getter, argument, "receiver must precede argument evaluation: %v", order)
		require.Less(t, argument, execute, "arguments must precede invocation: %v", order)
	}
}

func TestCSharp_Expression_ObjectCreation(t *testing.T) {
	t.Run("new with initializer", func(t *testing.T) {
		code := CreateCSharpProgram(`
var p = new Point { X = 1, Y = 2 };
println(p.X);
println(p.Y);
`)
		ssatest.CheckSyntaxFlow(t, code, `println(* as $v)`, map[string][]string{
			"v": {"1", "2"},
		}, ssaapi.WithLanguage(ssaconfig.CSHARP))
	})
	t.Run("collection initializer", func(t *testing.T) {
		code := CreateCSharpProgram(`
var list = new List<int> { 1, 2, 3 };
println(list[0]);
var dict = new Dictionary<string, int> { { "a", 1 }, { "b", 2 } };
println(dict["a"]);
println(dict["b"]);
var indexed = new Dictionary<string, int> { ["c"] = 3 };
println(indexed["c"]);
`)
		ssatest.CheckSyntaxFlow(t, code, `println(* as $v)`, map[string][]string{
			"v": {"1", "1", "2", "3"},
		}, ssaapi.WithLanguage(ssaconfig.CSHARP))
	})
	t.Run("array creation", func(t *testing.T) {
		CheckCSharpPrintlnValue(`
int[] arr = new int[] { 1, 2, 3 };
println(arr[1]);
int[] arr2 = { 4, 5 };
println(arr2[0]);
var arr3 = new[] { "a", "b" };
println(arr3[1]);
int[] arr4 = new int[10];
println(arr4);
`, []string{"2", "4", "\"b\"", "make([]number)"}, t)
	})
	t.Run("anonymous object", func(t *testing.T) {
		CheckCSharpPrintlnValue(`
var o = new { Name = "n", Age = 3 };
println(o.Name);
println(o.Age);
`, []string{"\"n\"", "3"}, t)
	})
	t.Run("constructor arguments", func(t *testing.T) {
		code := CreateCSharpProgram(`
Point p = new Point(1, 2);
println(p);
`)
		ssatest.CheckSyntaxFlow(t, code, `println(* as $v)`, map[string][]string{
			"v": {"Undefined-Point(Undefined-Point,1,2)"},
		}, ssaapi.WithLanguage(ssaconfig.CSHARP))
	})
}

func TestCSharp_Expression_Tuple(t *testing.T) {
	t.Run("tuple literal and deconstruct", func(t *testing.T) {
		CheckCSharpPrintlnValue(`
var t = (1, "a");
println(t.Item1);
println(t.Item2);
var (x, y) = (3, 4);
println(x);
println(y);
(int p, string q) = t;
println(p);
println(q);
`, []string{"1", "\"a\"", "3", "4", "1", "\"a\""}, t)
	})
	t.Run("named tuple", func(t *testing.T) {
		CheckCSharpPrintlnValue(`
var t = (Name: "n", Age: 5);
println(t.Name);
println(t.Age);
`, []string{"\"n\"", "5"}, t)
	})
}

func TestCSharp_Expression_Lambda(t *testing.T) {
	t.Run("lambda expression body", func(t *testing.T) {
		code := CreateCSharpProgram(`
Func<int, int> f = x => x + 1;
var r = f(2);
println(r);
`)
		ssatest.CheckSyntaxFlow(t, code, `println(* as $v)`, map[string][]string{
			"v": {"Function-Main.Main$1(2)"},
		}, ssaapi.WithLanguage(ssaconfig.CSHARP))
	})
	t.Run("lambda captures outer variable", func(t *testing.T) {
		code := CreateCSharpProgram(`
int a = 10;
Func<int, int> f = x => x + a;
println(f);
`)
		ssatest.CheckSyntaxFlow(t, code, `println(* as $f); $f<getReturns> #-> as $src`, map[string][]string{
			"src": {"10", "Parameter-x"},
		}, ssaapi.WithLanguage(ssaconfig.CSHARP))
	})
	t.Run("anonymous delegate", func(t *testing.T) {
		code := CreateCSharpProgram(`
Action<string> a = delegate(string s) { Console.WriteLine(s); };
a("hi");
`)
		ssatest.CheckSyntaxFlow(t, code, `Console.WriteLine(* as $arg); $arg #-> as $src`, map[string][]string{
			"arg": {"Parameter-s"},
			"src": {"\"hi\""},
		}, ssaapi.WithLanguage(ssaconfig.CSHARP))
	})
	t.Run("local function", func(t *testing.T) {
		CheckCSharpPrintlnValue(`
int Add(int x, int y) { return x + y; }
var r = Add(1, 2);
println(r);
`, []string{"Function-Main.Main.Add(1,2)"}, t)
	})
}

func TestCSharp_Expression_Keywords(t *testing.T) {
	t.Run("typeof default nameof sizeof", func(t *testing.T) {
		CheckCSharpPrintlnValue(`
var t = typeof(string);
println(t);
int d = default;
println(d);
var d2 = default(int);
println(d2);
var n = nameof(Console);
println(n);
`, []string{"Undefined-typeof(\"string\")", "0", "0", "\"Console\""}, t)
	})
	t.Run("is as cast", func(t *testing.T) {
		code := CreateCSharpProgram(`
object o = foo();
bool b = o is string;
println(b);
string s = o as string;
println(s);
int i = (int)o;
println(i);
`)
		ssatest.CheckSyntaxFlow(t, code, `println(* as $v)`, map[string][]string{
			"v": {"Undefined-is(Undefined-foo(),\"string\")", "castType(string, Undefined-foo())", "castType(number, Undefined-foo())"},
		}, ssaapi.WithLanguage(ssaconfig.CSHARP))
	})
	t.Run("await", func(t *testing.T) {
		code := CreateCSharpProgram(`
var r = await client.GetAsync("u");
println(r);
`)
		ssatest.CheckSyntaxFlow(t, code, `println(* as $v)`, map[string][]string{
			"v": {"Undefined-client.GetAsync(valid)(Undefined-client,\"u\")"},
		}, ssaapi.WithLanguage(ssaconfig.CSHARP))
	})
	t.Run("throw expression", func(t *testing.T) {
		code := CreateCSharpProgram(`
string s = foo() ?? throw new ArgumentNullException("s");
println(s);
`)
		ssatest.CheckSyntaxFlowContain(t, code, `ArgumentNullException(* as $arg)`, map[string][]string{
			"arg": {"\"s\""},
		}, ssaapi.WithLanguage(ssaconfig.CSHARP))
	})
}

func TestCSharp_Expression_Index(t *testing.T) {
	t.Run("array index read write", func(t *testing.T) {
		CheckCSharpPrintlnValue(`
int[] a = new int[3];
a[0] = 5;
println(a[0]);
a[1] = a[0] + 1;
println(a[1]);
`, []string{"5", "6"}, t)
	})
	t.Run("multi dim and range", func(t *testing.T) {
		code := CreateCSharpProgram(`
var s = foo();
var last = s[^1];
println(last);
var slice = s[1..3];
println(slice);
`)
		ssatest.CheckSyntaxFlow(t, code, `println(* as $v)`, map[string][]string{
			"v": {"Undefined-s.neg(1)(valid)", "Undefined-s[Undefined-range(1,3)]"},
		}, ssaapi.WithLanguage(ssaconfig.CSHARP))
	})
}

func TestCSharp_Expression_Linq(t *testing.T) {
	t.Run("method syntax", func(t *testing.T) {
		code := CreateCSharpProgram(`
var nums = new List<int> { 1, 2, 3 };
var evens = nums.Where(n => n % 2 == 0).Select(n => n * 2).ToList();
println(evens);
`)
		ssatest.CheckSyntaxFlowContain(t, code, `.Where(* as $pred)`, map[string][]string{
			"pred": {"Function-Main.Main$1"},
		}, ssaapi.WithLanguage(ssaconfig.CSHARP))
	})
	t.Run("query syntax", func(t *testing.T) {
		code := CreateCSharpProgram(`
var nums = foo();
var q = from n in nums where n > 1 select n * 2;
println(q);
`)
		ssatest.CheckSyntaxFlow(t, code, `println(* as $v)`, map[string][]string{
			"v": {"Undefined-foo().select(valid)(Undefined-foo().where(valid)(Undefined-foo(),Function-Main.Main$1),Function-Main.Main$2)"},
		}, ssaapi.WithLanguage(ssaconfig.CSHARP))
	})
	t.Run("query source evaluated once", func(t *testing.T) {
		prog := parseCSharpSemantics(t, CreateCSharpProgram(`
var q = from n in querySource() select n;
consume(q);
`))
		calls := csharpCallsToMethod(t, prog, "querySource")
		require.Len(t, calls, 1, "a query source expression must be evaluated exactly once")
	})
}

func TestCSharp_Expression_SwitchDefaultArmEmittedOnce(t *testing.T) {
	compile := func(t *testing.T, catchAll string) *ssaapi.Program {
		t.Helper()
		return parseCSharpSemantics(t, CreateCSharpProgram(`
var result = key() switch {
    1 => selected(),
    `+catchAll+` => fallback()
};
consume(result);
`))
	}

	t.Run("var catch-all", func(t *testing.T) {
		calls := csharpCallsToMethod(t, compile(t, "var _"), "fallback")
		require.Len(t, calls, 1, "the default switch-expression arm must be emitted exactly once")
	})
	t.Run("discard catch-all", func(t *testing.T) {
		calls := csharpCallsToMethod(t, compile(t, "_"), "fallback")
		require.Len(t, calls, 1, "the discard switch-expression arm must be emitted exactly once")
		require.NotNil(t, calls[0].GetBlock())
		require.True(t, strings.HasPrefix(calls[0].GetBlock().GetName(), ssa.SwitchDefault),
			"a discard arm must lower to the default block, got %q", calls[0].GetBlock().GetName())
	})
}

func csharpCallsToMethod(t *testing.T, prog *ssaapi.Program, methodName string) []*ssa.Call {
	t.Helper()
	require.NotEmpty(t, prog.Ref(methodName), "%s must be compiled", methodName)
	var calls []*ssa.Call
	prog.Program.EachFunction(func(function *ssa.Function) {
		for _, blockID := range function.Blocks {
			blockValue, ok := function.GetValueById(blockID)
			if !ok {
				continue
			}
			block, ok := ssa.ToBasicBlock(blockValue)
			if !ok {
				continue
			}
			for _, instructionID := range block.Insts {
				instruction, ok := function.GetValueById(instructionID)
				if !ok {
					continue
				}
				call, ok := ssa.ToCall(instruction)
				if !ok {
					continue
				}
				callee, ok := call.GetValueById(call.Method)
				if ok && callee.GetName() == methodName {
					calls = append(calls, call)
				}
			}
		}
	})
	return calls
}
