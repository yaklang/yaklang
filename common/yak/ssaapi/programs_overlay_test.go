package ssaapi_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

var valueName = "valueStr"

var baseValueStr = "Value from Base"
var extendValueStr = "Value from Extend"

func InitProgram(t *testing.T) (progBase *ssaapi.Program, progExtend *ssaapi.Program, progNameBaseUUID string, progNameExtendUUID string) {
	progNameBaseUUID = uuid.NewString()
	progNameExtendUUID = uuid.NewString()

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

	progBase, progExtend, progNameBaseUUID, progNameExtendUUID := InitProgram(t)
	require.NotNil(t, progBase)
	require.NotNil(t, progExtend)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progNameBaseUUID)
		ssadb.DeleteProgram(ssadb.GetDB(), progNameExtendUUID)
	}()

	// 打印调试信息
	t.Logf("progBase: %v", progBase.GetProgramName())
	t.Logf("progExtend: %v", progExtend.GetProgramName())

	// 创建 ProgramOverLay，注意参数顺序：layers[0] = Layer1（最底层），layers[1] = Layer2（上层）
	// progBase = Layer1 (最底层)
	// progExtend = Layer2 (上层，会覆盖 Layer1 中的同名文件)
	overProg := ssaapi.NewProgramOverLay(progBase, progExtend)
	require.NotNil(t, overProg)

	t.Run("test layer structure built correctly", func(t *testing.T) {
		// 验证 Layer 结构已经构建
		layerCount := overProg.GetLayerCount()
		fileCount := overProg.GetFileCount()
		t.Logf("Layer count: %d", layerCount)
		t.Logf("Unique file count: %d", fileCount)
		require.Equal(t, layerCount, 2, "Should have 2 layers")

		// 检查 Layer1 的文件
		layer1Files := overProg.GetFilesInLayer(1)
		t.Logf("Layer1 files: %v", layer1Files)
		require.Greater(t, len(layer1Files), 0, "Layer1 should have files")

		// 检查 Layer2 的文件
		layer2Files := overProg.GetFilesInLayer(2)
		t.Logf("Layer2 files: %v", layer2Files)
		require.Greater(t, len(layer2Files), 0, "Layer2 should have files")
	})

	check := func(p ssaapi.SyntaxFlowQueryInstance, rule string, expectOverridden bool, expectData string) {
		res, err := p.SyntaxFlowWithError(rule)
		res.Show()
		require.NoError(t, err)
		values := res.GetAllValuesChain()
		require.NotEmpty(t, values, "Should find values for rule: %s", rule)
		require.Len(t, values, 1, "Should find exactly one value for rule: %s", rule)
		v := values[0]
		isOverridden := overProg.IsOverridden(v)
		require.Equalf(t, expectOverridden, isOverridden, "Value %s overridden status should be %v", v.String(), expectOverridden)
		require.Containsf(t, v.String(), expectData, "Value %s data should contain %s", v.String(), expectData)
	}

	t.Run("test IsOverridden method : valueStr", func(t *testing.T) {
		rule := "valueStr as $res"
		// 从 progBase 获取一个 Value（应该被上层覆盖）
		check(progBase, rule, true, baseValueStr)
		// 从 progExtend 获取一个 Value（不会被覆盖，因为是最上层）
		check(progExtend, rule, false, extendValueStr)

		// 从 overlay 获取一个 Value（应该返回上层的值）
		check(overProg, rule, false, extendValueStr)
	})

	t.Run("test IsOverridden method : A.valueStr", func(t *testing.T) {
		rule := "A.valueStr as $res"
		// 从 progBase 获取一个 Value（应该被上层覆盖）
		check(progBase, rule, true, baseValueStr)
		// 从 progExtend 获取一个 Value（不会被覆盖）
		check(progExtend, rule, false, extendValueStr)

		// 从 overlay 获取一个 Value（应该返回上层的值）
		check(overProg, rule, false, extendValueStr)
	})

	t.Run("test Relocate method", func(t *testing.T) {
		// 从 Layer1 (progBase) 获取一个 Value
		baseValues := progBase.Ref(valueName)
		require.Equal(t, baseValues.Len(), 1)

		baseValue := baseValues[0]
		relocatedValue := overProg.Relocate(baseValue)
		require.NotNil(t, relocatedValue)

		// 打印信息用于调试
		t.Logf("Original value: %s", baseValue.String())
		t.Logf("Relocated value: %s", relocatedValue.String())

		// 如果文件在上层也存在，重定位后的值应该来自上层 Layer
		// 这里我们只验证重定位功能可以正常工作
		require.NotNil(t, relocatedValue)
		require.Equalf(t, relocatedValue.GetProgramName(), progNameExtendUUID, "Relocated value should come from extend program (Layer2)")
	})
}

func TestOverlay_CrossLayer_Flow(t *testing.T) {
	progBase, progExtend, progNameBaseUUID, progNameExtendUUID := InitProgram(t)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progNameBaseUUID)
		ssadb.DeleteProgram(ssadb.GetDB(), progNameExtendUUID)
	}()

	// 创建多层 Layer：progBase = Layer1 (最底层), progExtend = Layer2 (上层)
	overProg := ssaapi.NewProgramOverLay(progBase, progExtend)
	require.NotNil(t, overProg)

	rule := "println(, * as $arg); $arg #->  as $data"
	check := func(p ssaapi.SyntaxFlowQueryInstance, expectData string) {
		res, err := p.SyntaxFlowWithError(rule, ssaapi.QueryWithEnableDebug())
		require.NoError(t, err)
		res.Show()
		values := res.GetValues("data")
		require.NotEmpty(t, values, "Should find values for rule: %s", rule)
		require.Len(t, values, 1, "Should find exactly one value for rule: %s", rule)
		v := values[0]
		require.Containsf(t, v.String(), expectData, "Value %s data should contain %s", v.String(), expectData)
	}

	t.Run("test Cross-Layer Call Graph linking - baseline", func(t *testing.T) {
		// 在 Base 中，调用 A.getValue() 应该返回 "Value from A"
		check(progBase, "Value from A")
	})

	t.Run("test extend ", func(t *testing.T) {
		res, err := progExtend.SyntaxFlowWithError(rule, ssaapi.QueryWithEnableDebug())
		require.NoError(t, err)
		res.Show()
		values := res.GetAllValuesChain()
		require.Empty(t, values, "Should not find values in extend program alone")
	})

	t.Run("test Normal-Program Call Graph linking", func(t *testing.T) {
		check(ssaapi.Programs{progBase, progExtend}, "Value from A")
	})

	t.Run("test Cross-Layer Call Graph linking - overlay", func(t *testing.T) {
		check(overProg, "Value from Extended A")
	})
}
