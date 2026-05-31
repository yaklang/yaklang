package airaghttp

import (
	"net/http"

	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
)

// aiModeInfo 返回当前 AI 模式信息: quality(质量优先) 与 speed(速度优先) 两个通道
func (s *RAGHTTPServer) aiModeInfo() map[string]interface{} {
	quality := map[string]interface{}{"mode": "lightweight", "type": "aibalance", "model": LightweightModelName}
	if s.config.IsAIConfigured() {
		quality = map[string]interface{}{
			"mode":   "custom",
			"type":   s.config.AI.Type,
			"model":  s.config.AI.Model,
			"domain": s.config.AI.Domain,
		}
	}
	speed := map[string]interface{}{"mode": "lightweight", "type": "aibalance", "model": LightweightModelName}
	if s.config.IsLightweightAIConfigured() {
		speed = map[string]interface{}{
			"mode":   "custom",
			"type":   s.config.AILightweight.Type,
			"model":  s.config.AILightweight.Model,
			"domain": s.config.AILightweight.Domain,
		}
	}
	return map[string]interface{}{
		"quality": quality,
		"speed":   speed,
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
