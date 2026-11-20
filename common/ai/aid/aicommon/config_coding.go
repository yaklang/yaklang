package aicommon

import (
	"fmt"
	"path/filepath"

	"github.com/yaklang/yaklang/common/ai/aid/coding"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// CodingConfig is the session's resolved project snapshot, separate from Config.Workdir
// (disposable AI artifacts). Tools use explicit paths / cwd, never os.Chdir.
type CodingConfig struct {
	WorkspaceID string
	// PrimaryProjectDir is the session's actual source-code working directory.
	// With a managed worktree it points into that checkout, preserving any project subdirectory.
	PrimaryProjectDir string
	// AdditionalProjectDirs are the remaining user-selected folders, in selection order.
	// They stay in their original locations when only the primary project gets a worktree.
	AdditionalProjectDirs []string
	Worktree              *CodingWorktreeConfig // nil when working in the original project.
}

// CodingWorktreeConfig describes the managed checkout's origin, not its current branch/HEAD.
type CodingWorktreeConfig struct {
	ID               string
	SourceProjectDir string // Original primary project directory, before worktree substitution.
	BaseRef          string
	BaseCommit       string
}

// ProjectDirs returns [PrimaryProjectDir, AdditionalProjectDirs...].
// The returned slice is independent of the session snapshot.
func (c CodingConfig) ProjectDirs() []string {
	if c.PrimaryProjectDir == "" {
		return nil
	}
	return append([]string{c.PrimaryProjectDir}, c.AdditionalProjectDirs...)
}

// SourceProjectDirs restores only the original primary directory; secondary projects are shared.
func (c CodingConfig) SourceProjectDirs() []string {
	dirs := c.ProjectDirs()
	if len(dirs) > 0 && c.Worktree != nil {
		dirs[0] = c.Worktree.SourceProjectDir
	}
	return dirs
}

// Clone isolates child agents and prompt snapshots from subsequent mutations.
func (c CodingConfig) Clone() CodingConfig {
	c.AdditionalProjectDirs = append([]string(nil), c.AdditionalProjectDirs...)
	if c.Worktree != nil {
		worktree := *c.Worktree
		c.Worktree = &worktree
	}
	return c
}

// WithCodingConfig installs one complete, resolved project snapshot.
// Call WithAgentMode first; security sessions ignore Coding configuration.
func WithCodingConfig(config CodingConfig) ConfigOption {
	snapshot := config.Clone()
	return func(c *Config) error {
		if c.AgentMode != ypb.AgentModeCoding {
			return nil
		}
		if snapshot.PrimaryProjectDir == "" {
			return fmt.Errorf("coding primary project directory is required")
		}
		if tree := snapshot.Worktree; tree != nil {
			if tree.ID == "" || !filepath.IsAbs(tree.SourceProjectDir) {
				return fmt.Errorf("coding worktree requires an ID and absolute source project directory")
			}
		}
		// Only effective directories must exist. A worktree may outlive its source checkout.
		for _, dir := range snapshot.ProjectDirs() {
			canonical, err := coding.Directory(dir)
			if err != nil {
				return err
			}
			if canonical != dir {
				return fmt.Errorf("coding directory identity changed: %s", dir)
			}
		}
		c.Coding = snapshot.Clone()
		return nil
	}
}
