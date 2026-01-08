package ssaapi_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestOverlaySaveAndLoadFromDatabase(t *testing.T) {
	baseProgramName := uuid.NewString()
	diffProgramName := uuid.NewString()

	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), baseProgramName)
		ssadb.DeleteProgram(ssadb.GetDB(), diffProgramName)
	}()

	// 创建基础文件系统
	baseFS := filesys.NewVirtualFs()
	baseFS.AddFile("A.java", `
	public class A {
		static string valueStr = "Value from Base";
		public String getValue() {
			return "Value from A";
		}
	}`)
	baseFS.AddFile("Main.java", `
	public class Main{
		public static void main(String[] args) {
			A a = new A();
			System.out.println(a.getValue());
		}
	}
	`)
	baseFS.AddFile("Utils.java", `
	public class Utils {
		public static void helper() {
			System.out.println("Helper from Base");
		}
	}
	`)

	// 编译基础程序
	basePrograms, err := ssaapi.ParseProject(
		ssaapi.WithFileSystem(baseFS),
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithProgramName(baseProgramName),
	)
	require.NoError(t, err)
	require.NotNil(t, basePrograms)
	require.Greater(t, len(basePrograms), 0)
	_ = basePrograms[0] // baseProgram is used implicitly

	// 创建新文件系统（包含修改、新增、删除）
	newFS := filesys.NewVirtualFs()
	// 修改 A.java
	newFS.AddFile("A.java", `
	public class A {
		static string valueStr = "Value from Extend";
		public String getValue() {
			return "Value from Extended A";
		}
	}`)
	// Main.java 保持不变（不包含在差量中）
	newFS.AddFile("Main.java", `
	public class Main{
		public static void main(String[] args) {
			A a = new A();
			System.out.println(a.getValue());
		}
	}
	`)
	// 删除 Utils.java（不添加到 newFS）
	// 新增 NewFile.java
	newFS.AddFile("NewFile.java", `
	public class NewFile {
		public static void newMethod() {
			System.out.println("New method from Extend");
		}
	}
	`)

	t.Run("test overlay saved to database", func(t *testing.T) {
		// 使用增量编译 API
		ctx := context.Background()
		diffPrograms, err := ssaapi.ParseProjectWithIncrementalCompile(
			baseFS, newFS,
			baseProgramName, diffProgramName,
			ssaconfig.JAVA,
			ssaapi.WithContext(ctx),
		)
		require.NoError(t, err)
		require.NotNil(t, diffPrograms)
		require.Greater(t, len(diffPrograms), 0)
		diffProgram := diffPrograms[0]

		// 验证 overlay 已创建
		overlay := diffProgram.GetOverlay()
		require.NotNil(t, overlay, "overlay should be created")
		require.GreaterOrEqual(t, len(overlay.Layers), 2, "overlay should have at least 2 layers")

		// 验证数据库中的 overlay 信息
		irProgram := diffProgram.Program.GetIrProgram()
		require.NotNil(t, irProgram, "irProgram should exist")
		require.True(t, irProgram.IsOverlay, "IsOverlay should be true in database")
		require.NotEmpty(t, irProgram.OverlayLayers, "OverlayLayers should be saved in database")
		require.Equal(t, 2, len(irProgram.OverlayLayers), "OverlayLayers should contain 2 layer names")
		require.Contains(t, irProgram.OverlayLayers, baseProgramName, "OverlayLayers should contain base program name")
		require.Contains(t, irProgram.OverlayLayers, diffProgramName, "OverlayLayers should contain diff program name")

		// 验证 layer 顺序（base 应该是第一个，diff 应该是第二个）
		require.Equal(t, baseProgramName, irProgram.OverlayLayers[0], "base program should be the first layer")
		require.Equal(t, diffProgramName, irProgram.OverlayLayers[1], "diff program should be the second layer")

		// 验证所有 layer 的 program 都已保存到数据库
		for _, layerName := range irProgram.OverlayLayers {
			layerProg, err := ssaapi.FromDatabase(layerName)
			require.NoError(t, err, "layer program %s should be saved to database", layerName)
			require.NotNil(t, layerProg, "layer program %s should not be nil", layerName)
		}
	})

	t.Run("test overlay loaded from database", func(t *testing.T) {
		// 从数据库重新加载 diff program
		reloadedDiffProgram, err := ssaapi.FromDatabase(diffProgramName)
		require.NoError(t, err)
		require.NotNil(t, reloadedDiffProgram)

		// 验证 overlay 已从数据库加载并重建
		reloadedOverlay := reloadedDiffProgram.GetOverlay()
		require.NotNil(t, reloadedOverlay, "overlay should be loaded from database")
		require.GreaterOrEqual(t, len(reloadedOverlay.Layers), 2, "reloaded overlay should have at least 2 layers")

		// 验证 layer 的 program names
		layerNames := reloadedOverlay.GetLayerProgramNames()
		require.Equal(t, 2, len(layerNames), "reloaded overlay should have 2 layer names")
		require.Contains(t, layerNames, baseProgramName, "reloaded overlay should contain base program name")
		require.Contains(t, layerNames, diffProgramName, "reloaded overlay should contain diff program name")

		// 验证 layer 顺序
		require.Equal(t, baseProgramName, layerNames[0], "base program should be the first layer in reloaded overlay")
		require.Equal(t, diffProgramName, layerNames[1], "diff program should be the second layer in reloaded overlay")

		// 验证每个 layer 的 program 都已正确加载
		for i, layer := range reloadedOverlay.Layers {
			require.NotNil(t, layer, "layer %d should not be nil", i)
			require.NotNil(t, layer.Program, "layer %d program should not be nil", i)
			require.NotEmpty(t, layer.Program.GetProgramName(), "layer %d program should have a name", i)
		}
	})

	t.Run("test overlay functionality after loading from database", func(t *testing.T) {
		// 从数据库加载 overlay
		reloadedDiffProgram, err := ssaapi.FromDatabase(diffProgramName)
		require.NoError(t, err)
		require.NotNil(t, reloadedDiffProgram)

		overlay := reloadedDiffProgram.GetOverlay()
		require.NotNil(t, overlay, "overlay should be loaded from database")

		// 验证 overlay 可以查找类（上层覆盖下层）
		// A 类应该在 diffProgram (Layer2) 中，而不是 baseProgram (Layer1) 中
		classA := overlay.Ref("A")
		require.NotEmpty(t, classA, "overlay should contain class A")

		// 验证 overlay 包含新增的类
		newFileClass := overlay.Ref("NewFile")
		require.NotEmpty(t, newFileClass, "overlay should contain class NewFile")

		// 验证 overlay 包含未修改的类（来自 base）
		mainClass := overlay.Ref("Main")
		require.NotEmpty(t, mainClass, "overlay should contain class Main (from base)")

		// 验证 overlay 不包含已删除的类
		utilsClass := overlay.Ref("Utils")
		require.Empty(t, utilsClass, "overlay should not contain class Utils (deleted)")

		// 验证文件系统聚合
		aggFS := overlay.GetAggregatedFileSystem()
		require.NotNil(t, aggFS, "aggregated file system should not be nil")

		// 验证文件数量
		fileCount := overlay.GetFileCount()
		// Overlay 应该包含：A.java (来自 diff), Main.java (来自 base), NewFile.java (来自 diff)
		// Utils.java 应该不存在（被删除）
		require.GreaterOrEqual(t, fileCount, 3, "overlay should have at least 3 files")
	})

	t.Run("test overlay layer programs are all saved", func(t *testing.T) {
		// 从数据库加载 diff program
		reloadedDiffProgram, err := ssaapi.FromDatabase(diffProgramName)
		require.NoError(t, err)
		require.NotNil(t, reloadedDiffProgram)

		irProgram := reloadedDiffProgram.Program.GetIrProgram()
		require.NotNil(t, irProgram)
		require.True(t, irProgram.IsOverlay)

		// 验证所有 layer 的 program 都在数据库中
		for _, layerName := range irProgram.OverlayLayers {
			// 检查数据库中的记录
			layerIrProgram, err := ssadb.GetProgram(layerName, ssadb.Application)
			require.NoError(t, err, "layer program %s should exist in database", layerName)
			require.NotNil(t, layerIrProgram, "layer program %s should not be nil", layerName)
			require.Equal(t, layerName, layerIrProgram.ProgramName, "layer program name should match")
		}
	})

	t.Run("test overlay with multiple layers", func(t *testing.T) {
		// 确保第一个 diff program 已经保存到数据库（从之前的测试）
		// 先尝试从数据库加载，确保它存在
		firstDiffProgram, err := ssaapi.FromDatabase(diffProgramName)
		require.NoError(t, err, "first diff program should exist in database")
		require.NotNil(t, firstDiffProgram, "first diff program should not be nil")

		// 验证第一个 diff program 有 overlay
		firstOverlay := firstDiffProgram.GetOverlay()
		require.NotNil(t, firstOverlay, "first diff program should have overlay")
		require.GreaterOrEqual(t, len(firstOverlay.Layers), 2, "first overlay should have at least 2 layers")

		// 创建第三个 layer（第二次增量编译）
		diffProgramName2 := uuid.NewString()
		defer func() {
			ssadb.DeleteProgram(ssadb.GetDB(), diffProgramName2)
		}()

		// 创建第三个文件系统（再次修改 A.java，新增 AnotherFile.java）
		newFS2 := filesys.NewVirtualFs()
		newFS2.AddFile("A.java", `
		public class A {
			static string valueStr = "Value from Second Extend";
			public String getValue() {
				return "Value from Second Extended A";
			}
		}`)
		newFS2.AddFile("AnotherFile.java", `
		public class AnotherFile {
			public static void anotherMethod() {
				System.out.println("Another method");
			}
		}
		`)

		// 使用第一个 diff program 作为 base，创建第二个 diff program
		// 注意：baseFS 应该是第一个 diff program 对应的文件系统（newFS），而不是 baseFS
		ctx := context.Background()
		diffPrograms2, err := ssaapi.ParseProjectWithIncrementalCompile(
			newFS, newFS2, // 使用第一个 diff 的 newFS 作为 baseFS
			diffProgramName, diffProgramName2, // 使用第一个 diff 作为 base
			ssaconfig.JAVA,
			ssaapi.WithContext(ctx),
		)
		require.NoError(t, err)
		require.NotNil(t, diffPrograms2)
		require.Greater(t, len(diffPrograms2), 0)
		diffProgram2 := diffPrograms2[0]

		// 验证第二个 overlay 包含 3 个 layers
		overlay2 := diffProgram2.GetOverlay()
		require.NotNil(t, overlay2, "second overlay should be created")
		require.GreaterOrEqual(t, len(overlay2.Layers), 3, "second overlay should have at least 3 layers")

		// 验证数据库中的 overlay 信息
		irProgram2 := diffProgram2.Program.GetIrProgram()
		require.NotNil(t, irProgram2)
		require.True(t, irProgram2.IsOverlay, "second overlay IsOverlay should be true")
		require.GreaterOrEqual(t, len(irProgram2.OverlayLayers), 3, "second overlay should have at least 3 layer names")

		// 验证所有 layer 都已保存
		for _, layerName := range irProgram2.OverlayLayers {
			layerProg, err := ssaapi.FromDatabase(layerName)
			require.NoError(t, err, "layer program %s should be saved", layerName)
			require.NotNil(t, layerProg, "layer program %s should not be nil", layerName)
		}

		// 从数据库重新加载并验证
		reloadedDiffProgram2, err := ssaapi.FromDatabase(diffProgramName2)
		require.NoError(t, err)
		require.NotNil(t, reloadedDiffProgram2)

		reloadedOverlay2 := reloadedDiffProgram2.GetOverlay()
		require.NotNil(t, reloadedOverlay2, "second overlay should be loaded from database")
		require.GreaterOrEqual(t, len(reloadedOverlay2.Layers), 3, "reloaded second overlay should have at least 3 layers")

		// 验证可以查找最新的类
		classA := reloadedOverlay2.Ref("A")
		require.NotEmpty(t, classA, "overlay should contain class A (from latest layer)")

		anotherFileClass := reloadedOverlay2.Ref("AnotherFile")
		require.NotEmpty(t, anotherFileClass, "overlay should contain class AnotherFile (from latest layer)")
	})
}
