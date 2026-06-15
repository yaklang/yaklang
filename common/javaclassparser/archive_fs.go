package javaclassparser

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// JarRecursiveParseEnabled reports whether jar recursive parse is enabled.
// nil defaults to true, matching SSA jar_recursive_parse semantics.
func JarRecursiveParseEnabled(enabled *bool) bool {
	if enabled == nil {
		return true
	}
	return *enabled
}

// JarRecursiveParseEnabledFromString parses jar_recursive_parse query values.
// Empty or invalid values default to true.
func JarRecursiveParseEnabledFromString(raw string) bool {
	if raw == "" {
		return true
	}
	enabled, err := strconv.ParseBool(raw)
	if err != nil {
		return true
	}
	return enabled
}

// IsArchiveFile reports whether path points to a Java archive file on disk or inside an archive.
func IsArchiveFile(path string) bool {
	return isArchiveFile(path)
}

// NewExpandedLocalFileSystem wraps the OS local filesystem so archive files
// (.jar/.war/.ear/.par/.zip) behave as directories and .class entries are
// served as decompiled Java source.
func NewExpandedLocalFileSystem() fi.FileSystem {
	return NewExpandedLocalFileSystemWithOptions(true)
}

// NewExpandedLocalFileSystemWithOptions is like NewExpandedLocalFileSystem but
// controls nested-jar recursion inside expanded archives.
func NewExpandedLocalFileSystemWithOptions(recursiveParse bool) fi.FileSystem {
	return NewExpandedZipFSWithOptions(filesys.NewLocalFs(), nil, recursiveParse)
}

// NewLocalFileSystemForJarRecursiveParse matches SSA compile semantics for a local
// directory code source: when jarRecursiveParse is false, archives stay opaque files;
// when true, archives are expanded as directories with nested-jar recursion enabled.
func NewLocalFileSystemForJarRecursiveParse(jarRecursiveParse bool) fi.FileSystem {
	if !jarRecursiveParse {
		return filesys.NewLocalFs()
	}
	return NewExpandedLocalFileSystemWithOptions(true)
}

// MaybeWrapExpandedArchiveFS wraps fs when it contains .jar/.war/.zip entries so archives
// are treated as directories (with optional nested-jar recursion).
func MaybeWrapExpandedArchiveFS(fs fi.FileSystem, recursiveParse bool) fi.FileSystem {
	if fs == nil {
		return nil
	}

	var hasArchive bool
	_ = filesys.Recursive(".", filesys.WithFileSystem(fs), filesys.WithFileStat(func(path string, info os.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		if isArchiveFile(path) {
			hasArchive = true
			return filepath.SkipAll
		}
		return nil
	}))
	if !hasArchive {
		return fs
	}

	return NewExpandedZipFSWithOptions(fs, extractZipFSFromFS(fs), recursiveParse)
}

func extractZipFSFromFS(fs fi.FileSystem) *filesys.ZipFS {
	for fs != nil {
		switch v := fs.(type) {
		case *filesys.UnifiedFS:
			fs = v.GetFileSystem()
		case *filesys.ZipFS:
			return v
		default:
			return nil
		}
	}
	return nil
}

func (e *ExpandedZipFS) readArchiveBytes(archivePath string) ([]byte, error) {
	if e.zipFS != nil {
		if data, err := e.zipFS.ReadFile(archivePath); err == nil {
			return data, nil
		}
	}
	// Nested archive path such as test.war/WEB-INF/lib/foo.jar — read from parent archive,
	// not from the host filesystem (that path does not exist on disk).
	if isArchivePath(archivePath) {
		return e.readFileFromArchive(archivePath)
	}
	return e.underlying.ReadFile(archivePath)
}
