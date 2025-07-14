package ssaapi_test

import (
	"github.com/yaklang/yaklang/common/log"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

// findFunctionByName 在程序中查找指定名称的函数
func findFunctionByName(program *ssaapi.Program, namePattern string) *ssa.Function {
	var result *ssa.Function

	// 遍历函数映射
	program.Program.Funcs.ForEach(func(_ string, v *ssa.Function) bool {
		if strings.Contains(v.GetName(), namePattern) {
			result = v
			return false // 停止遍历
		}
		return true // 继续遍历
	})

	return result
}

// findFunctionInAllProgramFuncs 在程序中递归查找指定名称的函数
func findFunctionInAllProgramFuncs(program *ssaapi.Program, namePattern string) *ssa.Function {
	// 先在当前程序中查找
	if fn := findFunctionByName(program, namePattern); fn != nil {
		return fn
	}

	// 遍历所有函数并检查名称
	var result *ssa.Function
	program.Program.Funcs.ForEach(func(name string, fn *ssa.Function) bool {
		if strings.Contains(fn.GetName(), namePattern) {
			result = fn
			return false // 停止遍历
		}

		// 检查子函数
		for _, childValue := range fn.ChildFuncs {
			childValue, _ := fn.GetValueById(childValue)
			if childFn, ok := childValue.(*ssa.Function); ok {
				if strings.Contains(childFn.GetName(), namePattern) {
					result = childFn
					return false // 停止遍历
				}
			}
		}

		return true // 继续遍历
	})

	return result
}

func TestFunctionCFG(t *testing.T) {
	// 简单的Java函数测试
	code := `
	public class TestClass {
		public int testFunction(int a) {
			if (a > 10) {
				return a + 5;
			} else if (a > 5) {
				int result = 0;
				for (int i = 0; i < a; i++) {
					result += i;
				}
				return result;
			} else {
				return 0;
			}
		}
	}
	`

	progName := uuid.NewString()
	prog, err := ssaapi.Parse(code,
		ssaapi.WithLanguage("java"),
		ssaapi.WithProgramName(progName),
	)
	require.NoError(t, err)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progName)
	}()

	// 获取testFunction函数
	testFunction := findFunctionByName(prog, "testFunction")
	require.NotNil(t, testFunction, "未找到testFunction函数")

	// 生成函数的控制流图
	dot := ssaapi.FunctionDotGraph(testFunction)
	log.Infof("函数控制流图DOT: \n%s", dot)

	// 验证控制流图包含必要的元素
	require.True(t, strings.Contains(dot, "digraph"), "控制流图应该包含digraph定义")
	require.True(t, strings.Contains(dot, "->"), "控制流图应该包含边")

	// 验证分支信息
	require.True(t, strings.Contains(dot, "true") || strings.Contains(dot, "false"), "控制流图应该包含条件分支标签")
}
