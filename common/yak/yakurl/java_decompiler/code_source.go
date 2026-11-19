package java_decompiler

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/javajive/classparser"
	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// codeSource is a single JAR/WAR/EAR/ZIP archive or a local directory containing archives.
type codeSource struct {
	rootPath    string
	isDirectory bool
	expandedFS  fi.FileSystem
	// jarFS is set for archive roots. It owns the .class decompilation path;
	// expandedFS only expands nested archives and serves non-class entries.
	jarFS *javaclassparser.JarFS
}

func newCodeSource(rootPath string) (*codeSource, error) {
	rootPath = filepath.Clean(rootPath)
	st, err := os.Stat(rootPath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to stat code source: %s", rootPath)
	}
	if st.IsDir() {
		return &codeSource{
			rootPath:    rootPath,
			isDirectory: true,
			expandedFS:  javaclassparser.NewExpandedLocalFileSystem(),
		}, nil
	}
	jarFS, err := javaclassparser.NewJarFSFromLocal(rootPath)
	if err != nil {
		return nil, err
	}
	// Share one ZipFS index between listing and decompilation.
	expandedFS := javaclassparser.NewExpandedZipFS(jarFS.ZipFS, jarFS.ZipFS)
	return &codeSource{
		rootPath:    rootPath,
		isDirectory: false,
		expandedFS:  expandedFS,
		jarFS:       jarFS,
	}, nil
}

func (a *Action) resolveCodeSource(rootPath string) (*codeSource, error) {
	rootPath = filepath.Clean(rootPath)
	return a.codeSources.GetOrLoad(rootPath, func() (*codeSource, error) {
		return newCodeSource(rootPath)
	})
}

func (cs *codeSource) resolveFSPath(p string) string {
	if cs.isDirectory {
		clean := filepath.Clean(p)
		if filepath.IsAbs(clean) {
			return clean
		}
	}
	p = normalizeJarInternalPath(p)
	if cs.isDirectory {
		if p == "." {
			return cs.rootPath
		}
		return filepath.Join(cs.rootPath, filepath.FromSlash(p))
	}
	return p
}

func (cs *codeSource) listDirectory(relativeDir string) ([]fs.DirEntry, error) {
	return cs.expandedFS.ReadDir(cs.resolveFSPath(relativeDir))
}

func (cs *codeSource) readFile(relativePath string) ([]byte, error) {
	if classPath := cs.sourceClassPath(relativePath); classPath != "" {
		return cs.readClassFile(classPath)
	}
	return cs.expandedFS.ReadFile(cs.resolveFSPath(cs.pathKey(relativePath)))
}

// readClassFile returns decompiled Java source for a .class entry. Archive roots
// must go through JarFS: ExpandedZipFS returns the raw class bytes. Directory
// roots resolve the outermost archive and read through its JarFS, so nested
// archives decompile instead of returning the class bytes.
func (cs *codeSource) readClassFile(relativePath string) ([]byte, error) {
	key := cs.pathKey(relativePath)
	if !cs.isDirectory && cs.jarFS != nil {
		return cs.jarFS.ReadFile(key)
	}
	absPath := cs.resolveFSPath(key)
	if archivePath, internalPath, ok := splitLocalArchivePath(absPath); ok {
		jarFS, err := cs.openArchiveJarFS(archivePath)
		if err != nil {
			return nil, err
		}
		return jarFS.ReadFile(internalPath)
	}
	return cs.expandedFS.ReadFile(absPath)
}

// openArchiveJarFS returns the cached JarFS for an archive on the host disk.
// The ExpandedZipFS cache owns the handle, so callers must not close it.
func (cs *codeSource) openArchiveJarFS(archivePath string) (*javaclassparser.JarFS, error) {
	expanded, ok := cs.expandedFS.(*javaclassparser.ExpandedZipFS)
	if !ok {
		return nil, utils.Errorf("code source filesystem does not expand archives: %T", cs.expandedFS)
	}
	unified, err := expanded.GetJarFS(archivePath)
	if err != nil {
		return nil, err
	}
	jarFS, ok := unified.GetFileSystem().(*javaclassparser.JarFS)
	if !ok {
		return nil, utils.Errorf("expanded archive is not a JarFS: %T", unified.GetFileSystem())
	}
	return jarFS, nil
}

// splitLocalArchivePath splits /dir/lib.jar/com/A.class into the archive path
// and the internal path. The outermost archive wins so nested archives stay a
// JarFS concern.
func splitLocalArchivePath(absPath string) (archivePath, internalPath string, ok bool) {
	lower := strings.ToLower(absPath)
	best := -1
	bestExt := ""
	for _, ext := range javaArchiveExtensions {
		if idx := strings.Index(lower, ext+"/"); idx >= 0 && (best < 0 || idx < best) {
			best = idx
			bestExt = ext
		}
	}
	if best < 0 {
		return "", "", false
	}
	archivePath = absPath[:best+len(bestExt)]
	internalPath = strings.TrimPrefix(absPath[best+len(bestExt)+1:], "/")
	if internalPath == "" {
		return "", "", false
	}
	st, err := os.Stat(archivePath)
	if err != nil || st.IsDir() {
		return "", "", false
	}
	return archivePath, internalPath, true
}

func (cs *codeSource) stat(relativePath string) (fs.FileInfo, error) {
	key := cs.pathKey(relativePath)
	if classPath := cs.sourceClassPath(key); classPath != "" {
		key = classPath
	}
	return cs.expandedFS.Stat(cs.resolveFSPath(key))
}

func (cs *codeSource) walkFS() fi.FileSystem {
	return cs.expandedFS
}

func (cs *codeSource) walkRoot() string {
	if cs.isDirectory {
		return cs.rootPath
	}
	return "."
}

func (cs *codeSource) exportBaseName() string {
	if cs.isDirectory {
		return filepath.Base(cs.rootPath)
	}
	_, name := filepath.Split(cs.rootPath)
	return strings.TrimSuffix(name, filepath.Ext(name))
}

func isSupportedArchiveRoot(path string) bool {
	return javaclassparserIsArchiveLeaf(path)
}

func validateCodeSourceRoot(rootPath string) error {
	st, err := os.Stat(rootPath)
	if err != nil {
		return utils.Wrapf(err, "code source not found: %s", rootPath)
	}
	if st.IsDir() {
		return nil
	}
	if isSupportedArchiveRoot(rootPath) {
		return nil
	}
	return utils.Errorf("unsupported code source: %s (expected directory or .jar/.war/.ear/.zip file)", rootPath)
}
