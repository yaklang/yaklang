package aiskillloader

import (
	"bytes"
	"fmt"
	"io/fs"
	"sort"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

const (
	// FileTreeFullLimit is the maximum bytes for a full filesystem tree display.
	FileTreeFullLimit = 4096
	// FileTreeFoldedLimit is the maximum bytes for a folded filesystem tree display.
	FileTreeFoldedLimit = 1024
)

// RenderFileSystemTree renders a filesystem tree string from a FileSystem.
// The output is truncated to maxBytes with "..." appended if exceeded.
func RenderFileSystemTree(fsys fi.FileSystem, maxBytes int) string {
	var buf bytes.Buffer
	renderDir(&buf, fsys, ".", "", maxBytes)

	result := buf.String()
	if len(result) > maxBytes {
		// Truncate to maxBytes and append ellipsis
		result = result[:maxBytes-4] + "\n..."
	}
	return result
}

// RenderFileSystemTreeFull renders a full filesystem tree (up to 4096 bytes).
func RenderFileSystemTreeFull(fsys fi.FileSystem) string {
	return RenderFileSystemTree(fsys, FileTreeFullLimit)
}

// RenderFileSystemTreeFolded renders a folded filesystem tree (up to 1024 bytes).
func RenderFileSystemTreeFolded(fsys fi.FileSystem) string {
	return RenderFileSystemTree(fsys, FileTreeFoldedLimit)
}

// renderDir recursively renders directory entries into the buffer.
func renderDir(buf *bytes.Buffer, fsys fi.FileSystem, dirPath string, prefix string, maxBytes int) {
	if buf.Len() >= maxBytes {
		return
	}

	entries, err := fsys.ReadDir(dirPath)
	if err != nil {
		return
	}

	// Sort entries: directories first, then files, alphabetically within each group
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})

	for i, entry := range entries {
		if buf.Len() >= maxBytes {
			buf.WriteString(prefix + "...\n")
			return
		}

		isLast := i == len(entries)-1
		connector := "├── "
		childPrefix := prefix + "│   "
		if isLast {
			connector = "└── "
			childPrefix = prefix + "    "
		}

		if entry.IsDir() {
			buf.WriteString(fmt.Sprintf("%s%s%s/\n", prefix, connector, entry.Name()))
			childPath := fsys.Join(dirPath, entry.Name())
			renderDir(buf, fsys, childPath, childPrefix, maxBytes)
		} else {
			sizeStr := formatFileSize(entry)
			buf.WriteString(fmt.Sprintf("%s%s%s%s\n", prefix, connector, entry.Name(), sizeStr))
		}
	}
}

// formatFileSize returns a human-readable size string for a file entry.
func formatFileSize(entry fs.DirEntry) string {
	info, err := entry.Info()
	if err != nil {
		return ""
	}
	size := info.Size()
	if size < 1024 {
		return fmt.Sprintf(" (%dB)", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf(" (%.1fKB)", float64(size)/1024)
	}
	return fmt.Sprintf(" (%.1fMB)", float64(size)/(1024*1024))
}
