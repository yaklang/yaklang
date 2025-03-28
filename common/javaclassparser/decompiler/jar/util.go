package jar

import (
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// AnalyzeJarSecurity performs a basic security analysis of a JAR file
// Looking for potential security issues or vulnerabilities
func AnalyzeJarSecurity(jarPath string) (map[string]interface{}, error) {
	parser, err := NewJarParser(jarPath)
	if err != nil {
		return nil, err
	}

	results := map[string]interface{}{
		"jarPath":        jarPath,
		"securityIssues": []map[string]interface{}{},
	}

	// Check for sensitive files
	sensitiveFiles, err := findSensitiveFiles(parser)
	if err != nil {
		return nil, err
	}
	if len(sensitiveFiles) > 0 {
		results["sensitiveFiles"] = sensitiveFiles
	}

	// Check for potentially dangerous imports in Java classes
	dangerousImports, err := findDangerousImports(parser)
	if err != nil {
		return nil, err
	}
	if len(dangerousImports) > 0 {
		results["dangerousImports"] = dangerousImports
	}

	// Get manifest information
	manifest, err := parser.GetJarManifest()
	if err == nil {
		results["manifest"] = manifest
	}

	return results, nil
}

// findSensitiveFiles looks for potentially sensitive files in the JAR
func findSensitiveFiles(parser *JarParser) ([]string, error) {
	var sensitiveFiles []string

	sensitivePatterns := []string{
		".properties", ".xml", ".json", ".yml", ".yaml", ".key", ".pem",
		".keystore", ".jks", ".db", ".sqlite", ".credentials", "password",
		"secret", "token", "config", "jdbc",
	}

	err := filesys.Recursive(".",
		filesys.WithFileSystem(parser.jarFS.ZipFS),
		filesys.WithFileStat(func(path string, info fs.FileInfo) error {
			if info.IsDir() {
				return nil
			}

			fileName := strings.ToLower(filepath.Base(path))

			for _, pattern := range sensitivePatterns {
				if strings.Contains(fileName, pattern) {
					sensitiveFiles = append(sensitiveFiles, path)
					break
				}
			}

			return nil
		}),
	)

	if err != nil {
		return nil, utils.Wrapf(err, "error finding sensitive files")
	}

	return sensitiveFiles, nil
}

// findDangerousImports looks for potentially dangerous class imports in Java code
func findDangerousImports(parser *JarParser) (map[string][]string, error) {
	dangerousImportsByClass := make(map[string][]string)

	// List of potentially dangerous or security-sensitive Java packages
	dangerousPackages := []string{
		"java.lang.Runtime", "java.lang.ProcessBuilder", "java.io.FileInputStream",
		"java.lang.reflect", "java.security", "javax.crypto", "sun.misc.Unsafe",
		"java.sql", "javax.script", "com.sun.jndi", "jdk.jshell", "org.apache.commons.io.FileUtils",
		"java.util.concurrent.ThreadPoolExecutor", "java.beans.XMLDecoder",
	}

	// Get all class files
	classes, err := parser.FindJavaClasses()
	if err != nil {
		return nil, err
	}

	for _, className := range classes {
		decompiled, err := parser.DecompileClass(className)
		if err != nil {
			continue // Skip classes that can't be decompiled
		}

		foundImports := []string{}

		// Check for dangerous imports
		for _, pkg := range dangerousPackages {
			importPattern := fmt.Sprintf("import %s", pkg)
			if bytes.Contains(decompiled, []byte(importPattern)) ||
				bytes.Contains(decompiled, []byte(pkg+".")) {
				foundImports = append(foundImports, pkg)
			}
		}

		if len(foundImports) > 0 {
			dangerousImportsByClass[className] = foundImports
		}
	}

	return dangerousImportsByClass, nil
}

// GetJarDependencyTree analyzes dependencies between JAR files by examining MANIFEST.MF
func GetJarDependencyTree(jarPath string) (map[string]interface{}, error) {
	parser, err := NewJarParser(jarPath)
	if err != nil {
		return nil, err
	}

	manifest, err := parser.GetJarManifest()
	if err != nil {
		return nil, utils.Wrapf(err, "failed to read manifest from jar")
	}

	result := map[string]interface{}{
		"jarPath":      jarPath,
		"dependencies": []string{},
	}

	// Extract Class-Path entries from manifest
	if classPath, ok := manifest["Class-Path"]; ok {
		dependencies := strings.Fields(classPath)
		result["dependencies"] = dependencies
	}

	return result, nil
}

// ReadClassAsOriginalBytes reads a class file from the JAR without decompiling it
func ReadClassAsOriginalBytes(parser *JarParser, className string) ([]byte, error) {
	// Read the original class file bytes from the ZIP filesystem
	data, err := parser.jarFS.ZipFS.ReadFile(className)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to read class: %s", className)
	}

	return data, nil
}

// DecompileAndFormat decompiles a class file and formats the source code
func DecompileAndFormat(parser *JarParser, className string) (string, error) {
	decompiled, err := parser.DecompileClass(className)
	if err != nil {
		return "", err
	}

	// Simple formatting: fix indentation and spacing
	lines := strings.Split(string(decompiled), "\n")
	for i, line := range lines {
		// Adjust indentation for better readability
		lines[i] = strings.TrimRight(line, " \t")
	}

	return strings.Join(lines, "\n"), nil
}

// GetJavaVersion attempts to determine the Java version used to compile the JAR
func GetJavaVersion(parser *JarParser) (string, error) {
	// Find any class file to check its version
	classes, err := parser.FindJavaClasses()
	if err != nil {
		return "", err
	}

	if len(classes) == 0 {
		return "", utils.Error("no classes found in JAR")
	}

	// Read the original bytes of the first class
	classBytes, err := ReadClassAsOriginalBytes(parser, classes[0])
	if err != nil {
		return "", err
	}

	// Parse the class using javaclassparser
	classObj, err := javaclassparser.Parse(classBytes)
	if err != nil {
		return "", err
	}

	// Map the major version to a Java version
	javaVersion := ""
	switch classObj.MajorVersion {
	case 45:
		javaVersion = "1.1"
	case 46:
		javaVersion = "1.2"
	case 47:
		javaVersion = "1.3"
	case 48:
		javaVersion = "1.4"
	case 49:
		javaVersion = "5"
	case 50:
		javaVersion = "6"
	case 51:
		javaVersion = "7"
	case 52:
		javaVersion = "8"
	case 53:
		javaVersion = "9"
	case 54:
		javaVersion = "10"
	case 55:
		javaVersion = "11"
	case 56:
		javaVersion = "12"
	case 57:
		javaVersion = "13"
	case 58:
		javaVersion = "14"
	case 59:
		javaVersion = "15"
	case 60:
		javaVersion = "16"
	case 61:
		javaVersion = "17"
	case 62:
		javaVersion = "18"
	case 63:
		javaVersion = "19"
	case 64:
		javaVersion = "20"
	case 65:
		javaVersion = "21"
	default:
		javaVersion = fmt.Sprintf("Unknown (Major version: %d)", classObj.MajorVersion)
	}

	return javaVersion, nil
}
