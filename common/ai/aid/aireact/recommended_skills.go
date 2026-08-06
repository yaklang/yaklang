package aireact

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// RecommendedSkill describes a product-recommended built-in skill exposed to
// clients. Name is the value clients pass through AIStartParams.EnabledCapabilities.
type RecommendedSkill struct {
	Name            string
	DisplayNameZhCN string
	Description     string
	Content         string
	IsModified      bool
}

// recommendedBuiltinSkills is intentionally a fixed product list rather than
// an enumeration of every built-in or user-installed skill. Additions here are
// therefore explicit UI recommendations.
var recommendedBuiltinSkills = []struct {
	name string
}{
	{name: "security-engineering"},
	{name: "code-review"},
	{name: "pentest-task-design"},
}

var recommendedSkillsMu sync.Mutex

type recommendedSkillDefault struct {
	rawDocument string
	document    *aiskillloader.SkillDocument
}

// GetRecommendedBuiltinSkills returns the current editable content together
// with immutable presentation metadata from the embedded defaults.
func GetRecommendedBuiltinSkills() ([]RecommendedSkill, error) {
	recommendedSkillsMu.Lock()
	defer recommendedSkillsMu.Unlock()

	return getRecommendedBuiltinSkillsAtDir(consts.GetDefaultAISkillsDir())
}

func getRecommendedBuiltinSkillsAtDir(skillsDir string) ([]RecommendedSkill, error) {
	result := make([]RecommendedSkill, 0, len(recommendedBuiltinSkills))
	for _, definition := range recommendedBuiltinSkills {
		defaultSkill, err := readRecommendedDefault(definition.name)
		if err != nil {
			return nil, err
		}

		currentDocument := defaultSkill.rawDocument
		localPath := recommendedSkillLocalPath(skillsDir, definition.name)
		if content, readErr := os.ReadFile(localPath); readErr == nil {
			currentDocument = string(content)
		} else if !os.IsNotExist(readErr) {
			return nil, fmt.Errorf("read recommended skill %q: %w", definition.name, readErr)
		}

		result = append(result, recommendedSkillFromDocument(defaultSkill, currentDocument))
	}

	return result, nil
}

// UpdateRecommendedBuiltinSkill replaces only the editable Markdown body. The
// embedded frontmatter remains authoritative so identity and display metadata
// cannot be corrupted by a user edit.
func UpdateRecommendedBuiltinSkill(name, content string) (RecommendedSkill, error) {
	recommendedSkillsMu.Lock()
	defer recommendedSkillsMu.Unlock()

	return updateRecommendedBuiltinSkillAtDir(consts.GetDefaultAISkillsDir(), name, content)
}

func updateRecommendedBuiltinSkillAtDir(skillsDir, name, content string) (RecommendedSkill, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return RecommendedSkill{}, fmt.Errorf("recommended skill content cannot be empty")
	}

	defaultSkill, err := readRecommendedDefault(name)
	if err != nil {
		return RecommendedSkill{}, err
	}
	updatedDocument, err := defaultSkill.document.ReplaceBody(content)
	if err != nil {
		return RecommendedSkill{}, fmt.Errorf("build recommended skill %q: %w", name, err)
	}
	if err := writeRecommendedSkill(skillsDir, name, updatedDocument); err != nil {
		return RecommendedSkill{}, err
	}

	return recommendedSkillFromBody(defaultSkill, content, true), nil
}

// ResetRecommendedBuiltinSkill restores the complete embedded SKILL.md file.
func ResetRecommendedBuiltinSkill(name string) (RecommendedSkill, error) {
	recommendedSkillsMu.Lock()
	defer recommendedSkillsMu.Unlock()

	return resetRecommendedBuiltinSkillAtDir(consts.GetDefaultAISkillsDir(), name)
}

func resetRecommendedBuiltinSkillAtDir(skillsDir, name string) (RecommendedSkill, error) {
	defaultSkill, err := readRecommendedDefault(name)
	if err != nil {
		return RecommendedSkill{}, err
	}
	if err := writeRecommendedSkill(skillsDir, name, defaultSkill.rawDocument); err != nil {
		return RecommendedSkill{}, err
	}

	return recommendedSkillFromBody(defaultSkill, defaultSkill.document.Meta.Body, true), nil
}

