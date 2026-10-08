package ssaapi

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// ParseProjectFromPath compiles a local directory into SSA programs (alias: ssa.ParseLocalProject).
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

func PeepholeCompile(fs fi.FileSystem, size int, opts ...ssaconfig.Option) (Programs, error) {
	opts = append(opts, WithFileSystem(fs), WithPeepholeSize(size))
	return ParseProject(opts...)
}

// DiffProgressReporter reports diff (0.1-0.3) and compile (0.3-0.8) progress.
type DiffProgressReporter func(progress float64, msg string)

// CompileDiffProgramAndSaveToDB compiles a diff FS and persists metadata.
// Must not re-enable incremental compile here (would recurse into ParseProject).
func CompileDiffProgramAndSaveToDB(
	ctx context.Context,
	baseFS, newFS fi.FileSystem,
	baseProgramName, diffProgramName string,
	language ssaconfig.Language,
	progressReporter DiffProgressReporter,
	opts ...ssaconfig.Option,
) (*Program, error) {
	var err error

	if baseFS == nil {
		baseFS, err = buildFileSystemFromProgramName(baseProgramName)
		if err != nil {
			return nil, utils.Wrapf(err, "failed to build file system from base program name: %s", baseProgramName)
		}
	}

	var projectID uint64
	if baseProgramName != "" {
		baseIrProgram, err := ssadb.GetProgram(baseProgramName, ssadb.Application)
		if err == nil && baseIrProgram != nil && baseIrProgram.ProjectID > 0 {
			projectID = baseIrProgram.ProjectID
		}
	}

	if progressReporter != nil {
		progressReporter(0.1, "calculating file system diff...")
	}
	diffFS, fileHashMap, err := calculateFileSystemDiff(baseFS, newFS)
	if err != nil {
		return nil, utils.Wrap(err, "failed to calculate file system diff")
	}
	if progressReporter != nil {
		progressReporter(0.3, "file system diff calculated")
	}

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
		diffOpts = append(diffOpts, WithBaseProgramName(baseProgramName))
		diffOpts = append(diffOpts, WithEnableIncrementalCompile(false))
	}
	if len(fileHashMap) > 0 {
		diffOpts = append(diffOpts, WithFileHashMap(fileHashMap))
	}
	if progressReporter != nil {
		diffOpts = append(diffOpts, WithProcess(func(msg string, p float64) {
			progressReporter(0.3+p*0.5, msg)
		}))
	}
	diffOpts = append(diffOpts, opts...)

	config, err := DefaultConfig(diffOpts...)
	if err != nil {
		return nil, utils.Wrap(err, "failed to create config")
	}

	diffProgram, err := config.parseProjectWithFS(diffFS, func(f float64, s string, a ...any) {
		config.Processf(f, s, a...)
	})
	if err != nil {
		if !errors.Is(err, ErrNoFoundCompiledFile) || !hasDeleteEntries(fileHashMap) {
			return nil, utils.Wrap(err, "failed to compile diff file system")
		}
		diffProgram = createDeleteOnlyProgram(ctx, config.GetLatestProgramName(), projectID)
	}
	if diffProgram == nil {
		return nil, utils.Errorf("diff file system compilation produced no program")
	}

	if diffProgram.Program != nil {
		if baseProgramName != "" {
			diffProgram.Program.BaseProgramName = baseProgramName
		}
		if len(fileHashMap) > 0 {
			diffProgram.Program.FileHashMap = fileHashMap
		}
	}

	config.SetEnableIncrementalCompile(true)
	SaveConfig(config, diffProgram)

	return diffProgram, nil
}

// ParseProject compiles a project from options (local FS, git, etc.).
func ParseProject(opts ...ssaconfig.Option) (prog Programs, err error) {
	config, err := DefaultConfig(opts...)
	if err != nil {
		return nil, err
	}
	return config.parseProject()
}

