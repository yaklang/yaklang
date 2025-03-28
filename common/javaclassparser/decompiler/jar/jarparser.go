package jar

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// JarParser is a utility for parsing and analyzing JAR files
type JarParser struct {
	// jarPath is the path to the JAR file
	jarPath string

	// jarFS is the filesystem interface for accessing JAR contents
	jarFS *javaclassparser.FS

	// mutex protects concurrent access to resources
	mutex sync.Mutex

	// classCache caches decompiled class content to avoid redundant decompilation
	classCache map[string][]byte

	// failedFiles tracks files that couldn't be parsed/decompiled
	failedFiles map[string]error
}

// NewJarParser creates a new JarParser for the specified JAR file path
func NewJarParser(jarPath string) (*JarParser, error) {
	jarFS, err := javaclassparser.NewJarFSFromLocal(jarPath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to open jar file: %s", jarPath)
	}

	return &JarParser{
		jarPath:     jarPath,
		jarFS:       jarFS,
		classCache:  make(map[string][]byte),
		failedFiles: make(map[string]error),
	}, nil
}

// NewJarParserFromBytes creates a new JarParser from JAR content in memory
func NewJarParserFromBytes(jarContent []byte) (*JarParser, error) {
	zipFS, err := filesys.NewZipFSRaw(bytes.NewReader(jarContent), int64(len(jarContent)))
	if err != nil {
		return nil, utils.Wrapf(err, "failed to create zip filesystem from bytes")
	}

	jarFS := javaclassparser.NewJarFS(zipFS)

	return &JarParser{
		jarPath:     "memory-jar",
		jarFS:       jarFS,
		classCache:  make(map[string][]byte),
		failedFiles: make(map[string]error),
	}, nil
}

// GetJarFS returns the underlying filesystem for the JAR
func (j *JarParser) GetJarFS() *javaclassparser.FS {
	return j.jarFS
}

// ListDirectory lists the contents of a directory within the JAR
// Handles nested JAR paths like "lib/fastjson.jar/com/example" or "lib/outer.jar/libs/inner.jar/com/example"
func (j *JarParser) ListDirectory(dirPath string) ([]fs.DirEntry, error) {
	if dirPath == "" {
		dirPath = "."
	}

	// Handle multiple levels of nested JARs
	jarPaths, finalPath, err := j.parseMultiLevelJarPath(dirPath)
	if err != nil && len(jarPaths) == 0 {
		// No JAR path in dirPath, treat as regular directory
	} else if err != nil {
		return nil, err
	} else if len(jarPaths) > 0 {
		// We have at least one JAR in the path
		currentFS := j.jarFS

		// Traverse through the nested JARs
		for _, jarPath := range jarPaths {
			nestedContent, err := currentFS.ZipFS.ReadFile(jarPath)
			if err != nil {
				return nil, utils.Wrapf(err, "failed to read nested jar: %s", jarPath)
			}

			zipFS, err := filesys.NewZipFSRaw(bytes.NewReader(nestedContent), int64(len(nestedContent)))
			if err != nil {
				return nil, utils.Wrapf(err, "failed to create zip filesystem for nested jar: %s", jarPath)
			}

			currentFS = javaclassparser.NewJarFS(zipFS)
		}

		// Read directory from the final filesystem
		entries, err := currentFS.ReadDir(finalPath)
		if err != nil {
			lastJar := jarPaths[len(jarPaths)-1]
			return nil, utils.Wrapf(err, "failed to read directory: %s in nested jar: %s", finalPath, lastJar)
		}

		return entries, nil
	}

	// Regular JAR directory
	entries, err := j.jarFS.ReadDir(dirPath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to read directory: %s", dirPath)
	}

	return entries, nil
}

