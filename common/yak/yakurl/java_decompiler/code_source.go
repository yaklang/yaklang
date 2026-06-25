package java_decompiler

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/jar"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils"
)

// codeSource is a single JAR/WAR/EAR archive or a local directory containing archives.
type codeSource struct {
	rootPath    string
	isDirectory bool
	expandedFS  fi.FileSystem
	jarParser   *jar.JarParser
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
	jp, err := jar.NewJarParser(rootPath)
	if err != nil {
		return nil, err
	}
	return &codeSource{
		rootPath:    rootPath,
		isDirectory: false,
		jarParser:   jp,
	}, nil
}

func (a *Action) resolveCodeSource(rootPath string) (*codeSource, error) {
	rootPath = filepath.Clean(rootPath)
	return a.codeSources.GetOrLoad(rootPath, func() (*codeSource, error) {
		return newCodeSource(rootPath)
	})
}

func (cs *codeSource) toAbsPath(relativePath string) string {
	relativePath = normalizeJarInternalPath(relativePath)
	if !cs.isDirectory {
		return relativePath
	}
	if relativePath == "." {
		return cs.rootPath
	}
	return filepath.Join(cs.rootPath, filepath.FromSlash(relativePath))
}

func (cs *codeSource) listDirectory(relativeDir string) ([]fs.DirEntry, error) {
	relativeDir = normalizeJarInternalPath(relativeDir)
	if cs.isDirectory {
		return cs.expandedFS.ReadDir(cs.toAbsPath(relativeDir))
	}
	return cs.jarParser.ListDirectory(relativeDir)
}

func (cs *codeSource) readFile(relativePath string) ([]byte, error) {
	relativePath = normalizeJarInternalPath(relativePath)
	if cs.isDirectory {
		return cs.expandedFS.ReadFile(cs.toAbsPath(relativePath))
	}
	if strings.HasSuffix(strings.ToLower(relativePath), ".class") {
		return cs.jarParser.DecompileClass(relativePath)
	}
	jarFS := cs.jarParser.GetJarFS()
	data, err := jarFS.ReadFile(relativePath)
	if err != nil {
		return jarFS.ZipFS.ReadFile(relativePath)
	}
	return data, nil
}

func (cs *codeSource) stat(relativePath string) (fs.FileInfo, error) {
	relativePath = normalizeJarInternalPath(relativePath)
	if cs.isDirectory {
		return cs.expandedFS.Stat(cs.toAbsPath(relativePath))
	}
	return cs.jarParser.GetJarFS().Stat(relativePath)
}

func (cs *codeSource) walkFS() fi.FileSystem {
	if cs.isDirectory {
		return cs.expandedFS
	}
	return cs.jarParser.GetJarFS()
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
	lower := strings.ToLower(path)
	for _, ext := range []string{".jar", ".war", ".ear", ".zip"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
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
