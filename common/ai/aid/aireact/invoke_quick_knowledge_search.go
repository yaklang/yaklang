package aireact

import "context"

// Deprecated: use query_knowledge_base with explicit scope and token limits.
func (r *ReAct) QuickKnowledgeSearch(ctx context.Context, query string, keywords []string, collections ...string) (string, error) {
	return "", errLegacyRAGRetrieval
}
