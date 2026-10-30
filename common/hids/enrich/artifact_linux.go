//go:build hids && linux

package enrich

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"debug/elf"
	"encoding/binary"
	"encoding/hex"
	"errors"
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
	CaptureHashes    bool
	MaxHashBytes     int64
	MaxMetadataBytes int64
}

// The limit applies to complete-file hashes, never to a prefix presented as a
// full hash. Larger files retain their metadata and return an explicit error.
const DefaultArtifactHashMaxBytes int64 = 512 << 20
const DefaultArtifactMetadataMaxBytes int64 = 8 << 20

var ErrArtifactHashLimit = errors.New("artifact exceeds complete-file hash byte limit")
var ErrArtifactMetadataLimit = errors.New("artifact exceeds ELF metadata byte limit")

func SnapshotArtifact(path string, options ArtifactSnapshotOptions) (*model.Artifact, error) {
	return SnapshotArtifactContext(context.Background(), path, options)
}

func SnapshotArtifactContext(ctx context.Context, path string, options ArtifactSnapshotOptions) (*model.Artifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}

	info, err := os.Stat(path)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
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

	sample, err := readArtifactSampleContext(ctx, path, 4096)
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
		if elfArtifact, elfErr := snapshotELFArtifactContext(ctx, path, options.MaxMetadataBytes); elfErr == nil {
			artifact.ELF = elfArtifact
		} else if err == nil {
			err = elfErr
		}
	}

	if options.CaptureHashes {
		hashes, hashErr := snapshotArtifactHashesContext(ctx, path, options.MaxHashBytes)
		if hashErr == nil {
			artifact.Hashes = hashes
		} else if err == nil {
			err = hashErr
		}
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return artifact, ctxErr
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
	return readArtifactSampleContext(context.Background(), path, limit)
}

func readArtifactSampleContext(ctx context.Context, path string, limit int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open artifact %s: %w", path, err)
	}
	defer file.Close()

	if limit <= 0 {
		limit = 4096
	}
	buf := make([]byte, limit)
	n, readErr := io.ReadFull(artifactContextReader{ctx: ctx, reader: file}, buf)
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
	return snapshotArtifactHashesContext(context.Background(), path, 0)
}

func snapshotArtifactHashesContext(ctx context.Context, path string, maxBytes int64) (*model.ArtifactHashes, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if maxBytes <= 0 || maxBytes > DefaultArtifactHashMaxBytes {
		maxBytes = DefaultArtifactHashMaxBytes
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open artifact hashes %s: %w", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxBytes {
		return nil, fmt.Errorf("%w: path=%s size=%d max_bytes=%d", ErrArtifactHashLimit, path, info.Size(), maxBytes)
	}

	md5Hash := md5.New()
	sha256Hash := sha256.New()
	// Allow one sentinel byte so growth while hashing cannot turn a truncated
	// prefix into an apparently complete hash.
	reader := io.LimitReader(artifactContextReader{ctx: ctx, reader: file}, maxBytes+1)
	written, err := io.Copy(io.MultiWriter(md5Hash, sha256Hash), reader)
	if err != nil {
		return nil, fmt.Errorf("compute artifact hashes %s: %w", path, err)
	}
	if written > maxBytes {
		return nil, fmt.Errorf("%w: path=%s max_bytes=%d", ErrArtifactHashLimit, path, maxBytes)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &model.ArtifactHashes{
		SHA256: hex.EncodeToString(sha256Hash.Sum(nil)),
		MD5:    hex.EncodeToString(md5Hash.Sum(nil)),
	}, nil
}

func snapshotELFArtifact(path string) (*model.ELFArtifact, error) {
	return snapshotELFArtifactContext(context.Background(), path, 0)
}

func snapshotELFArtifactContext(ctx context.Context, path string, maxBytes int64) (*model.ELFArtifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reader, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open elf artifact %s: %w", path, err)
	}
	defer reader.Close()
	if maxBytes <= 0 || maxBytes > DefaultArtifactMetadataMaxBytes {
		maxBytes = DefaultArtifactMetadataMaxBytes
	}
	file, err := elf.NewFile(&artifactContextReaderAt{ctx: ctx, reader: reader, remaining: maxBytes})
	if err != nil {
		return nil, fmt.Errorf("read elf artifact %s: %w", path, err)
	}
	defer file.Close()

	sections := make([]string, 0, len(file.Sections))
	sectionItems := make([]model.ELFSectionArtifact, 0, len(file.Sections))
	for _, section := range file.Sections {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if section == nil {
			continue
		}
		name := strings.TrimSpace(section.Name)
		if name != "" {
			sections = append(sections, name)
		}
		sectionItems = append(sectionItems, model.ELFSectionArtifact{
			Name:     name,
			Type:     strings.TrimSpace(section.Type.String()),
			Flags:    formatELFSectionFlags(section.Flags),
			Addr:     fmt.Sprintf("0x%x", section.Addr),
			Offset:   int64(section.Offset),
			Size:     int64(section.Size),
			IsSymTab: isELFSymbolTableSection(section.Type),
			IsStrTab: section.Type == elf.SHT_STRTAB,
		})
	}
	segments := make([]string, 0, len(file.Progs))
	segmentItems := make([]model.ELFSegmentArtifact, 0, len(file.Progs))
	for _, prog := range file.Progs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if prog == nil {
			continue
		}
		name := strings.TrimSpace(prog.Type.String())
		if name != "" {
			segments = append(segments, name)
		}
		flags := formatELFProgFlags(prog.Flags)
		segmentItems = append(segmentItems, model.ELFSegmentArtifact{
			Type:   name,
			Flags:  flags,
			Offset: int64(prog.Off),
			VAddr:  fmt.Sprintf("0x%x", prog.Vaddr),
			FileSz: int64(prog.Filesz),
			MemSz:  int64(prog.Memsz),
			IsCode: strings.Contains(flags, "X"),
			IsData: strings.Contains(flags, "W") || strings.Contains(flags, "R"),
		})
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
		SectionItems: sectionItems,
		SegmentItems: segmentItems,
	}, nil
}

func formatELFProgFlags(flags elf.ProgFlag) string {
	var parts []string
	if flags&elf.PF_R != 0 {
		parts = append(parts, "R")
	}
	if flags&elf.PF_W != 0 {
		parts = append(parts, "W")
	}
	if flags&elf.PF_X != 0 {
		parts = append(parts, "X")
	}
	return strings.Join(parts, "")
}

func formatELFSectionFlags(flags elf.SectionFlag) string {
	var parts []string
	if flags&elf.SHF_ALLOC != 0 {
		parts = append(parts, "A")
	}
	if flags&elf.SHF_WRITE != 0 {
		parts = append(parts, "W")
	}
	if flags&elf.SHF_EXECINSTR != 0 {
		parts = append(parts, "X")
	}
	if flags&elf.SHF_STRINGS != 0 {
		parts = append(parts, "S")
	}
	return strings.Join(parts, "")
}

func isELFSymbolTableSection(value elf.SectionType) bool {
	return value == elf.SHT_SYMTAB || value == elf.SHT_DYNSYM
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
