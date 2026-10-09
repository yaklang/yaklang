package aimemory

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

// Amend manages one exact memory ID in a namespace. Change deletes the old
// memory and its indexes, then writes a replacement with a new ID.
func Amend(operator, memoryID, content string, tags []string, opts ...Option) (*Item, error) {
	c := &options{namespace: "default", ctx: context.Background()}
	for _, opt := range opts {
		opt(c)
	}
	if c.ctx == nil {
		return nil, fmt.Errorf("memory context is nil")
	}
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	if c.db == nil {
		c.db = consts.GetGormProjectDatabase()
	}
	if c.db == nil {
		return nil, fmt.Errorf("project database is unavailable")
	}
	if operator != "add" && operator != "delete" && operator != "change" {
		return nil, fmt.Errorf("operator must be add, delete or change")
	}
	if operator != "delete" && (strings.TrimSpace(content) == "" || len([]rune(content)) > 32768) {
		return nil, fmt.Errorf("content must contain 1..32768 characters")
	}
	if len(tags) > 32 {
		return nil, fmt.Errorf("at most 32 tags are allowed")
	}
	for _, tag := range tags {
		// The existing memory schema stores tags with CSV delimiters. Reject
		// ambiguous input instead of silently changing one tag into several.
		if strings.Contains(tag, ",") || tag != strings.TrimSpace(tag) || tag == "" {
			return nil, fmt.Errorf("memory tags must be nonempty, trimmed and contain no comma")
		}
		if len([]rune(tag)) > 128 {
			return nil, fmt.Errorf("tag must be at most 128 characters")
		}
	}
	var old schema.AIMemoryEntity
	if operator != "add" {
		if strings.TrimSpace(memoryID) == "" {
			return nil, fmt.Errorf("memory_id is required for delete/change")
		}
		if err := c.db.Where("session_id = ? AND memory_id = ?", c.namespace, memoryID).First(&old).Error; err != nil {
			return nil, fmt.Errorf("find memory in current namespace: %w", err)
		}
	} else if memoryID != "" {
		return nil, fmt.Errorf("add generates memory_id; do not supply one")
	}
	if operator == "delete" {
		if err := aimem.BatchCleanupMemories(c.ctx, c.db, c.namespace, []string{memoryID}); err != nil {
			return nil, err
		}
		return &Item{AIMemoryEntity: &old}, nil
	}
	// Prepare the storage/index backend before deleting anything.
	store, err := aimem.NewMemoryStore(c.namespace, aimem.WithDatabase(c.db))
	if err != nil {
		return nil, err
	}
	defer store.Close()
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	if operator == "change" {
		if err := aimem.BatchCleanupMemories(c.ctx, c.db, c.namespace, []string{memoryID}); err != nil {
			return nil, err
		}
	}
	entity := &aicommon.MemoryEntity{Id: uuid.NewString(), Content: content, Tags: tags, PotentialQuestions: []string{content}, T_Score: 1, CorePactVector: []float32{0, 0, 0, 0, 0, 0, 1}}
	saveErr := c.ctx.Err()
	if saveErr == nil {
		saveErr = store.SaveMemoryEntities(entity)
	}
	if err := saveErr; err != nil {
		if operator == "change" {
			restored := &aicommon.MemoryEntity{Id: old.MemoryID, Content: old.Content, Tags: []string(old.Tags), PotentialQuestions: []string(old.PotentialQuestions), C_Score: old.C_Score, O_Score: old.O_Score, R_Score: old.R_Score, E_Score: old.E_Score, P_Score: old.P_Score, A_Score: old.A_Score, T_Score: old.T_Score, CorePactVector: []float32(old.CorePactVector), ExpiresAt: old.ExpiresAt}
			if restoreErr := store.SaveMemoryEntities(restored); restoreErr != nil {
				return nil, fmt.Errorf("replacement failed: %v; restoring old memory failed: %w", err, restoreErr)
			}
		}
		return nil, err
	}
	var row schema.AIMemoryEntity
	if err := c.db.Where("memory_id = ? AND session_id = ?", entity.Id, c.namespace).First(&row).Error; err != nil {
		return nil, err
	}
	return &Item{AIMemoryEntity: &row}, nil
}