// GetDirectoryContents returns information about all entries in a directory
// Also supports nested JAR paths like "lib/fastjson.jar/com/example"
func (j *JarParser) GetDirectoryContents(dirPath string) ([]map[string]interface{}, error) {
	entries, err := j.ListDirectory(dirPath)
	if err != nil {
		return nil, err
	}

	result := make([]map[string]interface{}, 0, len(entries))

	// Check if this is a nested JAR path
	isNestedPath := strings.Index(dirPath, ".jar/") > 0

	// If dirPath is empty or ".", normalize it to avoid filepath.Join issues
	normalizedDirPath := dirPath
	if normalizedDirPath == "" || normalizedDirPath == "." {
		normalizedDirPath = ""
	} else if !strings.HasSuffix(normalizedDirPath, "/") {
		normalizedDirPath += "/"
	}

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			log.Errorf("failed to get info for %s: %v", entry.Name(), err)
			continue
		}

		// Build the full entry path correctly
		var entryPath string
		if normalizedDirPath == "" {
			entryPath = entry.Name()
		} else {
			entryPath = normalizedDirPath + entry.Name()
		}

		item := map[string]interface{}{
			"name":         entry.Name(),
			"path":         entryPath,
			"size":         info.Size(),
			"isDirectory":  entry.IsDir(),
			"lastModified": info.ModTime().Unix(),
		}

		// For directories, check if they have children
		if entry.IsDir() {
			// Use the entry path directly for ListDirectory to handle nested JARs correctly
			subEntries, err := j.ListDirectory(entryPath)
			if err == nil {
				item["hasChildren"] = len(subEntries) > 0
			}
		}

		// For class files, identify if it's an inner class
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".class") {
			className := entry.Name()
			dollarIndex := strings.Index(className, "$")
			if dollarIndex > 0 {
				outerClassName := className[:dollarIndex] + ".class"
				item["isInnerClass"] = true
				item["outerClass"] = outerClassName
			}
		}

		// For nested JAR paths, set a flag to indicate this is in a nested JAR
		if isNestedPath {
			item["inNestedJar"] = true
		}

		result = append(result, item)
	}

	return result, nil
}

// DecompileClass reads and decompiles a Java class file from the JAR
// Handles nested JAR paths like "lib/fastjson.jar/com/example/Main.class" or "lib/outer.jar/libs/inner.jar/com/example/Main.class"
func (j *JarParser) DecompileClass(className string) ([]byte, error) {
	j.mutex.Lock()
	defer j.mutex.Unlock()

	// Check if we've already decompiled this class
	if data, ok := j.classCache[className]; ok {
		return data, nil
	}

	// Check if this is a failed file
	if err, ok := j.failedFiles[className]; ok {
		return nil, err
	}

	// Handle multiple levels of nested JARs
	jarPaths, finalPath, err := j.parseMultiLevelJarPath(className)
	if err != nil && len(jarPaths) == 0 {
		// No JAR path in className, treat as regular class
	} else if err != nil {
		j.failedFiles[className] = err
		return nil, err
	} else if len(jarPaths) > 0 {
		// We have at least one JAR in the path
		currentFS := j.jarFS

		// Traverse through the nested JARs
		for _, jarPath := range jarPaths {
			nestedContent, err := currentFS.ZipFS.ReadFile(jarPath)
			if err != nil {
				j.failedFiles[className] = err
				return nil, utils.Wrapf(err, "failed to read nested jar: %s", jarPath)
			}

			zipFS, err := filesys.NewZipFSRaw(bytes.NewReader(nestedContent), int64(len(nestedContent)))
			if err != nil {
				j.failedFiles[className] = err
				return nil, utils.Wrapf(err, "failed to create zip filesystem for nested jar: %s", jarPath)
			}

			currentFS = javaclassparser.NewJarFS(zipFS)
		}

		// Read class from the final filesystem
		data, err := currentFS.ReadFile(finalPath)
		if err != nil {
			data, err = currentFS.ZipFS.ReadFile(finalPath)
			if err != nil {
				j.failedFiles[className] = err
				lastJar := jarPaths[len(jarPaths)-1]
				return nil, utils.Wrapf(err, "failed to read class: %s from nested jar: %s", finalPath, lastJar)
			}
		}

		// Cache the decompiled content
		j.classCache[className] = data

		return data, nil
	}

	// Regular class decompilation
	data, err := j.jarFS.ReadFile(className)
	if err != nil {
		data, err = j.jarFS.ZipFS.ReadFile(className)
		if err != nil {
			j.failedFiles[className] = err
			return nil, utils.Wrapf(err, "failed to read class: %s", className)
		}
	}

	// Cache the decompiled content
	j.classCache[className] = data

	return data, nil
}

