package trafficfixture

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateWorkspacePermissionsProtectNewDescendants(t *testing.T) {
	directory := t.TempDir()
	if err := preparePrivateWorkspace(directory); err != nil {
		t.Fatal(err)
	}
	if err := checkPrivateWorkspacePermissions(directory, true); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(directory, "nested")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	if err := checkPrivateWorkspacePermissions(nested, false); err != nil {
		t.Fatal("private directory inheritance", err)
	}
	name := filepath.Join(nested, "authored-fixture.keys")
	wire := []byte("authored permission control, no capture or session secret")
	if err := os.WriteFile(name, wire, 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkPrivateWorkspacePermissions(name, false); err != nil {
		t.Fatal("private file inheritance", err)
	}
	got, err := os.ReadFile(name)
	if err != nil || !bytes.Equal(got, wire) {
		t.Fatal("private ACL changed file bytes", err)
	}
	if err := preparePrivateWorkspace(name); err == nil {
		t.Fatal("accepted a file as a workspace root")
	}
}
