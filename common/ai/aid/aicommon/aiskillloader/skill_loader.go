package aiskillloader

import (
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// LoadedSkill represents a fully loaded skill with its metadata, filesystem, and content.
type LoadedSkill struct {
	// Meta is the parsed SKILL.md metadata.
	Meta *SkillMeta

	// FileSystem is the read-only filesystem of this skill.
	FileSystem fi.FileSystem

	// SkillMDContent is the raw content of SKILL.md.
	SkillMDContent string
}

// SkillLoader is the interface for loading skills from various sources.
// Implementations must be safe for concurrent use.
type SkillLoader interface {
	// LoadSkill loads a specific skill by name.
	// Returns the fully loaded skill with metadata, filesystem, and content.
	LoadSkill(name string) (*LoadedSkill, error)

	// GetFileSystem returns the read-only filesystem for a specific skill.
	GetFileSystem(name string) (fi.FileSystem, error)

	// HasSkills returns true if at least one skill is registered.
	HasSkills() bool

	// AllSkillMetas returns metadata for all available skills.
	// It is a pure data accessor; filtering/searching should be handled by manager.
	AllSkillMetas() []*SkillMeta
}
