package ssaapi

import (
	"io/fs"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// programSourceFS is the frontend mount of program sources.
// Paths stay /programName/... . A diff stack is filled by ExpandIncrementalSources.
// A full program and the first incremental base stay on that program's own ir_sources.
type programSourceFS struct {
	filesys_interface.FileSystem

	mu    sync.Mutex
	cache map[string]cachedExpand
}

type cachedExpand struct {
	updatedAt time.Time
	fs        filesys_interface.FileSystem
}

// NewProgramSourceFS serves ssadb paths. Incremental programs expand to the full tree.
func NewProgramSourceFS() filesys_interface.FileSystem {
	return &programSourceFS{
		FileSystem: ssadb.NewIrSourceFs(),
		cache:      map[string]cachedExpand{},
	}
}

func programNeedsFullExpand(ir *ssadb.IrProgram) bool {
	if ir == nil {
		return false
	}
	if len(ir.OverlayLayers) >= 2 {
		return true
	}
	incremental := ir.IsOverlay || ir.BaseProgramName != "" || len(ir.FileHashMap) > 0
	return incremental && ir.BaseProgramName != "" && ir.BaseProgramName != ir.ProgramName
}

func (f *programSourceFS) expanded(programName string) (filesys_interface.FileSystem, error) {
	ir, err := ssadb.GetProgram(programName, ssadb.Application)
	if err != nil || ir == nil || !programNeedsFullExpand(ir) {
		return nil, nil
	}

	f.mu.Lock()
	if cached, ok := f.cache[programName]; ok && cached.updatedAt.Equal(ir.UpdatedAt) && cached.fs != nil {
		expanded := cached.fs
		f.mu.Unlock()
		return expanded, nil
	}
	f.mu.Unlock()

	prog, err := NewProgramFromDB(programName)
	if err != nil {
		return nil, err
	}
	expanded, err := ExpandIncrementalSources(prog)
	if err != nil {
		log.Errorf("expand incremental program %s: %v", programName, err)
		return nil, err
	}

	f.mu.Lock()
	f.cache[programName] = cachedExpand{updatedAt: ir.UpdatedAt, fs: expanded}
	f.mu.Unlock()
	return expanded, nil
}

// route returns the expanded tree and the path inside it.
// The program root maps to ".". A non-incremental program returns a nil filesystem.
func (f *programSourceFS) route(name string) (filesys_interface.FileSystem, string, error) {
	program, rest, ok := splitProgramPath(name)
	if !ok {
		return nil, "", nil
	}
	expanded, err := f.expanded(program)
	if err != nil || expanded == nil {
		return nil, "", err
	}
	if rest == "" {
		return expanded, ".", nil
	}
	return expanded, rest, nil
}

func splitProgramPath(name string) (program, rest string, ok bool) {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Clean("/" + strings.TrimPrefix(name, "/"))
	if name == "/" || name == "." {
		return "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(name, "/"), "/")
	if len(parts) == 0 || parts[0] == "" || parts[0] == "." {
		return "", "", false
	}
	return parts[0], strings.Join(parts[1:], "/"), true
}

func (f *programSourceFS) ReadDir(name string) ([]fs.DirEntry, error) {
	expanded, innerName, err := f.route(name)
	if err != nil {
		return nil, err
	}
	if expanded != nil {
		return expanded.ReadDir(innerName)
	}
	return f.FileSystem.ReadDir(name)
}

func (f *programSourceFS) ReadFile(name string) ([]byte, error) {
	expanded, innerName, err := f.route(name)
	if err != nil {
		return nil, err
	}
	if expanded != nil && innerName != "." {
		return expanded.ReadFile(innerName)
	}
	return f.FileSystem.ReadFile(name)
}

func (f *programSourceFS) Open(name string) (fs.File, error) {
	expanded, innerName, err := f.route(name)
	if err != nil {
		return nil, err
	}
	if expanded != nil && innerName != "." {
		return expanded.Open(innerName)
	}
	return f.FileSystem.Open(name)
}

func (f *programSourceFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	expanded, innerName, err := f.route(name)
	if err != nil {
		return nil, err
	}
	if expanded != nil && innerName != "." {
		return expanded.OpenFile(innerName, flag, perm)
	}
	return f.FileSystem.OpenFile(name, flag, perm)
}

func (f *programSourceFS) Stat(name string) (fs.FileInfo, error) {
	expanded, innerName, err := f.route(name)
	if err != nil {
		return nil, err
	}
	if expanded != nil && innerName != "." {
		return expanded.Stat(innerName)
	}
	return f.FileSystem.Stat(name)
}

func (f *programSourceFS) Exists(name string) (bool, error) {
	expanded, innerName, err := f.route(name)
	if err != nil {
		return false, err
	}
	if expanded != nil && innerName != "." {
		return expanded.Exists(innerName)
	}
	return f.FileSystem.Exists(name)
}
