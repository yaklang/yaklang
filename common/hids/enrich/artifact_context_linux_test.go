//go:build hids && linux

package enrich

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotArtifactHashBudgetDoesNotReturnPrefixHashes(t *testing.T) {
	t.Parallel()
	content := []byte("complete artifact content")
	path := filepath.Join(t.TempDir(), "sample.txt")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	artifact, err := SnapshotArtifactContext(context.Background(), path, ArtifactSnapshotOptions{
		CaptureHashes: true, MaxHashBytes: int64(len(content) - 1),
	})
	if !errors.Is(err, ErrArtifactHashLimit) || artifact == nil || artifact.SizeBytes != int64(len(content)) || artifact.Hashes != nil {
		t.Fatalf("expected metadata and explicit limit error, never prefix hashes: artifact=%#v err=%v", artifact, err)
	}
	artifact, err = SnapshotArtifactContext(context.Background(), path, ArtifactSnapshotOptions{
		CaptureHashes: true, MaxHashBytes: int64(len(content)),
	})
	sha, md := sha256.Sum256(content), md5.Sum(content)
	if err != nil || artifact == nil || artifact.Hashes == nil || artifact.Hashes.SHA256 != hex.EncodeToString(sha[:]) || artifact.Hashes.MD5 != hex.EncodeToString(md[:]) {
		t.Fatalf("exact budget must hash the whole artifact: artifact=%#v err=%v", artifact, err)
	}
}

func TestSnapshotArtifactELFMetadataBudgetIsExplicit(t *testing.T) {
	t.Parallel()
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := SnapshotArtifactContext(context.Background(), path, ArtifactSnapshotOptions{MaxMetadataBytes: 1})
	if !errors.Is(err, ErrArtifactMetadataLimit) || artifact == nil || artifact.FileType != "elf" || artifact.ELF != nil {
		t.Fatalf("metadata budget must not return a partial ELF: artifact=%#v err=%v", artifact, err)
	}
}

func TestSnapshotArtifactCanceledBeforeIO(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	artifact, err := SnapshotArtifactContext(ctx, filepath.Join(t.TempDir(), "absent"), ArtifactSnapshotOptions{CaptureHashes: true})
	if !errors.Is(err, context.Canceled) || artifact != nil {
		t.Fatalf("canceled snapshot must not look like a successful missing file: artifact=%#v err=%v", artifact, err)
	}
	hashes, err := snapshotArtifactHashesContext(ctx, "/absent", 0)
	if !errors.Is(err, context.Canceled) || hashes != nil {
		t.Fatalf("canceled hash must not be complete: hashes=%#v err=%v", hashes, err)
	}
}

type cancelingArtifactReader struct{ cancel context.CancelFunc }

func (r cancelingArtifactReader) Read(buf []byte) (int, error) {
	buf[0] = 'x'
	r.cancel()
	return 1, nil
}

func (r cancelingArtifactReader) ReadAt(buf []byte, _ int64) (int, error) {
	return r.Read(buf)
}

func TestArtifactReadersRecheckCancellationAfterIO(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	reader := artifactContextReader{ctx: ctx, reader: cancelingArtifactReader{cancel}}
	if n, err := reader.Read(make([]byte, 8)); n != 1 || !errors.Is(err, context.Canceled) {
		t.Fatalf("read ignored cancellation during IO: n=%d err=%v", n, err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	readerAt := &artifactContextReaderAt{ctx: ctx, reader: cancelingArtifactReader{cancel}, remaining: 8}
	if n, err := readerAt.ReadAt(make([]byte, 8), 0); n != 1 || !errors.Is(err, context.Canceled) {
		t.Fatalf("ReadAt ignored cancellation during IO: n=%d err=%v", n, err)
	}
}

func TestArtifactMetadataReaderBoundsTotalBytes(t *testing.T) {
	t.Parallel()
	reader := &artifactContextReaderAt{ctx: context.Background(), reader: bytes.NewReader([]byte("12345678")), remaining: 4}
	if n, err := reader.ReadAt(make([]byte, 4), 0); n != 4 || err != nil {
		t.Fatalf("exact budget failed: n=%d err=%v", n, err)
	}
	if n, err := reader.ReadAt(make([]byte, 1), 4); n != 0 || !errors.Is(err, ErrArtifactMetadataLimit) {
		t.Fatalf("read beyond total budget was allowed: n=%d err=%v", n, err)
	}
	if reader.remaining != 0 {
		t.Fatalf("unexpected budget after rejected read: %d", reader.remaining)
	}
}
