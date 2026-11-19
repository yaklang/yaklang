package schema

import "github.com/yaklang/gorm"

// AICodingWorkspace references user source files. It never owns or deletes them.
type AICodingWorkspace struct {
	gorm.Model
	WorkspaceID string `gorm:"unique_index;not null"`
	Name        string
	RootPath    string      `gorm:"not null"` // Primary directory, mirrored from RootPaths[0] for Git operations.
	RootPaths   StringSlice `gorm:"type:text"`
	IsGit       bool
}

// AICodingSession freezes source/effective directories at the first session start.
// Stored in the profile DB so the workspace can enumerate sessions across project DBs.
type AICodingSession struct {
	gorm.Model
	SessionTitle     string
	SessionDeletedAt int64
	SessionID        string `gorm:"unique_index;not null"`
	WorkspaceID      string `gorm:"index;not null"`
	WorktreeID       string `gorm:"index"`
	WorkingDirectory string
	SourceRoots      StringSlice `gorm:"type:text"`
	WorkspaceRoots   StringSlice `gorm:"type:text"`
}

// AICodingWorktree records only checkouts managed by session startup, not every
// checkout known to Git. A failed reservation is kept for recovery and inspection.
type AICodingWorktree struct {
	gorm.Model
	WorktreeID       string `gorm:"unique_index;not null"`
	WorkspaceID      string `gorm:"index;index:idx_ai_coding_worktree_workspace_path;not null"`
	OwnerSessionID   string `gorm:"unique_index;not null"`
	SourceDirectory  string
	RepositoryRoot   string
	GitCommonDir     string
	Path             string `gorm:"unique_index;index:idx_ai_coding_worktree_workspace_path;not null"`
	WorkingDirectory string
	BaseRef          string
	BaseCommit       string
	State            string
	LastError        string `gorm:"type:text"`
}
