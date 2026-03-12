package filesys

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type gzipSerializeConfig struct {
	excludePaths map[string]struct{}
}

type GzipSerializeOption func(*gzipSerializeConfig)

// WithGzipFSExcludePaths excludes specific paths from serialized tar.gz output.
func WithGzipFSExcludePaths(paths ...string) GzipSerializeOption {
	return func(cfg *gzipSerializeConfig) {
		if cfg.excludePaths == nil {
			cfg.excludePaths = make(map[string]struct{})
		}
		for _, p := range paths {
			cleaned := normalizeGzipFSPath(p)
			if cleaned == "" {
				continue
			}
			cfg.excludePaths[cleaned] = struct{}{}
		}
	}
}

// GzipFS is a runtime tar.gz-backed filesystem materialized into a VirtualFS.
// It is primarily used to persist and restore small embedded file trees.
type GzipFS struct {
	*VirtualFS
	raw []byte
}

// NewGzipFSFromBytes restores a tar.gz-encoded filesystem into a VirtualFS-backed FS.
func NewGzipFSFromBytes(data []byte) (*GzipFS, error) {
	fsys := &GzipFS{
		VirtualFS: NewVirtualFs(),
		raw:       append([]byte(nil), data...),
	}
	if len(data) == 0 {
		return fsys, nil
	}

	gzReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, utils.Wrap(err, "create gzip reader failed")
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, utils.Wrap(err, "read tar header failed")
		}

		name := normalizeGzipFSPath(header.Name)
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
		}
	}

	return fsys, nil
}

// SerializeFileSystemToGzipBytes serializes a filesystem into tar.gz bytes.
func SerializeFileSystemToGzipBytes(fsys fi.FileSystem, opts ...GzipSerializeOption) ([]byte, error) {
	if fsys == nil {
		return nil, utils.Error("filesystem is nil")
	}

	cfg := &gzipSerializeConfig{excludePaths: make(map[string]struct{})}
	for _, opt := range opts {
		opt(cfg)
	}

	buf := new(bytes.Buffer)
	gzWriter := gzip.NewWriter(buf)
	tarWriter := tar.NewWriter(gzWriter)
	hasEntries := false
	for _, root := range []string{".", "", "/"} {
		entries, _ := fsys.ReadDir(root)
		if len(entries) > 0 {
			hasEntries = true
			break
		}
	}
	if !hasEntries {
		if err := tarWriter.Close(); err != nil {
			return nil, utils.Wrap(err, "close tar writer failed")
		}
		if err := gzWriter.Close(); err != nil {
			return nil, utils.Wrap(err, "close gzip writer failed")
		}
		return buf.Bytes(), nil
	}

	closeWithErr := func(err error) ([]byte, error) {
		_ = tarWriter.Close()
		_ = gzWriter.Close()
		return nil, err
	}

	written := make(map[string]struct{})
	err := Recursive(".",
		WithFileSystem(fsys),
		WithStat(func(isDir bool, pathname string, info fs.FileInfo) error {
			name := normalizeGzipFSPath(pathname)
			if name == "" || shouldSkipGzipFSPath(name, cfg.excludePaths) {
				return nil
			}
			if _, ok := written[name]; ok {
				return nil
			}

			if isDir {
				header := &tar.Header{
					Format:     tar.FormatPAX,
					Name:       name + "/",
					Mode:       int64(info.Mode().Perm()),
					ModTime:    time.Unix(0, 0),
					AccessTime: time.Unix(0, 0),
					ChangeTime: time.Unix(0, 0),
					Typeflag:   tar.TypeDir,
				}
				if err := tarWriter.WriteHeader(header); err != nil {
					return utils.Wrapf(err, "write tar directory header failed: %s", name)
				}
				written[name] = struct{}{}
				return nil
			}

			content, err := fsys.ReadFile(pathname)
			if err != nil {
				return utils.Wrapf(err, "read file content failed: %s", pathname)
			}
			header := &tar.Header{
				Format:     tar.FormatPAX,
				Name:       name,
				Mode:       int64(info.Mode().Perm()),
				Size:       int64(len(content)),
				ModTime:    time.Unix(0, 0),
				AccessTime: time.Unix(0, 0),
				ChangeTime: time.Unix(0, 0),
				Typeflag:   tar.TypeReg,
			}
			if err := tarWriter.WriteHeader(header); err != nil {
				return utils.Wrapf(err, "write tar file header failed: %s", name)
			}
			if _, err := tarWriter.Write(content); err != nil {
				return utils.Wrapf(err, "write tar file content failed: %s", name)
			}
			written[name] = struct{}{}
			return nil
		}),
	)
	if err != nil {
		return closeWithErr(err)
	}

	if err := tarWriter.Close(); err != nil {
		return nil, utils.Wrap(err, "close tar writer failed")
	}
	if err := gzWriter.Close(); err != nil {
		return nil, utils.Wrap(err, "close gzip writer failed")
	}
	return buf.Bytes(), nil
}

func normalizeGzipFSPath(name string) string {
	cleaned := strings.TrimSpace(name)
	if cleaned == "" || cleaned == "." || cleaned == "/" {
		return ""
	}
	cleaned = strings.ReplaceAll(cleaned, "\\", "/")
	cleaned = strings.TrimPrefix(cleaned, "./")
	cleaned = strings.TrimPrefix(cleaned, "/")
	cleaned = path.Clean(cleaned)
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func shouldSkipGzipFSPath(name string, excludes map[string]struct{}) bool {
	if len(excludes) == 0 {
		return false
	}
	for excluded := range excludes {
		if name == excluded || strings.HasPrefix(name, excluded+"/") {
			return true
		}
	}
	return false
}

var _ fi.FileSystem = (*GzipFS)(nil)
