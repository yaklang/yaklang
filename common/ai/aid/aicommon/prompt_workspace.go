package aicommon

import (
	"encoding/json"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ApplyAgentPromptContext propagates the session's mode and resolved binding to
// every prefix caller. Security callers retain their existing path semantics.
// This is a snapshot operation: no directory scans, Git calls or cwd changes.
func ApplyAgentPromptContext(m *PromptMaterials, config *Config) {
	if m == nil || config == nil {
		return
	}
	m.AgentMode = config.AgentMode
	if config.AgentMode != ypb.AgentModeCoding {
		return
	}
	m.Coding = config.Coding.Clone()
	m.AIArtifactsDir = config.Workdir
	m.Workspace = true
}

type codingPromptDirectory struct {
	Primary   bool   `json:"primary"`
	Source    string `json:"source"`
	Effective string `json:"effective"`
}

type codingPromptWorktree struct {
	ID                 string `json:"id"`
	CreationBaseRef    string `json:"creation_base_ref"`
	CreationBaseCommit string `json:"creation_base_commit"`
}

type codingWorkspacePromptData struct {
	WorkspaceID        string                  `json:"workspace_id"`
	OSArch             string                  `json:"os_arch"`
	WorkingDirectory   string                  `json:"working_directory"`
	ArtifactsDirectory string                  `json:"artifacts_directory"`
	Directories        []codingPromptDirectory `json:"directories"`
	Worktree           *codingPromptWorktree   `json:"worktree,omitempty"`
}

func (m *PromptMaterials) codingWorkspaceData() codingWorkspacePromptData {
	data := codingWorkspacePromptData{
		WorkspaceID: m.Coding.WorkspaceID, OSArch: m.OSArch,
		WorkingDirectory: m.Coding.PrimaryProjectDir, ArtifactsDirectory: m.AIArtifactsDir,
	}
	sources := m.Coding.SourceProjectDirs()
	for i, dir := range m.Coding.ProjectDirs() {
		data.Directories = append(data.Directories, codingPromptDirectory{Primary: i == 0, Source: sources[i], Effective: dir})
	}
	if worktree := m.Coding.Worktree; worktree != nil {
		data.Worktree = &codingPromptWorktree{worktree.ID, worktree.BaseRef, worktree.BaseCommit}
	}
	return data
}

// WorkspaceContext renders configured coordinates without inspecting the file system.
func (m *PromptMaterials) WorkspaceContext() string {
	if m == nil || !m.Workspace {
		return ""
	}
	workingDir := m.WorkingDir
	if m.AgentMode == ypb.AgentModeCoding {
		workingDir = m.Coding.PrimaryProjectDir
	}
	if strings.TrimSpace(m.OSArch+workingDir+m.AIArtifactsDir) == "" {
		return ""
	}
	var data any = m
	if m.AgentMode == ypb.AgentModeCoding {
		context := m.codingWorkspaceData()
		// The concrete DTO contains strings/bools only; path escaping belongs here,
		// not in the prompt template. In particular Go's %q is not JSON encoding.
		raw, err := json.MarshalIndent(context, "", "  ")
		if err != nil {
			log.Errorf("encode coding workspace: %v", err)
			return ""
		}
		data = struct {
			WorkspaceContextJSON string
			ManagedWorktree      bool
		}{string(raw), context.Worktree != nil}
	}
	content, err := RenderPromptTemplate("workspace", AgentPrompts(m.AgentMode).WorkspaceTemplate, data)
	if err != nil {
		log.Errorf("render workspace prompt: %v", err)
		return ""
	}
	return strings.TrimSpace(content)
}
