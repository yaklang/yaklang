package sessionruntime

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// codingConfigFromStartParams is the transport boundary. The runtime keeps only
// the primary project, additional projects and the worktree's original primary;
// the protocol's parallel directory arrays never become independent Config state.
func codingConfigFromStartParams(params *ypb.AIStartParams, worktree *schema.AICodingWorktree) (aicommon.CodingConfig, error) {
	var config aicommon.CodingConfig
	if params.GetAgentMode() != ypb.AgentModeCoding {
		return config, nil
	}
	config.WorkspaceID = params.GetCodingWorkspaceID()
	config.PrimaryProjectDir = params.GetCodingWorkingDirectory()
	dirs, sources := params.GetCodingWorkspaceRoots(), params.GetCodingSourceRoots()
	if len(dirs) == 0 {
		dirs = []string{config.PrimaryProjectDir}
	}
	if len(sources) == 0 && params.GetCodingWorktreeID() == "" {
		sources = dirs
	}
	if len(sources) != len(dirs) || dirs[0] != config.PrimaryProjectDir {
		return config, fmt.Errorf("inconsistent coding project directories")
	}
	for i, dir := range dirs {
		if (i > 0 || params.GetCodingWorktreeID() == "") && sources[i] != dir {
			return config, fmt.Errorf("only the primary project directory may be replaced by a worktree")
		}
	}
	config.AdditionalProjectDirs = append([]string(nil), dirs[1:]...)
	if id := params.GetCodingWorktreeID(); id != "" {
		config.Worktree = &aicommon.CodingWorktreeConfig{ID: id, SourceProjectDir: sources[0]}
		if worktree != nil {
			if worktree.WorktreeID != id || worktree.WorkspaceID != config.WorkspaceID ||
				worktree.WorkingDirectory != config.PrimaryProjectDir || worktree.SourceDirectory != sources[0] {
				return config, fmt.Errorf("coding worktree does not match the session project snapshot")
			}
			config.Worktree.BaseRef, config.Worktree.BaseCommit = worktree.BaseRef, worktree.BaseCommit
		}
	}
	return config, nil
}

// applyCodingConfigToStartParams derives the wire snapshot when attaching to an
// existing runtime. Its source binding is authoritative over the attach request.
func applyCodingConfigToStartParams(params *ypb.AIStartParams, config *aicommon.Config) {
	params.AgentMode = config.AgentMode
	params.CodingWorkspaceID = config.Coding.WorkspaceID
	params.CodingWorkingDirectory = config.Coding.PrimaryProjectDir
	params.CodingWorkspaceRoots = config.Coding.ProjectDirs()
	params.CodingSourceRoots = config.Coding.SourceProjectDirs()
	params.CodingWorktreeID = ""
	if tree := config.Coding.Worktree; tree != nil {
		params.CodingWorktreeID = tree.ID
	}
	params.CodingCreateWorktree = false
	params.CodingWorktreeBaseRef = ""
}
