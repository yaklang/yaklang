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

	// Code review skill with additional files
	vfs.AddFile("code-review/SKILL.md", `---
name: code-review
description: Perform automated code review
keywords: code, review, security, lint
---
# Code Review Skill

## Steps
1. Analyze code structure
2. Check for security issues
3. Suggest improvements
`)
	vfs.AddFile("code-review/rules/RULES.md", "# Code Review Rules\n\nAlways check for SQL injection.")

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
