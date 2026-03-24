package aiskillloader

import (
	"archive/zip"
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
