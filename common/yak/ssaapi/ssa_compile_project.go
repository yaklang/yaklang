package ssaapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

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
// 如果提供了 baseProgramName，将执行增量编译并返回包含 overlay 的 Programs
// 调用者可以通过 program.GetOverlay() 获取 ProgramOverLay 用于规则扫描
// newFS: 新文件系统（当前要编译的文件系统）
// baseProgramName: 基础程序的名称（必须已存在于数据库中，系统会从数据库构建基础文件系统）
// diffProgramName: 差量程序的名称
// language: 编译语言
// opts: 其他编译选项
// 返回：包含差量程序的 Programs，可以通过 program.GetOverlay() 获取 ProgramOverLay
func ParseProjectWithIncrementalCompile(
	newFS fi.FileSystem,
	baseProgramName, diffProgramName string,
	language ssaconfig.Language,
	opts ...ssaconfig.Option,
) (Programs, error) {
	// 设置增量编译选项
	incrementalOpts := []ssaconfig.Option{
		WithFileSystem(newFS),
		WithBaseProgramName(baseProgramName),
		WithProgramName(diffProgramName),
		WithLanguage(language),
	}
	// 合并用户提供的选项
	incrementalOpts = append(incrementalOpts, opts...)

	// 调用 ParseProject，它会检测增量编译配置并执行增量编译
	// 系统会自动从 baseProgramName 构建基础文件系统
	return ParseProject(incrementalOpts...)
}

func PeepholeCompile(fs fi.FileSystem, size int, opts ...ssaconfig.Option) (Programs, error) {
	opts = append(opts, WithFileSystem(fs), WithPeepholeSize(size))
	return ParseProject(opts...)
}

