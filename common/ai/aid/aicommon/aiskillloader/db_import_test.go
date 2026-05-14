package aiskillloader

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestImportAISkillsToDB(t *testing.T) {
	db := newSearchTestDB(t)
	defer db.Close()
	loader, err := NewAutoSkillLoader(WithAutoLoad_FileSystem(buildNestedTestVFS()))
	if err != nil {
		t.Fatalf("NewAutoSkillLoader failed: %v", err)
	}

	count, err := ImportAISkillsToDB(db, loader)
	if err != nil {
		t.Fatalf("ImportAISkillsToDB failed: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 imported skills, got %d", count)
	}
	if _, err := yakit.GetAIForgeByNameAndTypes(db, "top-skill", schema.FORGE_TYPE_SkillMD); err != nil {
		t.Fatalf("expected top-skill in db: %v", err)
	}
	if _, err := yakit.GetAIForgeByNameAndTypes(db, "hidden-skill", schema.FORGE_TYPE_SkillMD); err != nil {
		t.Fatalf("expected hidden-skill in db: %v", err)
	}
	if _, err := yakit.GetAIForgeByNameAndTypes(db, "code-review", schema.FORGE_TYPE_SkillMD); err != nil {
		t.Fatalf("expected code-review in db: %v", err)
	}
}

func TestImportAISkillsFromLocalDirToDB(t *testing.T) {
	db := newSearchTestDB(t)
	defer db.Close()

	baseDir := t.TempDir()
	if err := createLocalSkillFixture(baseDir); err != nil {
		t.Fatalf("createLocalSkillFixture failed: %v", err)
	}

	count, err := ImportAISkillsFromLocalDirToDB(db, baseDir)
	if err != nil {
		t.Fatalf("ImportAISkillsFromLocalDirToDB failed: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 imported skills, got %d", count)
	}
}

func TestImportAISkillsFromZipFileToDB(t *testing.T) {
	db := newSearchTestDB(t)
	defer db.Close()

	baseDir := t.TempDir()
	if err := createLocalSkillFixture(baseDir); err != nil {
		t.Fatalf("createLocalSkillFixture failed: %v", err)
	}
	zipPath := filepath.Join(t.TempDir(), "skills.zip")
	if err := zipDir(baseDir, zipPath); err != nil {
		t.Fatalf("zipDir failed: %v", err)
	}

	count, err := ImportAISkillsFromZipFileToDB(db, zipPath)
	if err != nil {
		t.Fatalf("ImportAISkillsFromZipFileToDB failed: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 imported skills, got %d", count)
	}
	if _, err := yakit.GetAIForgeByNameAndTypes(db, "hidden-skill", schema.FORGE_TYPE_SkillMD); err != nil {
		t.Fatalf("expected hidden-skill in db: %v", err)
	}
}

func TestImportAISkillsFromArchiveFileToDB(t *testing.T) {
	testCases := []struct {
		name    string
		archive string
		pack    func(string, string) error
	}{
		{name: "tar", archive: "skills.tar", pack: tarDir},
		{name: "tar.gz", archive: "skills.tar.gz", pack: tarGzDir},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db := newSearchTestDB(t)
			defer db.Close()

			baseDir := t.TempDir()
			if err := createLocalSkillFixture(baseDir); err != nil {
				t.Fatalf("createLocalSkillFixture failed: %v", err)
			}
			archivePath := filepath.Join(t.TempDir(), tc.archive)
			if err := tc.pack(baseDir, archivePath); err != nil {
				t.Fatalf("pack archive failed: %v", err)
			}

			count, err := ImportAISkillsFromArchiveFileToDB(db, archivePath)
			if err != nil {
				t.Fatalf("ImportAISkillsFromArchiveFileToDB failed: %v", err)
			}
			if count != 3 {
				t.Fatalf("expected 3 imported skills, got %d", count)
			}
			if _, err := yakit.GetAIForgeByNameAndTypes(db, "code-review", schema.FORGE_TYPE_SkillMD); err != nil {
				t.Fatalf("expected code-review in db: %v", err)
			}
		})
	}
}

func zipDir(srcDir, zipPath string) error {
	file, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer file.Close()

	zw := zip.NewWriter(file)
	defer zw.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == srcDir {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = rel
		if info.IsDir() {
			header.Name += "/"
			_, err = zw.CreateHeader(header)
			return err
		}
		header.Method = zip.Deflate
		writer, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(writer, f)
		return err
	})
}

func tarDir(srcDir, tarPath string) error {
	file, err := os.Create(tarPath)
	if err != nil {
		return err
	}
	defer file.Close()

	tw := tar.NewWriter(file)
	defer tw.Close()

	return filepath.Walk(srcDir, func(current string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if current == srcDir {
			return nil
		}
		rel, err := filepath.Rel(srcDir, current)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = rel
		if info.IsDir() {
			header.Name += "/"
			return tw.WriteHeader(header)
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		f, err := os.Open(current)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
}

func tarGzDir(srcDir, tarGzPath string) error {
	file, err := os.Create(tarGzPath)
	if err != nil {
		return err
	}
	defer file.Close()

	gw := gzip.NewWriter(file)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	return filepath.Walk(srcDir, func(current string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if current == srcDir {
			return nil
		}
		rel, err := filepath.Rel(srcDir, current)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = rel
		if info.IsDir() {
			header.Name += "/"
			return tw.WriteHeader(header)
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		f, err := os.Open(current)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
}

func createLocalSkillFixture(baseDir string) error {
	files := map[string]string{
		"top-skill/SKILL.md": `---
name: top-skill
description: Deploy applications to production
metadata:
  category: deploy
---
Deployment instructions.
`,
		"deep/nested/hidden-skill/SKILL.md": `---
name: hidden-skill
description: Security scanning and testing
metadata:
  category: security
---
Security workflow.
`,
		"tools/code-review/SKILL.md": `---
name: code-review
description: Review code quality with linters
metadata:
  category: review
---
Code review workflow.
`,
	}
	for rel, content := range files {
		abs := filepath.Join(baseDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}
