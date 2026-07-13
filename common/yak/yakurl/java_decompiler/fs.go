package java_decompiler

import (
	"path"
	"path/filepath"
	"strings"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/yaklang/common/utils"
)

// getJarFS gets or creates a javaclassparser.JarFS for the given jar path.
// Kept for tests that exercise JarFS directly.
func (a *Action) getJarFS(jarPath string) (*javaclassparser.JarFS, error) {
	return a.jarFS.GetOrLoad(jarPath, func() (*javaclassparser.JarFS, error) {
		fs, err := javaclassparser.NewJarFSFromLocal(jarPath)
		if err != nil {
			return nil, utils.Wrapf(err, "failed to open jar file: %s", jarPath)
		}
		return fs, nil
	})
}

func normalizeJarInternalPath(p string) string {
	p = strings.ReplaceAll(filepath.ToSlash(strings.TrimSpace(p)), "\\", "/")
	if p == "" || p == "." {
		return "."
	}
	p = path.Clean(strings.TrimLeft(p, "/"))
	if p == "" {
		return "."
	}
	return p
}
