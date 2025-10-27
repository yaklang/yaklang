package yakdiff

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/yaklib"
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

// createTestZipFileFromRaw creates a temporary ZIP file from raw bytes using yaklang zip.CompressRaw
func createTestZipFileFromRaw(t *testing.T, files map[string]string) string {
	// Convert map[string]string to map[string]interface{} for CompressRaw
	filesInterface := make(map[string]interface{})
	for k, v := range files {
		filesInterface[k] = v
	}

	// Use yaklang's zip.CompressRaw to create ZIP bytes
	zipBytes, err := yaklib.CompressRaw(filesInterface)
	if err != nil {
		t.Fatal(err)
	}

	// Write to temporary file
	tmpFile, err := os.CreateTemp("", "test-*.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer tmpFile.Close()

	_, err = tmpFile.Write(zipBytes)
	if err != nil {
		t.Fatal(err)
	}

	return tmpFile.Name()
}

func TestDiffZIPFile(t *testing.T) {
	// Create first ZIP file
	zip1Files := map[string]string{
		"config.txt": `server:
  host: localhost
  port: 8080
database:
  user: admin
  password: secret123`,
		"app/main.go": `package main

func main() {
	println("Hello World")
	start()
}`,
		"app/util.go": `package main

func helper() {
	return "help"
}`,
	}

	// Create second ZIP file with modifications
	zip2Files := map[string]string{
		"config.txt": `server:
  host: localhost
  port: 9090
database:
  user: admin
  password: newpassword456
  timeout: 30`,
		"app/main.go": `package main

// Added comment
func main() {
	println("Hello Yaklang")
	start()
	cleanup()
}`,
		// Note: app/util.go is deleted in zip2
		"app/logger.go": `package main

func log(msg string) {
	println(msg)
}`,
	}

	zip1Path := createTestZipFileFromRaw(t, zip1Files)
	defer os.Remove(zip1Path)

	zip2Path := createTestZipFileFromRaw(t, zip2Files)
	defer os.Remove(zip2Path)

	t.Run("Test DiffZIPFile with handler", func(t *testing.T) {
		foundConfigChange := false
		foundMainChange := false
		foundUtilDeleted := false
		foundLoggerAdded := false

		_, err := DiffZIPFile(zip1Path, zip2Path, func(commit *object.Commit, change *object.Change, patch *object.Patch) error {
			if patch == nil {
				return nil
			}

			patchStr := patch.String()
			fmt.Println("===========================================")
			fmt.Println(patchStr)

			// Check config.txt changes
			if utils.MatchAllOfSubString(patchStr, "config.txt", "port: 9090", "newpassword456") {
				foundConfigChange = true
			}

			// Check main.go changes
			if utils.MatchAllOfSubString(patchStr, "app/main.go", "Hello Yaklang", "cleanup") {
				foundMainChange = true
			}

			// Check util.go deletion
			if utils.MatchAllOfSubString(patchStr, "app/util.go", "--- a/") {
				foundUtilDeleted = true
			}

			// Check logger.go addition
			if utils.MatchAllOfSubString(patchStr, "app/logger.go", "+++ b/", "func log") {
				foundLoggerAdded = true
			}

			return nil
		})

		if err != nil {
			t.Fatal(err)
		}

		if !foundConfigChange {
			t.Error("config.txt changes not detected")
		}
		if !foundMainChange {
			t.Error("app/main.go changes not detected")
		}
		if !foundUtilDeleted {
			t.Error("app/util.go deletion not detected")
		}
		if !foundLoggerAdded {
			t.Error("app/logger.go addition not detected")
		}
	})

	t.Run("Test DiffZIPFile returns diff string", func(t *testing.T) {
		diffStr, err := DiffZIPFile(zip1Path, zip2Path)
		if err != nil {
			t.Fatal(err)
		}

		if diffStr == "" {
			t.Error("expected non-empty diff string")
		}

		// Verify the diff string contains expected changes
		if !utils.MatchAnyOfSubString(diffStr, "config.txt") {
			t.Error("diff string should contain config.txt")
		}
		if !utils.MatchAnyOfSubString(diffStr, "app/main.go") {
			t.Error("diff string should contain app/main.go")
		}

		fmt.Println("=== Complete Diff Output ===")
		fmt.Println(diffStr)
	})

	t.Run("Test DiffZIPFile with non-existent file", func(t *testing.T) {
		_, err := DiffZIPFile("/non/existent/file1.zip", zip2Path)
		if err == nil {
			t.Error("expected error for non-existent file")
		}
	})
}
