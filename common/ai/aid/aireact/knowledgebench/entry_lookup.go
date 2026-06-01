package knowledgebench

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// LookupKnowledgeEntryForRerank fetches a knowledge entry's summary fields
// for use in LLM reranking. Returns nil if the entry is not found.
func LookupKnowledgeEntryForRerank(entryID string) *RerankCandidate {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil
	}
	entry, err := yakit.GetKnowledgeBaseEntryByHiddenIndex(db, entryID)
	if err != nil {
		log.Debugf("lookup entry %s for rerank: %v", entryID, err)
		return nil
	}
	if entry == nil {
		return nil
	}
	return &RerankCandidate{
		EntryID:  entryID,
		Title:    entry.KnowledgeTitle,
		Summary:  entry.Summary,
		Keywords: entry.Keywords,
	}
}
