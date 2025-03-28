package java_decompiler

import (
	"bytes"
	"strings"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// getJarFS gets or creates a javaclassparser.FS for the given jar path
func (a *Action) getJarFS(jarPath string) (*javaclassparser.FS, error) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if fs, ok := a.jarFS[jarPath]; ok {
		return fs, nil
	}

	fs, err := javaclassparser.NewJarFSFromLocal(jarPath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to open jar file: %s", jarPath)
	}
	a.jarFS[jarPath] = fs
	return fs, nil
}
func (a *Action) getNestedJarFs(jarPath string, dirPath string) (*javaclassparser.FS, string, string, error) {
	currentDirPath := dirPath
	currentJarPath := jarPath
	jarFs, err := a.getJarFS(currentJarPath)
	if err != nil {
		return nil, "", "", err
	}
	for {
		physicalJarPath, internalPath, err := a.parseNestedJarPath(currentDirPath)
		if err == nil {
			currentDirPath = internalPath
			currentJarPath = physicalJarPath
			content, err := jarFs.ZipFS.ReadFile(physicalJarPath)
			if err != nil {
				return nil, "", "", err
			}
			zipFs, err := filesys.NewZipFSRaw(bytes.NewReader(content), int64(len(content)))
			if err != nil {
				return nil, "", "", err
			}
			jarFs = javaclassparser.NewJarFS(zipFs)
		} else {
			break
		}
	}

	return jarFs, currentJarPath, currentDirPath, nil
}

// parseNestedJarPath parses a path that may contain nested JAR files
// Returns the physical jar path, the internal path within the jar, and any error
func (a *Action) parseNestedJarPath(fullPath string) (string, string, error) {
	// Check if path contains .jar/
	jarSeparatorIndex := strings.Index(fullPath, ".jar/")
	if jarSeparatorIndex == -1 {
		// Not a nested path, check if it's a jar file itself
		if strings.HasSuffix(fullPath, ".jar") {
			return fullPath, "", nil
		}
		return "", "", utils.Errorf("path does not contain a jar file: %s", fullPath)
	}

	// Extract the JAR file path and the internal path
	jarPath := fullPath[:jarSeparatorIndex+4]      // include the .jar part
	internalPath := fullPath[jarSeparatorIndex+5:] // skip the .jar/ part

	return jarPath, internalPath, nil
}
