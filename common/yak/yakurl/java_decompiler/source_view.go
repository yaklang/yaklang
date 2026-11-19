package java_decompiler

import (
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	javaclassparser "github.com/yaklang/javajive/classparser"
)

// sourceEntry is one directory entry in the decompiled-source view.
//
// YakURL exposes the decompiler filesystem, not the raw archive layout. Every
// bytecode entry is presented as a .java file, so listing, /class and /export
// agree on one name for one piece of content.
type sourceEntry struct {
	// Name is the display name: X.class becomes X.java.
	Name string
	// Path is the display path below the listed directory.
	Path string
	// ClassPath is the raw .class entry backing this source entry. It is empty
	// for directories, plain files and real Java sources.
	ClassPath string
	Dir       bool
	Info      fs.FileInfo
}

// listSourceEntries lists one directory in the decompiled-source view.
//
// A real .java file and a .class file can share a base name (tools.jar ships
// both). Only one entry is listed; the bytecode entry wins so that its content
// is always the decompiler output.
func (cs *codeSource) listSourceEntries(dir string) ([]sourceEntry, error) {
	entries, err := cs.listDirectory(dir)
	if err != nil {
		return nil, err
	}

	out := make([]sourceEntry, 0, len(entries))
	index := make(map[string]int, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}

		rawName := javaclassparser.EntryOriginalName(entry)
		if rawName == "" {
			rawName = entry.Name()
		}

		item := sourceEntry{
			Name: entry.Name(),
			Dir:  entry.IsDir(),
			Info: info,
		}
		if !item.Dir && isClassEntryName(rawName) {
			item.Name = rawName[:len(rawName)-len(".class")] + ".java"
			item.ClassPath = path.Join(dir, rawName)
		}
		item.Path = path.Join(dir, item.Name)

		if prev, ok := index[item.Name]; ok {
			if out[prev].ClassPath == "" && item.ClassPath != "" {
				out[prev] = item
			}
			continue
		}
		index[item.Name] = len(out)
		out = append(out, item)
	}
	return out, nil
}

func isClassEntryName(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".class")
}

// sourceClassPath resolves a caller-visible path to the .class entry that
// backs it. It returns "" when the path is not bytecode-backed, e.g. a real
// .java source or a plain resource.
func (cs *codeSource) sourceClassPath(p string) string {
	key := cs.pathKey(p)
	lower := strings.ToLower(key)
	if strings.HasSuffix(lower, ".class") {
		return key
	}
	if !strings.HasSuffix(lower, ".java") {
		return ""
	}
	classPath := key[:len(key)-len(".java")] + ".class"
	if cs.entryExists(classPath) {
		return classPath
	}
	return ""
}

// pathKey keeps absolute directory-root paths absolute and normalizes archive
// internal paths the same way the rest of the package does.
func (cs *codeSource) pathKey(p string) string {
	if cs.isDirectory {
		clean := filepath.Clean(p)
		if filepath.IsAbs(clean) {
			return clean
		}
	}
	return normalizeJarInternalPath(p)
}

func (cs *codeSource) entryExists(p string) bool {
	_, err := cs.expandedFS.Stat(cs.resolveFSPath(p))
	return err == nil
}
