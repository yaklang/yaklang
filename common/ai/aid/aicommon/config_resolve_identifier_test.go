package aicommon

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// mockForgeFactory is a minimal mock of the AIForgeFactory interface for testing.
type mockForgeFactory struct {
	forges map[string]*schema.AIForge
}

func (m *mockForgeFactory) Query(ctx context.Context, opts ...ForgeQueryOption) ([]*schema.AIForge, error) {
	var result []*schema.AIForge
	for _, f := range m.forges {
		result = append(result, f)
	}
	return result, nil
}

func (m *mockForgeFactory) GetAIForge(name string) (*schema.AIForge, error) {
	if f, ok := m.forges[name]; ok {
		return f, nil
	}
	return nil, fmt.Errorf("forge %q not found", name)
}

func (m *mockForgeFactory) GenerateAIForgeListForPrompt(forges []*schema.AIForge) (string, error) {
	return "", nil
}

func (m *mockForgeFactory) GenerateAIJSONSchemaFromSchemaAIForge(forge *schema.AIForge) (string, error) {
	return "", nil
}

// createTestTool creates a simple test tool with a no-op callback.
func createTestTool(name, description string) *aitool.Tool {
	tool, _ := aitool.New(name,
		aitool.WithDescription(description),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			return "ok", nil
		}),
	)
	return tool
}

// createTestSkillLoader creates a skill loader from an in-memory filesystem
// containing a single skill with the given name.
func createTestSkillLoader(skillName string) *aiskillloader.AutoSkillLoader {
	memFS := filesys.NewVirtualFs()
	memFS.AddFile(skillName+"/SKILL.md", `---
name: `+skillName+`
description: "Test skill for `+skillName+`"
---

# `+skillName+`

This is a test skill.
`)

	loader, err := aiskillloader.NewAutoSkillLoader(
		aiskillloader.WithAutoLoad_FileSystem(memFS),
	)
	if err != nil {
		panic("failed to create test skill loader: " + err.Error())
	}
	return loader
}

func TestConfig_ResolveIdentifier_Tool(t *testing.T) {
	testTool := createTestTool("my-test-tool", "A test tool")
	require.NotNil(t, testTool)

	toolManager := buildinaitools.NewToolManagerByToolGetter(
		func() []*aitool.Tool { return []*aitool.Tool{testTool} },
		buildinaitools.WithExtendTools([]*aitool.Tool{testTool}, true),
	)

	cfg := &Config{
		AiToolManager: toolManager,
	}

	result := cfg.ResolveIdentifier("my-test-tool")
	assert.Equal(t, ResolvedAs_Tool, result.IdentityType)
	assert.Equal(t, schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL, result.ActionType)
	assert.Equal(t, "my-test-tool", result.Name)
	assert.Contains(t, result.Suggestion, "TOOL")
	assert.Contains(t, result.Suggestion, "require_tool")
	assert.False(t, result.IsUnknown())
}

func TestConfig_ResolveIdentifier_Forge(t *testing.T) {
	forgeFactory := &mockForgeFactory{
		forges: map[string]*schema.AIForge{
			"code-review-forge": {
				ForgeName:   "code-review-forge",
				Description: "A code review forge",
			},
		},
	}

	cfg := &Config{
		AiForgeManager: forgeFactory,
	}

	result := cfg.ResolveIdentifier("code-review-forge")
	assert.Equal(t, ResolvedAs_Forge, result.IdentityType)
	assert.Equal(t, schema.AI_REACT_LOOP_ACTION_REQUIRE_AI_BLUEPRINT, result.ActionType)
	assert.Equal(t, "code-review-forge", result.Name)
	assert.Contains(t, result.Suggestion, "AI Blueprint")
	assert.Contains(t, result.Suggestion, "require_ai_blueprint")
	assert.False(t, result.IsUnknown())
}

func TestConfig_ResolveIdentifier_Skill(t *testing.T) {
	skillLoader := createTestSkillLoader("vuln-verify")

	cfg := &Config{
		skillLoader: skillLoader,
	}

	result := cfg.ResolveIdentifier("vuln-verify")
	assert.Equal(t, ResolvedAs_Skill, result.IdentityType)
	assert.Equal(t, schema.AI_REACT_LOOP_ACTION_LOADING_SKILLS, result.ActionType)
	assert.Equal(t, "vuln-verify", result.Name)
	assert.Contains(t, result.Suggestion, "SKILL")
	assert.Contains(t, result.Suggestion, "loading_skills")
	assert.False(t, result.IsUnknown())
}

func TestConfig_ResolveIdentifier_Unknown(t *testing.T) {
	testTool := createTestTool("existing-tool", "A tool")
	toolManager := buildinaitools.NewToolManagerByToolGetter(
		func() []*aitool.Tool { return []*aitool.Tool{testTool} },
		buildinaitools.WithExtendTools([]*aitool.Tool{testTool}, true),
	)
	forgeFactory := &mockForgeFactory{
		forges: map[string]*schema.AIForge{
			"existing-forge": {ForgeName: "existing-forge"},
		},
	}
	skillLoader := createTestSkillLoader("existing-skill")

	cfg := &Config{
		AiToolManager:  toolManager,
		AiForgeManager: forgeFactory,
		skillLoader:    skillLoader,
	}

	// A name that doesn't exist anywhere
	result := cfg.ResolveIdentifier("non-existent-thing")
	assert.Equal(t, ResolvedAs_Unknown, result.IdentityType)
	assert.Equal(t, "", result.ActionType)
	assert.Equal(t, "non-existent-thing", result.Name)
	assert.Contains(t, result.Suggestion, "does not exist")
	assert.True(t, result.IsUnknown())
}

func TestConfig_ResolveIdentifier_NilManagers(t *testing.T) {
	// Config with all nil managers should not panic
	cfg := &Config{}

	result := cfg.ResolveIdentifier("anything")
	assert.Equal(t, ResolvedAs_Unknown, result.IdentityType)
	assert.True(t, result.IsUnknown())
}

func TestConfig_ResolveIdentifier_EmptyName(t *testing.T) {
	cfg := &Config{}

	result := cfg.ResolveIdentifier("")
	assert.Equal(t, ResolvedAs_Unknown, result.IdentityType)
	assert.True(t, result.IsUnknown())
	assert.Contains(t, result.Suggestion, "empty")
}

func TestConfig_ResolveIdentifier_Priority(t *testing.T) {
	// If a name exists as both a tool and a forge, tool should take priority
	// (since tools are checked first in the resolution order)
	testTool := createTestTool("shared-name", "A tool")
	toolManager := buildinaitools.NewToolManagerByToolGetter(
		func() []*aitool.Tool { return []*aitool.Tool{testTool} },
		buildinaitools.WithExtendTools([]*aitool.Tool{testTool}, true),
	)
	forgeFactory := &mockForgeFactory{
		forges: map[string]*schema.AIForge{
			"shared-name": {ForgeName: "shared-name"},
		},
	}

	cfg := &Config{
		AiToolManager:  toolManager,
		AiForgeManager: forgeFactory,
	}

	result := cfg.ResolveIdentifier("shared-name")
	// Tool should win because it's checked first
	assert.Equal(t, ResolvedAs_Tool, result.IdentityType)
	assert.Equal(t, schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL, result.ActionType)
}