// compileMode resolves, in one place, whether a config participates in the
// incremental/overlay compile flow. The raw EnableIncrementalCompile /
// BaseProgramName pair is intentionally inconsistent during an internal diff
// compile — CompileDiffProgramAndSaveToDB turns the enable flag off to avoid
// recursing into ParseProject while keeping BaseProgramName so the diff
// program still persists overlay metadata — so downstream code must ask
// compileMode() instead of re-deriving the state from the two flags.
type compileMode int

const (
	// compileModeFull：普通全量编译，不参与任何增量编译序列。
	compileModeFull compileMode = iota
	// compileModeIncrementalBase：增量序列的首个编译。实际执行的是一次全量编译，
	// 只是产物会被标记为后续差量编译的 overlay 基座（IsOverlay + OverlayLayers）。
	compileModeIncrementalBase
	// compileModeIncrementalDiff：基于 base program 的差量编译，只编译变更文件。
	// 内部 diff 编译（CompileDiffProgramAndSaveToDB）也处于此模式：它为了防递归
	// 关闭了 EnableIncrementalCompile，但仍携带 BaseProgramName。
	compileModeIncrementalDiff
)

func (c *Config) compileMode() compileMode {
	if c.GetBaseProgramName() != "" {
		return compileModeIncrementalDiff
	}
	if c.GetEnableIncrementalCompile() {
		return compileModeIncrementalBase
	}
	return compileModeFull
}

// cleanupProgramBeforeCompile clears programName's old IR rows before a full
// compile re-inserts under the same name: the UNIQUE indexes on
// ir_codes/ir_offsets otherwise reject the re-inserted rows (e.g.
// recompiling the same program twice in the risk-disposal inheritance test;
// mirrors the delete-then-insert contract documented in SaveIrOffsetBatch).
// forced (an explicit recompile) always clears; a plain compile only clears
// when the program already exists in the database.
func (c *Config) cleanupProgramBeforeCompile(programName string, forced bool) error {
	stage := "recompile project, delete old data"
	if !forced {
		if programName == "" {
			return nil
		}
		if _, err := ssadb.GetProgram(programName, ssadb.Application); err != nil {
			return nil // program does not exist yet, nothing to clear
		}
		stage = "recompile project, delete old data for existing program"
	}
	c.Processf(0, "%s...", stage)
	if err := ssadb.DeleteProgramIrCode(ssadb.GetDB(), programName); err != nil {
		return utils.Wrap(err, "failed to clear old IR data before compile")
	}
	ProgramCache.Remove(programName)
	c.Processf(0, "%s finish", stage)
	return nil
}

