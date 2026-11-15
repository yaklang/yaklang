package aicommon

import (
	"fmt"
	"path/filepath"

	"github.com/yaklang/yaklang/common/ai/aid/coding"
)

// WithCodingMode keeps source code separate from Workdir, which owns disposable artifacts.
// Tools must use explicit absolute paths / shell working directories, never os.Chdir.
func WithCodingMode(enabled bool, workspaceID, directory string) ConfigOption {
	return func(c *Config) error {
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
		c.CodingMode = enabled
		c.CodingWorkspaceID = workspaceID
		c.CodingWorkingDirectory = directory
		return nil
	}
}

// WithCodingWorktreeBase carries immutable creation metadata from the managed
// worktree record. It does not describe the current branch or HEAD.
func WithCodingWorktreeBase(ref, commit string) ConfigOption {
	return func(c *Config) error {
		if c.CodingMode && c.CodingWorktreeID != "" {
			c.CodingBaseRef, c.CodingBaseCommit = ref, commit
		}
		return nil
	}
}

// WithCodingDirectories propagates the server-resolved snapshot to child agents.
func WithCodingDirectories(worktreeID string, sourceRoots, workspaceRoots []string) ConfigOption {
	sources, roots := append([]string(nil), sourceRoots...), append([]string(nil), workspaceRoots...)
	return func(c *Config) error {
		if !c.CodingMode {
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
