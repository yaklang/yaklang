package ssaapi

import (
	"context"
	"errors"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func ParseProjectFromPath(path string, opts ...ssaconfig.Option) (Programs, error) {
	if path != "" {
		opts = append(opts, WithLocalFs(path))
	}
	return ParseProject(opts...)
}

func ParseProjectWithFS(fs fi.FileSystem, opts ...ssaconfig.Option) (Programs, error) {
	opts = append(opts, WithFileSystem(fs))
	return ParseProject(opts...)
}

// ParseProjectWithIncrementalCompile 解析项目并支持增量编译
// 如果提供了 baseFS 和 baseProgramName，将执行增量编译并返回包含 overlay 的 Programs
// 调用者可以通过 program.GetOverlay() 获取 ProgramOverLay 用于规则扫描
// baseFS: 基础文件系统（全量编译的文件系统）
// newFS: 新文件系统（当前要编译的文件系统）
// baseProgramName: 基础程序的名称（必须已存在于数据库中）
// diffProgramName: 差量程序的名称
// language: 编译语言
// opts: 其他编译选项
// 返回：包含差量程序的 Programs，可以通过 program.GetOverlay() 获取 ProgramOverLay
func ParseProjectWithIncrementalCompile(
	baseFS, newFS fi.FileSystem,
	baseProgramName, diffProgramName string,
	language ssaconfig.Language,
	opts ...ssaconfig.Option,
) (Programs, error) {
	// 设置增量编译选项
	incrementalOpts := []ssaconfig.Option{
		WithFileSystem(newFS),
		WithBaseFileSystem(baseFS),
		WithBaseProgramName(baseProgramName),
		WithProgramName(diffProgramName),
		WithLanguage(language),
	}
	// 合并用户提供的选项
	incrementalOpts = append(incrementalOpts, opts...)

	// 调用 ParseProject，它会检测增量编译配置并执行增量编译
	return ParseProject(incrementalOpts...)
}

func PeepholeCompile(fs fi.FileSystem, size int, opts ...ssaconfig.Option) (Programs, error) {
	opts = append(opts, WithFileSystem(fs), WithPeepholeSize(size))
	return ParseProject(opts...)
}

// CompileDiffProgramAndSaveToDB 编译差量程序并保存到数据库
// 注意：这个函数不应该通过选项触发增量编译，因为它已经在增量编译的上下文中被调用了
// 它只负责编译差量文件系统，并在编译后手动设置增量编译元数据
// baseFS: 基础文件系统（全量编译）
// newFS: 新文件系统（用于计算差量）
// baseProgramName: 基础程序的名称（用于设置 BaseProgramName）
// diffProgramName: 差量程序的名称
// language: 编译语言
// 返回：编译后的差量程序（BaseProgramName 和 FileHashMap 已设置）
func CompileDiffProgramAndSaveToDB(
	ctx context.Context,
	baseFS, newFS fi.FileSystem,
	baseProgramName, diffProgramName string,
	language ssaconfig.Language,
	opts ...ssaconfig.Option,
) (*Program, error) {
	// Step 1: 计算文件系统差异
	diffFS, fileHashMap, err := calculateFileSystemDiff(ctx, baseFS, newFS)
	if err != nil {
		return nil, utils.Wrap(err, "failed to calculate file system diff")
	}

	// Step 2: 准备编译选项
	// 设置 WithBaseProgramName 以便在 parseProjectWithFS 中自动设置到 Program.BaseProgramName
	// 但是显式禁用增量编译检测，避免触发增量编译逻辑（因为我们已经在这个上下文中了）
	diffOpts := []ssaconfig.Option{WithLanguage(language)}
	if diffProgramName != "" {
		diffOpts = append(diffOpts, WithProgramName(diffProgramName))
	}
	if baseProgramName != "" {
		diffOpts = append(diffOpts, WithBaseProgramName(baseProgramName))
		// 显式禁用增量编译检测，避免循环调用
		// 因为 CompileDiffProgramAndSaveToDB 已经在增量编译的上下文中被调用了
		diffOpts = append(diffOpts, WithEnableIncrementalCompile(false))
	}
	if baseFS != nil {
		diffOpts = append(diffOpts, WithBaseFileSystem(baseFS))
	}
	if len(fileHashMap) > 0 {
		diffOpts = append(diffOpts, WithFileHashMap(fileHashMap))
	}
	// 合并用户提供的选项
	diffOpts = append(diffOpts, opts...)

	// Step 3: 编译差量文件系统（只包含变更的文件：新增+修改）
	// 这里调用 ParseProjectWithFS 不会触发增量编译，因为我们显式设置了 WithEnableIncrementalCompile(false)
	diffPrograms, err := ParseProjectWithFS(diffFS, diffOpts...)
	if err != nil {
		return nil, utils.Wrap(err, "failed to compile diff file system")
	}
	if len(diffPrograms) == 0 {
		return nil, utils.Errorf("diff file system compilation produced no programs")
	}
	diffProgram := diffPrograms[0]

	// Step 4: 确保增量编译元数据已正确设置
	// WithBaseProgramName 和 WithFileHashMap 已经通过选项设置到 config 中，
	// 并在 parseProjectWithFS 中自动设置到 Program.BaseProgramName 和 Program.FileHashMap
	// 这里只需要确保一致性
	if diffProgram.Program != nil {
		if baseProgramName != "" && diffProgram.Program.BaseProgramName == "" {
			diffProgram.Program.BaseProgramName = baseProgramName
		}
		if len(fileHashMap) > 0 && len(diffProgram.Program.FileHashMap) == 0 {
			diffProgram.Program.FileHashMap = fileHashMap
		}
	}

	return diffProgram, nil
}

func ParseProject(opts ...ssaconfig.Option) (prog Programs, err error) {
	config, err := DefaultConfig(opts...)
	if err != nil {
		return nil, err
	}
	if config.DiagnosticsEnabled() {
		defer config.LogDiagnostics("ssa.compile")
	}
	f1 := func() error {
		prog, err = config.parseProject()
		return nil
	}
	config.DiagnosticsTrack("ssaapi.ParseProject", f1)
	return
}

func (c *Config) parseProject() (progs Programs, err error) {
	// 添加defer清理逻辑，确保编译失败或panic时清理已保存的数据
	programName := c.GetProgramName()
	defer func() {
		// 无论成功还是失败，都要清理临时资源（如 Git 克隆的临时目录）
		c.Cleanup()

		if r := recover(); r != nil {
			err = utils.Errorf("compile panic: %v", r)
			log.Errorf("compile panic: %v", r)
			utils.PrintCurrentGoroutineRuntimeStack()
			// panic时清理已保存的Program数据
			if programName != "" {
				log.Infof("cleaning up program data due to panic: %s", programName)
				ssadb.DeleteProgram(ssadb.GetDB(), programName)
			}
		} else if err != nil {
			// 编译出错时清理已保存的Program数据
			if programName != "" {
				log.Infof("cleaning up program data due to error: %s", programName)
				ssadb.DeleteProgram(ssadb.GetDB(), programName)
			}
		}
	}()

	// 检查是否启用增量编译
	// 如果 isIncremental 为 true，表示启用了增量编译
	// 如果 baseProgramName 不为空，表示这是差量编译（基于已有程序）
	// 如果 baseProgramName 为空，表示这是第一次增量编译（base program，全量编译但设置 IsOverlay = true）
	isIncrementalCompile := c.isIncremental && c.fs != nil
	isDiffCompile := isIncrementalCompile && c.baseProgramName != ""

	if c.GetCompileReCompile() {
		// 非增量编译的项目会先删除原 program，然后重新编译
		// 增量编译的项目不应该删除原 program，因为需要保留 base program
		if !isIncrementalCompile {
			c.Processf(0, "recompile project, delete old data...")
			ssadb.DeleteProgramIrCode(ssadb.GetDB(), programName)
			ProgramCache.Remove(programName)
			c.Processf(0, "recompile project, delete old data finish")
		} else {
			c.Processf(0, "recompile incremental project, keep base program...")
			// 只清理缓存，不删除数据库中的 program
			ProgramCache.Remove(programName)
		}
	}

	c.Processf(0, "recompile project, start compile")

	if isIncrementalCompile {
		if isDiffCompile {
			c.Processf(0.1, "incremental compile detected, base program: %s", c.baseProgramName)
			// 差量编译：编译差量程序并创建 overlay
			return c.parseProjectWithIncrementalCompile()
		} else {
			c.Processf(0.1, "first incremental compile (base program), performing full compilation")
			// 第一次增量编译：全量编译但设置 IsOverlay = true
			return c.parseProjectWithFirstIncrementalCompile()
		}
	}

	if c.GetCompilePeepholeSize() != 0 {
		// peephole compile
		if progs, err = c.peephole(); err != nil {
			return nil, err
		} else {
			SaveConfig(c, nil)
			c.Processf(1, "programs finish")
			return progs, nil
		}
	} else {
		// normal compile
		if prog, err := c.parseProjectWithFS(c.fs, func(f float64, s string, a ...any) {
			c.Processf(f*0.99, s, a...)
		}); err != nil {
			return nil, err
		} else {
			SaveConfig(c, prog)
			c.Processf(1, "program %s finish", prog.GetProgramName())
			return Programs{prog}, nil
		}
	}
}

func (c *Config) peephole() (Programs, error) {

	originFs := c.fs
	if originFs == nil {
		return nil, utils.Errorf("need set filesystem")
	}

	progs := make(Programs, 0)
	var errs error

	filesys.Peephole(originFs,
		filesys.WithPeepholeSize(c.GetCompilePeepholeSize()),
		filesys.WithPeepholeContext(c.ctx),
		filesys.WithPeepholeCallback(func(count, totalCount int, system filesys_interface.FileSystem) {
			totalCount = totalCount + 1
			baseProcess := float64(count-1) / float64(totalCount)
			prog, err := c.parseProjectWithFS(system, func(f float64, s string, a ...any) {
				c.Processf(baseProcess+f/float64(totalCount), s, a)
			})
			process := float64(count) / float64(totalCount) // max is 99%
			c.Processf(process, "finish peephole filesystem")
			// if no err just append and return
			if err == nil {
				progs = append(progs, prog)
				return
			}

			// check error
			if errors.Is(err, ErrNoFoundCompiledFile) {
				return
			}
			errs = utils.JoinErrors(errs, err)
		}),
	)
	return progs, errs
}

// parseProjectWithIncrementalCompile 执行增量编译：编译差量程序并创建 ProgramOverLay
func (c *Config) parseProjectWithIncrementalCompile() (Programs, error) {
	// Step 1: 从数据库加载基础程序
	c.Processf(0.1, "loading base program from database: %s", c.baseProgramName)
	baseProgram, err := FromDatabase(c.baseProgramName)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to load base program from database: %s", c.baseProgramName)
	}
	c.Processf(0.2, "base program loaded: %s", baseProgram.GetProgramName())

	// Step 2: 处理 base program 的情况
	// 问题2：当一个差量的 program 要继续进行增量编译时，需要先将这个差量 program 聚合生成 ProgramOverLay
	var baseOverlay *ProgramOverLay
	var baseFSForDiff fi.FileSystem

	// 检查 base program 是否本身就是一个 overlay（已保存的 overlay）
	baseOverlay = baseProgram.GetOverlay()
	if baseOverlay != nil && len(baseOverlay.Layers) > 0 {
		// base program 是一个已保存的 overlay，直接使用
		baseFSForDiff = baseOverlay.GetAggregatedFileSystem()
		if baseFSForDiff == nil {
			return nil, utils.Errorf("base overlay has no aggregated file system")
		}
		c.Processf(0.3, "base program is an overlay with %d layers", len(baseOverlay.Layers))
	} else if baseProgram.IsIncrementalCompile() && !baseProgram.IsBaseProgram() {
		// base program 是一个差量 program（增量编译但不是 base program），需要先聚合生成 ProgramOverLay
		// 从数据库加载 base program 的 base program
		baseProgramName := baseProgram.GetBaseProgramName()
		baseBaseProgram, err := FromDatabase(baseProgramName)
		if err != nil {
			return nil, utils.Wrapf(err, "failed to load base program's base program: %s", baseProgramName)
		}
		// 创建 overlay：baseBaseProgram 作为 Layer1，baseProgram 作为 Layer2
		baseOverlay = NewProgramOverLay(baseBaseProgram, baseProgram)
		if baseOverlay == nil {
			return nil, utils.Errorf("failed to create overlay for diff base program")
		}
		baseFSForDiff = baseOverlay.GetAggregatedFileSystem()
		if baseFSForDiff == nil {
			return nil, utils.Errorf("base overlay has no aggregated file system")
		}
		c.Processf(0.3, "base program is a diff program, created overlay with 2 layers")
	} else {
		// base program 是全量编译的 program
		// 如果提供了 baseFS，直接使用；否则从基础程序的配置重新构建文件系统
		if c.baseFS != nil {
			baseFSForDiff = c.baseFS
			c.Processf(0.3, "base program is a full compilation program, using provided baseFS")
		} else {
			// 从基础程序的配置重新构建文件系统
			baseConfig := baseProgram.GetConfig()
			if baseConfig == nil {
				// 尝试从数据库中的配置重建文件系统
				if baseProgram.irProgram != nil && baseProgram.irProgram.ConfigInput != "" {
					baseConfigRaw, err := ssaconfig.New(ssaconfig.ModeAll, ssaconfig.WithConfigJson(baseProgram.irProgram.ConfigInput))
					if err != nil {
						return nil, utils.Wrapf(err, "failed to rebuild config from base program: %s", c.baseProgramName)
					}
					// 将 ssaconfig.Config 转换为 ssaapi.Config
					baseConfig = &Config{Config: baseConfigRaw}
				} else {
					return nil, utils.Errorf("base program %s has no config to rebuild file system", c.baseProgramName)
				}
			}
			// 使用基础程序的配置重新构建文件系统
			var err error
			baseFSForDiff, err = baseConfig.parseFSFromInfo()
			if err != nil {
				return nil, utils.Wrapf(err, "failed to rebuild file system from base program config: %s", c.baseProgramName)
			}
			if baseFSForDiff == nil {
				return nil, utils.Errorf("failed to rebuild file system from base program: %s", c.baseProgramName)
			}
			c.Processf(0.3, "base program is a full compilation program, rebuilt file system from config")
		}
	}

	// Step 3: 编译差量程序并保存到数据库
	c.Processf(0.4, "compiling diff program...")
	diffProgram, err := CompileDiffProgramAndSaveToDB(
		c.ctx,
		baseFSForDiff,      // 基础文件系统（可能是 overlay 的聚合文件系统）
		c.fs,               // 新文件系统
		c.baseProgramName,  // 基础程序名称
		c.GetProgramName(), // 差量程序名称
		c.GetLanguage(),    // 语言
	)
	if err != nil {
		return nil, utils.Wrap(err, "failed to compile diff program")
	}
	c.Processf(0.7, "diff program compiled: %s", diffProgram.GetProgramName())

	// Step 4: 创建 ProgramOverLay
	c.Processf(0.8, "creating program overlay...")
	var overlay *ProgramOverLay

	if baseOverlay != nil && len(baseOverlay.Layers) > 0 {
		// base program 是一个 overlay，需要合并所有 layers
		// 收集所有 base overlay 的 layers
		allLayers := make([]*Program, 0, len(baseOverlay.Layers)+1)
		for _, layer := range baseOverlay.Layers {
			if layer != nil && layer.Program != nil {
				allLayers = append(allLayers, layer.Program)
			}
		}
		// 添加新的 diff program 作为最上层 layer
		allLayers = append(allLayers, diffProgram)

		// 创建包含所有 layers 的新 overlay
		overlay = NewProgramOverLay(allLayers...)
	} else {
		// base program 不是 overlay，直接创建包含 base 和 diff 的 overlay
		overlay = NewProgramOverLay(baseProgram, diffProgram)
	}

	if overlay == nil {
		return nil, utils.Errorf("failed to create program overlay")
	}

	// Step 4: 将 overlay 存储到 diffProgram 中，供规则扫描使用
	diffProgram.overlay = overlay

	// Step 5: 确保 diffProgram 本身已保存到数据库（在保存 overlay 之前）
	c.Processf(0.94, "saving diff program to database...")
	if diffProgram.Program != nil {
		wait := diffProgram.Program.UpdateToDatabase()
		if wait != nil {
			wait() // 等待保存完成
		}
	}
	c.Processf(0.95, "diff program saved to database")

	// Step 6: 保存所有 layer 的 program 到数据库
	c.Processf(0.96, "saving all layer programs to database...")
	if err := saveOverlayToDatabase(overlay, diffProgram); err != nil {
		return nil, utils.Wrap(err, "failed to save overlay to database")
	}
	c.Processf(0.98, "all layer programs saved to database")

	// Step 7: 更新缓存（在设置 overlay 后）
	SetProgramCache(diffProgram)

	// Step 8: 保存配置
	SaveConfig(c, diffProgram)
	c.Processf(1, "incremental compile finish, overlay created and saved")

	return Programs{diffProgram}, nil
}

