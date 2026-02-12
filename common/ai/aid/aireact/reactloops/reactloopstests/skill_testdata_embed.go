package reactloopstests

import (
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// BuildTestSkillVFS creates a VirtualFS with test skills for testing.
// This function creates skills programmatically without relying on embedded files.
func BuildTestSkillVFS() *filesys.VirtualFS {
	vfs := filesys.NewVirtualFs()

	// Simple test skill
	vfs.AddFile("test-skill/SKILL.md", `---
name: test-skill
description: A simple test skill for unit testing
---
# Test Skill

This is a test skill for verifying the skill loading functionality.

## Usage
1. Load this skill using loading_skills action
2. Check that the content appears in context
`)

	// Lint check skill (test-only, not the production code-review skill)
	vfs.AddFile("test-lint-check/SKILL.md", `---
name: test-lint-check
description: A test skill for lint checking in unit tests
---
# Test Lint Check Skill

## Steps
1. Analyze code structure
2. Check for lint issues
3. Suggest improvements
`)
	vfs.AddFile("test-lint-check/rules/RULES.md", "# Lint Check Rules\n\nAlways check for common lint issues.")

	return vfs
}

// BuildLargeSkillVFS creates a VirtualFS with a large skill for truncation testing.
func BuildLargeSkillVFS() *filesys.VirtualFS {
	vfs := filesys.NewVirtualFs()

	// Create a skill with many lines to test truncation
	var body string
	for i := 1; i <= 500; i++ {
		body += "Line of content for truncation testing.\n"
	}

	vfs.AddFile("large-skill/SKILL.md", `---
name: large-skill
description: A large skill for view offset testing
---
# Large Skill

`+body)

	return vfs
}

// NewSkillTestReAct creates a test ReAct instance with skills loaded from VFS.
// Uses the accumulative WithSkillsFS option so multiple calls can add more sources.
func NewSkillTestReAct(t *testing.T, vfs *filesys.VirtualFS, opts ...aicommon.ConfigOption) *aireact.ReAct {
	t.Helper()

	allOpts := append([]aicommon.ConfigOption{
		aicommon.WithSkillsFS(vfs),
	}, opts...)

	react, err := aireact.NewTestReAct(allOpts...)
	if err != nil {
		t.Fatalf("failed to create ReAct with skills: %v", err)
	}

	return react
}
