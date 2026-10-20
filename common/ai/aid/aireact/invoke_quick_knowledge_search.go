package aireact

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const quickKnowledgeSearchPerSourceLimit = 5

type quickKnowledgeSearchItem struct {
	SourceType string
	KBName     string
	Title      string
	Content    string
	Keywords   []string
	UniqueKey  string
}

func normalizeQuickKnowledgeKeywords(query string, keywords []string) []string {
	var normalized []string
	seen := make(map[string]struct{})
	for _, item := range append(append([]string{}, keywords...), strings.Fields(query)...) {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, item)
	}
	return normalized
}

func appendQuickKnowledgeItem(items []quickKnowledgeSearchItem, seen map[string]struct{}, item quickKnowledgeSearchItem) []quickKnowledgeSearchItem {
	item.Title = strings.TrimSpace(item.Title)
	item.Content = strings.TrimSpace(item.Content)
	item.KBName = strings.TrimSpace(item.KBName)
	if item.UniqueKey == "" {
		item.UniqueKey = strings.ToLower(strings.Join([]string{item.KBName, item.Title, item.SourceType}, "|"))
	}
	if item.KBName == "" || item.Title == "" {
		return items
	}
	if _, ok := seen[item.UniqueKey]; ok {
		return items
	}
	seen[item.UniqueKey] = struct{}{}
	return append(items, item)
}

func formatQuickKnowledgeSearchItems(items []quickKnowledgeSearchItem) string {
	if len(items) == 0 {
		return ""
	}

	var buf strings.Builder
	for index, item := range items {
		buf.WriteString(fmt.Sprintf("%d. [%s][%s] %s\n", index+1, item.KBName, item.SourceType, item.Title))
		if item.Content != "" {
			buf.WriteString(utils.ShrinkString(item.Content, 800))
			buf.WriteString("\n")
		}
		if len(item.Keywords) > 0 {
			buf.WriteString("关键词: ")
			buf.WriteString(strings.Join(item.Keywords, ", "))
			buf.WriteString("\n")
		}
		buf.WriteString("\n")
	}
	return strings.TrimSpace(buf.String())
}

func loadQuickKnowledgeBaseInfos(db *gorm.DB, collections []string) ([]*schema.KnowledgeBaseInfo, error) {
	var kbInfos []*schema.KnowledgeBaseInfo
	err := db.Model(&schema.KnowledgeBaseInfo{}).
		Select("id, knowledge_base_name, rag_id").
		Where("knowledge_base_name IN (?)", collections).
		Find(&kbInfos).Error
	if err != nil {
		return nil, err
	}
	sort.SliceStable(kbInfos, func(i, j int) bool {
		return kbInfos[i].KnowledgeBaseName < kbInfos[j].KnowledgeBaseName
	})
	return kbInfos, nil
}

func quickKnowledgeLikeSearch(db *gorm.DB, kbIDs []int64, kbNameByID map[int64]string, keywords []string) ([]quickKnowledgeSearchItem, error) {
	if len(kbIDs) == 0 || len(keywords) == 0 {
		return nil, nil
	}

	var entries []*schema.KnowledgeBaseEntry
	query := db.Model(&schema.KnowledgeBaseEntry{}).Where("knowledge_base_id IN (?)", kbIDs)
	query = bizhelper.FuzzSearchWithStringArrayOrEx(query, []string{"knowledge_title", "knowledge_details", "keywords"}, keywords, false)
	query = query.Order("importance_score desc").Order("updated_at desc").Limit(quickKnowledgeSearchPerSourceLimit)
	if err := query.Find(&entries).Error; err != nil {
		return nil, err
	}

	items := make([]quickKnowledgeSearchItem, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		items = append(items, quickKnowledgeSearchItem{
			SourceType: "LIKE",
			KBName:     kbNameByID[entry.KnowledgeBaseID],
			Title:      entry.KnowledgeTitle,
			Content:    entry.KnowledgeDetails,
			Keywords:   entry.Keywords,
			UniqueKey:  fmt.Sprintf("entry:%d", entry.ID),
		})
	}
	return items, nil
}

func loadVectorCollectionUUIDs(db *gorm.DB, collections []string) (map[string]string, error) {
	var rows []*schema.VectorStoreCollection
	err := db.Model(&schema.VectorStoreCollection{}).
		Select("name, uuid").
		Where("name IN (?)", collections).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	result := make(map[string]string, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		result[row.Name] = row.UUID
	}
	return result, nil
}

func quickKnowledgeBM25Search(db *gorm.DB, collections []string, query string, keywords []string) ([]quickKnowledgeSearchItem, error) {
	searchTerms := normalizeQuickKnowledgeKeywords(query, keywords)
	if len(collections) == 0 || len(searchTerms) == 0 {
		return nil, nil
	}

	collectionUUIDByName, err := loadVectorCollectionUUIDs(db, collections)
	if err != nil {
		return nil, err
	}
	if len(collectionUUIDByName) == 0 {
		return nil, nil
	}

	entryIDsByCollection := make(map[string][]int64)
	entryIDSetByCollection := make(map[string]map[int64]struct{})
	for _, collection := range collections {
		uuid := collectionUUIDByName[collection]
		if uuid == "" {
			continue
		}
		docs, err := yakit.SearchVectorStoreDocumentBM25(db, &yakit.VectorDocumentFilter{
			CollectionUUID: uuid,
			Keywords:       searchTerms,
		}, quickKnowledgeSearchPerSourceLimit, 0)
		if err != nil {
			log.Warnf("quick knowledge search: bm25 search failed for collection %s: %v", collection, err)
			continue
		}
		for _, doc := range docs {
			if doc == nil {
				continue
			}
			entryID, convErr := strconv.ParseInt(strings.TrimSpace(doc.DocumentID), 10, 64)
			if convErr != nil || entryID <= 0 {
				continue
			}
			if entryIDSetByCollection[collection] == nil {
				entryIDSetByCollection[collection] = make(map[int64]struct{})
			}
			if _, ok := entryIDSetByCollection[collection][entryID]; ok {
				continue
			}
			entryIDSetByCollection[collection][entryID] = struct{}{}
			entryIDsByCollection[collection] = append(entryIDsByCollection[collection], entryID)
		}
	}

	var items []quickKnowledgeSearchItem
	for _, collection := range collections {
		entryIDs := entryIDsByCollection[collection]
		if len(entryIDs) == 0 {
			continue
		}

		var entries []*schema.KnowledgeBaseEntry
		if err := db.Model(&schema.KnowledgeBaseEntry{}).
			Where("id IN (?)", entryIDs).
			Find(&entries).Error; err != nil {
			return nil, err
		}

		entryByID := make(map[int64]*schema.KnowledgeBaseEntry, len(entries))
		for _, entry := range entries {
			if entry == nil {
				continue
			}
			entryByID[int64(entry.ID)] = entry
		}
		for _, entryID := range entryIDs {
			entry := entryByID[entryID]
			if entry == nil {
				continue
			}
			content := entry.Summary
			if content == "" {
				content = entry.KnowledgeDetails
			}
			items = append(items, quickKnowledgeSearchItem{
				SourceType: "BM25",
				KBName:     collection,
				Title:      entry.KnowledgeTitle,
				Content:    content,
				Keywords:   entry.Keywords,
				UniqueKey:  fmt.Sprintf("entry:%d", entry.ID),
			})
		}
	}
	return items, nil
}

// Deprecated: use query_knowledge_base with explicit scope and token limits.
func (r *ReAct) QuickKnowledgeSearch(ctx context.Context, query string, keywords []string, collections ...string) (string, error) {
	return "", errLegacyRAGRetrieval
}