// parseProjectWithFirstIncrementalCompile 处理第一次增量编译（base program）
// 这种情况下进行全量编译，但设置 IsOverlay = true，表示这是增量编译流程的一部分
func (c *Config) parseProjectWithFirstIncrementalCompile() (Programs, error) {
	c.Processf(0.2, "first incremental compile (base program), performing full compilation")

	// 进行全量编译
	if prog, err := c.parseProjectWithFS(c.fs, func(f float64, s string, a ...any) {
		c.Processf(0.2+f*0.7, s, a...)
	}); err != nil {
		return nil, err
	} else {
		// 确保 program 已保存到数据库
		if prog.Program != nil {
			wait := prog.Program.UpdateToDatabase()
			if wait != nil {
				wait()
			}
		}

		// 确保 irProgram 存在
		irProgram := prog.Program.GetIrProgram()
		if irProgram != nil {
			// 第一次增量编译：设置 IsOverlay = true，因为它是增量编译流程的一部分
			// OverlayLayers 只包含它自己的名称，表示它是 overlay 的第一个层
			programName := prog.GetProgramName()
			irProgram.IsOverlay = true
			if programName != "" {
				irProgram.OverlayLayers = []string{programName}
			} else {
				irProgram.OverlayLayers = nil
			}
			ssadb.UpdateProgram(irProgram)
			// 更新 prog.irProgram 字段，确保 IsIncrementalCompile() 能正确工作
			prog.irProgram = irProgram
		}

		SaveConfig(c, prog)
		c.Processf(1, "first incremental compile (base program) finish")
		return Programs{prog}, nil
	}
}

