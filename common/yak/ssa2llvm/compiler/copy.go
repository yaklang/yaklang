package compiler

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func CopyFilePreserveMode(src, dst string) error {
	if strings.TrimSpace(dst) == "" {
		return fmt.Errorf("copy output failed: empty destination path")
	}
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)
	if src == dst {
		return nil
	}
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("copy output failed: stat source: %w", err)
	}
	if srcInfo.IsDir() {
		return fmt.Errorf("copy output failed: source is a directory: %s", src)
	}
	if dstInfo, err := os.Stat(dst); err == nil {
		// Prevent truncating the file when src and dst resolve to the same inode
		// (symlinks, ./ prefix, etc).
		if os.SameFile(srcInfo, dstInfo) {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("copy output failed: create output dir: %w", err)
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("copy output failed: open source: %w", err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("copy output failed: create destination: %w", err)
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("copy output failed: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("copy output failed: close destination: %w", closeErr)
	}
	_ = os.Chmod(dst, srcInfo.Mode())
	return nil
}
