//go:build hids && linux

package enrich

import (
	"os"
	"os/user"
	"strconv"
	"syscall"
	"testing"
)

func TestFileIdentityResolvesLocalAccount(t *testing.T) {
	got, err := SnapshotFileIdentity(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	uid, gid := strconv.Itoa(os.Getuid()), strconv.Itoa(os.Getgid())
	owner, err := user.LookupId(uid)
	if err != nil {
		t.Fatal(err)
	}
	group, err := user.LookupGroupId(gid)
	if err != nil {
		t.Fatal(err)
	}
	if got.UID != uid || got.GID != gid || got.Owner != owner.Username || got.Group != group.Name {
		t.Fatalf("local file account identity lost: %+v", got)
	}
}

type identityFileInfo struct {
	os.FileInfo
	stat syscall.Stat_t
}

func (info identityFileInfo) Sys() any { return &info.stat }

func TestFileIdentityPreservesUnknownNumericAccount(t *testing.T) {
	info, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// No chown or elevated permissions are needed to exercise missing accounts.
	const unknown = uint32(4294967294)
	const unknownID = "4294967294"
	if _, err := user.LookupId(unknownID); err == nil {
		t.Fatal("unknown UID fixture unexpectedly exists")
	}
	if _, err := user.LookupGroupId(unknownID); err == nil {
		t.Fatal("unknown GID fixture unexpectedly exists")
	}
	got := FileIdentityFromFileInfo(identityFileInfo{FileInfo: info, stat: syscall.Stat_t{Uid: unknown, Gid: unknown}})
	if got.UID != unknownID || got.GID != unknownID || got.Owner != "" || got.Group != "" {
		t.Fatalf("unknown account must preserve numeric IDs without inventing names: %+v", got)
	}
}
