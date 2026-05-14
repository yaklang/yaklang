package filesys

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

const (
	archiveFormatZip   = "zip"
	archiveFormatTar   = "tar"
	archiveFormatTarGz = "tar.gz"
)

// NewArchiveFSFromLocal creates a read-only filesystem from a local archive file.
// Supported archive formats are zip, tar, tar.gz and tgz.
func NewArchiveFSFromLocal(archivePath string) (fi.FileSystem, error) {
	format := detectArchiveFormatFromPath(archivePath)
	switch format {
	case archiveFormatZip:
		return NewZipFSFromLocal(archivePath)
	case archiveFormatTarGz:
		data, err := os.ReadFile(archivePath)
		if err != nil {
			return nil, utils.Wrapf(err, "read archive failed: %s", archivePath)
		}
		return NewGzipFSFromBytes(data)
	case archiveFormatTar:
		data, err := os.ReadFile(archivePath)
		if err != nil {
			return nil, utils.Wrapf(err, "read archive failed: %s", archivePath)
		}
		return NewTarFSFromBytes(data)
	default:
		return nil, utils.Errorf("unsupported archive format: %s", archivePath)
	}
}

// NewTarFSFromBytes restores a tar-encoded filesystem into a VirtualFS-backed FS.
func NewTarFSFromBytes(data []byte) (*VirtualFS, error) {
	return newTarFSFromReader(bytes.NewReader(data))
}

func newTarFSFromReader(reader io.Reader) (*VirtualFS, error) {
	fsys := NewVirtualFs()
	tarReader := tar.NewReader(reader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return fsys, nil
		}
		if err != nil {
			return nil, utils.Wrap(err, "read tar header failed")
		}

		name, err := normalizeArchiveFSPath(header.Name)
		if err != nil {
			return nil, err
		}
		if name == "" {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			fsys.AddDir(name)
		case tar.TypeReg, tar.TypeRegA:
			content, readErr := io.ReadAll(tarReader)
			if readErr != nil {
				return nil, utils.Wrapf(readErr, "read tar file content failed: %s", name)
			}
			fsys.AddFile(name, string(content))
		case tar.TypeXHeader, tar.TypeXGlobalHeader:
			continue
		default:
			return nil, utils.Errorf("unsupported tar entry type %d: %s", header.Typeflag, header.Name)
		}
	}
}

func detectArchiveFormatFromPath(archivePath string) string {
	lowerPath := strings.ToLower(strings.TrimSpace(archivePath))
	switch {
	case strings.HasSuffix(lowerPath, ".tar.gz"), strings.HasSuffix(lowerPath, ".tgz"):
		return archiveFormatTarGz
	case strings.HasSuffix(lowerPath, ".tar"):
		return archiveFormatTar
	case strings.HasSuffix(lowerPath, ".zip"):
		return archiveFormatZip
	default:
		return strings.TrimPrefix(strings.ToLower(filepath.Ext(lowerPath)), ".")
	}
}

func normalizeArchiveFSPath(name string) (string, error) {
	cleaned := strings.TrimSpace(name)
	if cleaned == "" || cleaned == "." || cleaned == "/" {
		return "", nil
	}
	cleaned = strings.ReplaceAll(cleaned, "\\", "/")
	cleaned = strings.TrimPrefix(cleaned, "./")
	cleaned = strings.TrimPrefix(cleaned, "/")
	cleaned = path.Clean(cleaned)
	if cleaned == "." {
		return "", nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", utils.Errorf("invalid archive path: %s", name)
	}
	return cleaned, nil
}
