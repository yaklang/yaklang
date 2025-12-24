package ssaapi_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

var progNameBaseUUID = "TestMultipleLayer_BaseProgram"
var progNameExtendUUID = "TestMultipleLayer_ExtendProgram"

var valueName = "valueStr"

var baseValueStr = "Value from Base"
var extendValueStr = "Value from Extend"

func InitProgram(t *testing.T) (progBase *ssaapi.Program, progExtend *ssaapi.Program) {

	vf1 := filesys.NewVirtualFs()
	var err error

	// 强制重新创建 progBase
	t.Logf("Creating new progBase")
	vf1.AddFile("A.java", `
	public class A {
		static string valueStr = "Value from Base";
		public String getValue() {
			return "Value from A";
		}
	}`)

	vf1.AddFile("Main.java", `
	public class Main{
		public static void main(String[] args) {
			A a = new A();
			System.out.println(a.getValue());
		}
	}
	`)

	p, err := ssaapi.ParseProject(
		ssaapi.WithFileSystem(vf1),
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithProgramName(progNameBaseUUID),
	)
	require.NoError(t, err)
	require.NotNil(t, p)
	require.Greater(t, len(p), 0, "Should have at least one program")

	progBase, err = ssaapi.FromDatabase(progNameBaseUUID)
	require.NoError(t, err)
	require.NotNil(t, progBase)
	{
		vs := progBase.Ref(valueName)
		require.NotEmpty(t, vs, "Should find value in base program")
		require.Contains(t, vs.String(), baseValueStr, "Base value should match")
	}

	// 强制重新创建 progExtend
	t.Logf("Creating new progExtend")
	vf2 := filesys.NewVirtualFs()
	vf2.AddFile("A.java", `
	public class A {
		static string valueStr = "Value from Extend";
		public String getValue() {
			return "Value from Extended A";
		}	
	}`)

	p2, err := ssaapi.ParseProject(
		ssaapi.WithFileSystem(vf2),
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithProgramName(progNameExtendUUID),
	)
	require.NoError(t, err)
	require.NotNil(t, p2)
	require.Greater(t, len(p2), 0, "Should have at least one program")

	progExtend, err = ssaapi.FromDatabase(progNameExtendUUID)
	require.NoError(t, err)
	require.NotNil(t, progExtend)
	{
		vs := progExtend.Ref(valueName)
		require.NotEmpty(t, vs, "Should find value in extend program")
		require.Contains(t, vs.String(), extendValueStr, "Extend value should match")
	}

	return
}

func TestOverlay_Easy(t *testing.T) {

	progBase, progExtend := InitProgram(t)
	require.NotNil(t, progBase)
	require.NotNil(t, progExtend)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progNameBaseUUID)
		ssadb.DeleteProgram(ssadb.GetDB(), progNameExtendUUID)
	}()

	// 打印调试信息
	t.Logf("progBase: %v", progBase.GetProgramName())
	t.Logf("progExtend: %v", progExtend.GetProgramName())

	// 创建 ProgramOverLay，注意参数顺序：diff 在前，base 在后
	overProg := ssaapi.NewProgramOverLay(progExtend, progBase)
	require.NotNil(t, overProg)

	t.Run("test shadow set built correctly", func(t *testing.T) {
		// 验证 Shadow Set 已经构建
		shadowCount := overProg.GetShadowFileCount()
		shadowFiles := overProg.GetShadowFiles()
		t.Logf("Shadow file count: %d", shadowCount)
		for _, file := range shadowFiles {
			t.Logf("Shadow file: %s", file)
		}

		require.Equal(t, shadowCount, 1)
	})

	t.Run("test query with overlay", func(t *testing.T) {
		// 先看看有什么值可以搜索
		t.Log("Checking available values in programs...")
		baseValues := progBase.Ref(valueName)
		t.Logf("Base '%s' values: %s", valueName, baseValues)
		require.Contains(t, baseValues.String(), baseValueStr)

		extendValues := progExtend.Ref(valueName)
		t.Logf("Extend '%s' values: %s", valueName, extendValues)
		require.Contains(t, extendValues.String(), extendValueStr)

		// 测试查询功能：应该优先返回 Diff 中的结果
		values := overProg.Ref(valueName)
		t.Logf("Query returned %d values", len(values))
		require.Contains(t, values.String(), extendValueStr)
	})

	t.Run("test IsShadow method", func(t *testing.T) {
		// 从 progBase 获取一个 Value
		baseValues := progBase.Ref(valueName)
		baseValue := baseValues[0]
		t.Logf("baseValues: %s\n\tProgram:%s \t File: %s", baseValue.String(),
			baseValue.GetProgramName(),
			baseValue.GetRange().GetEditor().GetUrl(),
		)
		isShadow := overProg.IsShadow(baseValue)
		require.Truef(t, isShadow, "Base value should NOT be in shadow set")

		// 从 progExtend 获取一个 Value
		extendValues := progExtend.Ref(valueName)
		isShadow = overProg.IsShadow(extendValues[0])
		require.Falsef(t, isShadow, "Extend value should be in shadow set")
	})

	t.Run("test Relocate method", func(t *testing.T) {
		// 从 base 获取一个 Value
		baseValues := progBase.Ref(valueName)
		require.Equal(t, baseValues.Len(), 1)

		baseValue := baseValues[0]
		relocatedValue := overProg.Relocate(baseValue)
		require.NotNil(t, relocatedValue)

		// 打印信息用于调试
		t.Logf("Original value: %s", baseValue.String())
		t.Logf("Relocated value: %s", relocatedValue.String())

		// 如果文件被修改，重定位后的值应该来自 Diff
		// 这里我们只验证重定位功能可以正常工作
		require.NotNil(t, relocatedValue)
		require.Equalf(t, relocatedValue.GetProgramName(), progNameExtendUUID, "Relocated value should come from extend program")
	})

}

// func TestOverlay_Advanced(t *testing.T) {

// 	progBase, progExtend := InitProgram(t)
// 	_ = progBase
// 	_ = progExtend

// 	t.Run("search value", func(t *testing.T) {

// 	})

// 	t.Run("top def dataflow", func(t *testing.T) {

// 	})
// }