// saveOverlayToDatabase 保存 overlay 及其所有 layer 到数据库
func saveOverlayToDatabase(overlay *ProgramOverLay, diffProgram *Program) error {
	if overlay == nil || len(overlay.Layers) == 0 {
		return utils.Errorf("overlay is nil or has no layers")
	}

	// Step 1: 确保所有 layer 的 program 都已保存到数据库
	layerNames := make([]string, 0, len(overlay.Layers))
	for _, layer := range overlay.Layers {
		if layer == nil || layer.Program == nil {
			continue
		}
		layerProg := layer.Program
		layerName := layerProg.GetProgramName()
		if layerName == "" {
			continue
		}

		// 确保 layer program 已保存到数据库
		if layerProg.Program != nil {
			wait := layerProg.Program.UpdateToDatabase()
			if wait != nil {
				wait() // 等待保存完成
			}
		}
		layerNames = append(layerNames, layerName)
	}

	if len(layerNames) == 0 {
		return utils.Errorf("no valid layer programs found")
	}

	// Step 2: 保存 overlay 的 metadata 到 diffProgram 的 irProgram
	if diffProgram.Program == nil {
		return utils.Errorf("diffProgram.Program is nil")
	}

	// 确保 diffProgram 的 irProgram 存在
	if diffProgram.Program.GetIrProgram() == nil {
		// 如果 irProgram 不存在，需要先创建
		diffProgram.Program.UpdateToDatabase()
		// 等待保存完成
		if wait := diffProgram.Program.UpdateToDatabase(); wait != nil {
			wait()
		}
	}

	irProgram := diffProgram.Program.GetIrProgram()
	if irProgram == nil {
		return utils.Errorf("failed to get irProgram for diffProgram")
	}

	// 设置 overlay 信息
	irProgram.IsOverlay = true
	irProgram.OverlayLayers = layerNames

	// 更新数据库
	ssadb.UpdateProgram(irProgram)

	return nil
}
