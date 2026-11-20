package yakit

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/coding"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Serialize binding mutations with worktree cleanup, including shared checkouts.
var codingLifecycleMu sync.RWMutex

func validateCodingWorkspace(ctx context.Context, name string, paths []string) (string, []string, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return "", nil, false, fmt.Errorf("workspace name must contain 1 to 100 characters")
	}
	if len(paths) == 0 {
		return "", nil, false, fmt.Errorf("workspace requires at least one source directory")
	}
	roots := make([]string, 0, len(paths))
	seen := make(map[string]bool)
	primaryGit := false
	for i, path := range paths {
		root, isGit, err := coding.Source(ctx, path)
		if err != nil {
			return "", nil, false, err
		}
		if seen[root] {
			return "", nil, false, fmt.Errorf("duplicate workspace directory: %s", root)
		}
		seen[root] = true
		roots = append(roots, root)
		if i == 0 {
			primaryGit = isGit
		}
	}
	return name, roots, primaryGit, nil
}

// The first directory is primary. Workspace identity is independent of its paths.
func CreateAICodingWorkspace(ctx context.Context, db *gorm.DB, name string, paths ...string) (*schema.AICodingWorkspace, error) {
	if db == nil {
		return nil, fmt.Errorf("profile database unavailable")
	}
	name, roots, isGit, err := validateCodingWorkspace(ctx, name, paths)
	if err != nil {
		return nil, err
	}
	item := &schema.AICodingWorkspace{WorkspaceID: uuid.NewString(), Name: name, RootPath: roots[0], RootPaths: roots, IsGit: isGit}
	if err := db.Create(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func GetAICodingWorkspace(db *gorm.DB, id string) (*schema.AICodingWorkspace, error) {
	if db == nil {
		return nil, fmt.Errorf("profile database unavailable")
	}
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("select a coding workspace first")
	}
	var item schema.AICodingWorkspace
	if err := db.Where("workspace_id = ?", id).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// Updates affect future sessions; existing session bindings retain their snapshots.
func UpdateAICodingWorkspace(ctx context.Context, db *gorm.DB, id, name string, paths []string) (*schema.AICodingWorkspace, error) {
	item, err := GetAICodingWorkspace(db, id)
	if err != nil {
		return nil, err
	}
	name, roots, isGit, err := validateCodingWorkspace(ctx, name, paths)
	if err != nil {
		return nil, err
	}
	item.Name, item.RootPath, item.RootPaths, item.IsGit = name, roots[0], roots, isGit
	if err := db.Save(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func GetAICodingSession(db *gorm.DB, sessionID string) (*schema.AICodingSession, error) {
	var binding schema.AICodingSession
	if err := db.Where("session_id = ?", sessionID).First(&binding).Error; err != nil {
		return nil, err
	}
	return &binding, nil
}

func ListAICodingSessions(db *gorm.DB, workspaceID string) ([]*schema.AICodingSession, error) {
	var items []*schema.AICodingSession
	err := db.Where("workspace_id = ?", workspaceID).Order("id desc").Find(&items).Error
	return items, err
}

func ListAICodingWorktrees(db *gorm.DB, workspaceID string) ([]*schema.AICodingWorktree, error) {
	var items []*schema.AICodingWorktree
	err := db.Where("workspace_id = ?", workspaceID).Order("id desc").Find(&items).Error
	return items, err
}

// Lookup uses persisted ownership; common-dir alone cannot distinguish workspaces
// referencing the same repository. Ancestor candidates retain subdirectory lookup
// without scanning other workspaces or loading the full table.
func GetAICodingWorktree(db *gorm.DB, workspaceID, id, path string) (*schema.AICodingWorktree, error) {
	if workspaceID != "" {
		db = db.Where("workspace_id = ?", workspaceID)
	}
	if id != "" && path != "" {
		return nil, fmt.Errorf("specify worktree ID or path, not both")
	}
	if id != "" {
		var item schema.AICodingWorktree
		if err := db.Where("worktree_id = ?", id).First(&item).Error; err != nil {
			return nil, err
		}
		return &item, nil
	}
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("worktree lookup requires an ID or absolute path")
	}
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	var candidates []string
	for dir := path; ; dir = filepath.Dir(dir) {
		candidates = append(candidates, dir)
		if filepath.Dir(dir) == dir {
			break
		}
	}
	var item schema.AICodingWorktree
	err := db.Where("path IN (?)", candidates).
		Order("length(path) desc").First(&item).Error
	return &item, err
}

func applyCodingBinding(params *ypb.AIStartParams, binding *schema.AICodingSession) {
	params.AgentMode = ypb.AgentModeCoding
	params.CodingWorkspaceID = binding.WorkspaceID
	params.CodingWorkingDirectory = binding.WorkingDirectory
	params.CodingWorktreeID = binding.WorktreeID
	params.CodingWorkspaceRoots = append([]string(nil), binding.WorkspaceRoots...)
	params.CodingSourceRoots = append([]string(nil), binding.SourceRoots...)
	// A creation request is consumed once. Reconnects use the durable worktree ID.
	params.CodingCreateWorktree = false
	params.CodingWorktreeBaseRef = ""
}

func clearCodingBinding(params *ypb.AIStartParams) {
	params.CodingWorkspaceID, params.CodingWorkingDirectory, params.CodingWorktreeID = "", "", ""
	params.CodingWorkspaceRoots, params.CodingSourceRoots = nil, nil
	params.CodingCreateWorktree, params.CodingWorktreeBaseRef = false, ""
}

func validateCodingBinding(ctx context.Context, db *gorm.DB, binding *schema.AICodingSession, progress func(string, *schema.AICodingWorktree)) error {
	if binding.SessionDeletedAt != 0 {
		return fmt.Errorf("coding session has been deleted")
	}
	if binding.WorktreeID != "" {
		item, err := GetAICodingWorktree(db, binding.WorkspaceID, binding.WorktreeID, "")
		if err != nil {
			return err
		}
		if item.WorkspaceID != binding.WorkspaceID || item.WorkingDirectory != binding.WorkingDirectory {
			return fmt.Errorf("session worktree binding is inconsistent")
		}
		if err := ensureCodingWorktree(ctx, db, item, progress); err != nil {
			return err
		}
	}
	for _, root := range binding.WorkspaceRoots {
		canonical, err := coding.Directory(root)
		if err != nil {
			return err
		}
		if canonical != root {
			return fmt.Errorf("session directory identity changed: %s", root)
		}
	}
	return nil
}

// A reservation is persisted before Git runs. Failed setup retries the same path
// and commit; ready checkouts that disappear are reported, never silently recreated.
func ensureCodingWorktree(ctx context.Context, db *gorm.DB, item *schema.AICodingWorktree, progress func(string, *schema.AICodingWorktree)) error {
	if progress != nil {
		progress("checkout", item)
	}
	if item.State == "deleting" {
		return fmt.Errorf("managed worktree deletion is in progress; retry deletion")
	}
	err := validateManagedWorktreePath(item)
	if err == nil && item.State != "ready" {
		err = coding.RecoverWorktree(ctx, item.RepositoryRoot, item.Path, item.GitCommonDir, item.WorkingDirectory, item.BaseCommit)
	}
	if err == nil {
		_, err = coding.InspectWorktree(ctx, item.Path, item.GitCommonDir, item.WorkingDirectory)
	}
	if err != nil {
		if item.State != "ready" {
			if saveErr := db.Model(item).Updates(map[string]interface{}{"state": "failed", "last_error": err.Error()}).Error; saveErr != nil {
				return fmt.Errorf("worktree setup: %v; persist failure: %w", err, saveErr)
			}
		}
		return fmt.Errorf("managed worktree %s unavailable: %w", item.WorktreeID, err)
	}
	if item.State != "ready" {
		return db.Model(item).Updates(map[string]interface{}{"state": "ready", "last_error": ""}).Error
	}
	return nil
}

// CodingWorktreeProgress is a startup snapshot. Only implemented Git stages are
// reported; fetching and environment setup are not part of managed creation.
type CodingWorktreeProgress struct {
	Stage      string `json:"stage"`
	WorktreeID string `json:"worktree_id,omitempty"`
	Path       string `json:"path,omitempty"`
	BaseRef    string `json:"base_ref,omitempty"`
	BaseCommit string `json:"base_commit,omitempty"`
	Error      string `json:"error,omitempty"`
}

// ResolveCodingSessionBinding runs under sessionruntime's per-session start lock.
// It owns all source selection; incoming effective roots are never trusted.
func ResolveCodingSessionBinding(ctx context.Context, profileDB, projectDB *gorm.DB, sessionID string, params *ypb.AIStartParams, observers ...func(CodingWorktreeProgress)) (resultErr error) {
	codingLifecycleMu.RLock()
	defer codingLifecycleMu.RUnlock()
	if params == nil {
		return fmt.Errorf("AI start params required")
	}
	// Progress belongs to the startup attempt, not the model's message stream.
	var progress func(string, *schema.AICodingWorktree)
	if params.GetAgentMode() == ypb.AgentModeCoding && params.GetCodingCreateWorktree() && len(observers) > 0 {
		snapshot := CodingWorktreeProgress{Stage: "workspace"}
		progress = func(stage string, item *schema.AICodingWorktree) {
			snapshot.Stage = stage
			if item != nil {
				snapshot.WorktreeID, snapshot.Path = item.WorktreeID, item.WorkingDirectory
				snapshot.BaseRef, snapshot.BaseCommit = item.BaseRef, item.BaseCommit
			}
			for _, observer := range observers {
				if observer != nil {
					observer(snapshot)
				}
			}
		}
		progress("workspace", nil)
		defer func() {
			if resultErr != nil {
				snapshot.Error = resultErr.Error()
				progress("failed", nil)
			} else {
				progress("ready", nil)
			}
		}()
	}
	if profileDB != nil && profileDB.HasTable(&schema.AICodingSession{}) {
		binding, err := GetAICodingSession(profileDB, sessionID)
		if err == nil {
			if err := validateCodingBinding(ctx, profileDB, binding, progress); err != nil {
				return err
			}
			applyCodingBinding(params, binding)
			return nil
		}
		if !gorm.IsRecordNotFoundError(err) {
			return err
		}
	}
	// Preserve a saved session mode even without cached-config opt-in. Coding
	// sessions normally return above using their durable binding record.
	if projectDB != nil && sessionID != "" {
		meta, err := GetAISessionMetaBySessionID(projectDB, sessionID)
		if err != nil && !gorm.IsRecordNotFoundError(err) {
			return err
		}
		if err == nil && meta.StartParams != "" {
			cached, err := UnmarshalAISessionStartParams(meta.StartParams)
			if err != nil {
				return err
			}
			if cached != nil {
				params.AgentMode = cached.GetAgentMode()
				params.CodingWorkspaceID = cached.GetCodingWorkspaceID()
				params.CodingWorkingDirectory = cached.GetCodingWorkingDirectory()
				params.CodingWorktreeID = cached.GetCodingWorktreeID()
				params.CodingCreateWorktree = cached.GetCodingCreateWorktree()
				params.CodingWorktreeBaseRef = cached.GetCodingWorktreeBaseRef()
			}
		}
	}
	if err := params.NormalizeAgentMode(); err != nil {
		return err
	}
	if params.GetAgentMode() != ypb.AgentModeCoding {
		clearCodingBinding(params)
		return nil
	}
	if sessionID == "" {
		return fmt.Errorf("coding sessions require a persistent session ID")
	}
	workspace, err := GetAICodingWorkspace(profileDB, params.GetCodingWorkspaceID())
	if err != nil {
		return fmt.Errorf("coding workspace unavailable: %w", err)
	}
	if params.CodingCreateWorktree && params.CodingWorktreeID != "" {
		return fmt.Errorf("choose either a new or existing worktree")
	}
	if !params.CodingCreateWorktree && params.CodingWorktreeBaseRef != "" {
		return fmt.Errorf("base ref is only valid for a new worktree")
	}
	binding := &schema.AICodingSession{SessionID: sessionID, WorkspaceID: workspace.WorkspaceID,
		SourceRoots: append([]string(nil), workspace.RootPaths...), WorkspaceRoots: append([]string(nil), workspace.RootPaths...), WorkingDirectory: workspace.RootPath}
	var reserved *schema.AICodingWorktree
	if params.CodingWorktreeID != "" {
		item, err := GetAICodingWorktree(profileDB, workspace.WorkspaceID, params.CodingWorktreeID, "")
		if err != nil {
			return err
		}
		if item.WorkspaceID != workspace.WorkspaceID || item.State != "ready" {
			return fmt.Errorf("worktree is not ready or belongs to a different workspace")
		}
		if _, err := coding.InspectWorktree(ctx, item.Path, item.GitCommonDir, item.WorkingDirectory); err != nil {
			return err
		}
		binding.WorktreeID, binding.WorkingDirectory = item.WorktreeID, item.WorkingDirectory
		binding.SourceRoots[0], binding.WorkspaceRoots[0] = item.SourceDirectory, item.WorkingDirectory
	} else if params.CodingCreateWorktree {
		repo, err := coding.InspectRepository(ctx, workspace.RootPath)
		if err != nil {
			return err
		}
		base := params.CodingWorktreeBaseRef
		if base == "" {
			branches, err := coding.ListBranches(ctx, workspace.RootPath)
			if err != nil {
				return err
			}
			base = branches.DefaultRef
		}
		commit, err := coding.ResolveBase(ctx, repo, base)
		if err != nil {
			return err
		}
		id := uuid.NewString()
		parent, err := coding.Directory(consts.GetDefaultYakitBaseDir())
		if err != nil {
			return err
		}
		path := filepath.Join(parent, "coding-worktrees", workspace.WorkspaceID, id)
		cwd := filepath.Join(path, repo.RelativeDirectory)
		reserved = &schema.AICodingWorktree{WorktreeID: id, WorkspaceID: workspace.WorkspaceID, OwnerSessionID: sessionID,
			SourceDirectory: workspace.RootPath, RepositoryRoot: repo.Root, GitCommonDir: repo.CommonDir,
			Path: path, WorkingDirectory: cwd, BaseRef: base, BaseCommit: commit, State: "creating"}
		binding.WorktreeID, binding.WorkingDirectory, binding.WorkspaceRoots[0] = id, cwd, cwd
	}
	if params.CodingWorkingDirectory != "" && filepath.Clean(params.CodingWorkingDirectory) != binding.WorkingDirectory {
		return fmt.Errorf("working directory is resolved from the workspace and managed worktree")
	}
	// Validate source/secondary directories before recording a binding. The new
	// primary checkout is materialized only after its reservation is durable.
	for i, root := range binding.WorkspaceRoots {
		if i == 0 && reserved != nil {
			continue
		}
		canonical, err := coding.Directory(root)
		if err != nil {
			return err
		}
		if canonical != root {
			return fmt.Errorf("workspace directory identity changed: %s", root)
		}
	}
	tx := profileDB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback()
	if reserved != nil {
		if err := tx.Create(reserved).Error; err != nil {
			return err
		}
	}
	if err := tx.Create(binding).Error; err != nil {
		return err
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}
	if reserved != nil {
		if err := ensureCodingWorktree(ctx, profileDB, reserved, progress); err != nil {
			return err
		}
	}
	applyCodingBinding(params, binding)
	return nil
}