// CompileDiffProgramAndSaveToDB 编译差量程序并保存到数据库
// 注意：这个函数不应该通过选项触发增量编译，因为它已经在增量编译的上下文中被调用了
// 它只负责编译差量文件系统，并在编译后手动设置增量编译元数据
// newFS: 新文件系统（用于计算差量）
// baseProgramName: 基础程序的名称（用于设置 BaseProgramName，系统会从数据库构建基础文件系统）
// diffProgramName: 差量程序的名称
// language: 编译语言
// 返回：编译后的差量程序（BaseProgramName 和 FileHashMap 已设置）
// 注意：projectID 会自动从 base program 中获取，确保 diff program 和 base program 属于同一个 project
func CompileDiffProgramAndSaveToDB(
	ctx context.Context,
	baseFS, newFS fi.FileSystem,
	baseProgramName, diffProgramName string,
	language ssaconfig.Language,
	opts ...ssaconfig.Option,
) (*Program, error) {
	var err error

	// Step 1: 从 baseProgramName/baseFS 构建基础文件系统
	if baseFS == nil {
		baseFS, err = buildFileSystemFromProgramName(baseProgramName)
		if err != nil {
			return nil, utils.Wrapf(err, "failed to build file system from base program name: %s", baseProgramName)
		}
	}

	// 从 base program 中获取 projectID（diff program 应该和 base program 属于同一个 project）
	var projectID uint64
	if baseProgramName != "" {
		baseIrProgram, err := ssadb.GetProgram(baseProgramName, ssadb.Application)
		if err == nil && baseIrProgram != nil && baseIrProgram.ProjectID > 0 {
			projectID = baseIrProgram.ProjectID
		}
	}

	// Step 2: 计算文件系统差异
	// 传入 diffProgramName，确保 diff 文件系统中的路径包含 program name 前缀
	diffFS, fileHashMap, err := calculateFileSystemDiff(baseFS, newFS)
	if err != nil {
		return nil, utils.Wrap(err, "failed to calculate file system diff")
	}

	// Step 3: 准备编译选项
	// 注意：不要设置 WithEnableIncrementalCompile(true)，否则会触发增量编译检测导致死循环
	// 我们直接使用底层 API parseProjectWithFS 来编译差量文件系统
	diffOpts := []ssaconfig.Option{
		WithLanguage(language),
		WithFileSystem(diffFS),
	}
	if diffProgramName != "" {
		diffOpts = append(diffOpts, WithProgramName(diffProgramName))
	}
	if projectID > 0 {
		diffOpts = append(diffOpts, ssaconfig.WithProjectID(projectID))
	}
	if baseProgramName != "" {
		// 强制禁用增量编译检测，避免死循环
		// 因为 CompileDiffProgramAndSaveToDB 已经在增量编译的上下文中，不应该再次触发增量编译检测
		diffOpts = append(diffOpts, WithBaseProgramName(baseProgramName))
		diffOpts = append(diffOpts, WithEnableIncrementalCompile(false))
	}
	if len(fileHashMap) > 0 {
		diffOpts = append(diffOpts, WithFileHashMap(fileHashMap))
	}
	// 合并用户提供的选项
	diffOpts = append(diffOpts, opts...)

	// Step 3: 创建 Config 并直接调用 parseProjectWithFS（底层 API）
	// 这样可以避免触发增量编译检测，直接编译差量文件系统
	config, err := DefaultConfig(diffOpts...)
	if err != nil {
		return nil, utils.Wrap(err, "failed to create config")
	}

	// 直接调用 parseProjectWithFS，不会触发增量编译检测
	diffProgram, err := config.parseProjectWithFS(diffFS, func(f float64, s string, a ...any) {
		config.Processf(f, s, a...)
	})
	if err != nil {
		return nil, utils.Wrap(err, "failed to compile diff file system")
	}
	if diffProgram == nil {
		return nil, utils.Errorf("diff file system compilation produced no program")
	}

	// Step 4: 手动设置增量编译元数据
	// 因为直接调用 parseProjectWithFS 不会自动设置这些元数据
	if diffProgram.Program != nil {
		if baseProgramName != "" {
			diffProgram.Program.BaseProgramName = baseProgramName
		}
		if len(fileHashMap) > 0 {
			diffProgram.Program.FileHashMap = fileHashMap
		}
	}

	// Step 5: 保存配置（包含增量编译元数据）
	config.SetEnableIncrementalCompile(true)
	SaveConfig(config, diffProgram)

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
	// 对于增量编译，需要删除最新的 layer，而不是第一个 layer
	isIncrementalCompile := c.GetEnableIncrementalCompile() && c.fs != nil
	isDiffCompile := isIncrementalCompile && c.GetBaseProgramName() != ""
	var programNameToDelete string
	if isDiffCompile {
		// 增量编译失败时，删除最新的 layer（diff program）
		programNameToDelete = c.GetLatestProgramName()
	} else {
		// 非增量编译或第一次增量编译，删除当前 program
		programNameToDelete = programName
	}
	defer func() {
		// 无论成功还是失败，都要清理临时资源（如 Git 克隆的临时目录）
		c.Cleanup()

		if r := recover(); r != nil {
			err = utils.Errorf("compile panic: %v", r)
			log.Errorf("compile panic: %v", r)
			utils.PrintCurrentGoroutineRuntimeStack()
			// panic时清理已保存的Program数据
			if programNameToDelete != "" {
				log.Infof("cleaning up program data due to panic: %s", programNameToDelete)
				ssadb.DeleteProgram(ssadb.GetDB(), programNameToDelete)
			}
		} else if err != nil {
			// 编译出错时清理已保存的Program数据
			if programNameToDelete != "" {
				log.Infof("cleaning up program data due to error: %s", programNameToDelete)
				ssadb.DeleteProgram(ssadb.GetDB(), programNameToDelete)
			}
		}
	}()

	// 检查是否启用增量编译
	// 如果 isIncremental 为 true，表示启用了增量编译
	// 如果 baseProgramName 不为空，表示这是差量编译（基于已有程序）
	// 如果 baseProgramName 为空，表示这是第一次增量编译（base program，全量编译但设置 IsOverlay = true）

	if c.GetCompileReCompile() {
		if !isIncrementalCompile {
			c.Processf(0, "recompile project, delete old data...")
			ssadb.DeleteProgramIrCode(ssadb.GetDB(), programName)
			ProgramCache.Remove(programName)
			c.Processf(0, "recompile project, delete old data finish")
		} else {
			c.Processf(0, "recompile incremental project, keep base program...")
		}
	}

	c.Processf(0, "recompile project, start compile")

	if isIncrementalCompile {
		var prog *Program
		if isDiffCompile {
			c.Processf(0.1, "incremental compile detected, base program: %s", c.GetBaseProgramName())
			// 差量编译：编译差量程序并创建 overlay
			prog, err = c.parseProjectWithIncrementalCompile()
		} else {
			c.Processf(0.1, "first incremental compile (base program), performing full compilation")
			// 第一次增量编译：全量编译但设置 IsOverlay = true
			prog, err = c.parseProjectWithFirstIncrementalCompile()
		}
		if err != nil {
			return nil, err
		} else {
			SaveConfig(c, prog)
			c.Processf(1, "program %s finish", prog.GetProgramName())
			return Programs{prog}, nil
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

// removeProgramNamePrefix 去掉文件路径中的 program name 前缀
// 输入格式可能是: /mytest(2026-01-20 11:48:20)/test.go 或 /mytest(2026-01-20 11:48:20)/folder/test.go
// 输出格式: /test.go 或 /folder/test.go
func removeProgramNamePrefix(filePath, programName string) string {
	if filePath == "" || programName == "" {
		return filePath
	}

	path := strings.TrimPrefix(filePath, "/")
	if path == "" {
		return filePath
	}

	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return filePath
	}

	firstPart := parts[0]
	if firstPart == programName {
		if len(parts) > 1 {
			return strings.Join(parts[1:], "/")
		}
		return "/"
	}

	if strings.HasPrefix(firstPart, programName+"(") {
		if len(parts) > 1 {
			return "/" + strings.Join(parts[1:], "/")
		}
		return "/"
	}

	return filePath
}

// removeProgramNamePrefixFromFS 从文件系统中去掉 program name 前缀
// 创建一个新的 VirtualFS，遍历原始文件系统的所有文件，去掉前缀后添加到新文件系统中
func removeProgramNamePrefixFromFS(fs fi.FileSystem, programName string) (fi.FileSystem, error) {
	if fs == nil {
		return nil, utils.Errorf("file system is nil")
	}
	if programName == "" {
		return fs, nil
	}

	vfs := filesys.NewVirtualFs()

	err := filesys.Recursive(".", filesys.WithFileSystem(fs), filesys.WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
		if isDir {
			return nil
		}
		if pathname == "" {
			return nil
		}

		content, err := fs.ReadFile(pathname)
		if err != nil {
			log.Warnf("failed to read file %s: %v", pathname, err)
			return nil
		}

		// 去掉 program name 前缀
		cleanPath := removeProgramNamePrefix(pathname, programName)
		if cleanPath == "" || cleanPath == "/" {
			return nil
		}

		vfs.AddFile(cleanPath, string(content))
		return nil
	}))

	if err != nil {
		return nil, utils.Wrap(err, "failed to traverse file system")
	}

	return vfs, nil
}