// parseMultiLevelJarPath parses a path that may contain multiple nested JAR files
// Returns a slice of JAR paths, the final internal path, and any error
// Example: "lib/outer.jar/libs/inner.jar/com/example/Main.class" returns
// ["lib/outer.jar", "libs/inner.jar"], "com/example/Main.class", nil
func (j *JarParser) parseMultiLevelJarPath(fullPath string) ([]string, string, error) {
	var jarPaths []string
	remainingPath := fullPath

	for {
		jarSeparatorIndex := strings.Index(remainingPath, ".jar/")
		if jarSeparatorIndex == -1 {
			// No more .jar/ patterns, check if it ends with .jar
			if strings.HasSuffix(remainingPath, ".jar") && len(jarPaths) == 0 {
				jarPaths = append(jarPaths, remainingPath)
				remainingPath = ""
			}
			break
		}

		// Extract the JAR file path and update remaining path
		jarPath := remainingPath[:jarSeparatorIndex+4] // include the .jar part
		jarPaths = append(jarPaths, jarPath)

		// Move past the jar part
		remainingPath = remainingPath[jarSeparatorIndex+5:] // skip the .jar/ part
	}

	if len(jarPaths) == 0 {
		return nil, fullPath, utils.Errorf("path does not contain a jar file: %s", fullPath)
	}

	return jarPaths, remainingPath, nil
}

// GetNestedJarFS handles the case of a JAR file within the JAR
// Deprecated: This is a simple version that only handles a single level of nesting.
// For multi-level nested JARs, the ListDirectory and DecompileClass methods now
// automatically handle multiple levels of nesting.
func (j *JarParser) GetNestedJarFS(nestedJarPath string) (*javaclassparser.FS, error) {
	content, err := j.jarFS.ZipFS.ReadFile(nestedJarPath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to read nested jar: %s", nestedJarPath)
	}

	zipFS, err := filesys.NewZipFSRaw(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, utils.Wrapf(err, "failed to create zip filesystem for nested jar: %s", nestedJarPath)
	}

	return javaclassparser.NewJarFS(zipFS), nil
}

