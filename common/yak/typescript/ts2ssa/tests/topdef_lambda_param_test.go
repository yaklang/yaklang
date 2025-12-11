package tests

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// TestTopDef_LambdaAsParameter 测试当函数参数是lambda函数时，topdef能正确追踪
// 这个测试用例来自于一个bug修复：
// 之前当callee是Parameter类型时，代码错误地使用callee.GetFunc()获取参数数量
// 导致为fn1(value)调用添加了额外的Undefined参数
func TestTopDef_LambdaAsParameter(t *testing.T) {
	t.Run("lambda function as parameter with topdef", func(t *testing.T) {
		code := `var process = (value, fn1, fn2, fn3) => {
    var r1 = fn1(value)
    var r2 = fn2(r1)
    var r3 = fn3(r2)
    return r3
}

var a = process(
    11111,
    (x) => x + 1,
    (x) => x * 2,
    (x) => x - 100
)`
		// topdef应该追踪到实际的值：11111, 1, 2, 100
		// 而不是之前错误的结果：11111, 1, Undefined-, Undefined-, Undefined-, 2, ...
		ssatest.CheckSyntaxFlow(t, code, `a #-> as $res`, map[string][]string{
			"res": {"11111", "1", "2", "100"},
		}, ssaapi.WithLanguage(ssaconfig.TS))
	})

	t.Run("chained lambda parameter call", func(t *testing.T) {
		// 测试链式调用lambda参数
		code := `var pipe = (value, fn1, fn2) => {
    var temp = fn1(value)
    return fn2(temp)
}

var result = pipe(10, (x) => x + 5, (x) => x * 3)`
		ssatest.CheckSyntaxFlow(t, code, `result #-> as $res`, map[string][]string{
			"res": {"10", "5", "3"},
		}, ssaapi.WithLanguage(ssaconfig.TS))
	})
}

func TestTopDef_Anonymous(t *testing.T) {
	t.Run("closure capture", func(t *testing.T) {
		code := `let i = 333333
let f = () => {
	let j = 444444
	return () => {
		return i + j
    }
}
let f1 = f()
let a = f1()
console.log(a)
`
		ssatest.CheckSyntaxFlow(t, code, `a #-> as $res`, map[string][]string{
			"res": {"333333", "444444"}}, ssaapi.WithLanguage(ssaconfig.TS))
	})
	t.Run("nested side-effect - modifier called", func(t *testing.T) {
		// 当 modifier 被调用时，a 应该追踪到 222222
		code := `
var x = 111111
var modifier = () => {x = 222222}

var execute = (fn) => {fn()}
execute(modifier)
var a = x
`
		ssatest.CheckSyntaxFlow(t, code, `a #-> as $res`, map[string][]string{"res": {"222222"}}, ssaapi.WithLanguage(ssaconfig.TS))
	})

	t.Run("nested side-effect - modifier not called", func(t *testing.T) {
		// 当 modifier 没有被调用时，a 应该只追踪到 111111
		// 注意：由于 Mask 机制，当前仍会追踪到 222222，这是一个已知问题 OverApproximate Analysis
		code := `
var x = 111111
var modifier = () => {x = 222222}
var a = x
`
		ssatest.CheckSyntaxFlow(t, code, `a #-> as $res`, map[string][]string{
			"res": {"111111", "222222"}, // 由于 Mask 机制，222222 也会被追踪到
		}, ssaapi.WithLanguage(ssaconfig.TS))
	})

	t.Run("nested side-effect - function passed but not called", func(t *testing.T) {
		// 当函数作为参数传递但在被调用函数内部没有被调用时，side-effect 不应该被传播
		// 这个测试验证 handleArgumentFunctionSideEffect 的修复：
		// 只有当参数函数在 callee 内部真正被调用时，才应该传播 side-effect
		code := `
var x = 111111
var modifier = () => {x = 222222}

// store 函数接收 fn 参数但不调用它，只是存储
var store = (fn) => { var arr = [fn]; return arr }
store(modifier)
var a = x
`
		// 由于 modifier 在 store 中没有被调用，x 的值应该还是 111111
		// 但由于 Mask 机制，222222 仍然会被追踪到
		ssatest.CheckSyntaxFlow(t, code, `a #-> as $res`, map[string][]string{
			"res": {"111111", "222222"}, // 由于 Mask 机制，222222 也会被追踪到
		}, ssaapi.WithLanguage(ssaconfig.TS))
	})

	t.Run("nested side-effect case - return closure and call", func(t *testing.T) {
		// 这是一个更复杂的场景：函数返回一个闭包，然后调用这个闭包
		// f 返回一个修改 i 的闭包，f1 = f() 获取这个闭包，然后 f1() 调用它
		//
		// 现在 side-effect 能够正确识别：
		// 1. f 的返回类型被正确推断为函数类型（带有 side-effect）
		// 2. f1() 调用时能够正确处理 side-effect
		// 3. 因此 a 的值能够追踪到 444444（f1() 执行后 i 被修改为 j 的值）
		code := `
var i = 333333
var f = () => {
    var j = 444444
    return () => {
        i = j
    }
}
var f1 = f()
f1()
var a = i
`
		ssatest.CheckSyntaxFlow(t, code, `a #-> as $res`, map[string][]string{
			"res": {"444444"}, // f1() 执行后，i 被修改为 j 的值 444444
		}, ssaapi.WithLanguage(ssaconfig.TS))
	})

	t.Run("recursive call", func(t *testing.T) {
		// 互递归场景：isEven 和 isOdd 互相调用
		// TopDef 追踪 isEven(4) 的返回值，应该能找到：
		// - true (isEven 的基本情况返回)
		// - false (isOdd 的基本情况返回)
		// - Function-isOdd (作为 FreeValue 的默认值被追踪到)
		//
		// 注意：由于递归调用的复杂性，当前实现会追踪到 FreeValue 的 Default 值
		// 理想情况下应该进一步追踪参数 4，但这需要更复杂的过程间分析
		code := `
var isEven = (n) => {
    if(n==0){return true}
    return isOdd(n-1)
}
var isOdd = (n) => {
    if(n==0){return false}
    return isEven(n-1)
}
var a = isEven(4)
`
		ssatest.CheckSyntaxFlow(t, code, `a #-> as $res`, map[string][]string{
			"res": {"true", "false", "Function-isOdd"},
		}, ssaapi.WithLanguage(ssaconfig.TS))
	})
}

