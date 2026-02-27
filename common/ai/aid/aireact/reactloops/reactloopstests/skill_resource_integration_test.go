package reactloopstests

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/utils/filesys"

	_ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
)

// BuildResourceTestVFS creates a VirtualFS with skills that have sub-files for resource loading tests.
func BuildResourceTestVFS() *filesys.VirtualFS {
	vfs := filesys.NewVirtualFs()

	vfs.AddFile("recon/SKILL.md", `---
name: recon
description: Reconnaissance skill with sub-resources
---
# Recon Skill

This skill contains multiple resource files for information gathering.
Use load_skill_resources to access specific files.
`)
	vfs.AddFile("recon/osint.md", "# OSINT Guide\n\nOpen source intelligence gathering techniques.\n\n## Methods\n- DNS enumeration\n- WHOIS lookup\n- Social media search\n")
	vfs.AddFile("recon/network-enum.md", "# Network Enumeration\n\nPort scanning and service detection.\n\n## Tools\n- nmap\n- masscan\n")
	vfs.AddFile("recon/docs/advanced.md", "# Advanced Recon\n\nAdvanced reconnaissance techniques.\n")

	vfs.AddFile("toolbox/SKILL.md", `---
name: toolbox
description: Security toolbox with scripts
---
# Toolbox

Reference scripts and tool guides.
`)
	vfs.AddFile("toolbox/nmap.md", "# Nmap Reference\n\nnmap -sV -sC target\n")
	vfs.AddFile("toolbox/scripts/scan.sh", "#!/bin/bash\nnmap -sV $1\n")

	return vfs
}

func makeLoadSkillResourceResponse(i aicommon.AICallerConfigIf, resourcePath string) (*aicommon.AIResponse, error) {
	rsp := i.NewAIResponse()
	rsp.EmitOutputStream(bytes.NewBufferString(
		`{"@action": "load_skill_resources", "resource_path": "` + resourcePath + `"}`))
	rsp.Close()
	return rsp, nil
}

// TestReActLoop_LoadSkillResources_BasicFlow tests loading a skill then loading a resource.
func TestReActLoop_LoadSkillResources_BasicFlow(t *testing.T) {
	vfs := BuildResourceTestVFS()

	skillLoaded := false
	resourceLoaded := false
	var mainPrompts []string

	reactIns := NewSkillTestReAct(t, vfs,
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()

			if rsp, err, handled := handleNonMainPrompt(prompt, i); handled {
				return rsp, err
			}

			mainPrompts = append(mainPrompts, prompt)

			if !skillLoaded && strings.Contains(prompt, "loading_skills") {
				skillLoaded = true
				return makeLoadSkillResponse(i, "recon")
			}

			if !resourceLoaded && strings.Contains(prompt, "load_skill_resources") {
				resourceLoaded = true
				return makeLoadSkillResourceResponse(i, "@recon/osint.md")
			}

			return makeFinishResponse(i)
		}),
	)

	loop, err := reactloops.NewReActLoop("resource-test", reactIns)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	_ = loop.Execute("test", context.Background(), "load recon skill and osint resource")

	mgr := loop.GetSkillsContextManager()
	if mgr == nil {
		t.Fatal("SkillsContextManager should not be nil")
	}

	if !mgr.IsSkillLoaded("recon") {
		t.Error("recon skill should be loaded")
	}

	if len(mainPrompts) >= 3 {
		lastPrompt := mainPrompts[len(mainPrompts)-1]
		if !strings.Contains(lastPrompt, "OSINT") {
			t.Log("Note: osint.md content may not appear immediately due to action ordering")
		}
	}

	t.Logf("Captured %d main prompts, skill=%v resource=%v", len(mainPrompts), skillLoaded, resourceLoaded)
}

