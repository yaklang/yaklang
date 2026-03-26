package aiskillloader

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"gopkg.in/yaml.v3"
)

const ForgeTagUnknownKey = "unknowkey"

// GenerateSkillMDFromAIForge renders the canonical SKILL.md content from forge fields.
func GenerateSkillMDFromAIForge(forge *schema.AIForge) (string, error) {
	if forge == nil {
		return "", utils.Error("forge is nil")
	}
	return renderSkillMDContent(&SkillMeta{
		Name:          forge.ForgeName,
		Description:   forge.Description,
		Compatibility: "",
		Metadata:      forgeTagsToMetadata(forge.Tags),
		Body:          forge.InitPrompt,
	})
}

// SerializeSkillFileSystemWithGeneratedSkillMD rewrites SKILL.md from forge fields
// before serializing the rest of the skill filesystem into gzip bytes.
func SerializeSkillFileSystemWithGeneratedSkillMD(forge *schema.AIForge, fsys fi.FileSystem) ([]byte, error) {
	if fsys == nil {
		return nil, utils.Error("skill filesystem is nil")
	}

	skillMDContent, err := GenerateSkillMDFromAIForge(forge)
	if err != nil {
		return nil, utils.Wrap(err, "render skill markdown from forge failed")
	}
	if err := fsys.WriteFile(skillMDFilename, []byte(skillMDContent), 0o644); err != nil {
		return nil, utils.Wrap(err, "write generated skill markdown failed")
	}
	fsBytes, err := filesys.SerializeFileSystemToGzipBytes(fsys, filesys.WithGzipFSExcludePaths(skillMDFilename))
	if err != nil {
		return nil, utils.Wrap(err, "serialize skill filesystem failed")
	}
	return fsBytes, nil
}

// LoadedSkillToAIForge converts a loaded skill into a forge record of type skillmd.
// SKILL.md itself is synthesized from mapped forge fields, so it is excluded from FSBytes.
func LoadedSkillToAIForge(skill *LoadedSkill) (*schema.AIForge, error) {
	if skill == nil || skill.Meta == nil {
		return nil, utils.Error("loaded skill or meta is nil")
	}
	if skill.FileSystem == nil {
		return nil, utils.Error("loaded skill filesystem is nil")
	}

	fsBytes, err := filesys.SerializeFileSystemToGzipBytes(skill.FileSystem, filesys.WithGzipFSExcludePaths(skillMDFilename))
	if err != nil {
		return nil, utils.Wrap(err, "serialize skill filesystem failed")
	}

	return &schema.AIForge{
		ForgeName:          skill.Meta.Name,
		Description:        buildForgeDescriptionFromSkillMeta(skill.Meta),
		Tags:               metadataToForgeTags(skill.Meta.Metadata),
		InitPrompt:         skill.Meta.Body,
		ForgeType:          schema.FORGE_TYPE_SkillMD,
		FSBytes:            fsBytes,
		UserPersistentData: "",
	}, nil
}

// AIForgeToLoadedSkill converts a skillmd forge back into a loaded skill.
func AIForgeToLoadedSkill(forge *schema.AIForge) (*LoadedSkill, error) {
	if forge == nil {
		return nil, utils.Error("forge is nil")
	}
	if forge.ForgeType != schema.FORGE_TYPE_SkillMD {
		return nil, utils.Errorf("forge %q is not skillmd type", forge.ForgeName)
	}

	skillMDContent, err := GenerateSkillMDFromAIForge(forge)
	if err != nil {
		return nil, utils.Wrap(err, "render skill markdown from forge failed")
	}

	meta, err := ParseSkillMeta(skillMDContent)
	if err != nil {
		return nil, utils.Wrap(err, "parse synthesized skill markdown failed")
	}

	fsys, err := filesys.NewGzipFSFromBytes(forge.FSBytes)
	if err != nil {
		return nil, utils.Wrap(err, "restore forge gzip filesystem failed")
	}
	fsys.AddFile(skillMDFilename, skillMDContent)

	return &LoadedSkill{
		Meta:           meta,
		FileSystem:     fsys,
		SkillMDContent: skillMDContent,
	}, nil
}