// TestSideEffect_Debug_Analysis 是一个辅助调试测试，用于分析 SideEffect 机制的工作原理
// 默认跳过，仅在本地调试时使用
//
// SideEffect 机制说明：
// 1. 当闭包修改外部变量时（如 modifier 中的 x = 222222），会记录 FunctionSideEffect
// 2. 当函数被调用时（如 execute(modifier)），handleArgumentFunctionSideEffect 会：
//   - 检查参数是否是带有 SideEffect 的函数
//   - 在调用点创建 SideEffect 指令
//   - 将变量重新绑定到 SideEffect 指令
//
// 3. TopDef 追踪 SideEffect 时，只追踪 SideEffect.Value（即修改后的值）
//
// 为什么 "modifier called" 场景只追踪到 222222：
//   - execute(modifier) 调用后，x 被重新绑定到 SideEffect 指令
//   - var a = x 读取的是 SideEffect 指令，而不是原始的 111111
//   - TopDef 处理 SideEffect 时只追踪 SideEffect.Value = 222222
//
// 为什么 "modifier not called" 场景会追踪到 111111 和 222222：
//   - modifier 没有被调用，所以没有创建 SideEffect 指令
//   - var a = x 读取的是原始的 111111 (ConstInst)
//   - 但由于 Mask 机制，111111 记录了 222222 作为可能的遮蔽值
//   - TopDef 的 visitedDefs 会遍历 Mask，导致 222222 也被追踪到
func TestSideEffect_Debug_Analysis(t *testing.T) {
	t.Skip("Debug test - skip in CI, run locally for analysis")

	t.Run("analyze side-effect mechanism", func(t *testing.T) {
		code := `
var x = 111111
var modifier = () => {x = 222222}
var execute = (fn) => {fn()}
execute(modifier)
var a = x
`
		prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.TS))
		if err != nil {
			t.Fatal(err)
		}

		fmt.Println("==================== SSA Program ====================")
		prog.Show()

		fmt.Println("\n==================== Variable 'x' Analysis ====================")
		fmt.Println("x 变量在不同阶段有不同的值：")
		xVals := prog.Ref("x")
		for _, v := range xVals {
			fmt.Printf("  - %s (opcode: %s, id: %d)\n", v.String(), v.GetOpcode(), v.GetId())
			if masks := v.GetMask(); len(masks) > 0 {
				fmt.Printf("    masks: ")
				for _, m := range masks {
					fmt.Printf("%s ", m.String())
				}
				fmt.Println()
			}
		}

		fmt.Println("\n==================== Variable 'a' Analysis ====================")
		fmt.Println("a 变量绑定到的值：")
		aVals := prog.Ref("a")
		for _, v := range aVals {
			fmt.Printf("  - %s (opcode: %s, id: %d)\n", v.String(), v.GetOpcode(), v.GetId())
			if se, ok := ssa.ToSideEffect(v.GetSSAInst()); ok {
				fmt.Printf("    这是一个 SideEffect 指令:\n")
				fmt.Printf("      CallSite (调用点): %d\n", se.CallSite)
				fmt.Printf("      Value (修改后的值ID): %d\n", se.Value)
				if val, ok := se.GetValueById(se.Value); ok {
					fmt.Printf("      Value 指向: %s\n", val.String())
				}
			}
		}

		fmt.Println("\n==================== TopDef Results ====================")
		res, _ := prog.SyntaxFlowWithError("a #-> as $res")
		resVals := res.GetValues("res")
		fmt.Printf("TopDef 追踪到 %d 个结果:\n", len(resVals))
		for _, v := range resVals {
			fmt.Printf("  - %s (opcode: %s)\n", v.String(), v.GetOpcode())
		}

		fmt.Println("\n==================== 结论 ====================")
		fmt.Println("因为 execute(modifier) 被调用后:")
		fmt.Println("  1. handleArgumentFunctionSideEffect 检测到 modifier 有 SideEffect")
		fmt.Println("  2. 创建了 SideEffect 指令 (t27)")
		fmt.Println("  3. 变量 x 被重新绑定到 SideEffect 指令")
		fmt.Println("  4. var a = x 读取的是 SideEffect，而不是原始的 111111")
		fmt.Println("  5. TopDef 处理 SideEffect 时只追踪 SideEffect.Value = 222222")
	})
}