func recommendedSkillFromDocument(defaultSkill *recommendedSkillDefault, currentDocument string) RecommendedSkill {
	currentBody := currentDocument
	valid := false
	if current, err := aiskillloader.ParseSkillDocument(currentDocument); err == nil && current.Meta.Name == defaultSkill.document.Meta.Name {
		currentBody = current.Meta.Body
		valid = true
	}
	return recommendedSkillFromBody(defaultSkill, currentBody, valid)
}

func recommendedSkillFromBody(defaultSkill *recommendedSkillDefault, body string, valid bool) RecommendedSkill {
	defaultMeta := defaultSkill.document.Meta
	body = strings.TrimSpace(body)
	return RecommendedSkill{
		Name:            defaultMeta.Name,
		DisplayNameZhCN: defaultMeta.GetDisplayName(aiskillloader.SkillLocaleZhCN),
		Description:     defaultMeta.Description,
		Content:         body,
		IsModified:      !valid || body != strings.TrimSpace(defaultMeta.Body),
	}
}

func readRecommendedDefault(name string) (*recommendedSkillDefault, error) {
	if !isRecommendedBuiltinSkill(name) {
		return nil, fmt.Errorf("skill %q is not in the recommended list", name)
	}
	builtinFS := GetBuiltinSkillsFS()
	if builtinFS == nil {
		return nil, fmt.Errorf("built-in skills filesystem is not initialized")
	}

	skillPath := path.Join("skills", name, "SKILL.md")
	content, err := builtinFS.ReadFile(skillPath)
	if err != nil {
		return nil, fmt.Errorf("read recommended built-in skill %q: %w", name, err)
	}
	document, err := aiskillloader.ParseSkillDocument(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse recommended built-in skill %q: %w", name, err)
	}
	meta := document.Meta
	if meta.Name != name {
		return nil, fmt.Errorf("recommended built-in skill name mismatch: registry=%q, metadata=%q", name, meta.Name)
	}
	if meta.GetDisplayName(aiskillloader.SkillLocaleZhCN) == "" {
		return nil, fmt.Errorf("recommended built-in skill %q is missing metadata.%s", name, aiskillloader.SkillMetadataDisplayNameZhCN)
	}

	return &recommendedSkillDefault{rawDocument: string(content), document: document}, nil
}

func isRecommendedBuiltinSkill(name string) bool {
	for _, definition := range recommendedBuiltinSkills {
		if definition.name == name {
			return true
		}
	}
	return false
}

func recommendedSkillLocalPath(skillsDir, name string) string {
	return filepath.Join(skillsDir, "builtin", name, "SKILL.md")
}

func writeRecommendedSkill(skillsDir, name, document string) error {
	targetPath := recommendedSkillLocalPath(skillsDir, name)
	parentDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Errorf("create recommended skill directory %q: %w", name, err)
	}

	temporaryFile, err := os.CreateTemp(parentDir, ".SKILL.md-*")
	if err != nil {
		return fmt.Errorf("create temporary recommended skill %q: %w", name, err)
	}
	temporaryPath := temporaryFile.Name()
	defer os.Remove(temporaryPath)

	if _, err := temporaryFile.WriteString(document); err != nil {
		_ = temporaryFile.Close()
		return fmt.Errorf("write temporary recommended skill %q: %w", name, err)
	}
	if err := temporaryFile.Chmod(0o644); err != nil {
		_ = temporaryFile.Close()
		return fmt.Errorf("set recommended skill permissions %q: %w", name, err)
	}
	if err := temporaryFile.Close(); err != nil {
		return fmt.Errorf("close temporary recommended skill %q: %w", name, err)
	}
	if err := os.Rename(temporaryPath, targetPath); err != nil {
		return fmt.Errorf("replace recommended skill %q: %w", name, err)
	}

	relPath := filepath.ToSlash(filepath.Join(name, "SKILL.md"))
	if db := builtinSkillReleaseDB(); db != nil {
		yakit.DelKey(db, builtinSkillSuppressedKey(relPath))
	}
	markBuiltinSkillReleased(relPath, time.Now())
	return nil
}