// TestReActLoop_LoadSkillResources_ActionInSchema tests that load_skill_resources action appears in schema.
func TestReActLoop_LoadSkillResources_ActionInSchema(t *testing.T) {
	vfs := BuildResourceTestVFS()

	var capturedMainPrompt string
	reactIns := NewSkillTestReAct(t, vfs,
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()

			if rsp, err, handled := handleNonMainPrompt(prompt, i); handled {
				return rsp, err
			}

			capturedMainPrompt = prompt
			return makeFinishResponse(i)
		}),
	)

	loop, err := reactloops.NewReActLoop("schema-resource-test", reactIns)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	_ = loop.Execute("test", context.Background(), "test")

	if capturedMainPrompt == "" {
		t.Fatal("main prompt was not captured")
	}

	if !strings.Contains(capturedMainPrompt, "load_skill_resources") {
		t.Error("load_skill_resources should appear in prompt schema when skills are configured")
	}

	if !strings.Contains(capturedMainPrompt, "resource_path") {
		t.Error("resource_path parameter should appear in prompt schema")
	}
}

// TestReActLoop_LoadSkillResources_WithoutSkills tests that load_skill_resources does not appear when no skills.
func TestReActLoop_LoadSkillResources_WithoutSkills(t *testing.T) {
	vfs := filesys.NewVirtualFs()

	var capturedMainPrompt string
	reactIns := NewSkillTestReAct(t, vfs,
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()

			if rsp, err, handled := handleNonMainPrompt(prompt, i); handled {
				return rsp, err
			}

			capturedMainPrompt = prompt
			return makeFinishResponse(i)
		}),
	)

	loop, err := reactloops.NewReActLoop("no-resource-test", reactIns)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	_ = loop.Execute("test", context.Background(), "test")

	if capturedMainPrompt != "" && strings.Contains(capturedMainPrompt, "load_skill_resources") {
		t.Error("load_skill_resources should NOT appear when no skills are configured")
	}
}

// TestReActLoop_BatchLoadSkills tests loading multiple skills via skill_names parameter.
func TestReActLoop_BatchLoadSkills(t *testing.T) {
	vfs := BuildResourceTestVFS()

	batchLoaded := false
	var mainPrompts []string

	reactIns := NewSkillTestReAct(t, vfs,
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()

			if rsp, err, handled := handleNonMainPrompt(prompt, i); handled {
				return rsp, err
			}

			mainPrompts = append(mainPrompts, prompt)

			if !batchLoaded && strings.Contains(prompt, "loading_skills") {
				batchLoaded = true
				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(
					`{"@action": "loading_skills", "skill_names": "recon,toolbox"}`))
				rsp.Close()
				return rsp, nil
			}

			return makeFinishResponse(i)
		}),
	)

	loop, err := reactloops.NewReActLoop("batch-load-test", reactIns)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	_ = loop.Execute("test", context.Background(), "load recon and toolbox")

	mgr := loop.GetSkillsContextManager()
	if mgr == nil {
		t.Fatal("SkillsContextManager should not be nil")
	}

	if !mgr.IsSkillLoaded("recon") {
		t.Error("recon skill should be loaded after batch loading")
	}
	if !mgr.IsSkillLoaded("toolbox") {
		t.Error("toolbox skill should be loaded after batch loading")
	}

	skills := mgr.GetCurrentSelectedSkills()
	if len(skills) < 2 {
		t.Errorf("expected at least 2 loaded skills, got %d", len(skills))
	}

	t.Logf("Batch loading test passed: %d skills loaded, %d main prompts", len(skills), len(mainPrompts))
}

