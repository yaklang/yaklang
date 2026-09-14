package scannode

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxInlineCodeSourceSnapshotBytes = 64 * 1024

// captureSourceSnapshot only reads node-owned workspace files, never model data.
// An omitted snapshot means the complete file cannot be represented inline.
func (w *legionCodeWorkspaceRuntime) captureSourceSnapshot(file string) (*aiFocusCodeSourceSnapshot, error) {
	resolved, rel, err := w.resolve(file)
	if err != nil {
		return nil, err
	}
	current := w.root
	var sourceInfo os.FileInfo
	for _, component := range strings.Split(rel, "/") {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return nil, err
		}
		sourceInfo = info
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("source snapshot path cannot contain symbolic links")
		}
	}
	if !sourceInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("source snapshot requires a regular file")
	}
	input, err := os.Open(resolved)
	if err != nil {
		return nil, err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || !os.SameFile(sourceInfo, info) {
		return nil, fmt.Errorf("source snapshot requires a regular file")
	}
	content, err := io.ReadAll(io.LimitReader(input, maxInlineCodeSourceSnapshotBytes+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxInlineCodeSourceSnapshotBytes || !utf8.Valid(content) || strings.ContainsRune(string(content), '\x00') {
		return nil, nil
	}
	return &aiFocusCodeSourceSnapshot{Path: rel, Content: string(content), SHA256: fmt.Sprintf("%x", sha256.Sum256(content))}, nil
}

func validateLegionCodeSourceSnapshot(snapshot *aiFocusCodeSourceSnapshot, file string) error {
	if snapshot == nil {
		return nil
	}
	cleaned, err := cleanLegionCodeRelativePath(snapshot.Path, false)
	if err != nil || cleaned != snapshot.Path || snapshot.Path != file {
		return fmt.Errorf("ai code finding source snapshot path must match file")
	}
	if len(snapshot.Content) > maxInlineCodeSourceSnapshotBytes || !utf8.ValidString(snapshot.Content) || strings.ContainsRune(snapshot.Content, '\x00') {
		return fmt.Errorf("ai code finding source snapshot must be UTF-8 text within %d bytes", maxInlineCodeSourceSnapshotBytes)
	}
	if snapshot.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(snapshot.Content))) {
		return fmt.Errorf("ai code finding source snapshot sha256 mismatch")
	}
	return nil
}
