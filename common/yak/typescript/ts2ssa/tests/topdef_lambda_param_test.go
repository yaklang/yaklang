package tests

import (
	"testing"

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
		})
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
		})
	})
}
