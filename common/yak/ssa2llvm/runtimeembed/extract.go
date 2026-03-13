package runtimeembed

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

const (
	embeddedPrefix      = "ssa2llvm-runtime"
	embeddedArchivePath = embeddedPrefix + "/libyak.a"
	archiveBaseName     = "libyak.a"
)

var ErrNoEmbeddedRuntime = errors.New("embedded ssa2llvm runtime is not available (build with -tags gzip_embed and generate ssa2llvm-runtime.tar.gz)")

type readFileFS interface {
	ReadFile(name string) ([]byte, error)
}

func ExtractLibyakToDirFromFS(fs readFileFS, dstDir string) (string, error) {
	if fs == nil {
		return "", utils.Errorf("extract runtime archive failed: fs is nil")
	}
	dstDir = strings.TrimSpace(dstDir)
	if dstDir == "" {
		return "", utils.Errorf("extract runtime archive failed: empty dstDir")
	}

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return "", utils.Errorf("extract runtime archive failed: mkdir %s: %v", dstDir, err)
	}

	data, err := fs.ReadFile(embeddedArchivePath)
	if err != nil {
		return "", utils.Errorf("extract runtime archive failed: read %s: %v", embeddedArchivePath, err)
	}

	outPath := filepath.Join(dstDir, archiveBaseName)
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return "", utils.Errorf("extract runtime archive failed: write %s: %v", outPath, err)
	}
	return outPath, nil
}

func ExtractLibyakToDir(dstDir string) (string, error) {
	fs, ok := embeddedRuntimeFS()
	if !ok {
		return "", ErrNoEmbeddedRuntime
	}
	return ExtractLibyakToDirFromFS(fs, dstDir)
}
