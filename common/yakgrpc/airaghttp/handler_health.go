package airaghttp

import (
	"net/http"

	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
)

// aiModeInfo 返回当前 AI 模式信息
func (s *RAGHTTPServer) aiModeInfo() map[string]interface{} {
	if s.config.UseCustomAIConfig() {
		return map[string]interface{}{
			"mode":   "single",
			"type":   s.config.AI.Type,
			"model":  s.config.AI.Model,
			"domain": s.config.AI.Domain,
		}
	}
	return map[string]interface{}{
		"mode": "tiered",
		"type": s.config.AI.Type,
	}
}

// handleHealth GET /health 健康检查
// 关键词: health endpoint, readiness, inflight, ai mode
func (s *RAGHTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":              true,
		"collectionCount": len(s.readyCollections),
		"collections":     s.readyCollections,
		"concurrent":      s.config.Concurrent,
		"inflight":        s.getInflight(),
		"language":        s.config.Language,
		"maxIteration":    s.config.MaxIteration,
		"timeout":         s.config.Timeout,
		"authRequired":    s.config.AuthToken != "",
		"ai":              s.aiModeInfo(),
	})
}

// collectionDetail 单个集合的详细信息
type collectionDetail struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ModelName   string `json:"modelName"`
	Dimension   int    `json:"dimension"`
}

// handleCollections GET /collections 列出可用知识库集合
// 关键词: list collections, collection info
func (s *RAGHTTPServer) handleCollections(w http.ResponseWriter, r *http.Request) {
	details := make([]collectionDetail, 0, len(s.readyCollections))
	for _, name := range s.readyCollections {
		detail := collectionDetail{Name: name}
		if s.db != nil {
			if info, err := vectorstore.GetCollectionInfo(s.db, name); err == nil && info != nil {
				detail.Description = info.Description
				detail.ModelName = info.ModelName
				detail.Dimension = info.Dimension
			}
		}
		details = append(details, detail)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":          true,
		"total":       len(details),
		"collections": details,
	})
}
