package reactloops_yak

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// 此文件提供两类 yak 专注模式加载器：
//   - LoadAllFromEmbed：扫描 embed.FS（默认入口，触发内置 hello_yak 等示例注册）
//   - LoadAllFromDir  ：扫描磁盘目录（CLI 与开发期热加载用）
//
// 关键词: yak focus mode loader, scan ai-focus.yak, embed loader, dir loader

const (
	// FocusModeFileSuffix 与 reactloops 包中的 FocusModeFileSuffix 一致，
	// 这里再次声明便于纯 reactloops_yak 使用者无需引用上层包。
	FocusModeFileSuffix    = reactloops.FocusModeFileSuffix
	FocusModeYakFileSuffix = reactloops.FocusModeYakFileSuffix
)

var (
	loadAllOnce sync.Once
	loadAllErr  error
)

// LoadAllFromEmbed 把 reactloops_yak/focus_modes/ 下所有 *.ai-focus.yak
// 主入口扫描出来，配合同级 *.yak 作为 sidekick 拼装为 bundle，注册到
// reactloops 全局工厂表。多次调用幂等。
//
// 关键词: load all yak focus modes from embed, builtin registration
func LoadAllFromEmbed() error {
	loadAllOnce.Do(func() {
		loadAllErr = loadAllFromEmbed()
	})
	return loadAllErr
}

func loadAllFromEmbed() error {
	root := "focus_modes"
	entries, err := iterateFocusEntriesFS(focusModesFS, root)
	if err != nil {
		return utils.Wrapf(err, "yak focus loader: walk embed fs failed")
	}
	for _, entry := range entries {
		bundle, err := readFocusBundleFromFS(focusModesFS, entry.dir, entry.entryFile)
		if err != nil {
			log.Errorf("yak focus loader: read embed bundle %s failed: %v", entry.entryFile, err)
			continue
		}
		if err := reactloops.RegisterYakFocusModeFromBundle(bundle); err != nil {
			log.Errorf("yak focus loader: register %s failed: %v", bundle.Name, err)
			continue
		}
	}
	return nil
}

// LoadAllFromDir 扫描磁盘目录下所有子目录中的 *.ai-focus.yak，
// 同级 *.yak 作为 sidekick 一并加载。CLI 与开发期使用。
// 关键词: load yak focus modes from disk, dir scan
func LoadAllFromDir(dir string) error {
	if dir == "" {
		return utils.Error("yak focus loader: empty dir")
	}
	entries, err := iterateFocusEntriesDir(dir)
	if err != nil {
		return utils.Wrapf(err, "yak focus loader: walk dir %s failed", dir)
	}
	var firstErr error
	for _, entry := range entries {
		bundle, err := readFocusBundleFromDir(entry.dir, entry.entryFile)
		if err != nil {
			log.Errorf("yak focus loader: read disk bundle %s failed: %v", entry.entryFile, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := reactloops.RegisterYakFocusModeFromBundle(bundle); err != nil {
			log.Errorf("yak focus loader: register %s failed: %v", bundle.Name, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
	}
	return firstErr
}

// LoadSingleFile 加载磁盘上的单个 *.ai-focus.yak，自动收集同级 sidekick。
// 主要给 `yak ai-focus --file` CLI 直接使用。
// 关键词: load single yak focus file, with sidekick scan
func LoadSingleFile(path string) (*reactloops.FocusModeBundle, error) {
	if path == "" {
		return nil, utils.Error("yak focus loader: empty file path")
	}
	if !strings.HasSuffix(path, FocusModeFileSuffix) {
		return nil, utils.Errorf("yak focus loader: file must end with %s", FocusModeFileSuffix)
	}
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	return readFocusBundleFromDir(dir, base)
}

// focusEntry 描述一个待加载的专注模式（dir + 主入口文件名）。
type focusEntry struct {
	dir       string // 相对或绝对目录
	entryFile string // 主入口文件名（含 .ai-focus.yak 后缀）
}

func iterateFocusEntriesFS(fsys fs.FS, root string) ([]focusEntry, error) {
	var entries []focusEntry
	err := fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, FocusModeFileSuffix) {
			return nil
		}
		entries = append(entries, focusEntry{
			dir:       filepath.Dir(p),
			entryFile: name,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].dir == entries[j].dir {
			return entries[i].entryFile < entries[j].entryFile
		}
		return entries[i].dir < entries[j].dir
	})
	return entries, nil
}

func iterateFocusEntriesDir(root string) ([]focusEntry, error) {
	var entries []focusEntry
	err := filepath.Walk(root, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		name := info.Name()
		if !strings.HasSuffix(name, FocusModeFileSuffix) {
			return nil
		}
		entries = append(entries, focusEntry{
			dir:       filepath.Dir(p),
			entryFile: name,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].dir == entries[j].dir {
			return entries[i].entryFile < entries[j].entryFile
		}
		return entries[i].dir < entries[j].dir
	})
	return entries, nil
}

func readFocusBundleFromFS(fsys fs.FS, dir, entryFile string) (*reactloops.FocusModeBundle, error) {
	entryPath := pathJoinFS(dir, entryFile)
	mainContent, err := fs.ReadFile(fsys, entryPath)
	if err != nil {
		return nil, utils.Wrapf(err, "read main entry %s", entryPath)
	}

	dirEntries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, utils.Wrapf(err, "read dir %s", dir)
	}

	sidekicks, err := collectSidekicksFS(fsys, dir, dirEntries, entryFile)
	if err != nil {
		return nil, err
	}

	return &reactloops.FocusModeBundle{
		Name:      deriveName(entryFile),
		EntryFile: entryPath,
		EntryCode: string(mainContent),
		Sidekicks: sidekicks,
	}, nil
}

func readFocusBundleFromDir(dir, entryFile string) (*reactloops.FocusModeBundle, error) {
	entryPath := filepath.Join(dir, entryFile)
	mainContent, err := os.ReadFile(entryPath)
	if err != nil {
		return nil, utils.Wrapf(err, "read main entry %s", entryPath)
	}

	osEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil, utils.Wrapf(err, "read dir %s", dir)
	}

	sidekicks, err := collectSidekicksDir(dir, osEntries, entryFile)
	if err != nil {
		return nil, err
	}

	return &reactloops.FocusModeBundle{
		Name:      deriveName(entryFile),
		EntryFile: entryPath,
		EntryCode: string(mainContent),
		Sidekicks: sidekicks,
	}, nil
}

