package aicommon

import (
	"fmt"
	"path/filepath"

	"github.com/yaklang/yaklang/common/ai/aid/coding"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// WithAgentMode selects the session persona. Unspecified mode is the security agent.
func WithAgentMode(mode string) ConfigOption {
	return func(c *Config) error {
		normalized, err := ypb.NormalizeAgentMode(mode)
		if err != nil {
			return err
		}
		c.AgentMode = normalized
		if normalized != ypb.AgentModeCoding {
			c.CodingWorkspaceID, c.CodingWorkingDirectory, c.CodingWorktreeID = "", "", ""
			c.CodingBaseRef, c.CodingBaseCommit = "", ""
			c.CodingSourceRoots, c.CodingWorkspaceRoots = nil, nil
		}
		return nil
	}
}

// Deprecated: use WithAgentMode and WithCodingWorkspace.
func WithCodingMode(enabled bool, workspaceID, directory string) ConfigOption {
	mode := ypb.AgentModeSecurity
	if enabled {
		mode = ypb.AgentModeCoding
	}
	return func(c *Config) error {
		if err := WithAgentMode(mode)(c); err != nil {
			return err
		}
		return WithCodingWorkspace(workspaceID, directory)(c)
	}
}

// WithCodingWorkspace keeps source code separate from Workdir, which owns disposable artifacts.
// Tools must use explicit absolute paths / shell working directories, never os.Chdir.
func WithCodingWorkspace(workspaceID, directory string) ConfigOption {
	return func(c *Config) error {
		enabled := c.AgentMode == ypb.AgentModeCoding
		if enabled {
			path, err := coding.Directory(directory)
			if err != nil {
				return fmt.Errorf("coding working directory: %w", err)
			}
			directory = filepath.Clean(path)
		} else {
			workspaceID, directory = "", ""
		}
		c.CodingWorktreeID = ""
		c.CodingBaseRef, c.CodingBaseCommit = "", ""
		c.CodingWorkspaceRoots, c.CodingSourceRoots = nil, nil
		if enabled {
			c.CodingWorkspaceRoots = []string{directory}
			c.CodingSourceRoots = []string{directory}
		}
		c.CodingWorkspaceID = workspaceID
		c.CodingWorkingDirectory = directory
		return nil
	}
}

// WithCodingWorktreeBase carries immutable creation metadata from the managed
// worktree record. It does not describe the current branch or HEAD.
func WithCodingWorktreeBase(ref, commit string) ConfigOption {
	return func(c *Config) error {
		if c.AgentMode == ypb.AgentModeCoding && c.CodingWorktreeID != "" {
			c.CodingBaseRef, c.CodingBaseCommit = ref, commit
		}
		return nil
	}
}

// WithCodingDirectories propagates the server-resolved snapshot to child agents.
func WithCodingDirectories(worktreeID string, sourceRoots, workspaceRoots []string) ConfigOption {
	sources, roots := append([]string(nil), sourceRoots...), append([]string(nil), workspaceRoots...)
	return func(c *Config) error {
		if c.AgentMode != ypb.AgentModeCoding {
			return nil
		}
		effective, original := roots, sources
		if len(effective) == 0 {
			effective = []string{c.CodingWorkingDirectory}
		}
		if len(original) == 0 {
			original = effective
		}
		if len(original) != len(effective) || effective[0] != c.CodingWorkingDirectory {
			return fmt.Errorf("inconsistent coding directories")
		}
		for _, root := range effective {
			path, err := coding.Directory(root)
			if err != nil {
				return err
			}
			if path != root {
				return fmt.Errorf("coding directory identity changed: %s", root)
			}
		}
		c.CodingWorktreeID = worktreeID
		c.CodingSourceRoots = append([]string(nil), original...)
		c.CodingWorkspaceRoots = append([]string(nil), effective...)
		return nil
	}
}
