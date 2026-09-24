package scannode

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

const (
	maxContextSkillArchiveBytes  = 16 << 20
	maxContextSkillExpandedBytes = 32 << 20
	maxContextSkillEntryBytes    = 2 << 20
	maxContextSkillEntries       = 512
	maxContextSkillDepth         = 8
)

// contextSkillBundleFS verifies an immutable, single-skill bundle before it is
// exposed to the native Skill loader. No archive path is ever written to disk.
func contextSkillBundleFS(bundle *aiv1.ContextSkillBundle, ownerUserID string) (fi.FileSystem, error) {
	if bundle == nil || strings.TrimSpace(bundle.GetForgeId()) == "" || strings.TrimSpace(bundle.GetName()) == "" || strings.TrimSpace(ownerUserID) == "" {
		return nil, fmt.Errorf("Skill bundle identity is incomplete")
	}
	if strings.TrimSpace(bundle.GetOwnerUserId()) != strings.TrimSpace(ownerUserID) {
		return nil, fmt.Errorf("Skill bundle owner mismatch")
	}
	archive := bundle.GetArchiveZip()
	if len(archive) == 0 || len(archive) > maxContextSkillArchiveBytes {
		return nil, fmt.Errorf("Skill bundle archive size exceeds limit")
	}
	digest := sha256.Sum256(archive)
	if bundle.GetSha256() != hex.EncodeToString(digest[:]) {
		return nil, fmt.Errorf("Skill bundle SHA-256 mismatch")
	}
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil || len(reader.File) == 0 || len(reader.File) > maxContextSkillEntries {
		return nil, fmt.Errorf("Skill bundle ZIP has invalid entries")
	}
	files := make(map[string][]byte, len(reader.File))
	var total uint64
	var skillPath string
	for _, entry := range reader.File {
		name := entry.Name
		clean := path.Clean(strings.TrimSuffix(name, "/"))
		if name == "" || strings.ContainsAny(name, "\\\x00") || strings.HasPrefix(name, "/") || strings.Contains(name, ":") || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != strings.TrimSuffix(name, "/") || strings.Count(clean, "/")+1 > maxContextSkillDepth {
			return nil, fmt.Errorf("Skill bundle contains unsafe path")
		}
		mode := entry.Mode()
		if mode&fs.ModeSymlink != 0 || (!entry.FileInfo().IsDir() && !mode.IsRegular()) {
			return nil, fmt.Errorf("Skill bundle contains nonregular entry")
		}
		if entry.FileInfo().IsDir() {
			continue
		}
		if _, exists := files[clean]; exists {
			return nil, fmt.Errorf("Skill bundle contains duplicate entry")
		}
		if entry.UncompressedSize64 > maxContextSkillEntryBytes || total+entry.UncompressedSize64 > maxContextSkillExpandedBytes {
			return nil, fmt.Errorf("Skill bundle expanded size exceeds limit")
		}
		file, openErr := entry.Open()
		if openErr != nil {
			return nil, fmt.Errorf("Skill bundle entry cannot be opened: %w", openErr)
		}
		content, readErr := io.ReadAll(io.LimitReader(file, maxContextSkillEntryBytes+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil || len(content) > maxContextSkillEntryBytes {
			return nil, fmt.Errorf("Skill bundle entry cannot be read within limit")
		}
		total += uint64(len(content))
		if total > maxContextSkillExpandedBytes {
			return nil, fmt.Errorf("Skill bundle expanded size exceeds limit")
		}
		files[clean] = content
		if path.Base(clean) == "SKILL.md" {
			if skillPath != "" {
				return nil, fmt.Errorf("Skill bundle must contain exactly one SKILL.md")
			}
			skillPath = clean
		}
	}
	if skillPath == "" {
		return nil, fmt.Errorf("Skill bundle has no SKILL.md")
	}
	prefix := strings.TrimSuffix(skillPath, "SKILL.md")
	meta, err := aiskillloader.ParseSkillMeta(string(files[skillPath]))
	if err != nil || meta.Name != bundle.GetName() || strings.Contains(meta.Name, "/") || strings.Contains(meta.Name, "\\") || meta.Name == "." || meta.Name == ".." {
		return nil, fmt.Errorf("Skill bundle name does not match SKILL.md")
	}
	root := filesys.NewVirtualFs()
	for name, content := range files {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		rel := strings.TrimPrefix(name, prefix)
		if rel == "" {
			continue
		}
		root.AddFile(path.Join(meta.Name, rel), string(content))
	}
	return root, nil
}