func (c *Config) parseProject() (progs Programs, err error) {
	// Wire up debug/pprof output when debug_dir is set.
	// Keep the shared Postgres SSA IR DB (redirectSSADB=false) so the
	// two-job compile -> scan flow can reuse the compiled program.
	debugCleanup := SetupDebugDir(c.GetDebugDir(), false)
	defer debugCleanup()

	programName := c.GetProgramName()
	// Routing mode: the incremental paths diff against a filesystem, so a
	// config without one degrades to a full compile regardless of the flags.
	mode := c.compileMode()
	if mode != compileModeFull && c.fs == nil {
		mode = compileModeFull
	}

	// Roll back the program this compile produces on failure or panic: a
	// diff compile writes a fresh program (latest name) and must never take
	// the base down with it; everything else rolls back its own name.
	programNameToDelete := programName
	if mode == compileModeIncrementalDiff {
		programNameToDelete = c.GetLatestProgramName()
	}
	defer func() {
		c.Cleanup()

		if r := recover(); r != nil {
			err = utils.Errorf("compile panic: %v", r)
			log.Errorf("compile panic: %v", r)
			utils.PrintCurrentGoroutineRuntimeStack()
			if programNameToDelete != "" {
				log.Infof("cleaning up program data due to panic: %s", programNameToDelete)
				ssadb.DeleteProgram(ssadb.GetDB(), programNameToDelete)
			}
		} else if err != nil {
			if programNameToDelete != "" {
				log.Infof("cleaning up program data due to error: %s", programNameToDelete)
				ssadb.DeleteProgram(ssadb.GetDB(), programNameToDelete)
			}
		}
	}()

	// Old IR rows must go before a full compile re-inserts under the same
	// name; an incremental compile keeps its base program untouched.
	if mode != compileModeFull {
		if c.GetCompileReCompile() {
			c.Processf(0, "recompile incremental project, keep base program...")
		}
	} else if err := c.cleanupProgramBeforeCompile(programName, c.GetCompileReCompile()); err != nil {
		return nil, err
	}

	c.Processf(0, "recompile project, start compile")

	if mode != compileModeFull {
		var prog *Program
		if mode == compileModeIncrementalDiff {
			c.Processf(0.02, "incremental compile detected, base program: %s", c.GetBaseProgramName())
			prog, err = c.parseProjectWithIncrementalCompile()
		} else {
			c.Processf(0.1, "first incremental compile (base program), performing full compilation")
			prog, err = c.parseProjectWithFirstIncrementalCompile()
		}
		if err != nil {
			return nil, err
		}
		SaveConfig(c, prog)
		c.Processf(1, "program %s finish", prog.GetProgramName())
		return Programs{prog}, nil
	}

	if c.GetCompilePeepholeSize() != 0 {
		if c.structScan != nil && c.structScan.wantsScan() {
			c.structScan.skipped = true
			c.structScan.skipReason = "peephole compile"
			log.Warnf("[struct_scan] skipped: %s", c.structScan.skipReason)
		}
		if progs, err = c.peephole(); err != nil {
			return nil, err
		}
		SaveConfig(c, nil)
		c.Processf(1, "programs finish")
		return progs, nil
	}

	prog, err := c.parseProjectWithFS(c.fs, func(f float64, s string, a ...any) {
		c.Processf(f*0.99, s, a...)
	})
	if err != nil {
		return nil, err
	}
	SaveConfig(c, prog)
	c.Processf(1, "program %s finish", prog.GetProgramName())
	return Programs{prog}, nil
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
			if c.isStop() {
				errs = utils.JoinErrors(errs, ErrContextCancel)
				return
			}
			totalCount = totalCount + 1
			baseProcess := float64(count-1) / float64(totalCount)
			prog, err := c.parseProjectWithFS(system, func(f float64, s string, a ...any) {
				c.Processf(baseProcess+f/float64(totalCount), s, a...)
			})
			if err == nil {
				if c.isStop() {
					errs = utils.JoinErrors(errs, ErrContextCancel)
					return
				}
				process := float64(count) / float64(totalCount)
				c.Processf(process, "finish peephole filesystem")
				progs = append(progs, prog)
				return
			}

			if errors.Is(err, ErrNoFoundCompiledFile) {
				return
			}
			errs = utils.JoinErrors(errs, err)
		}),
	)
	if c.isStop() && errs == nil {
		errs = ErrContextCancel
	}
	return progs, errs
}

// removeProgramNamePrefix strips program-name prefix from overlay paths.
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

func cleanOverlayLogicalPath(p string) string {
	p = filepath.ToSlash(strings.TrimSpace(p))
	for strings.HasPrefix(p, "./") {
		p = p[2:]
	}
	return filepath.ToSlash(filepath.Clean(p))
}

func normalizeOverlayFilePath(filePath, programName string) string {
	if filePath == "" {
		return ""
	}
	path := filePath
	if programName != "" {
		path = removeProgramNamePrefix(filePath, programName)
	}
	path = cleanOverlayLogicalPath(strings.TrimPrefix(filepath.ToSlash(path), "/"))
	if path == "" || path == "." {
		return "/"
	}
	return "/" + path
}

