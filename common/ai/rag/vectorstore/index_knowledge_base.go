package vectorstore

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func BuildVectorIndexForKnowledgeBaseEntry(db *gorm.DB, knowledgeBaseId int64, id string, opts ...any) (*SQLiteVectorStoreHNSW, error) {

	knowledgeBase, err := yakit.GetKnowledgeBase(db, knowledgeBaseId)
	if err != nil {
		return nil, err
	}

	defaultOptions := []any{
		WithDescription(knowledgeBase.KnowledgeBaseDescription),
	}
	collectionMg, err := GetCollection(db, knowledgeBase.KnowledgeBaseName, append(defaultOptions, opts...)...)
	if err != nil {
		return nil, err
	}

	entry, err := yakit.GetKnowledgeBaseEntryByHiddenIndex(db, id)
	if err != nil {
		return nil, err
	}

	content := entry.KnowledgeTitle
	if entry.Summary != "" {
		content += "\n\n" + entry.Summary
	}
	if entry.KnowledgeDetails != "" {
		content += "\n\n" + entry.KnowledgeDetails
	}

	// 构建元数据
	metadata := map[string]any{
		"knowledge_base_id":   entry.KnowledgeBaseID,
		"knowledge_title":     entry.KnowledgeTitle,
		"knowledge_type":      entry.KnowledgeType,
		"importance_score":    entry.ImportanceScore,
		"keywords":            entry.Keywords,
		"source_page":         entry.SourcePage,
		"potential_questions": entry.PotentialQuestions,
	}

	// 使用条目ID作为文档ID
	documentID := utils.InterfaceToString(entry.HiddenIndex)

	// 添加文档到RAG系统
	doc := &Document{
		ID:       documentID,
		Content:  content,
		Metadata: metadata,
	}

	err = collectionMg.Add(doc)
	if err != nil {
		count, err := collectionMg.Count()
		if err != nil {
			return nil, utils.Errorf("获取文档数量失败: %v", err)
		}
		println(count)
		return nil, utils.Errorf("添加文档到RAG系统失败 (ID: %s): %v", documentID, err)
	}
	return collectionMg, nil
}

// BuildVectorIndexForKnowledgeBase 构建向量索引
func BuildVectorIndexForKnowledgeBase(db *gorm.DB, id int64, opts ...any) (*SQLiteVectorStoreHNSW, error) {

	knowledgeBase, err := yakit.GetKnowledgeBase(db, id)
	if err != nil {
		return nil, err
	}

	defaultOptions := []any{
		WithDescription(knowledgeBase.KnowledgeBaseDescription),
	}
	collectionMg, err := GetCollection(db, knowledgeBase.KnowledgeBaseName, append(defaultOptions, opts...)...)
	if err != nil {
		return nil, err
	}

	log.Infof("loaded knowledge base: %s, id: %d", knowledgeBase.KnowledgeBaseName, id)

	// 清空所有索引并重建索引
	err = collectionMg.Clear()
	if err != nil {
		return nil, utils.Errorf("清空索引失败: %v", err)
	}

	log.Infof("start to build vector index for knowledge base: %s", knowledgeBase.KnowledgeBaseName)
	// 通过SearchKnowledgeBaseEntry函数，翻页去调用AddDocument函数，将知识项添加到知识库中
	page := 1
	limit := 100 // 每页处理100条记录

	for {
		// 分页获取知识库条目
		paging := &ypb.Paging{
			Page:  int64(page),
			Limit: int64(limit),
		}

		_, entries, err := yakit.GetKnowledgeBaseEntryByFilter(db, id, "", paging)
		if err != nil {
			return nil, utils.Errorf("搜索知识库条目失败: %v", err)
		}

		log.Infof("page %d: found %d entries in knowledge base: %s", page, len(entries), knowledgeBase.KnowledgeBaseName)
		// 如果没有更多条目，退出循环
		if len(entries) == 0 {
			break
		}

		// 将条目转换为文档并添加到RAG系统
		for _, entry := range entries {
			// 构建文档内容，包含标题、摘要和详细信息
			content := entry.KnowledgeTitle
			if entry.Summary != "" {
				content += "\n\n" + entry.Summary
			}
			if entry.KnowledgeDetails != "" {
				content += "\n\n" + entry.KnowledgeDetails
			}

			// 构建元数据
			metadata := map[string]any{
				"knowledge_base_id":   entry.KnowledgeBaseID,
				"knowledge_title":     entry.KnowledgeTitle,
				"knowledge_type":      entry.KnowledgeType,
				"importance_score":    entry.ImportanceScore,
				"keywords":            entry.Keywords,
				"source_page":         entry.SourcePage,
				"potential_questions": entry.PotentialQuestions,
			}

			// 使用条目ID作为文档ID
			documentID := utils.InterfaceToString(entry.ID)

			// 添加文档到RAG系统
			doc := &Document{
				ID:       documentID,
				Content:  content,
				Metadata: metadata,
			}

			err = collectionMg.Add(doc)
			if err != nil {
				count, coutnErr := collectionMg.Count()
				if coutnErr != nil {
					return nil, utils.Errorf("获取文档数量失败: %v", coutnErr)
				}
				println(count)
				return nil, utils.Errorf("添加文档到RAG系统失败 (ID: %s): %v", documentID, err)
			}
		}

		// 如果返回的条目数量少于限制，说明已经是最后一页
		if len(entries) < limit {
			break
		}

		// 准备下一页
		page++
	}

	return collectionMg, nil
}
