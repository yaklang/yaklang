package diff

import (
	"fmt"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/yaklang/yaklang/common/utils"
	"testing"
)

func TestDemo(t *testing.T) {
	check := false
	err := Diff(`		return utils.Wrap(err, "init git repos")
	}
	wt, err := repo.Worktree()
	if err != nil {
		return utils.Wrap(err, "get worktree")
	}
	wt.Filesystem.MkdirAll("main", 0755)
	if err != nil {
		return utils.Wrap(err, "mkdir main")
	}
	filename := path.Join("main", "main.txt")
	fp, err := wt.Filesystem.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return utils.Wrap(err, "open file")
	}
	fp.Write(r1)
	fp.Close()
	_, err = wt.Add(filename)
	if err != nil {
		return utils.Wrap(err, "add file")
	}
	commit, err := wt.Commit("add first file", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Yaklang",
			Email: "yaklang@example.com",`, `
		return utils.Wrap(err, "init git repos")
	}
	wt, err := repo.Worktree()
	if err != nil {
		return utils.Wrap(err, "get worktree")
	}
	wt.Filesystem.MkdirAll("main", 0755)
	if err != nil {
		return utils.Wrap(err, "mkdir main")
	}
	filename := path.Join("main", "main.txt")
	_, err = wt.Add(filename)
	if err != nil {
		return utils.Wrap(err, "add file")
	}
	commit, err := wt.Commit("add first file", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Yaklang",
			Email: "yaklang@example.com",
`, func(_ *object.Commit, change *object.Change, patch *object.Patch) error {
		raw := patch.String()
		if utils.MatchAllOfSubString(raw, `@@ -9,12 +10,6 @@`, `@@ -1,3 +1,4 @@`, `@@ -22,4 +17,4 @@`) {
			check = true
			fmt.Println(string(raw))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !check {
		t.Fatal("not match")
	}
}
