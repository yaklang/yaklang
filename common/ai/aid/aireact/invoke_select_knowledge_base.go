package aireact

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// Deprecated: select explicit names using manage_knowledge_base(op=list).
func (r *ReAct) SelectKnowledgeBase(ctx context.Context, originQuery string) (*aicommon.SelectedKnowledgeBaseResult, error) {
	return nil, errLegacyRAGRetrieval
}
