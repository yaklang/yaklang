//go:build hids && linux

package enrich

import (
	"crypto/md5"
	"crypto/sha256"
	"debug/elf"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/h2non/filetype"

	"github.com/yaklang/yaklang/common/hids/model"
)

type ArtifactSnapshotOptions struct {
	CaptureHashes bool
}

func SnapshotArtifact(path string, options ArtifactSnapshotOptions) (*model.Artifact, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &model.Artifact{
				Path:      path,
				Exists:    false,
				Extension: normalizeArtifactExtension(filepath.Ext(path)),
			}, nil
		}
		return nil, err
	}

	artifact := &model.Artifact{
		Path:      path,
		Exists:    true,
		SizeBytes: info.Size(),
		Extension: normalizeArtifactExtension(filepath.Ext(path)),
	}

	if info.IsDir() {
		artifact.FileType = "directory"
		artifact.TypeSource = "fs"
		return artifact, nil
	}
	if !info.Mode().IsRegular() {
		artifact.FileType = normalizeArtifactModeType(info.Mode())
		artifact.TypeSource = "fs"
		return artifact, nil
	}

	sample, err := readArtifactSample(path, 4096)
	if err != nil {
		return artifact, err
	}
	artifact.Magic = encodeArtifactMagic(sample)

	kind, mimeType := detectArtifactKind(sample)
	switch {
	case kind != "":
		artifact.FileType = kind
		artifact.TypeSource = "magic"
	case artifact.Extension != "":
		artifact.FileType = artifact.Extension
		artifact.TypeSource = "extension"
	default:
		artifact.FileType = "unknown"
		artifact.TypeSource = "unknown"
	}
	artifact.MimeType = mimeType

	if isELFArtifact(sample) {
		artifact.FileType = "elf"
		artifact.TypeSource = "magic"
		if elfArtifact, elfErr := snapshotELFArtifact(path); elfErr == nil {
			artifact.ELF = elfArtifact
		} else if err == nil {
			err = elfErr
		}
	}

	if options.CaptureHashes {
		hashes, hashErr := snapshotArtifactHashes(path)
		if hashErr == nil {
			artifact.Hashes = hashes
		} else if err == nil {
			err = hashErr
		}
	}

	return artifact, err
}

func detectArtifactKind(sample []byte) (string, string) {
	if len(sample) == 0 {
		return "", ""
	}
	if kind, err := filetype.Match(sample); err == nil && kind != filetype.Unknown {
		return normalizeArtifactKind(kind.Extension), strings.TrimSpace(kind.MIME.Value)
	}
	return "", strings.TrimSpace(http.DetectContentType(sample))
}

func normalizeArtifactKind(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "":
		return ""
	case "x-executable", "x-sharedlib":
		return "elf"
	default:
		return value
	}
}

func normalizeArtifactExtension(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(value, ".")
	return value
}

func normalizeArtifactModeType(mode os.FileMode) string {
	switch {
	case mode&os.ModeNamedPipe != 0:
		return "fifo"
	case mode&os.ModeSocket != 0:
		return "socket"
	case mode&os.ModeDevice != 0:
		return "device"
	case mode&os.ModeSymlink != 0:
		return "symlink"
	default:
		return "unknown"
	}
}

func readArtifactSample(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open artifact %s: %w", path, err)
	}
	defer file.Close()

	if limit <= 0 {
		limit = 4096
	}
	buf := make([]byte, limit)
	n, readErr := io.ReadFull(file, buf)
	switch {
	case readErr == nil:
		return buf[:n], nil
	case readErr == io.EOF, readErr == io.ErrUnexpectedEOF:
		return buf[:n], nil
	default:
		return nil, fmt.Errorf("read artifact sample %s: %w", path, readErr)
	}
}

func encodeArtifactMagic(sample []byte) string {
	if len(sample) == 0 {
		return ""
	}
	limit := len(sample)
	if limit > 8 {
		limit = 8
	}
	return hex.EncodeToString(sample[:limit])
}

func isELFArtifact(sample []byte) bool {
	return len(sample) >= 4 &&
		sample[0] == 0x7f &&
		sample[1] == 'E' &&
		sample[2] == 'L' &&
		sample[3] == 'F'
}

func snapshotArtifactHashes(path string) (*model.ArtifactHashes, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open artifact hashes %s: %w", path, err)
	}
	defer file.Close()

	md5Hash := md5.New()
	sha256Hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(md5Hash, sha256Hash), file); err != nil {
		return nil, fmt.Errorf("compute artifact hashes %s: %w", path, err)
	}
	return &model.ArtifactHashes{
		SHA256: hex.EncodeToString(sha256Hash.Sum(nil)),
		MD5:    hex.EncodeToString(md5Hash.Sum(nil)),
	}, nil
}

func snapshotELFArtifact(path string) (*model.ELFArtifact, error) {
	file, err := elf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open elf artifact %s: %w", path, err)
	}
	defer file.Close()

	sections := make([]string, 0, len(file.Sections))
	for _, section := range file.Sections {
		if section == nil {
			continue
		}
		name := strings.TrimSpace(section.Name)
		if name == "" {
			continue
		}
		sections = append(sections, name)
	}
	segments := make([]string, 0, len(file.Progs))
	for _, prog := range file.Progs {
		if prog == nil {
			continue
		}
		name := strings.TrimSpace(prog.Type.String())
		if name == "" {
			continue
		}
		segments = append(segments, name)
	}

	return &model.ELFArtifact{
		Class:        strings.TrimSpace(file.Class.String()),
		Machine:      strings.TrimSpace(file.Machine.String()),
		ByteOrder:    artifactByteOrder(file.ByteOrder),
		EntryAddress: fmt.Sprintf("0x%x", file.Entry),
		SectionCount: len(file.Sections),
		SegmentCount: len(file.Progs),
		Sections:     sections,
		Segments:     segments,
	}, nil
}

func artifactByteOrder(order binary.ByteOrder) string {
	switch order {
	case binary.LittleEndian:
		return "little-endian"
	case binary.BigEndian:
		return "big-endian"
	default:
		if order == nil {
			return ""
		}
		return fmt.Sprintf("%T", order)
	}
}
