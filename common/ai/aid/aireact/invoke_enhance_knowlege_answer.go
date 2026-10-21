package aireact

import (
	"context"

	"github.com/yaklang/yaklang/common/utils"
)

var errLegacyRAGRetrieval = utils.Error("legacy AI RAG retrieval is disabled; load yaklang-system, use manage_knowledge_base(op=list) to select knowledge bases, then query_knowledge_base with an explicit name/names or all=true and token_limit")

// Deprecated: use the Skill-guided knowledge tools with explicit scope and budgets.
func (r *ReAct) EnhanceKnowledgeGetterEx(ctx context.Context, userQuery string, enhancePlans []string, collections ...string) (string, error) {
	return "", errLegacyRAGRetrieval
}
