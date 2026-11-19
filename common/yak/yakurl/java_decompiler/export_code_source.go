package java_decompiler

import (
	"archive/zip"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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
	written := make(map[string]bool)
	if err := cs.writeExportTree(zipWriter, cs.walkRoot(), written); err != nil {
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

// writeExportTree walks the decompiled-source view and writes one .java file
// per bytecode entry. The walk uses the same entries as /jar, so the exported
// archive cannot disagree with the listing.
func (cs *codeSource) writeExportTree(zipWriter *zip.Writer, dir string, written map[string]bool) error {
	entries, err := cs.listSourceEntries(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		targetPath, err := cs.exportZipEntryPath(entry.Path)
		if err != nil {
			return err
		}

		if entry.Dir {
			if err := writeZipDirectory(zipWriter, targetPath); err != nil {
				return err
			}
			if err := cs.writeExportTree(zipWriter, entry.Path, written); err != nil {
				return err
			}
			continue
		}

		if javaclassparserIsArchiveLeaf(entry.Name) {
			if err := writeZipDirectory(zipWriter, targetPath); err != nil {
				return err
			}
			if err := cs.writeExportTree(zipWriter, entry.Path, written); err != nil {
				return err
			}
			continue
		}

		var content []byte
		if entry.ClassPath != "" {
			targetPath = strings.TrimSuffix(targetPath, path.Ext(targetPath)) + ".java"
			content, err = cs.readClassFile(entry.ClassPath)
			if err != nil {
				log.Warnf("Failed to decompile %s: %v", entry.ClassPath, err)
				content = []byte(exportErrorComment(err))
			}
		} else {
			content, err = cs.expandedFS.ReadFile(cs.resolveFSPath(entry.Path))
			if err != nil {
				return utils.Wrapf(err, "failed to read file: %s", entry.Path)
			}
		}

		if written[targetPath] {
			continue
		}
		written[targetPath] = true
		writer, err := zipWriter.Create(targetPath)
		if err != nil {
			return utils.Wrapf(err, "failed to create zip entry for: %s", targetPath)
		}
		if _, err := writer.Write(content); err != nil {
			return err
		}
	}
	return nil
}

func writeZipDirectory(zipWriter *zip.Writer, targetPath string) error {
	targetPath = strings.TrimSuffix(targetPath, "/") + "/"
	if _, err := zipWriter.Create(targetPath); err != nil {
		return utils.Wrapf(err, "failed to create zip directory: %s", targetPath)
	}
	return nil
}

func exportErrorComment(err error) string {
	return "// decompile failed: " + strings.ReplaceAll(err.Error(), "\n", " ") + "\n"
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
