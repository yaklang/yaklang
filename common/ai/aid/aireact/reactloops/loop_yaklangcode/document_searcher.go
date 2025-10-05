package loop_yaklangcode

import (
	"bytes"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ziputil"
)

// handleQueryDocument handles document query action
// Returns: documentResults string, shouldContinue bool
func handleQueryDocument(
	invoker aicommon.AIInvokeRuntime,
	searcher *ziputil.ZipGrepSearcher,
	payloads aitool.InvokeParams,
) (string, bool) {
	// If searcher is nil, try to create it
	if searcher == nil {
		log.Warn("document searcher not available, query_document action will be skipped")
		return "Document searcher not available. Please ensure yaklang-aikb is properly installed; ", false
	}

	caseSensitive := payloads.GetBool("case_sensitive")
	contextLines := payloads.GetInt("context_lines")
	if contextLines == 0 {
		contextLines = 2 // default context
	}
	limit := payloads.GetInt("limit")
	if limit == 0 {
		limit = 20 // default limit
	}

	var results []*ziputil.GrepResult

	// Build grep options
	grepOpts := []ziputil.GrepOption{
		ziputil.WithGrepCaseSensitive(caseSensitive),
		ziputil.WithContext(int(contextLines)),
	}

	// Add path filters if specified
	includePathSubString := payloads.GetStringSlice("include_path_substring")
	if len(includePathSubString) > 0 {
		grepOpts = append(grepOpts, ziputil.WithIncludePathSubString(includePathSubString...))
	}

	excludePathSubString := payloads.GetStringSlice("exclude_path_substring")
	if len(excludePathSubString) > 0 {
		grepOpts = append(grepOpts, ziputil.WithExcludePathSubString(excludePathSubString...))
	}

	includePathRegexp := payloads.GetStringSlice("include_path_regexp")
	if len(includePathRegexp) > 0 {
		grepOpts = append(grepOpts, ziputil.WithIncludePathRegexp(includePathRegexp...))
	}

	excludePathRegexp := payloads.GetStringSlice("exclude_path_regexp")
	if len(excludePathRegexp) > 0 {
		grepOpts = append(grepOpts, ziputil.WithExcludePathRegexp(excludePathRegexp...))
	}

	// Search by keywords
	for _, keyword := range payloads.GetStringSlice("keywords") {
		searchResult, err := searcher.GrepSubString(keyword, grepOpts...)
		if err != nil {
			log.Warnf("failed to grep keyword '%s': %v", keyword, err)
			continue
		}
		results = append(results, searchResult...)
	}

	// Search by regexp
	for _, reg := range payloads.GetStringSlice("regexp") {
		searchResults, err := searcher.GrepRegexp(reg, grepOpts...)
		if err != nil {
			log.Warnf("failed to grep regexp '%s': %v", reg, err)
			continue
		}
		results = append(results, searchResults...)
	}

	if len(results) == 0 {
		invoker.AddToTimeline("query_document", "no results found")
		return "No matching documents found for the query; ", false
	}

	// Apply RRF ranking to merge and rank results from multiple searches
	results = ziputil.MergeGrepResults(results)
	rankedResults := utils.RRFRankWithDefaultK(results)

	// Apply limit
	if limit > 0 && len(rankedResults) > int(limit) {
		rankedResults = rankedResults[:limit]
	}

	// Get max size from config (default 20KB)
	var maxSize int64 = 20 * 1024 // invoker.GetConfig().aikbResultMaxSize

	// Format results for AI consumption with size control
	var docBuffer bytes.Buffer
	docBuffer.WriteString("\n=== Document Query Results ===\n")
	docBuffer.WriteString(fmt.Sprintf("Found %d relevant documents:\n\n", len(rankedResults)))

	var includedResults int
	var truncated bool

	// Add results one by one, checking size limit
	for i, result := range rankedResults {
		resultStr := fmt.Sprintf("--- Result %d (Score: %.4f) ---\n", i+1, result.Score)
		resultStr += result.String()
		resultStr += "\n"

		// Check if adding this result would exceed the limit
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
	invoker.AddToTimeline("document_query_results", documentResults)
	return documentResults, true
}
