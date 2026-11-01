//go:build !windows

package trafficfixture

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateWorkspaceUnixModesAreEnforced(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0755); err != nil {
		t.Fatal(err)
	}
	if err := preparePrivateWorkspace(directory); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(directory)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("workspace root must be 0700", err)
	}
	name := filepath.Join(directory, "authored.keys")
	if err := os.WriteFile(name, []byte("permission control"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := checkPrivateWorkspacePermissions(name, false); err == nil {
		t.Fatal("accepted group/other-readable output")
	}
	if err := os.Chmod(name, 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkPrivateWorkspacePermissions(name, false); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(directory, link); err != nil {
		t.Fatal(err)
	}
	if err := preparePrivateWorkspace(link); err == nil {
		t.Fatal("accepted a symlink workspace root")
	}
}
