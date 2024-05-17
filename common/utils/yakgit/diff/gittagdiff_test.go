package diff

import (
	"context"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"os"
	"testing"
)

func TestGitTagOrHashDiff(t *testing.T) {
	// build a git repo
	repo, err := git.Init(memory.NewStorage(), memfs.New())
	if err != nil {
		t.Fatal(err)
	}
	tree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	fp1, err := tree.Filesystem.OpenFile("1.txt", os.O_CREATE|os.O_RDWR, 0755)
	if err != nil {
		t.Fatal(err)
	}
	fp1.Write([]byte(`			f, err := gitFS.OpenFile(pathname, os.O_CREATE|os.O_RDWR, 0755)
			if err != nil {
			}
			defer f.Close()
			origin, err := originFS.Open(pathname)
			if err != nil {
				return utils.Wrap(err, "origin fs1 open failed")
			}
			origin.Close()
			io.Copy(f, origin)
			return nil`))
	fp1.Close()
	_, err = tree.Add("1.txt")
	if err != nil {
		t.Fatal(err)
	}
	hash1, err := tree.Commit(
		"first",
		&git.CommitOptions{Author: &object.Signature{Name: "yaklang", Email: ""}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.CommitObject(hash1)
	if err != nil {
		t.Fatal(err)
	}

	hash1Tag, err := repo.CreateTag("tag1", hash1, &git.CreateTagOptions{Message: "tag1"})
	if err != nil {
		t.Fatal(err)
	}
	repo.CommitObject(hash1Tag.Hash())

	err = tree.Filesystem.Remove("1.txt")
	if err != nil {
		t.Fatal(err)
	}
	fp2, _ := tree.Filesystem.OpenFile("1.txt", os.O_CREATE|os.O_RDWR, 0755)
	fp2.Write([]byte(`			f, err := gitFS.OpenFile(pathname, os.O_CREATE|os.O_RDWR, 0755)
			if err != nil {
// return hHHHh
			}
			defer f.Close()
			if err != nil {
				return utils.Wrap(err, "origin fs1 open failed")
			}
			origin.Close()
			io.Copy(f, origin)
			return nil`))
	fp2.Close()
	_, err = tree.Add("1.txt")
	if err != nil {
		t.Fatal(err)
	}
	// commit and submit
	hash2, err := tree.Commit(
		"second",
		&git.CommitOptions{Author: &object.Signature{Name: "yaklang", Email: ""}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.CommitObject(hash2)
	if err != nil {
		t.Fatal(err)
	}
	hash2Tag, _ := repo.CreateTag("tag2", hash2, &git.CreateTagOptions{Message: "tag2"})
	_, _ = hash1Tag, hash2Tag
	log.Infof("tag1: %s, tag2: %s", hash1Tag.String(), hash2Tag.String())
	log.Infof("hash1: %s, hash2: %s", hash1.String(), hash2.String())
	repo.CommitObject(hash2Tag.Hash())

	var passed bool

	passed = false
	err = GitHashDiffContext(context.Background(), repo, hash1.String(), hash2.String(), func(commit *object.Commit, change *object.Change, patch *object.Patch) error {
		if patch == nil {
			return nil
		}
		if utils.MatchAllOfSubString(patch.String(), `+// return hHHHh`, `-			origin, err := originFS.Open(pathname)`, `--- a/1.txt`, `@@ -1,8 +1,8 @@`) {
			passed = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !passed {
		t.Fatal("git hash diff failed")
	}

	passed = false
	err = GitHashDiffContext(context.Background(), repo, "tag1", hash2.String(), func(commit *object.Commit, change *object.Change, patch *object.Patch) error {
		if patch == nil {
			return nil
		}
		if utils.MatchAllOfSubString(patch.String(), `+// return hHHHh`, `-			origin, err := originFS.Open(pathname)`, `--- a/1.txt`, `@@ -1,8 +1,8 @@`) {
			passed = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !passed {
		t.Fatal("git tag diff failed")
	}

	passed = false
	err = GitHashDiffContext(context.Background(), repo, hash1.String(), "tag2", func(commit *object.Commit, change *object.Change, patch *object.Patch) error {
		if patch == nil {
			return nil
		}
		if utils.MatchAllOfSubString(patch.String(), `+// return hHHHh`, `-			origin, err := originFS.Open(pathname)`, `--- a/1.txt`, `@@ -1,8 +1,8 @@`) {
			passed = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !passed {
		t.Fatal("git tag diff failed")
	}

	passed = false
	err = GitHashDiffContext(context.Background(), repo, "tag1", "tag2", func(commit *object.Commit, change *object.Change, patch *object.Patch) error {
		if patch == nil {
			return nil
		}
		if utils.MatchAllOfSubString(patch.String(), `+// return hHHHh`, `-			origin, err := originFS.Open(pathname)`, `--- a/1.txt`, `@@ -1,8 +1,8 @@`) {
			passed = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !passed {
		t.Fatal("git tag diff failed")
	}
}
