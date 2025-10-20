package loop_yaklangcode

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ziputil"
	"github.com/yaklang/yaklang/common/yak/yakurl"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// YakdocSearchResult represents a search result from yakdoc
type YakdocSearchResult struct {
	Path    string
	Content string
}

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
		contextLines = 5 // default context
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

	// Search by lib_names and lib_function_globs using yakdoc
	yakdocResults := searchYakdocLibraries(payloads)
	if len(yakdocResults) > 0 {
		// Convert yakdoc results to ziputil.GrepResult format for consistency
		for _, yakdocResult := range yakdocResults {
			grepResult := &ziputil.GrepResult{
				FileName:    yakdocResult.Path,
				LineNumber:  1,
				Line:        yakdocResult.Content,
				Score:       1.0, // Default score for yakdoc results
				ScoreMethod: "yakdoc",
			}
			results = append(results, grepResult)
		}
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
	return documentResults, true
}

// searchYakdocLibraries searches yakdoc for library names and function globs using yakurl
func searchYakdocLibraries(payloads aitool.InvokeParams) []*YakdocSearchResult {
	var results []*YakdocSearchResult

	// Search by lib_names (case insensitive)
	libNames := payloads.GetStringSlice("lib_names")
	for _, libName := range libNames {
		// Use case insensitive library search
		response, err := searchLibraryCaseInsensitive(libName)
		if err != nil {
			log.Warnf("failed to load yakdocument for lib '%s': %v", libName, err)
			continue
		}

		resources := response.GetResources()
		if len(resources) > 0 {
			// Format library content from yakurl resources
			content := fmt.Sprintf("# Library: %s\n\n", libName)

			// Group resources by type
			functions := make([]string, 0)
			variables := make([]string, 0)

			for _, resource := range resources {
				switch resource.ResourceType {
				case "function":
					funcContent := fmt.Sprintf("### %s\n", resource.VerboseName)
					// Extract content from Extra field
					for _, extra := range resource.Extra {
						if extra.Key == "Content" {
							funcContent += extra.Value + "\n"
							break
						}
					}
					functions = append(functions, funcContent)
				case "variable":
					varContent := fmt.Sprintf("### %s\n", resource.VerboseName)
					variables = append(variables, varContent)
				}
			}

			// Add functions section
			if len(functions) > 0 {
				content += "## Functions:\n\n"
				for _, funcContent := range functions {
					content += funcContent + "\n"
				}
			}

			// Add variables section
			if len(variables) > 0 {
				content += "## Variables/Instances:\n\n"
				for _, varContent := range variables {
					content += varContent + "\n"
				}
			}

			results = append(results, &YakdocSearchResult{
				Path:    fmt.Sprintf("yakdoc://lib/%s", libName),
				Content: content,
			})
		}
	}

	// Search by lib_function_globs using yakurl fuzzy search (case insensitive)
	functionGlobs := payloads.GetStringSlice("lib_function_globs")
	for _, glob := range functionGlobs {
		// Use yakurl fuzzy search for function patterns
		// yakurl internally converts search terms to lowercase for fuzzy matching
		yakURL := fmt.Sprintf("yakdocument://%s", glob)
		response, err := yakurl.LoadGetResource(yakURL)
		if err != nil {
			log.Warnf("failed to search yakdocument for pattern '%s': %v", glob, err)
			continue
		}

		resources := response.GetResources()
		for _, resource := range resources {
			if resource.ResourceType == "function" {
				content := fmt.Sprintf("# Function: %s\n\n", resource.VerboseName)

				// Extract function content from Extra field
				for _, extra := range resource.Extra {
					if extra.Key == "Content" {
						content += extra.Value + "\n"
						break
					}
				}

				results = append(results, &YakdocSearchResult{
					Path:    fmt.Sprintf("yakdoc://func/%s", resource.ResourceName),
					Content: content,
				})
			}
		}
	}

	return results
}

// searchLibraryCaseInsensitive searches for a library with case insensitive matching
func searchLibraryCaseInsensitive(libName string) (*ypb.RequestYakURLResponse, error) {
	// Try original case first
	yakURL := fmt.Sprintf("yakdocument://%s/", libName)
	response, err := yakurl.LoadGetResource(yakURL)

	// If found resources, return immediately
	if err == nil && len(response.GetResources()) > 0 {
		return response, nil
	}

	// Try lowercase
	lowerLibName := strings.ToLower(libName)
	if lowerLibName != libName {
		yakURL = fmt.Sprintf("yakdocument://%s/", lowerLibName)
		response, err = yakurl.LoadGetResource(yakURL)
		if err == nil && len(response.GetResources()) > 0 {
			return response, nil
		}
	}

	// Try uppercase
	upperLibName := strings.ToUpper(libName)
	if upperLibName != libName && upperLibName != lowerLibName {
		yakURL = fmt.Sprintf("yakdocument://%s/", upperLibName)
		response, err = yakurl.LoadGetResource(yakURL)
		if err == nil && len(response.GetResources()) > 0 {
			return response, nil
		}
	}

	// Try title case
	titleLibName := strings.Title(strings.ToLower(libName))
	if titleLibName != libName && titleLibName != lowerLibName && titleLibName != upperLibName {
		yakURL = fmt.Sprintf("yakdocument://%s/", titleLibName)
		response, err = yakurl.LoadGetResource(yakURL)
		if err == nil && len(response.GetResources()) > 0 {
			return response, nil
		}
	}

	// If all attempts failed, return the last error
	return response, err
}
