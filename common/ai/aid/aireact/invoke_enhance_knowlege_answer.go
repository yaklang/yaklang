package aireact

import (
	"context"
	"fmt"
)

// EnhanceKnowledgeGetterEx is retained for external runtime implementations.
// Deprecated: use the yaklang-system skill and its explicitly scoped knowledge
// tools. Automatic RAG retrieval and reference artifacts are no longer enabled.
func (r *ReAct) EnhanceKnowledgeGetterEx(ctx context.Context, userQuery string, enhancePlans []string, collections ...string) (string, error) {
	return "", fmt.Errorf("automatic knowledge enhancement is unavailable; require manage_knowledge_base, call op=list, then use query_knowledge_base with an explicit name, names or all=true and token_limit")
}
