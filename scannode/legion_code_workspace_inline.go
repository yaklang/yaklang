package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	legionInlineSourceMaxFiles      = 16
	legionInlineSourceMaxFileBytes  = 64 * 1024
	legionInlineSourceMaxTotalBytes = 256 * 1024
	legionInlineSourceMaxPathBytes  = 512
)

func cloneLegionInlineFiles(files map[string]string) map[string]string {
	if files == nil {
		return nil
	}
	cloned := make(map[string]string, len(files))
	for path, content := range files {
		cloned[path] = content
	}
	return cloned
}

func cleanLegionRuleSourcePath(raw string) (string, error) {
	if len(raw) > legionInlineSourceMaxPathBytes || raw != strings.TrimSpace(raw) || strings.Contains(raw, ":") || !utf8.ValidString(raw) {
		return "", fmt.Errorf("source path must be a bounded canonical relative path")
	}
	for _, ch := range raw {
		if unicode.IsControl(ch) {
			return "", fmt.Errorf("source path must not contain control characters")
		}
	}
	cleaned, err := cleanLegionCodeRelativePath(raw, false)
	if err != nil {
		return "", err
	}
	for _, part := range strings.Split(cleaned, "/") {
		if strings.EqualFold(part, ".git") {
			return "", fmt.Errorf("source path must not access Git metadata")
		}
	}
	return cleaned, nil
}

func validateLegionInlineFiles(files map[string]string, allowEmpty bool) error {
	if len(files) > legionInlineSourceMaxFiles || (!allowEmpty && len(files) == 0) {
		minimum := 1
		if allowEmpty {
			minimum = 0
		}
		return fmt.Errorf("inline sources must contain between %d and %d files", minimum, legionInlineSourceMaxFiles)
	}
	total := 0
	seen := make(map[string]struct{}, len(files))
	for raw, content := range files {
		path, err := cleanLegionRuleSourcePath(raw)
		if err != nil {
			return err
		}
		if len(content) > legionInlineSourceMaxFileBytes || !utf8.ValidString(content) || strings.ContainsRune(content, '\x00') {
			return fmt.Errorf("inline source %q must be UTF-8 text of at most %d bytes", path, legionInlineSourceMaxFileBytes)
		}
		total += len(content)
		if total > legionInlineSourceMaxTotalBytes {
			return fmt.Errorf("inline sources exceed %d bytes", legionInlineSourceMaxTotalBytes)
		}
		key := strings.ToLower(path)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("inline sources contain colliding paths")
		}
		seen[key] = struct{}{}
	}
	for name := range seen {
		parts := strings.Split(name, "/")
		for i := 1; i < len(parts); i++ {
			if _, exists := seen[strings.Join(parts[:i], "/")]; exists {
				return fmt.Errorf("inline sources contain a file/directory collision")
			}
		}
	}
	return nil
}

// Hash the canonical relative path and exact UTF-8 bytes for every file. Source
// bytes cannot contain NUL, so the separators make this encoding unambiguous.
func legionInlineSourceDigest(files map[string]string) string {
	hash := sha256.New()
	for _, path := range legionSortedSourcePaths(files) {
		_, _ = io.WriteString(hash, path+"\x00"+files[path]+"\x00")
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func legionSortedSourcePaths(files map[string]string) []string {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func materializeLegionInlineWorkspace(ctx context.Context, spec legionCodeWorkspaceSpec) (_ *legionCodeWorkspaceRuntime, finalErr error) {
	if err := normalizeLegionCodeWorkspaceSpec(&spec); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp("", "legion-inline-source-*")
	if err != nil {
		return nil, fmt.Errorf("create private inline source workspace: %w", err)
	}
	cleanup := func() error { return os.RemoveAll(root) }
	defer func() {
		if finalErr != nil {
			_ = cleanup()
		}
	}()
	var total int64
	for _, path := range legionSortedSourcePaths(spec.InlineFiles) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		destination := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			return nil, err
		}
		// A fresh private directory and validated non-colliding paths cannot
		// resolve through an existing symlink or overwrite another file.
		file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0400)
		if err != nil {
			return nil, err
		}
		_, writeErr := io.WriteString(file, spec.InlineFiles[path])
		closeErr := file.Close()
		if writeErr != nil {
			return nil, writeErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		total += int64(len(spec.InlineFiles[path]))
	}
	digest := legionInlineSourceDigest(spec.InlineFiles)
	return &legionCodeWorkspaceRuntime{
		spec: publicLegionCodeWorkspaceSpec(spec), root: root,
		inlineFiles:    cloneLegionInlineFiles(spec.InlineFiles),
		lockedRevision: digest, sha256: digest,
		files: len(spec.InlineFiles), bytes: total, cleanup: cleanup,
	}, nil
}
