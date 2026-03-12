package filesys

import "testing"

func TestGzipFSRoundTrip(t *testing.T) {
	vfs := NewVirtualFs()
	vfs.AddDir("empty-dir")
	vfs.AddFile("docs/readme.txt", "hello world")
	vfs.AddFile("bin/tool.py", "print('hi')")

	raw, err := SerializeFileSystemToGzipBytes(vfs)
	if err != nil {
		t.Fatalf("SerializeFileSystemToGzipBytes failed: %v", err)
	}
	restored, err := NewGzipFSFromBytes(raw)
	if err != nil {
		t.Fatalf("NewGzipFSFromBytes failed: %v", err)
	}

	content, err := restored.ReadFile("docs/readme.txt")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(content) != "hello world" {
		t.Fatalf("unexpected content: %q", string(content))
	}
	if _, err := restored.Stat("empty-dir"); err != nil {
		t.Fatalf("expected empty dir to exist: %v", err)
	}
}

func TestSerializeFileSystemToGzipBytes_EmptyFS(t *testing.T) {
	vfs := NewVirtualFs()
	raw, err := SerializeFileSystemToGzipBytes(vfs)
	if err != nil {
		t.Fatalf("SerializeFileSystemToGzipBytes failed: %v", err)
	}
	restored, err := NewGzipFSFromBytes(raw)
	if err != nil {
		t.Fatalf("NewGzipFSFromBytes failed: %v", err)
	}
	entries, err := restored.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty fs, got %d entries", len(entries))
	}
}
