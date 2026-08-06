//go:build hids && linux

package enrich

import (
	"os"
	"testing"
)

func TestSnapshotArtifactDetectsELFAndHashes(t *testing.T) {
	t.Parallel()

	executablePath, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	artifact, err := SnapshotArtifact(executablePath, ArtifactSnapshotOptions{CaptureHashes: true})
	if err != nil {
		t.Fatalf("SnapshotArtifact returned error: %v", err)
	}
	if artifact == nil {
		t.Fatal("expected artifact")
	}
	if !artifact.Exists {
		t.Fatal("expected artifact to exist")
	}
	if artifact.FileType != "elf" {
		t.Fatalf("unexpected artifact file type: %q", artifact.FileType)
	}
	if artifact.Hashes == nil || artifact.Hashes.SHA256 == "" || artifact.Hashes.MD5 == "" {
		t.Fatalf("expected artifact hashes, got %#v", artifact.Hashes)
	}
	if artifact.ELF == nil {
		t.Fatal("expected elf metadata")
	}
	if artifact.ELF.Machine == "" || artifact.ELF.EntryAddress == "" {
		t.Fatalf("unexpected elf metadata: %#v", artifact.ELF)
	}
	if artifact.ELF.SectionCount <= 0 || len(artifact.ELF.SectionItems) == 0 {
		t.Fatalf("expected structured elf sections, got %#v", artifact.ELF)
	}
	if artifact.ELF.SegmentCount <= 0 || len(artifact.ELF.SegmentItems) == 0 {
		t.Fatalf("expected structured elf segments, got %#v", artifact.ELF)
	}
	if artifact.ELF.SectionItems[0].Type == "" {
		t.Fatalf("expected section item type, got %#v", artifact.ELF.SectionItems[0])
	}
	if artifact.ELF.SegmentItems[0].Type == "" {
		t.Fatalf("expected segment item type, got %#v", artifact.ELF.SegmentItems[0])
	}
}
