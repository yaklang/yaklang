package java_decompiler

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/javajive/classparser"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils"
)

// codeSource is a single JAR/WAR/EAR/ZIP archive or a local directory containing archives.
type codeSource struct {
	rootPath    string
	isDirectory bool
	expandedFS  fi.FileSystem
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
	expandedFS, err := javaclassparser.NewExpandedArchiveFileSystemFromLocal(rootPath)
	if err != nil {
		return nil, err
	}
	return &codeSource{
		rootPath:    rootPath,
		isDirectory: false,
		expandedFS:  expandedFS,
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
	return cs.expandedFS.ReadFile(cs.resolveFSPath(relativePath))
}

func (cs *codeSource) stat(relativePath string) (fs.FileInfo, error) {
	return cs.expandedFS.Stat(cs.resolveFSPath(relativePath))
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