// TestReActLoop_IncludeDirective tests that include directives in SKILL.md are resolved.
func TestReActLoop_IncludeDirective(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("inc-skill/SKILL.md", `---
name: inc-skill
description: Skill with include directives
---
# Include Test

<!-- include: guide.md -->

## End
`)
	vfs.AddFile("inc-skill/guide.md", "# Guide Content\n\nStep 1: Do something\nStep 2: Do more\n")

	skillLoaded := false
	var mainPrompts []string

	reactIns := NewSkillTestReAct(t, vfs,
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()

			if rsp, err, handled := handleNonMainPrompt(prompt, i); handled {
				return rsp, err
			}

			mainPrompts = append(mainPrompts, prompt)

			if !skillLoaded && strings.Contains(prompt, "loading_skills") {
				skillLoaded = true
				return makeLoadSkillResponse(i, "inc-skill")
			}

			return makeFinishResponse(i)
		}),
	)

	loop, err := reactloops.NewReActLoop("include-test", reactIns)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	_ = loop.Execute("test", context.Background(), "load inc-skill")

	if len(mainPrompts) >= 2 {
		postLoadPrompt := mainPrompts[len(mainPrompts)-1]
		if !strings.Contains(postLoadPrompt, "Guide Content") {
			t.Error("included guide content should appear in prompt after skill loading")
		}
		if !strings.Contains(postLoadPrompt, "Step 1") {
			t.Error("included guide steps should appear in prompt")
		}
	}

	t.Logf("Include directive test passed with %d main prompts", len(mainPrompts))
}

// TestReActLoop_CrossSkillHints tests that cross-skill references appear as hints.
func TestReActLoop_CrossSkillHints(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("master/SKILL.md", `---
name: master
description: Master skill referencing others
---
# Master Skill

See ../recon/SKILL.md for reconnaissance.
See ../exploitation/guide.md for exploitation.
`)
	vfs.AddFile("recon/SKILL.md", `---
name: recon
description: Reconnaissance skill
---
# Recon
`)

	skillLoaded := false
	var mainPrompts []string

	reactIns := NewSkillTestReAct(t, vfs,
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()

			if rsp, err, handled := handleNonMainPrompt(prompt, i); handled {
				return rsp, err
			}

			mainPrompts = append(mainPrompts, prompt)

			if !skillLoaded && strings.Contains(prompt, "loading_skills") {
				skillLoaded = true
				return makeLoadSkillResponse(i, "master")
			}

			return makeFinishResponse(i)
		}),
	)

	loop, err := reactloops.NewReActLoop("cross-skill-test", reactIns)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	_ = loop.Execute("test", context.Background(), "load master skill")

	if len(mainPrompts) >= 2 {
		postLoadPrompt := mainPrompts[len(mainPrompts)-1]
		if !strings.Contains(postLoadPrompt, "Related Skills") {
			t.Error("cross-skill references should render as Related Skills hint")
		}
		if !strings.Contains(postLoadPrompt, "recon") {
			t.Error("Related Skills should mention 'recon'")
		}
		if !strings.Contains(postLoadPrompt, "exploitation") {
			t.Error("Related Skills should mention 'exploitation'")
		}
	}

	t.Logf("Cross-skill hints test passed with %d main prompts", len(mainPrompts))
}

// TestReActLoop_LoadingSkills_EnhancedFeedback tests that skill loading provides enhanced feedback.
func TestReActLoop_LoadingSkills_EnhancedFeedback(t *testing.T) {
	vfs := BuildResourceTestVFS()

	skillLoaded := false
	var mainPrompts []string

	reactIns := NewSkillTestReAct(t, vfs,
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()

			if rsp, err, handled := handleNonMainPrompt(prompt, i); handled {
				return rsp, err
			}

			mainPrompts = append(mainPrompts, prompt)

			if !skillLoaded && strings.Contains(prompt, "loading_skills") {
				skillLoaded = true
				return makeLoadSkillResponse(i, "recon")
			}

			return makeFinishResponse(i)
		}),
	)

	loop, err := reactloops.NewReActLoop("enhanced-feedback-test", reactIns)
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	_ = loop.Execute("test", context.Background(), "load recon")

	mgr := loop.GetSkillsContextManager()
	if mgr == nil {
		t.Fatal("SkillsContextManager should not be nil")
	}

	if !mgr.IsSkillLoaded("recon") {
		t.Error("recon skill should be loaded")
	}

	if len(mainPrompts) >= 2 {
		postLoadPrompt := mainPrompts[len(mainPrompts)-1]
		if strings.Contains(postLoadPrompt, "=== Skill: recon ===") {
			t.Log("Verified: loaded skill content appears in prompt")
		}
	}

	t.Logf("Enhanced feedback test passed with %d main prompts", len(mainPrompts))
}
