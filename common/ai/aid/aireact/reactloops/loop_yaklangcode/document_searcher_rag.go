package loop_yaklangcode

import (
	"bytes"
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func handleRAGQueryDocument(
	invoker aicommon.AIInvokeRuntime,
	db *gorm.DB,
	collectionName string,
	payloads aitool.InvokeParams,
) (string, bool) {
	ragSys, err := rag.LoadCollection(db, collectionName)
	if err != nil {
		log.Warn("rag db not available, rag query action will be skipped")
		return fmt.Sprintf("please check yaklang enhance collection"), false
	}

	limit := int(payloads.GetInt("limit"))
	if limit == 0 {
		limit = 20 // default limit
	}

	allResult := make([]*aicommon.LazyEnhanceKnowledge, 0)

	ragSearch := func(query string) {
		results, err := ragSys.QueryTopN(query, limit) // 搜索最相似的5个文档
		if err != nil {
			log.Warnf("RAG search failed for question '%s': %v", query, err)
		}

		for _, result := range results {
			if result.Score >= 0.6 {
				log.Infof("high question similarity detected: '%s' vs existing document, similarity=%.3f",
					query, result.Score)

				var contentLoader func() string
				if result.Document.Type == schema.RAGDocumentType_QuestionIndex {
					contentLoader = func() string {
						uuid, ok := result.Document.Metadata.GetDataUUID()
						if ok && uuid != "" {
							originResult, err := yakit.GetKnowledgeBaseEntryByUUID(db, uuid)
							if err != nil {
								return ""
							}
							return originResult.KnowledgeDetails
						}
						return ""
					}
				} else {
					contentLoader = func() string {
						return result.Document.Content
					}
				}

				allResult = append(allResult, &aicommon.LazyEnhanceKnowledge{
					BasicEnhanceKnowledge: aicommon.BasicEnhanceKnowledge{
						UUID:   result.Document.ID,
						Score:  result.Score,
						Source: "rag_search",
					},
					ContentLoader: contentLoader,
				})
			}

		}
	}

	// Search by keywords
	for _, keyword := range payloads.GetStringSlice("keywords") {
		ragSearch(keyword)
	}

	// Search by regexp
	for _, q := range payloads.GetStringSlice("question") {
		ragSearch(q)
	}

	// Search by lib_names and lib_function_globs using yakdoc
	yakdocResults := searchYakdocLibraries(payloads)
	if len(yakdocResults) > 0 {
		// Convert yakdoc results to ziputil.GrepResult format for consistency
		for _, yakdocResult := range yakdocResults {
			searchResult := &aicommon.LazyEnhanceKnowledge{
				BasicEnhanceKnowledge: aicommon.BasicEnhanceKnowledge{
					Content: yakdocResult.Content,
					Source:  "yakdoc_library_search",
					Score:   0.8, // yak doc results get a fixed score
					UUID:    yakdocResult.Path,
				},
			}
			allResult = append(allResult, searchResult)
		}
	}

	if len(allResult) == 0 {
		invoker.AddToTimeline("query_document", "no results found")
		return "No matching documents found for the query; ", false
	}

	rankedResults := utils.RRFRankWithDefaultK(allResult)

	// Apply limit
	if limit > 0 && len(rankedResults) > int(limit) {
		rankedResults = rankedResults[:limit]
	}

	// Get max size from config (default 20KB)

	var maxSize int64 = 20 * 1024 // invoker.GetConfig().aikbResultMaxSize
	if ret := invoker.GetConfig().GetConfigInt64("aikb_result_max_size", 20*1024); ret > 0 {
		maxSize = int64(ret)
	}

	// Format results for AI consumption with size control
	var docBuffer bytes.Buffer
	docBuffer.WriteString("\n=== Document Query Results ===\n")
	docBuffer.WriteString(fmt.Sprintf("Found %d relevant documents:\n\n", len(rankedResults)))

	var includedResults int
	var truncated bool

	// Add results one by one, checking size limit
	for i, result := range rankedResults {
		resultStr := fmt.Sprintf("--- Result %d (Score: %.4f) ---\n", i+1, result.Score)
		resultStr += result.GetContent()
		resultStr += "\n"

		// Check if adding this allResult would exceed the limit
		if int64(docBuffer.Len()+len(resultStr)+100) > maxSize { // +100 for footer
			truncated = true
			break
		}

		docBuffer.WriteString(resultStr)
		includedResults++
	}

	// Add truncation notice if needed
	if truncated {
		docBuffer.WriteString(fmt.Sprintf("\n...[truncated: %d more results not shown due to size limit %d bytes]\n",
			len(rankedResults)-includedResults, maxSize))
		log.Warnf("document query results truncated: %d/%d results included (size limit: %d bytes)",
			includedResults, len(rankedResults), maxSize)
	}

	docBuffer.WriteString("=== End of Document Query Results ===\n")

	documentResults := docBuffer.String()

	// Final safety check - hard truncate if still too large
	if int64(len(documentResults)) > maxSize {
		documentResults = documentResults[:maxSize-100] + "\n...[truncated]\n=== End of Document Query Results ===\n"
		log.Warnf("document query results hard truncated to %d bytes", maxSize)
	}

	invoker.AddToTimeline("query_document", fmt.Sprintf("found %d documents (%d included)", len(rankedResults), includedResults))
	invoker.GetConfig().GetEmitter().EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, "query_document", documentResults)
	return documentResults, true
}
