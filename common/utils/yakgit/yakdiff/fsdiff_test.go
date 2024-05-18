package yakdiff

import (
	"context"
	"fmt"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"testing"
)

func TestFSDIFF(t *testing.T) {
	fs1 := filesys.NewVirtualFs()
	fs1.AddFile("1.txt", `package privileged

import (
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

func isPrivileged() bool {
	header := unix.CapUserHeader{
		Version: unix.LINUX_CAPABILITY_VERSION_3,
		Pid:     int32(os.Getpid()),
	}
	data := unix.CapUserData{}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := unix.Capget(&header, &data); err == nil {
		data.Inheritable = (1 << unix.CAP_NET_RAW)

		if err := unix.Capset(&header, &data); err == nil {
			return true
		}
	}
	return os.Geteuid() == 0
}
`)
	fs1.AddFile("c/2.txt", `		if err != nil {
			return nil, nil, utils.Wrap(err, "commit")
		}
		_ = commit
		commitIns, err := repo.CommitObject(commit)
		if err != nil {
			return nil, nil, utils.Wrap(err, "get commit object")
		}
		tree, err := commitIns.Tree()
		if err != nil {
			return nil, nil, utils.Wrap(err, "get tree")
		}
		return commitIns, tree, nil`)

	fs2 := filesys.NewVirtualFs()
	fs2.AddFile("1.txt", `package privileged

import (
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

func isPrivileged() bool {
	header := unix.CapUserHeader{
		Version: unix.LINUX_CAPABILITY_VERSION_3,
		Pid:     int32(os.Getpid()),
	}

	// Hello ME Access Token

	data := unix.CapUserData{}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := unix.Capget(&header, &data); err == nil {
		data.Inheritable = (1 << unix.CAP_NET_RAW)

		if err := unix.Capset(&header, &data); err == nil {
			return true
		}
	}
	return os.Geteuid() == 0
}
`)
	fs2.AddFile("c/2.txt", `		if err != nil {
		_ = commit
		commitIns, err := repo.CommitObject(commit)
		if err != nil {
			return nil, nil, utils.Wrap(err, "get commit object")
		}
		if err != nil {
			return nil, nil, utils.Wrap(err, "get tree")
		}
		return commitIns, tree, nil`)

	check1txt := false
	check2txt := false
	err := FileSystemDiffContext(context.Background(), fs1, fs2, func(commit *object.Commit, change *object.Change, patch *object.Patch) error {
		if patch == nil {
			return nil
		}
		fmt.Println("-------------------------------------")
		raw := patch.String()
		fmt.Println(raw)

		if utils.MatchAllOfSubString(raw, `--- a/1.txt`, `+++ b/1.txt`, `@@ -12,6 +12,9 @@`, `Hello ME Access Token`) {
			check1txt = true
			return nil
		}
		if utils.MatchAllOfSubString(raw, `--- a/c/2.txt`, `+++ b/c/2.txt`, `@@ -1,12 +1,9 @@`, `tree, err := commitIns.Tree()`, `return nil, nil, utils.Wrap(err, "commit")`) {
			check2txt = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !check2txt {
		t.Fatal("2.txt is not diffed rightly")
	}
	if !check1txt {
		t.Fatal("1.txt is not diffed right")
	}
}