func ensureOverlayPathSlash(path string) string {
	if path == "" {
		return ""
	}
	path = cleanOverlayLogicalPath(strings.TrimPrefix(filepath.ToSlash(path), "/"))
	if path == "" || path == "." {
		return "/"
	}
	return "/" + path
}

func overlayAggregatedFSPath(canonical string) string {
	if canonical == "" || canonical == "/" {
		return ""
	}
	p := cleanOverlayLogicalPath(strings.TrimPrefix(filepath.ToSlash(canonical), "/"))
	if p == "" || p == "." {
		return ""
	}
	return p
}

// wrapBaseFSWithoutProgramPrefix rewrites leftover "/{programName}/..." path
// prefixes into program-relative paths on lookup, so the diff engine can walk
// an aggregated base FS with plain relative paths. Aggregated paths are
// already stored without the program name; "." is left unchanged so VirtualFS
// listings still resolve.
func wrapBaseFSWithoutProgramPrefix(fs fi.FileSystem, programName string) fi.FileSystem {
	hooked := filesys.NewHookFS(fs)
	hooked.SetPathHook(func(name string) (string, error) {
		if name == "" || name == "." || name == "/" {
			return name, nil
		}
		cleaned := removeProgramNamePrefix(name, programName)
		if cleaned == "" || cleaned == "/" {
			return name, nil
		}
		return strings.TrimPrefix(cleaned, "/"), nil
	})
	return hooked
}

func buildFileSystemFromProgramName(programName string) (fi.FileSystem, error) {
	irProg, err := ssadb.GetProgram(programName, ssadb.Application)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to get program: %s", programName)
	}

	vfs := filesys.NewVirtualFs()
	fileCount := 0

	if len(irProg.FileList) > 0 {
		for filePath, hash := range irProg.FileList {
			cleanPath := removeProgramNamePrefix(filePath, programName)
			editor, err := ssadb.GetEditorByHash(hash)
			if err != nil {
				log.Warnf("failed to get editor for file %s (hash: %s): %v", filePath, hash, err)
				continue
			}
			addFileToAggregatedFS(vfs, ensureOverlayPathSlash(cleanPath), editor.GetSourceCode())
			fileCount++
		}
		if fileCount > 0 {
			return vfs, nil
		}
	}

	editors, err := ssadb.GetEditorByProgramName(programName)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to get editors for program: %s", programName)
	}

	if len(editors) == 0 {
		return nil, utils.Errorf("program %s has no files in database", programName)
	}

	for _, editor := range editors {
		filePath := editor.GetFilePath()
		if filePath == "" {
			folderPath := editor.GetFolderPath()
			fileName := editor.GetFilename()
			if folderPath != "" && fileName != "" {
				filePath = filepath.Join("/", folderPath, fileName)
			} else if fileName != "" {
				filePath = "/" + fileName
			}
		}
		cleanPath := removeProgramNamePrefix(filePath, programName)
		content := editor.GetSourceCode()
		addFileToAggregatedFS(vfs, ensureOverlayPathSlash(cleanPath), content)
	}

	return vfs, nil
}

var buildFileSystemFromProgramNameForIncremental = buildFileSystemFromProgramName

// resolveBaseOverlay 判断差量编译的 base program 自身是否是增量编译产物
// （"对增量项目做增量编译"的情况），是则返回它的 overlay：
//   - 优先使用从 OverlayLayers 元数据还原的 overlay（FromDatabase 加载时恢复）；
//   - 元数据缺失/损坏且 base 是差量 program 时，回退为
//     "base 的 base + base" 两层重建。
// 返回 nil 表示 base 是普通全量编译 program。
// 约定：返回非 nil 时 Base 与 Diff 必然就绪，可直接用于 extendOverlayWithNewLayer。
func (c *Config) resolveBaseOverlay(baseProgram *Program) (*ProgramOverLay, error) {
	if overlay := baseProgram.GetOverlay(); overlay != nil && overlay.Base != nil && len(overlay.Diff) > 0 {
		c.Processf(0.08, "base program is an overlay with %d layers", overlay.ProgramCount())
		return overlay, nil
	}
	if !baseProgram.IsIncrementalCompile() || baseProgram.IsBaseProgram() {
		return nil, nil
	}
	baseBaseProgram, err := FromDatabase(baseProgram.GetBaseProgramName())
	if err != nil {
		return nil, utils.Wrapf(err, "failed to load base program's base program: %s", baseProgram.GetBaseProgramName())
	}
	overlay := NewProgramOverLay(baseBaseProgram, baseProgram)
	if overlay == nil {
		return nil, utils.Errorf("failed to create overlay for diff base program")
	}
	c.Processf(0.08, "base program is a diff program, created overlay with 2 layers")
	return overlay, nil
}

