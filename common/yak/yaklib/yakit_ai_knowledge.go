package yaklib

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type aiKnowledgeAmendment struct {
	Operator        string   `json:"operator"`
	KnowledgeBaseID int64    `json:"knowledge_base_id"`
	Collection      string   `json:"collection"`
	EntryID         int64    `json:"entry_id"`
	EntryUUID       string   `json:"entry_uuid"`
	Title           string   `json:"title"`
	Content         string   `json:"content"`
	Summary         string   `json:"summary"`
	KnowledgeType   string   `json:"knowledge_type"`
	Importance      int      `json:"importance"`
	SourcePage      int      `json:"source_page"`
	Keywords        []string `json:"keywords"`
	Questions       []string `json:"questions"`
}

// ManageKnowledge performs exact, database-bound amendments. Replacement and
// index cleanup share a transaction; evict graphs only after a successful commit.
// Keyword recall needs no external embedding service. New semantic embeddings
// are explicitly pending rather than retaining embeddings of replaced content.
func ManageKnowledge(requestJSON string, opts ...AIQueryOption) (map[string]any, error) {
	c, err := aiQueryOptions(opts)
	if err != nil {
		return nil, err
	}
	f := aiKnowledgeAmendment{Importance: 5, KnowledgeType: "note"}
	if err := decodeAIResourceFilter(requestJSON, &f); err != nil {
		return nil, err
	}
	if c.projectID != 0 {
		return nil, fmt.Errorf("knowledge uses the runtime profile database")
	}
	if f.Operator != "add" && f.Operator != "delete" && f.Operator != "change" {
		return nil, fmt.Errorf("operator must be add, delete or change")
	}
	if f.KnowledgeBaseID < 0 || f.EntryID < 0 || (f.KnowledgeBaseID == 0 && strings.TrimSpace(f.Collection) == "") {
		return nil, fmt.Errorf("select a knowledge base using knowledge_base_id or collection")
	}
	if utf8.RuneCountInString(f.Collection) > 256 {
		return nil, fmt.Errorf("collection name must be at most 256 characters")
	}
	if f.Operator == "add" {
		if f.EntryID != 0 || f.EntryUUID != "" {
			return nil, fmt.Errorf("add generates entry IDs; do not supply entry_id/entry_uuid")
		}
	} else if f.EntryID == 0 && strings.TrimSpace(f.EntryUUID) == "" {
		return nil, fmt.Errorf("delete/change requires an exact entry_id or entry_uuid from query_knowledge")
	}
	if f.Operator != "delete" {
		if strings.TrimSpace(f.Title) == "" || utf8.RuneCountInString(f.Title) > 512 || strings.TrimSpace(f.Content) == "" || utf8.RuneCountInString(f.Content) > 32768 {
			return nil, fmt.Errorf("title must be 1..512 characters and content 1..32768 characters")
		}
		if utf8.RuneCountInString(f.Summary) > 2048 || f.Importance < 1 || f.Importance > 10 || f.SourcePage < 0 || utf8.RuneCountInString(f.KnowledgeType) > 128 || strings.TrimSpace(f.KnowledgeType) == "" {
			return nil, fmt.Errorf("invalid metadata: summary <=2048, importance 1..10, source_page >=0, knowledge_type 1..128 characters")
		}
		if len(f.Keywords) > 32 || len(f.Questions) > 32 {
			return nil, fmt.Errorf("at most 32 keywords and questions")
		}
		for _, keyword := range f.Keywords {
			if utf8.RuneCountInString(keyword) > 128 {
				return nil, fmt.Errorf("keyword must be at most 128 characters")
			}
		}
		for _, question := range f.Questions {
			if utf8.RuneCountInString(question) > 512 {
				return nil, fmt.Errorf("question must be at most 512 characters")
			}
		}
	} else if f.Title != "" || f.Content != "" || f.Summary != "" || len(f.Keywords) > 0 || len(f.Questions) > 0 || f.SourcePage != 0 {
		return nil, fmt.Errorf("delete accepts selectors only; omit content metadata")
	}
	if c.profileDB == nil {
		return nil, fmt.Errorf("profile database is unavailable")
	}
	var kb schema.KnowledgeBaseInfo
	var old, row schema.KnowledgeBaseEntry
	var invalidated []*schema.VectorStoreCollection
	createdBase, removedIndexes := false, 0
	err = utils.GormTransaction(c.profileDB, func(tx *gorm.DB) error {
		if err := c.ctx.Err(); err != nil {
			return err
		}
		q := tx.Model(&schema.KnowledgeBaseInfo{})
		if f.KnowledgeBaseID > 0 {
			q = q.Where("id = ?", f.KnowledgeBaseID)
		}
		if f.Collection != "" {
			q = q.Where("knowledge_base_name = ?", f.Collection)
		}
		if err := q.First(&kb).Error; err != nil {
			if !gorm.IsRecordNotFoundError(err) || f.Operator != "add" || f.KnowledgeBaseID != 0 {
				return fmt.Errorf("find target knowledge base: %w", err)
			}
			kb = schema.KnowledgeBaseInfo{KnowledgeBaseName: f.Collection, KnowledgeBaseType: "knowledge", CreatedFromUI: false}
			if err := yakit.CreateKnowledgeBase(tx, &kb); err != nil {
				return err
			}
			createdBase = true
		}
		if f.Operator != "add" {
			q := tx.Model(&schema.KnowledgeBaseEntry{}).Where("knowledge_base_id = ?", kb.ID)
			if f.EntryID > 0 {
				q = q.Where("id = ?", f.EntryID)
			}
			if f.EntryUUID != "" {
				q = q.Where("hidden_index = ?", f.EntryUUID)
			}
			if err := q.First(&old).Error; err != nil {
				return fmt.Errorf("find exact entry in target knowledge base: %w", err)
			}
			if old.HiddenIndex == "" {
				return fmt.Errorf("entry has no stable UUID; repair the knowledge entry first")
			}
			if tx.HasTable(&schema.VectorStoreDocument{}) {
				// substr avoids treating '_'/'%' in imported UUIDs as LIKE wildcards.
				prefix := old.HiddenIndex + "_question_"
				docs := tx.Model(&schema.VectorStoreDocument{}).Where("document_id = ? OR substr(document_id,1,?) = ? OR (CASE WHEN json_valid(metadata) THEN json_extract(metadata,'$.meta_data_UUID') ELSE '' END) = ?", old.HiddenIndex, utf8.RuneCountInString(prefix), prefix, old.HiddenIndex)
				var collectionIDs []uint
				if err := docs.Pluck("DISTINCT collection_id", &collectionIDs).Error; err != nil {
					return err
				}
				if err := tx.Model(&schema.VectorStoreCollection{}).Select("id, uuid").Where("id IN (?)", collectionIDs).Find(&invalidated).Error; err != nil {
					return err
				}
				deleted := docs.Unscoped().Delete(&schema.VectorStoreDocument{})
				if deleted.Error != nil {
					return deleted.Error
				}
				removedIndexes = int(deleted.RowsAffected)
				if len(invalidated) > 0 {
					if err := tx.Model(&schema.VectorStoreCollection{}).Where("id IN (?)", collectionIDs).UpdateColumn("graph_binary", []byte(nil)).Error; err != nil {
						return err
					}
				}
			}
			if err := yakit.DeleteKnowledgeBaseEntryByHiddenIndex(tx, old.HiddenIndex); err != nil {
				return err
			}
			row = old
		}
		if f.Operator != "delete" {
			row = schema.KnowledgeBaseEntry{KnowledgeBaseID: int64(kb.ID), KnowledgeTitle: f.Title, KnowledgeDetails: f.Content, KnowledgeType: f.KnowledgeType, ImportanceScore: f.Importance, Summary: f.Summary, SourcePage: f.SourcePage, Keywords: schema.StringArray(f.Keywords), PotentialQuestions: schema.StringArray(f.Questions)}
			if err := yakit.CreateKnowledgeBaseEntry(tx, &row); err != nil {
				return err
			}
		}
		return c.ctx.Err()
	})
	if err != nil {
		return nil, err
	}
	yakit.InvalidateKnowledgeGraphs(c.profileDB, invalidated)
	status := "keyword_ready"
	semanticStatus := "pending"
	if f.Operator == "delete" {
		status = "deleted"
		semanticStatus = "removed"
	}
	result := map[string]any{"operator": f.Operator, "knowledge_base_id": kb.ID, "collection": kb.KnowledgeBaseName, "created_knowledge_base": createdBase, "entry_id": row.ID, "entry_uuid": row.HiddenIndex, "removed_index_count": removedIndexes, "index_status": status, "semantic_index_status": semanticStatus}
	if f.Operator != "add" {
		result["previous_entry_id"], result["previous_entry_uuid"] = old.ID, old.HiddenIndex
	}
	result["next_step"] = "Use query_knowledge with source=entries, knowledge_base_id and entry_id to read the saved original. New semantic/question vectors require the normal Yakit index-generation workflow; keyword retrieval is ready immediately."
	return result, nil
}