func buildFileSystemFromProgramName(programName string) (fi.FileSystem, error) {
	// Step 1: 从数据库获取 program
	irProg, err := ssadb.GetProgram(programName, ssadb.Application)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to get program: %s", programName)
	}

	// Step 2: 构建 VirtualFS
	vfs := filesys.NewVirtualFs()
	fileCount := 0

	// 优先使用 FileList（如果存在且不为空）
	if len(irProg.FileList) > 0 {
		for filePath, hash := range irProg.FileList {
			// 去掉 program name 前缀
			cleanPath := removeProgramNamePrefix(filePath, programName)
			editor, err := ssadb.GetEditorByHash(hash)
			if err != nil {
				log.Warnf("failed to get editor for file %s (hash: %s): %v", filePath, hash, err)
				continue
			}
			vfs.AddFile(cleanPath, editor.GetSourceCode())
			fileCount++
		}
		if fileCount > 0 {
			return vfs, nil
		}
	}

	// 回退：使用 GetEditorByProgramName
	editors, err := ssadb.GetEditorByProgramName(programName)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to get editors for program: %s", programName)
	}

	if len(editors) == 0 {
		return nil, utils.Errorf("program %s has no files in database", programName)
	}

	for _, editor := range editors {
		// 使用 GetFilePath() 获取完整路径（不包含 program name）
		filePath := editor.GetFilePath()
		if filePath == "" {
			// 如果 GetFilePath() 为空，尝试构建路径
			folderPath := editor.GetFolderPath()
			fileName := editor.GetFilename()
			if folderPath != "" && fileName != "" {
				filePath = filepath.Join("/", folderPath, fileName)
			} else if fileName != "" {
				filePath = "/" + fileName
			}
		}
		// 确保去掉 program name 前缀（如果存在）
		cleanPath := removeProgramNamePrefix(filePath, programName)
		content := editor.GetSourceCode()
		vfs.AddFile(cleanPath, content)
	}

	return vfs, nil
}