func (c *Config) parseProjectWithIncrementalCompile() (*Program, error) {
	baseProgramName := c.GetBaseProgramName()
	c.Processf(0.03, "loading base program from database: %s", baseProgramName)
	baseProgram, err := FromDatabase(baseProgramName)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to load base program from database: %s", baseProgramName)
	}
	c.Processf(0.06, "base program loaded: %s", baseProgram.GetProgramName())

	// 情况判定收敛到一个决策点：非 nil = base 自身是增量产物（情况4），nil = 普通全量（情况3）
	baseOverlay, err := c.resolveBaseOverlay(baseProgram)
	if err != nil {
		return nil, err
	}

	// diff 的基准文件系统：情况4 直接取 base overlay 的聚合 FS；情况3 从数据库/配置重建
	var baseFSForDiff fi.FileSystem
	if baseOverlay != nil {
		aggregatedFS := baseOverlay.GetAggregatedFileSystem()
		if aggregatedFS == nil {
			return nil, utils.Errorf("base overlay has no aggregated file system")
		}
		baseFSForDiff = wrapBaseFSWithoutProgramPrefix(aggregatedFS, baseProgramName)
	} else {
		baseFSForDiff, err = buildFileSystemFromProgramNameForIncremental(baseProgramName)
		if err == nil && baseFSForDiff != nil {
			c.Processf(0.08, "base program is a full compilation program, rebuilt file system from program name")
		} else {
			baseFSForDiff, cleanupBaseConfig, err := rebuildBaseFileSystemFromConfig(baseProgram)
			if err != nil {
				return nil, utils.Wrapf(err, "failed to rebuild config from base program: %s", baseProgramName)
			}
			defer cleanupBaseConfig()
			if baseFSForDiff == nil {
				return nil, utils.Errorf("failed to rebuild file system from base program: %s", baseProgramName)
			}
			c.Processf(0.08, "base program is a full compilation program, rebuilt file system from config")
		}
	}

	diffProgram, err := CompileDiffProgramAndSaveToDB(
		c.ctx,
		baseFSForDiff, c.fs,
		baseProgramName,
		c.GetLatestProgramName(),
		c.GetLanguage(),
		func(p float64, msg string) { c.Processf(p, "%s", msg) },
	)
	if err != nil {
		return nil, utils.Wrap(err, "failed to compile diff program")
	}
	c.Processf(0.8, "diff program compiled: %s", diffProgram.GetProgramName())

	c.Processf(0.85, "creating program overlay...")
	// 情况4：在 base overlay 上追加一层；情况3：与 base 组成两层 overlay
	var overlay *ProgramOverLay
	if baseOverlay != nil {
		overlay = extendOverlayWithNewLayer(baseOverlay, diffProgram)
	} else {
		overlay = NewProgramOverLay(baseProgram, diffProgram)
	}

	if overlay == nil {
		return nil, utils.Errorf("failed to create program overlay")
	}

	diffProgram.overlay = overlay

	c.Processf(0.9, "saving diff program to database...")
	if diffProgram.Program != nil {
		wait := diffProgram.Program.UpdateToDatabase()
		if wait != nil {
			wait()
		}
	}
	c.Processf(0.92, "diff program saved to database")

	c.Processf(0.96, "saving overlay metadata to database...")
	if err := saveOverlayToDatabase(overlay, diffProgram); err != nil {
		return nil, utils.Wrap(err, "failed to save overlay to database")
	}
	c.Processf(0.98, "overlay metadata saved to database")

	SetProgramCache(diffProgram)

	SaveConfig(c, diffProgram)
	c.Processf(1, "incremental compile finish, overlay created and saved")

	return diffProgram, nil
}

