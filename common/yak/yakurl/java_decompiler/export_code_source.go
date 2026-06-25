package java_decompiler

import (
	"archive/zip"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (a *Action) exportDecompiledCodeSource(rootPath string) (*ypb.RequestYakURLResponse, error) {
	if err := validateCodeSourceRoot(rootPath); err != nil {
		return nil, err
	}
	cs, err := a.resolveCodeSource(rootPath)
	if err != nil {
		return nil, err
	}

	exportedFileName := cs.exportBaseName() + "-decompiled.zip"
	exportDir := filepath.Join(consts.GetDefaultYakitBaseTempDir(), "java-decompiled")
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		return nil, utils.Wrapf(err, "failed to create export dir: %s", exportDir)
	}
	zipFilePath := filepath.Join(exportDir, exportedFileName)
	zipFile, err := os.Create(zipFilePath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to create zip file: %s", zipFilePath)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	walkFS := cs.walkFS()
	walkRoot := cs.walkRoot()

	err = filesys.Recursive(walkRoot, filesys.WithFileSystem(walkFS), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		if info.IsDir() {
			targetPath, err := cs.exportZipEntryPath(s)
			if err != nil {
				return err
			}
			_, err = zipWriter.Create(targetPath + "/")
			return err
		}

		targetPath, err := cs.exportZipEntryPath(s)
		if err != nil {
			return err
		}

		var fileContent []byte
		if strings.HasSuffix(strings.ToLower(s), ".class") {
			decompiled, err := cs.readFileForExport(s)
			if err != nil {
				log.Warnf("Failed to decompile %s: %v", s, err)
				fileContent, err = cs.readRawForExport(s)
				if err != nil {
					return utils.Wrapf(err, "failed to read class file: %s", s)
				}
			} else {
				fileContent = decompiled
				targetPath = strings.TrimSuffix(targetPath, ".class") + ".java"
			}
		} else {
			fileContent, err = cs.readRawForExport(s)
			if err != nil {
				return utils.Wrapf(err, "failed to read file: %s", s)
			}
		}

		entry, err := zipWriter.Create(targetPath)
		if err != nil {
			return utils.Wrapf(err, "failed to create zip entry for: %s", targetPath)
		}
		_, err = entry.Write(fileContent)
		return err
	}))
	if err != nil {
		return nil, utils.Wrapf(err, "failed to process code source files")
	}

	if err := zipWriter.Close(); err != nil {
		return nil, utils.Wrapf(err, "failed to close zip writer")
	}

	zipSize := int64(0)
	if fi, err := zipFile.Stat(); err == nil {
		zipSize = fi.Size()
	}

	resourceURL := &ypb.YakURL{
		Schema: "javaDec",
		Path:   "/export",
		Query: []*ypb.KVPair{
			{Key: "jar", Value: rootPath},
		},
	}

	resource := &ypb.YakURLResource{
		ResourceName:      exportedFileName,
		VerboseName:       exportedFileName,
		ResourceType:      "file",
		VerboseType:       "decompiled-jar-zip",
		Size:              zipSize,
		SizeVerbose:       utils.ByteSize(uint64(zipSize)),
		ModifiedTimestamp: time.Now().Unix(),
		Path:              zipFilePath,
		Url:               resourceURL,
		Extra: []*ypb.KVPair{
			{Key: "path", Value: zipFilePath},
		},
	}

	return &ypb.RequestYakURLResponse{
		Resources: []*ypb.YakURLResource{resource},
		Total:     1,
		Page:      1,
		PageSize:  1,
	}, nil
}

func (cs *codeSource) exportZipEntryPath(absOrRelativePath string) (string, error) {
	if cs.isDirectory {
		rel, err := filepath.Rel(cs.rootPath, filepath.Clean(absOrRelativePath))
		if err != nil {
			return "", err
		}
		return filepath.ToSlash(rel), nil
	}
	return normalizeJarInternalPath(absOrRelativePath), nil
}

func (cs *codeSource) readFileForExport(path string) ([]byte, error) {
	return cs.expandedFS.ReadFile(cs.resolveFSPath(path))
}

func (cs *codeSource) readRawForExport(path string) ([]byte, error) {
	return cs.readFileForExport(path)
}
