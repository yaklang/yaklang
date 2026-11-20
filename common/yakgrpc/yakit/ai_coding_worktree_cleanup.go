package yakit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/coding"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

// Validate the allocation layout and each existing parent before destructive
// operations. A symlink replacing a bucket must not redirect cleanup elsewhere.
func validateManagedWorktreePath(item *schema.AICodingWorktree) error {
	base, err := coding.Directory(consts.GetDefaultYakitBaseDir())
	if err != nil {
		return err
	}
	for _, id := range []string{item.WorkspaceID, item.WorktreeID} {
		if id == "" || id == "." || id == ".." || filepath.Base(id) != id {
			return fmt.Errorf("invalid managed worktree identifier")
		}
	}
	expected := filepath.Join(base, "coding-worktrees", item.WorkspaceID, item.WorktreeID)
	if item.Path != expected {
		return fmt.Errorf("worktree path is outside its managed allocation")
	}
	for path := expected; path != base; path = filepath.Dir(path) {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("managed worktree path contains a non-directory or symlink: %s", path)
		}
	}
	return nil
}

type CodingCleanupResult struct {
	DeletedSessions   int64
	DeletedWorktrees  int64
	RetainedWorktrees int64
}

func removeCodingWorktreeFiles(ctx context.Context, db *gorm.DB, item *schema.AICodingWorktree) error {
	if err := validateManagedWorktreePath(item); err != nil {
		return err
	}
	// A retry after interruption must finish deletion, never recreate the checkout.
	if item.State == "ready" {
		if err := db.Model(item).Update("state", "deleting").Error; err != nil {
			return err
		}
	}
	if err := coding.RemoveWorktree(ctx, item.RepositoryRoot, item.Path, item.GitCommonDir); err != nil {
		if saveErr := db.Model(item).Update("last_error", err.Error()).Error; saveErr != nil {
			return fmt.Errorf("remove worktree: %v; persist failure: %w", err, saveErr)
		}
		return err
	}
	// Remove only an empty workspace bucket, never recursively remove siblings.
	_ = os.Remove(filepath.Dir(item.Path))
	return nil
}

// DeleteAICodingWorktree cleans up an orphan, a never-started reservation, or an
// interrupted deletion whose sessions were already quiesced by DeleteAISession.
// Ready checkouts with live session bindings must be deleted through those
// sessions, so their runtime and history are stopped/removed together.
func DeleteAICodingWorktree(ctx context.Context, db *gorm.DB, workspaceID, id string) error {
	if workspaceID == "" || id == "" {
		return fmt.Errorf("workspace ID and worktree ID are required")
	}
	codingLifecycleMu.Lock()
	defer codingLifecycleMu.Unlock()
	item, err := GetAICodingWorktree(db, workspaceID, id, "")
	if gorm.IsRecordNotFoundError(err) {
		return nil // A repeated delete is harmless.
	}
	if err != nil {
		return err
	}
	var count int
	if err := db.Model(&schema.AICodingSession{}).Where("worktree_id = ? AND (session_deleted_at IS NULL OR session_deleted_at = 0)", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 && item.State != "creating" && item.State != "failed" && item.State != "deleting" {
		return fmt.Errorf("worktree is used by %d session(s); delete the associated sessions first", count)
	}
	if err := removeCodingWorktreeFiles(ctx, db, item); err != nil {
		return err
	}
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback()
	if err := tx.Unscoped().Where("worktree_id = ?", id).Delete(&schema.AICodingSession{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Delete(item).Error; err != nil {
		return err
	}
	return tx.Commit().Error
}

// DeleteCodingSessions wraps project-history deletion. Disk failures leave the
// history and profile bindings available for retry. Callers must first quiesce
// the selected sessions. Bindings in other project DBs count as live references.
func DeleteCodingSessions(ctx context.Context, db *gorm.DB, sessionIDs []string, deleteHistory func() error) (CodingCleanupResult, error) {
	result := CodingCleanupResult{}
	codingLifecycleMu.Lock()
	defer codingLifecycleMu.Unlock()
	if db == nil || !db.HasTable(&schema.AICodingSession{}) || len(sessionIDs) == 0 {
		return result, deleteHistory()
	}
	var bindings []*schema.AICodingSession
	if err := db.Where("session_id IN (?)", sessionIDs).Find(&bindings).Error; err != nil {
		return result, err
	}
	worktrees := make(map[string]bool)
	var removed []*schema.AICodingWorktree
	retained := make(map[string]string)
	for _, binding := range bindings {
		if binding.WorktreeID == "" || worktrees[binding.WorktreeID] {
			continue
		}
		worktrees[binding.WorktreeID] = true
		var remaining int
		if err := db.Model(&schema.AICodingSession{}).
			Where("worktree_id = ? AND session_id NOT IN (?) AND (session_deleted_at IS NULL OR session_deleted_at = 0)", binding.WorktreeID, sessionIDs).
			Count(&remaining).Error; err != nil {
			return result, err
		}
		if remaining > 0 {
			var next schema.AICodingSession
			if err := db.Where("worktree_id = ? AND session_id NOT IN (?) AND (session_deleted_at IS NULL OR session_deleted_at = 0)", binding.WorktreeID, sessionIDs).
				Order("id asc").First(&next).Error; err != nil {
				return result, err
			}
			retained[binding.WorktreeID] = next.SessionID
			result.RetainedWorktrees++
			continue
		}
		item, err := GetAICodingWorktree(db, binding.WorkspaceID, binding.WorktreeID, "")
		if gorm.IsRecordNotFoundError(err) {
			continue
		}
		if err != nil {
			return result, err
		}
		if err := removeCodingWorktreeFiles(ctx, db, item); err != nil {
			return result, err
		}
		removed = append(removed, item)
	}
	if err := deleteHistory(); err != nil {
		return result, err
	}
	tx := db.Begin()
	if tx.Error != nil {
		return result, tx.Error
	}
	defer tx.Rollback()
	deleted := tx.Unscoped().Where("session_id IN (?)", sessionIDs).Delete(&schema.AICodingSession{})
	if deleted.Error != nil {
		return result, deleted.Error
	}
	result.DeletedSessions = deleted.RowsAffected
	for id, owner := range retained {
		if err := tx.Model(&schema.AICodingWorktree{}).Where("worktree_id = ? AND owner_session_id IN (?)", id, sessionIDs).
			Update("owner_session_id", owner).Error; err != nil {
			return result, err
		}
	}
	for _, item := range removed {
		// Also discard any legacy tombstones referring to the deleted checkout.
		if err := tx.Unscoped().Where("worktree_id = ?", item.WorktreeID).Delete(&schema.AICodingSession{}).Error; err != nil {
			return result, err
		}
		if err := tx.Unscoped().Delete(item).Error; err != nil {
			return result, err
		}
		result.DeletedWorktrees++
	}
	return result, tx.Commit().Error
}