func rebuildBaseFileSystemFromConfig(baseProgram *Program) (fi.FileSystem, func(), error) {
	baseConfig, err := independentBaseProgramConfig(baseProgram)
	if err != nil {
		return nil, nil, err
	}
	cleanupOwned := true
	defer func() {
		if cleanupOwned {
			baseConfig.Cleanup()
		}
	}()
	// parseFSFromInfo can clone a Git-backed base program and registers
	// ownership on baseConfig. Always use a private JSON reconstruction here:
	// FromDatabase may return a ProgramCache entry shared by concurrent diff
	// compiles, and registering cleanup on that cached Config would race and
	// allow one compile to remove another compile's workspace.
	baseFS, err := baseConfig.parseFSFromInfo()
	if err != nil {
		return nil, nil, err
	}
	cleanupOwned = false
	return baseFS, baseConfig.Cleanup, nil
}

func independentBaseProgramConfig(baseProgram *Program) (*Config, error) {
	if baseProgram == nil {
		return nil, utils.Error("base program is nil")
	}

	configInput := ""
	if baseProgram.irProgram != nil {
		configInput = baseProgram.irProgram.ConfigInput
	}
	if configInput == "" {
		cachedConfig := baseProgram.GetConfig()
		if cachedConfig != nil && cachedConfig.Config != nil {
			configInput = cachedConfig.Config.JSON()
		}
	}
	if configInput == "" {
		return nil, utils.Error("base program has no persisted config")
	}

	config, err := ssaconfig.New(ssaconfig.ModeAll, ssaconfig.WithConfigJson(configInput))
	if err != nil {
		return nil, err
	}
	return &Config{Config: config}, nil
}

// parseProjectWithFirstIncrementalCompile full-compiles the first base layer and marks IsOverlay.
func (c *Config) parseProjectWithFirstIncrementalCompile() (*Program, error) {
	c.Processf(0.2, "first incremental compile (base program), performing full compilation")

	prog, err := c.parseProjectWithFS(c.fs, func(f float64, s string, a ...any) {
		c.Processf(0.2+f*0.7, s, a...)
	})
	if err != nil {
		return nil, err
	}

	if prog.Program != nil {
		wait := prog.Program.UpdateToDatabase()
		if wait != nil {
			wait()
		}
	}

	irProgram := prog.Program.GetIrProgram()
	if irProgram != nil {
		programName := prog.GetProgramName()
		irProgram.IsOverlay = true
		if programName != "" {
			irProgram.OverlayLayers = []string{programName}
		} else {
			irProgram.OverlayLayers = nil
		}
		if err := ssadb.UpdateProgramWithError(irProgram); err != nil {
			log.Errorf("update incremental base program overlay failed: name=%s err=%v", irProgram.ProgramName, err)
		}
		prog.irProgram = irProgram
	}

	SaveConfig(c, prog)
	c.Processf(1, "first incremental compile (base program) finish")
	return prog, nil
}