// ParseNestedJarPath parses a path that may contain nested JAR files
// Returns the physical jar path, the internal path within the jar, and any error
//
// Deprecated: For paths with multiple nested JARs, use parseMultiLevelJarPath instead.
// This method only handles a single level of nesting.
//
// Example: "lib/sample.jar/com/example/Main.class" returns "lib/sample.jar", "com/example/Main.class", nil
// But "lib/outer.jar/libs/inner.jar/class.class" returns "lib/outer.jar", "libs/inner.jar/class.class", nil
// which treats everything after the first .jar/ as a single path
func (j *JarParser) ParseNestedJarPath(fullPath string) (string, string, error) {
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

// FindJavaClasses finds all Java class files in the JAR recursively
// If includeNestedJars is true, it will also look inside JARs within the JAR,
// including multiple levels of nesting (jar inside jar inside jar)
func (j *JarParser) FindJavaClasses(includeNestedJars ...bool) ([]string, error) {
	var classes []string
	checkNested := false
	if len(includeNestedJars) > 0 {
		checkNested = includeNestedJars[0]
	}

	// Keep track of nested JARs we find
	var nestedJars []string

	// First pass: scan for classes and identify nested JARs
	err := filesys.Recursive(".",
		filesys.WithFileSystem(j.jarFS.ZipFS),
		filesys.WithFileStat(func(path string, info fs.FileInfo) error {
			if info.IsDir() {
				return nil
			}

			if strings.HasSuffix(path, ".class") {
				classes = append(classes, path)
			} else if checkNested && strings.HasSuffix(path, ".jar") {
				nestedJars = append(nestedJars, path)
			}
			return nil
		}),
	)

	if err != nil {
		return nil, utils.Wrapf(err, "failed to search for Java classes")
	}

	// Second pass: if requested, look inside nested JARs (with multi-level support)
	if checkNested && len(nestedJars) > 0 {
		for _, nestedJarPath := range nestedJars {
			// Process this nested JAR
			err := j.findNestedJarClasses(nestedJarPath, "", &classes)
			if err != nil {
				log.Warnf("Error processing nested JAR %s: %v", nestedJarPath, err)
			}
		}
	}

	return classes, nil
}

// findNestedJarClasses recursively searches for classes in nested JARs
// jarPath is the current jar being processed
// prefix is used for building the full path for nested jars
// classes is a pointer to the slice where found classes will be stored
func (j *JarParser) findNestedJarClasses(jarPath, prefix string, classes *[]string) error {
	// Construct full path for this JAR
	fullJarPath := jarPath
	if prefix != "" {
		fullJarPath = prefix + "/" + jarPath
	}

	// Get the nested JAR content
	nestedContent, err := j.jarFS.ZipFS.ReadFile(jarPath)
	if err != nil {
		return utils.Wrapf(err, "failed to read nested jar: %s", jarPath)
	}

	// Create a filesystem for this JAR
	zipFS, err := filesys.NewZipFSRaw(bytes.NewReader(nestedContent), int64(len(nestedContent)))
	if err != nil {
		return utils.Wrapf(err, "failed to create zip filesystem for nested jar: %s", jarPath)
	}

	nestedFS := javaclassparser.NewJarFS(zipFS)

	// Track nested JARs found within this JAR
	var deeperNestedJars []string

	// Find classes and nested JARs within this JAR
	err = filesys.Recursive(".",
		filesys.WithFileSystem(nestedFS.ZipFS),
		filesys.WithFileStat(func(path string, info fs.FileInfo) error {
			if info.IsDir() {
				return nil
			}

			if strings.HasSuffix(path, ".class") {
				// Add this class to the results with full path
				*classes = append(*classes, fullJarPath+"/"+path)
			} else if strings.HasSuffix(path, ".jar") {
				// Found a deeper nested JAR
				deeperNestedJars = append(deeperNestedJars, path)
			}
			return nil
		}),
	)

	if err != nil {
		return utils.Wrapf(err, "error scanning nested JAR %s", jarPath)
	}

	// Recursively process deeper nested JARs if any
	for _, deepJarPath := range deeperNestedJars {
		err := j.findNestedJarClasses(deepJarPath, fullJarPath, classes)
		if err != nil {
			log.Warnf("Error processing deeper nested JAR %s in %s: %v", deepJarPath, fullJarPath, err)
		}
	}

	return nil
}

// FindInnerClasses finds all inner classes for a given outer class
func (j *JarParser) FindInnerClasses(outerClassPath string) ([]string, error) {
	if !strings.HasSuffix(outerClassPath, ".class") {
		return nil, utils.Errorf("not a class file: %s", outerClassPath)
	}

	// Get the directory and base name of the outer class
	dir := filepath.Dir(outerClassPath)
	baseName := filepath.Base(outerClassPath)
	baseName = strings.TrimSuffix(baseName, ".class")

	// Find all classes in the same directory
	entries, err := j.ListDirectory(dir)
	if err != nil {
		return nil, err
	}

	var innerClasses []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".class") {
			continue
		}

		// Check if this is an inner class of our outer class
		if strings.HasPrefix(name, baseName+"$") {
			innerClassPath := filepath.Join(dir, name)
			innerClasses = append(innerClasses, innerClassPath)
		}
	}

	return innerClasses, nil
}