// parseProjectWithIncrementalCompile 执行增量编译：编译差量程序并创建 ProgramOverLay
func (c *Config) parseProjectWithIncrementalCompile() (*Program, error) {
	// Step 1: 从数据库加载基础程序
	baseProgramName := c.GetBaseProgramName()
	c.Processf(0.1, "loading base program from database: %s", baseProgramName)
	baseProgram, err := FromDatabase(baseProgramName)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to load base program from database: %s", baseProgramName)
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
		aggregatedFS := baseOverlay.GetAggregatedFileSystem()
		if aggregatedFS == nil {
			return nil, utils.Errorf("base overlay has no aggregated file system")
		}
		// 去掉文件系统中的 program name 前缀
		baseFSForDiff, err = removeProgramNamePrefixFromFS(aggregatedFS, baseProgramName)
		if err != nil {
			return nil, utils.Wrapf(err, "failed to remove program name prefix from aggregated file system")
		}
		c.Processf(0.3, "base program is an overlay with %d layers", len(baseOverlay.Layers))
	} else if baseProgram.IsIncrementalCompile() && !baseProgram.IsBaseProgram() {
		// base program 是一个差量 program，需要先聚合生成 ProgramOverLay
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
		aggregatedFS := baseOverlay.GetAggregatedFileSystem()
		if aggregatedFS == nil {
			return nil, utils.Errorf("base overlay has no aggregated file system")
		}
		// 去掉文件系统中的 program name 前缀
		baseFSForDiff, err = removeProgramNamePrefixFromFS(aggregatedFS, baseProgramName)
		if err != nil {
			return nil, utils.Wrapf(err, "failed to remove program name prefix from aggregated file system")
		}
		c.Processf(0.3, "base program is a diff program, created overlay with 2 layers")
	} else {
		// base program 是全量编译的 program
		// 优先从 program name 构建文件系统，如果失败则回退到配置重建
		var err error
		baseFSForDiff, err = buildFileSystemFromProgramName(baseProgramName)
		if err == nil && baseFSForDiff != nil {
			c.Processf(0.3, "base program is a full compilation program, rebuilt file system from program name")
		} else {
			// 回退：从基础程序的配置重新构建文件系统
			baseConfig := baseProgram.GetConfig()
			if baseConfig == nil {
				// 尝试从数据库中的配置重建文件系统
				if baseProgram.irProgram != nil && baseProgram.irProgram.ConfigInput != "" {
					baseConfigRaw, err := ssaconfig.New(ssaconfig.ModeAll, ssaconfig.WithConfigJson(baseProgram.irProgram.ConfigInput))
					if err != nil {
						return nil, utils.Wrapf(err, "failed to rebuild config from base program: %s", baseProgramName)
					}
					// 将 ssaconfig.Config 转换为 ssaapi.Config
					baseConfig = &Config{Config: baseConfigRaw}
				} else {
					return nil, utils.Errorf("base program %s has no config or files to rebuild file system", baseProgramName)
				}
			}
			// 使用基础程序的配置重新构建文件系统
			baseFSForDiff, err = baseConfig.parseFSFromInfo()
			if err != nil {
				return nil, utils.Wrapf(err, "failed to rebuild file system from base program config: %s", baseProgramName)
			}
			if baseFSForDiff == nil {
				return nil, utils.Errorf("failed to rebuild file system from base program: %s", baseProgramName)
			}
			c.Processf(0.3, "base program is a full compilation program, rebuilt file system from config")
		}
	}

	// Step 3: 编译差量程序并保存到数据库
	c.Processf(0.4, "compiling diff program...")
	diffProgram, err := CompileDiffProgramAndSaveToDB(
		c.ctx,
		baseFSForDiff, c.fs, // 新文件系统
		baseProgramName,          // 基础程序名称（系统会从数据库构建基础文件系统）
		c.GetLatestProgramName(), // 差量程序名称
		c.GetLanguage(),          // 语言
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
		// 复用 baseOverlay 的 layer1，避免重新创建（防止更新 layer1 的 updated_at）
		// 创建新的 overlay，复用 baseOverlay 的 layer1，只创建新的 layer（layer3）的 ProgramLayer
		overlay = extendOverlayWithNewLayer(baseOverlay, diffProgram)
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

	// Step 6: 保存 overlay 信息到数据库（只更新当前 program，不更新 layer）
	c.Processf(0.96, "saving overlay metadata to database...")
	if err := saveOverlayToDatabase(overlay, diffProgram); err != nil {
		return nil, utils.Wrap(err, "failed to save overlay to database")
	}
	c.Processf(0.98, "overlay metadata saved to database")

	// Step 7: 更新缓存（在设置 overlay 后）
	SetProgramCache(diffProgram)

	// Step 8: 保存配置
	SaveConfig(c, diffProgram)
	c.Processf(1, "incremental compile finish, overlay created and saved")

	return diffProgram, nil
}