func saveOverlayToDatabase(overlay *ProgramOverLay, diffProgram *Program) error {
	if overlay == nil || overlay.Base == nil {
		return utils.Errorf("overlay is nil or has no base")
	}

	layerNames := overlay.ProgramNames()
	if len(layerNames) == 0 {
		return utils.Errorf("no valid layer programs found")
	}

	if diffProgram.Program == nil {
		return utils.Errorf("diffProgram.Program is nil")
	}

	irProgram := diffProgram.Program.GetIrProgram()
	if irProgram == nil {
		return utils.Errorf("diffProgram irProgram is nil, please save diffProgram first")
	}

	irProgram.IsOverlay = true
	irProgram.OverlayLayers = layerNames

	if err := ssadb.UpdateProgramWithError(irProgram); err != nil {
		log.Errorf("save overlay metadata failed: name=%s err=%v", irProgram.ProgramName, err)
	}

	return nil
}

// calculateFileSystemDiff builds the incremental compile inputs:
//   - diffFS: only added/modified file contents (what gets compiled)
//   - fileHashMap: -1=deleted, 0=modified, 1=added (drives overlay File/ExcludeFile)
//
// Walks newFS once against a base snapshot; unchanged files are never copied.
func calculateFileSystemDiff(baseFS, newFS fi.FileSystem) (*filesys.VirtualFS, map[string]int, error) {
	diffFS := filesys.NewVirtualFs()
	fileHashMap := make(map[string]int)

	baseFiles := make(map[string][]byte)
	err := filesys.Recursive(".", filesys.WithFileSystem(baseFS), filesys.WithFileStat(func(pathname string, info os.FileInfo) error {
		if pathname == "" {
			return nil
		}
		content, err := baseFS.ReadFile(pathname)
		if err != nil {
			return nil
		}
		baseFiles[pathname] = content
		return nil
	}))
	if err != nil {
		return nil, nil, utils.Wrap(err, "failed to collect baseFS files")
	}

	err = filesys.Recursive(".", filesys.WithFileSystem(newFS), filesys.WithFileStat(func(pathname string, info os.FileInfo) error {
		if pathname == "" {
			return nil
		}
		content, err := newFS.ReadFile(pathname)
		if err != nil {
			return nil
		}
		baseContent, existsInBase := baseFiles[pathname]
		delete(baseFiles, pathname)
		if !existsInBase {
			fileHashMap[pathname] = 1
			diffFS.AddFile(pathname, string(content))
			return nil
		}
		if !bytes.Equal(baseContent, content) {
			fileHashMap[pathname] = 0
			diffFS.AddFile(pathname, string(content))
		}
		return nil
	}))
	if err != nil {
		return nil, nil, utils.Wrap(err, "failed to walk newFS files")
	}

	for filePath := range baseFiles {
		fileHashMap[filePath] = -1
	}
	return diffFS, fileHashMap, nil
}

func hasDeleteEntries(fileHashMap map[string]int) bool {
	for _, v := range fileHashMap {
		if v == -1 {
			return true
		}
	}
	return false
}

func createDeleteOnlyProgram(ctx context.Context, programName string, projectID uint64) *Program {
	irProg, err := ssadb.CreateProgramWithError(programName, "", ssadb.Application)
	if err != nil {
		log.Errorf("create delete-only program failed: name=%s err=%v", programName, err)
		irProg = &ssadb.IrProgram{
			ProgramName: programName,
			ProgramKind: ssadb.Application,
		}
	}
	if projectID > 0 {
		irProg.ProjectID = projectID
		if irProg.ID > 0 {
			if err := ssadb.UpdateProgramWithError(irProg); err != nil {
				log.Errorf("update delete-only program project id failed: name=%s err=%v", irProg.ProgramName, err)
			}
		}
	}
	cfg, err := ssaconfig.New(
		ssaconfig.ModeSSACompile,
		ssaconfig.WithContext(ctx),
		ssaconfig.WithSetProgramName(programName),
	)
	if err != nil {
		log.Warnf("create delete-only program config failed: %v", err)
		cfg = nil
	}
	ssaProg := ssa.NewProgram(
		cfg, ssa.ProgramCacheDBWrite, ssadb.Application,
		filesys.NewVirtualFs(), "", 0,
	)
	return &Program{
		Program:   ssaProg,
		irProgram: irProg,
	}
}
