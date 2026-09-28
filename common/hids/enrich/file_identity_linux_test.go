//go:build hids && linux

package enrich

import (
	"os"
	"os/user"
	"strconv"
	"syscall"
	"testing"
)

func TestFileIdentityLocalAccount(t *testing.T) {
	path := t.TempDir()
	got, err := SnapshotFileIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	current, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	group, err := user.LookupGroupId(strconv.Itoa(os.Getgid()))
	if err != nil {
		t.Fatal(err)
	}
	if got.UID != strconv.Itoa(os.Getuid()) || got.GID != strconv.Itoa(os.Getgid()) || got.Owner != current.Username || got.Group != group.Name {
		t.Fatalf("local account identity lost: %+v", got)
	}
}

type identityFileInfo struct {
	os.FileInfo
	stat syscall.Stat_t
}

func (info identityFileInfo) Sys() any { return &info.stat }

func TestFileIdentityUnknownAccountPreservesIDs(t *testing.T) {
	info, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Synthetic identities exercise failed enrichment without requiring chown.
	const unknown = uint32(4294967294)
	if _, err := user.LookupId(strconv.FormatUint(uint64(unknown), 10)); err == nil {
		t.Fatal("unknown UID fixture unexpectedly exists")
	}
	if _, err := user.LookupGroupId(strconv.FormatUint(uint64(unknown), 10)); err == nil {
		t.Fatal("unknown GID fixture unexpectedly exists")
	}
	got := FileIdentityFromFileInfo(identityFileInfo{FileInfo: info, stat: syscall.Stat_t{Uid: unknown, Gid: unknown}})
	if got.UID != "4294967294" || got.GID != "4294967294" || got.Owner != "" || got.Group != "" {
		t.Fatalf("unknown account must preserve IDs without inventing names: %+v", got)
	}
}