func collectSidekicksFS(fsys fs.FS, dir string, dirEntries []fs.DirEntry, entryFile string) ([]reactloops.FocusModeSidekick, error) {
	var names []string
	for _, e := range dirEntries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !shouldTreatAsSidekick(n, entryFile) {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)
	var sidekicks []reactloops.FocusModeSidekick
	for _, n := range names {
		p := pathJoinFS(dir, n)
		body, err := fs.ReadFile(fsys, p)
		if err != nil {
			return nil, utils.Wrapf(err, "read sidekick %s", p)
		}
		sidekicks = append(sidekicks, reactloops.FocusModeSidekick{
			Path:    p,
			Content: string(body),
		})
	}
	return sidekicks, nil
}

func collectSidekicksDir(dir string, osEntries []os.DirEntry, entryFile string) ([]reactloops.FocusModeSidekick, error) {
	var names []string
	for _, e := range osEntries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !shouldTreatAsSidekick(n, entryFile) {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)
	var sidekicks []reactloops.FocusModeSidekick
	for _, n := range names {
		p := filepath.Join(dir, n)
		body, err := os.ReadFile(p)
		if err != nil {
			return nil, utils.Wrapf(err, "read sidekick %s", p)
		}
		sidekicks = append(sidekicks, reactloops.FocusModeSidekick{
			Path:    p,
			Content: string(body),
		})
	}
	return sidekicks, nil
}

// shouldTreatAsSidekick：仅当文件 .yak 结尾、且不是 .ai-focus.yak 主文件，
// 才被视为 sidekick；同目录其它 *.ai-focus.yak 主文件不能互相吞并。
func shouldTreatAsSidekick(filename, entryFile string) bool {
	if filename == entryFile {
		return false
	}
	if strings.HasSuffix(filename, FocusModeFileSuffix) {
		return false
	}
	if !strings.HasSuffix(filename, FocusModeYakFileSuffix) {
		return false
	}
	return true
}

// deriveName 把 hello_yak.ai-focus.yak → hello_yak。
func deriveName(filename string) string {
	if strings.HasSuffix(filename, FocusModeFileSuffix) {
		return strings.TrimSuffix(filename, FocusModeFileSuffix)
	}
	if strings.HasSuffix(filename, FocusModeYakFileSuffix) {
		return strings.TrimSuffix(filename, FocusModeYakFileSuffix)
	}
	return filename
}

// pathJoinFS：embed.FS 强制使用 / 作为分隔符，即使 windows 上也是。
func pathJoinFS(dir, name string) string {
	if dir == "" || dir == "." {
		return name
	}
	return strings.TrimRight(dir, "/") + "/" + name
}