// ExportDecompiledJar exports the JAR with all class files decompiled to .java files
func (j *JarParser) ExportDecompiledJar() (*bytes.Buffer, error) {
	// Create an in-memory buffer for the zip file
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	// Walk through all files in the JAR and add them to the zip
	err := filesys.Recursive(".",
		filesys.WithFileSystem(j.jarFS.ZipFS),
		filesys.WithFileStat(func(s string, info fs.FileInfo) error {
			if info.IsDir() {
				// Create directory entries in the zip
				_, err := zipWriter.Create(s + "/")
				return err
			}

			// Create a new file entry in the zip
			var fileContent []byte
			var targetPath string

			if filepath.Ext(s) == ".class" {
				// For class files, decompile them and save as .java
				decompiled, err := j.DecompileClass(s)
				if err != nil {
					// If decompilation fails, use the original class file
					log.Warnf("Failed to decompile %s: %v", s, err)
					decompiled, err = j.jarFS.ZipFS.ReadFile(s)
					if err != nil {
						return utils.Wrapf(err, "failed to read class file: %s", s)
					}
					fileContent = decompiled
					targetPath = s
				} else {
					// Decompilation succeeded, save as .java
					fileContent = decompiled
					targetPath = strings.TrimSuffix(s, ".class") + ".java"
				}
			} else {
				// For non-class files, just copy them as is
				var err error
				fileContent, err = j.jarFS.ZipFS.ReadFile(s)
				if err != nil {
					return utils.Wrapf(err, "failed to read file: %s", s)
				}
				targetPath = s
			}

			// Create and write the file to the zip
			zipFile, err := zipWriter.Create(targetPath)
			if err != nil {
				return utils.Wrapf(err, "failed to create zip entry for: %s", targetPath)
			}

			_, err = zipFile.Write(fileContent)
			if err != nil {
				return utils.Wrapf(err, "failed to write content for: %s", targetPath)
			}

			return nil
		}),
	)

	if err != nil {
		return nil, utils.Wrapf(err, "failed to process jar files")
	}

	// Close the zip writer to flush its contents
	err = zipWriter.Close()
	if err != nil {
		return nil, utils.Wrapf(err, "failed to close zip writer")
	}

	return &buf, nil
}

// GetJarManifest retrieves the manifest information from the JAR
func (j *JarParser) GetJarManifest() (map[string]string, error) {
	manifestData, err := j.jarFS.ZipFS.ReadFile("META-INF/MANIFEST.MF")
	if err != nil {
		return nil, utils.Wrapf(err, "failed to read manifest from jar")
	}

	manifest := make(map[string]string)
	lines := bytes.Split(manifestData, []byte("\n"))

	for _, line := range lines {
		trimmedLine := bytes.TrimSpace(line)
		if len(trimmedLine) == 0 {
			continue
		}

		parts := bytes.SplitN(trimmedLine, []byte(":"), 2)
		if len(parts) != 2 {
			continue
		}

		key := string(bytes.TrimSpace(parts[0]))
		value := string(bytes.TrimSpace(parts[1]))
		manifest[key] = value
	}

	return manifest, nil
}

// FindClassByName searches for a class by its name (not path)
func (j *JarParser) FindClassByName(className string) (string, error) {
	// Normalize the class name
	normalizedClassName := strings.Replace(className, ".", "/", -1)
	if !strings.HasSuffix(normalizedClassName, ".class") {
		normalizedClassName += ".class"
	}

	var foundPath string

	err := filesys.Recursive(".",
		filesys.WithFileSystem(j.jarFS.ZipFS),
		filesys.WithFileStat(func(path string, info fs.FileInfo) error {
			if info.IsDir() {
				return nil
			}

			// Check if this path ends with our normalized class name
			if strings.HasSuffix(path, normalizedClassName) {
				foundPath = path
				// Return an error to break out of the recursive walk
				return fmt.Errorf("found")
			}

			return nil
		}),
	)

	// If we found the class, the error will be "found"
	if err != nil && err.Error() == "found" {
		return foundPath, nil
	}

	if err != nil {
		return "", utils.Wrapf(err, "error while searching for class")
	}

	return "", utils.Errorf("class not found: %s", className)
}
