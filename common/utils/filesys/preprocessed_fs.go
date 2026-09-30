package filesys

import (
	"fmt"
	"io/fs"
	"os"
	"strings"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type HookFS struct {
	underlying fi.FileSystem
	enabled    bool

	readHooks []*ReadHook
	// pathHook rewrites a caller path into the underlying filesystem path.
	pathHook MapPathHook
}

// MapPathHook rewrites a path before it is passed to the underlying filesystem.
// Read hooks still observe the caller's original path.
type MapPathHook func(name string) (string, error)

var _ fi.FileSystem = (*HookFS)(nil)

type ReadHook struct {
	Matcher    HookMatcher
	BeforeRead HookBeforeFunc
	AfterRead  HookAfterFunc
}

type HookMatcher interface {
	Match(name string) bool
}

type HookMatcherFunc func(name string) bool

func (f HookMatcherFunc) Match(name string) bool {
	return f(name)
}

type HookBeforeFunc func(ctx *ReadHookContext) error
type HookAfterFunc func(ctx *ReadHookContext, data []byte) ([]byte, error)

type ReadHookContext struct {
	Name       string
	FS         fi.FileSystem
	Underlying fi.FileSystem
}

// NewHookFS 创建 HookFS。
func NewHookFS(underlying fi.FileSystem) *HookFS {
	return &HookFS{
		underlying: underlying,
		enabled:    true,
	}
}

// AddReadHook 注册一个 read hook。
func (f *HookFS) AddReadHook(hook *ReadHook) {
	if hook == nil {
		return
	}
	f.readHooks = append(f.readHooks, hook)
}

// SetEnabled 控制是否启用 hook 能力。
func (f *HookFS) SetEnabled(enabled bool) {
	f.enabled = enabled
}

// SetPathHook 注册路径映射。加前缀、去前缀都走这里，不再单独包一层文件系统。
func (f *HookFS) SetPathHook(hook MapPathHook) {
	f.pathHook = hook
}

func (f *HookFS) mapPath(name string) (string, error) {
	if f == nil || f.pathHook == nil {
		return name, nil
	}
	return f.pathHook(name)
}

func (f *HookFS) matchReadHooks(name string) []*ReadHook {
	if !f.enabled {
		return nil
	}
	if len(f.readHooks) == 0 {
		return nil
	}
	matched := make([]*ReadHook, 0, len(f.readHooks))
	for _, hook := range f.readHooks {
		if hook == nil {
			continue
		}
		if hook.Matcher == nil || hook.Matcher.Match(name) {
			matched = append(matched, hook)
		}
	}
	return matched
}

func (f *HookFS) ReadFile(name string) ([]byte, error) {
	mapped, err := f.mapPath(name)
	if err != nil {
		return nil, err
	}
	ctx := &ReadHookContext{
		Name:       name,
		FS:         f,
		Underlying: f.underlying,
	}

	hooks := f.matchReadHooks(name)
	for _, hook := range hooks {
		if hook.BeforeRead == nil {
			continue
		}
		if err := hook.BeforeRead(ctx); err != nil {
			return nil, err
		}
	}

	data, err := f.underlying.ReadFile(mapped)
	if err != nil {
		return nil, err
	}

	for _, hook := range hooks {
		if hook.AfterRead == nil {
			continue
		}
		data, err = hook.AfterRead(ctx, data)
		if err != nil {
			return nil, err
		}
	}

	return data, nil
}

// ------- Hook Matchers & Helpers -------

// MatchAll 返回一个总是匹配的 matcher。
func MatchAll() HookMatcher {
	return HookMatcherFunc(func(string) bool { return true })
}

// SuffixMatcher 根据后缀匹配文件名，不区分大小写。
func SuffixMatcher(suffixes ...string) HookMatcher {
	normalized := make([]string, 0, len(suffixes))
	for _, suffix := range suffixes {
		if suffix == "" {
			continue
		}
		if !strings.HasPrefix(suffix, ".") {
			suffix = "." + suffix
		}
		normalized = append(normalized, strings.ToLower(suffix))
	}
	return HookMatcherFunc(func(name string) bool {
		lowerName := strings.ToLower(name)
		for _, suffix := range normalized {
			if strings.HasSuffix(lowerName, suffix) {
				return true
			}
		}
		return false
	})
}

// CustomMatcher 允许外部直接提供函数。
func CustomMatcher(fn func(string) bool) HookMatcher {
	if fn == nil {
		return nil
	}
	return HookMatcherFunc(fn)
}

// ------- FileSystem 接口默认实现 -------

func (f *HookFS) Open(name string) (fs.File, error) {
	mapped, err := f.mapPath(name)
	if err != nil {
		return nil, err
	}
	return f.underlying.Open(mapped)
}

func (f *HookFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	mapped, err := f.mapPath(name)
	if err != nil {
		return nil, err
	}
	return f.underlying.OpenFile(mapped, flag, perm)
}

func (f *HookFS) Stat(name string) (fs.FileInfo, error) {
	mapped, err := f.mapPath(name)
	if err != nil {
		return nil, err
	}
	return f.underlying.Stat(mapped)
}

func (f *HookFS) ReadDir(dirname string) ([]fs.DirEntry, error) {
	mapped, err := f.mapPath(dirname)
	if err != nil {
		return nil, err
	}
	return f.underlying.ReadDir(mapped)
}

func (f *HookFS) GetSeparators() rune {
	return f.underlying.GetSeparators()
}

func (f *HookFS) Join(paths ...string) string {
	return f.underlying.Join(paths...)
}

func (f *HookFS) IsAbs(name string) bool {
	return f.underlying.IsAbs(name)
}

func (f *HookFS) Getwd() (string, error) {
	return f.underlying.Getwd()
}

func (f *HookFS) Exists(name string) (bool, error) {
	mapped, err := f.mapPath(name)
	if err != nil {
		return false, err
	}
	return f.underlying.Exists(mapped)
}

func (f *HookFS) Rename(old string, newName string) error {
	mappedOld, err := f.mapPath(old)
	if err != nil {
		return err
	}
	mappedNew, err := f.mapPath(newName)
	if err != nil {
		return err
	}
	return f.underlying.Rename(mappedOld, mappedNew)
}

func (f *HookFS) Rel(base string, target string) (string, error) {
	mappedBase, err := f.mapPath(base)
	if err != nil {
		return "", err
	}
	mappedTarget, err := f.mapPath(target)
	if err != nil {
		return "", err
	}
	return f.underlying.Rel(mappedBase, mappedTarget)
}

func (f *HookFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	mapped, err := f.mapPath(name)
	if err != nil {
		return err
	}
	return f.underlying.WriteFile(mapped, data, perm)
}

func (f *HookFS) Delete(name string) error {
	mapped, err := f.mapPath(name)
	if err != nil {
		return err
	}
	return f.underlying.Delete(mapped)
}

func (f *HookFS) MkdirAll(name string, perm os.FileMode) error {
	mapped, err := f.mapPath(name)
	if err != nil {
		return err
	}
	return f.underlying.MkdirAll(mapped, perm)
}

func (f *HookFS) ExtraInfo(name string) map[string]any {
	mapped, err := f.mapPath(name)
	if err != nil {
		return nil
	}
	return f.underlying.ExtraInfo(mapped)
}

func (f *HookFS) Base(p string) string {
	return f.underlying.Base(p)
}

func (f *HookFS) PathSplit(s string) (string, string) {
	return f.underlying.PathSplit(s)
}

func (f *HookFS) Ext(s string) string {
	return f.underlying.Ext(s)
}

func (f *HookFS) String() string {
	underlyingStr := "FileSystem"
	if stringer, ok := f.underlying.(fmt.Stringer); ok {
		underlyingStr = stringer.String()
	}
	return fmt.Sprintf("HookFS{underlying: %s}", underlyingStr)
}

func (f *HookFS) Root() string {
	if rooter, ok := f.underlying.(interface{ Root() string }); ok {
		return rooter.Root()
	}
	return ""
}
