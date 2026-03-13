package runtimeembed

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

const (
	embeddedPrefix      = "ssa2llvm-runtime"
	embeddedArchivePath = embeddedPrefix + "/libyak.a"
	embeddedGCLibPath   = embeddedPrefix + "/libgc.a"
	archiveBaseName     = "libyak.a"
	gcBaseName          = "libgc.a"

	embeddedSrcPrefix = "ssa2llvm-runtime-src"
)

var ErrNoEmbeddedRuntime = errors.New("embedded ssa2llvm runtime is not available (build with -tags ssa2llvm_gzip_embed and generate ssa2llvm-runtime.tar.gz)")
var ErrNoEmbeddedRuntimeSource = errors.New("embedded ssa2llvm runtime source is not available (build with -tags ssa2llvm_gzip_embed and generate ssa2llvm-runtime-src.tar.gz)")

type readFileFS interface {
	ReadFile(name string) ([]byte, error)
}

type readDirFileFS interface {
	ReadFile(name string) ([]byte, error)
	ReadDir(name string) ([]fs.DirEntry, error)
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

func ExtractLibgcToDirFromFS(fs readFileFS, dstDir string) (string, error) {
	if fs == nil {
		return "", utils.Errorf("extract libgc failed: fs is nil")
	}
	dstDir = strings.TrimSpace(dstDir)
	if dstDir == "" {
		return "", utils.Errorf("extract libgc failed: empty dstDir")
	}

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return "", utils.Errorf("extract libgc failed: mkdir %s: %v", dstDir, err)
	}

	data, err := fs.ReadFile(embeddedGCLibPath)
	if err != nil {
		return "", utils.Errorf("extract libgc failed: read %s: %v", embeddedGCLibPath, err)
	}

	outPath := filepath.Join(dstDir, gcBaseName)
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return "", utils.Errorf("extract libgc failed: write %s: %v", outPath, err)
	}
	return outPath, nil
}

func ExtractLibgcToDir(dstDir string) (string, error) {
	fs, ok := embeddedRuntimeFS()
	if !ok {
		return "", ErrNoEmbeddedRuntime
	}
	return ExtractLibgcToDirFromFS(fs, dstDir)
}

func ExtractRuntimeSourceToDirFromFS(fs readDirFileFS, dstDir string) (string, error) {
	if fs == nil {
		return "", utils.Errorf("extract runtime source failed: fs is nil")
	}
	dstDir = strings.TrimSpace(dstDir)
	if dstDir == "" {
		return "", utils.Errorf("extract runtime source failed: empty dstDir")
	}

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return "", utils.Errorf("extract runtime source failed: mkdir %s: %v", dstDir, err)
	}

	if err := copyTree(fs, embeddedSrcPrefix, dstDir); err != nil {
		return "", err
	}
	return dstDir, nil
}

func ExtractRuntimeSourceToDir(dstDir string) (string, error) {
	fs, ok := embeddedRuntimeSourceFS()
	if !ok {
		return "", ErrNoEmbeddedRuntimeSource
	}
	return ExtractRuntimeSourceToDirFromFS(fs, dstDir)
}

func copyTree(fs readDirFileFS, srcDir, dstDir string) error {
	entries, err := fs.ReadDir(srcDir)
	if err != nil {
		return utils.Errorf("extract runtime source failed: readdir %s: %v", srcDir, err)
	}
	for _, ent := range entries {
		name := ent.Name()
		if strings.TrimSpace(name) == "" {
			continue
		}
		srcPath := path.Join(srcDir, name)
		dstPath := filepath.Join(dstDir, name)
		if ent.IsDir() {
			if err := os.MkdirAll(dstPath, 0o755); err != nil {
				return utils.Errorf("extract runtime source failed: mkdir %s: %v", dstPath, err)
			}
			if err := copyTree(fs, srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		data, err := fs.ReadFile(srcPath)
		if err != nil {
			return utils.Errorf("extract runtime source failed: read %s: %v", srcPath, err)
		}
		if err := os.WriteFile(dstPath, data, 0o644); err != nil {
			return utils.Errorf("extract runtime source failed: write %s: %v", dstPath, err)
		}
	}
	return nil
}