// BuildSkillSourceFSFromForges converts skillmd forges into a root filesystem compatible
// with the existing filesystem-based skill loaders.
func BuildSkillSourceFSFromForges(forges []*schema.AIForge) (fi.FileSystem, int, error) {
	root := filesys.NewVirtualFs()
	count := 0
	for _, forge := range forges {
		if forge == nil || forge.ForgeType != schema.FORGE_TYPE_SkillMD {
			continue
		}
		loaded, err := AIForgeToLoadedSkill(forge)
		if err != nil {
			return nil, count, utils.Wrapf(err, "convert forge %q to skill failed", forge.ForgeName)
		}
		if err := copyFileSystemWithPrefix(root, loaded.FileSystem, loaded.Meta.Name); err != nil {
			return nil, count, utils.Wrapf(err, "copy skill filesystem for %q failed", forge.ForgeName)
		}
		count++
	}
	return root, count, nil
}

func copyFileSystemWithPrefix(dest *filesys.VirtualFS, src fi.FileSystem, prefix string) error {
	if dest == nil || src == nil {
		return utils.Error("source or destination filesystem is nil")
	}
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		return utils.Error("filesystem prefix is empty")
	}

	return filesys.Recursive(".",
		filesys.WithFileSystem(src),
		filesys.WithStat(func(isDir bool, pathname string, info fs.FileInfo) error {
			rel := strings.Trim(strings.TrimPrefix(strings.ReplaceAll(pathname, "\\", "/"), "./"), "/")
			if rel == "" || rel == "." {
				return nil
			}
			target := path.Join(prefix, rel)
			if isDir {
				dest.AddDir(target)
				return nil
			}
			content, err := src.ReadFile(pathname)
			if err != nil {
				return utils.Wrapf(err, "read source file failed: %s", pathname)
			}
			dest.AddFile(target, string(content))
			return nil
		}),
	)
}

func buildForgeDescriptionFromSkillMeta(meta *SkillMeta) string {
	if meta == nil {
		return ""
	}
	if strings.TrimSpace(meta.Compatibility) == "" {
		return meta.Description
	}
	return strings.TrimSpace(meta.Description) + "\nCompatibility: " + strings.TrimSpace(meta.Compatibility)
}

func metadataToForgeTags(metadata map[string]string) string {
	if len(metadata) == 0 {
		return ""
	}
	keys := make([]string, 0, len(metadata))
	for k := range metadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s:%s", k, metadata[k]))
	}
	return strings.Join(parts, ",")
}

func forgeTagsToMetadata(tags string) map[string]string {
	parts := utils.StringSplitAndStrip(tags, ",")
	if len(parts) == 0 {
		return nil
	}
	result := make(map[string]string, len(parts))
	for _, part := range parts {
		key, value, ok := strings.Cut(part, ":")
		if !ok {
			result[ForgeTagUnknownKey] = part
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			key = ForgeTagUnknownKey
		}
		result[key] = value
	}
	return result
}

func renderSkillMDContent(meta *SkillMeta) (string, error) {
	if meta == nil {
		return "", utils.Error("skill meta is nil")
	}
	type skillMetaYAML struct {
		Name                   string            `yaml:"name"`
		Description            string            `yaml:"description"`
		License                string            `yaml:"license,omitempty"`
		Compatibility          string            `yaml:"compatibility,omitempty"`
		Metadata               map[string]string `yaml:"metadata,omitempty"`
		DisableModelInvocation bool              `yaml:"disable-model-invocation,omitempty"`
	}
	payload := skillMetaYAML{
		Name:                   meta.Name,
		Description:            meta.Description,
		License:                meta.License,
		Compatibility:          meta.Compatibility,
		Metadata:               meta.Metadata,
		DisableModelInvocation: meta.DisableModelInvocation,
	}
	frontmatter, err := yaml.Marshal(payload)
	if err != nil {
		return "", utils.Wrap(err, "marshal skill metadata failed")
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(frontmatter)
	buf.WriteString("---\n")
	if strings.TrimSpace(meta.Body) != "" {
		buf.WriteString(strings.TrimSpace(meta.Body))
		buf.WriteString("\n")
	}
	return buf.String(), nil
}