// parseProjectWithFirstIncrementalCompile 处理第一次增量编译（base program）
// 这种情况下进行全量编译，但设置 IsOverlay = true，表示这是增量编译流程的一部分
func (c *Config) parseProjectWithFirstIncrementalCompile() (*Program, error) {
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
		return prog, nil
	}
}

// saveOverlayToDatabase 保存 overlay 信息到数据库（只更新当前 program，不更新 layer）
func saveOverlayToDatabase(overlay *ProgramOverLay, diffProgram *Program) error {
	if overlay == nil || len(overlay.Layers) == 0 {
		return utils.Errorf("overlay is nil or has no layers")
	}

	// Step 1: 收集所有 layer 的 program 名称（不更新 layer，只收集名称）
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
		layerNames = append(layerNames, layerName)
	}

	if len(layerNames) == 0 {
		return utils.Errorf("no valid layer programs found")
	}

	// Step 2: 保存 overlay 的 metadata 到 diffProgram 的 irProgram
	if diffProgram.Program == nil {
		return utils.Errorf("diffProgram.Program is nil")
	}

	// 确保 diffProgram 的 irProgram 存在（Step 5 已经保存过了，这里应该已经存在）
	irProgram := diffProgram.Program.GetIrProgram()
	if irProgram == nil {
		return utils.Errorf("diffProgram irProgram is nil, please save diffProgram first")
	}

	// 设置 overlay 信息
	irProgram.IsOverlay = true
	irProgram.OverlayLayers = layerNames

	// 更新数据库（只更新当前 program 的 overlay 信息，不更新 layer）
	ssadb.UpdateProgram(irProgram)

	return nil
}
