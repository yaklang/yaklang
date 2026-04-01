package searchers

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/omnisearch/ostype"
)

type OmniQwenSearchClient struct {
}

func buildQwenOfficialContent(sr qwenSearchResult, response *QwenSearchResponse) string {
	if content := strings.TrimSpace(sr.Content); content != "" {
		return content
	}

	if len(response.ExtraToolInfo) == 0 {
		return ""
	}

	parts := make([]string, 0, len(response.ExtraToolInfo))
	seen := make(map[string]struct{})
	for _, info := range response.ExtraToolInfo {
		result, _ := info["result"].(string)
		result = strings.TrimSpace(result)
		if result == "" {
			continue
		}
		tool, _ := info["tool"].(string)
		content := result
		if tool != "" {
			content = fmt.Sprintf("[%s]\n%s", tool, result)
		}
		if _, ok := seen[content]; ok {
			continue
		}
		seen[content] = struct{}{}
		parts = append(parts, content)
	}

	return strings.Join(parts, "\n\n")
}

func NewOmniQwenSearchClient() *OmniQwenSearchClient {
	return &OmniQwenSearchClient{}
}

func (c *OmniQwenSearchClient) GetType() ostype.SearcherType {
	return ostype.SearcherTypeQwen
}

func (c *OmniQwenSearchClient) Search(query string, config *ostype.SearchConfig) ([]*ostype.OmniSearchResult, error) {
	qwenConfig := NewDefaultQwenConfig()

	if config.ApiKey != "" {
		qwenConfig.APIKey = config.ApiKey
	}
	if config.Timeout > 0 && config.Timeout.Seconds() >= 1 {
		qwenConfig.Timeout = config.Timeout.Seconds()
	}
	if config.BaseURL != "" {
		qwenConfig.BaseURL = config.BaseURL
	}
	if config.Proxy != "" {
		qwenConfig.Proxy = config.Proxy
	}
	if config.PageSize > 0 {
		// DashScope does not expose a search-result count parameter for native search_info,
		// so pagination is handled locally on official search_info results.
	}

	if config.Extra != nil {
		if model, ok := config.Extra["model"].(string); ok && model != "" {
			qwenConfig.Model = model
		}
		if strategy, ok := config.Extra["search_strategy"].(string); ok && strategy != "" {
			qwenConfig.SearchStrategy = strategy
		}
		if forced, ok := config.Extra["forced_search"].(bool); ok && forced {
			qwenConfig.ForcedSearch = true
		}
		if enableCitation, ok := config.Extra["enable_citation"].(bool); ok {
			qwenConfig.EnableCitation = enableCitation
		}
		if citationFormat, ok := config.Extra["citation_format"].(string); ok && citationFormat != "" {
			qwenConfig.CitationFormat = citationFormat
		}
		if enableSearchExtension, ok := config.Extra["enable_search_extension"].(bool); ok {
			qwenConfig.EnableSearchExtension = enableSearchExtension
		}
		if freshness, ok := config.Extra["freshness"].(int); ok {
			qwenConfig.Freshness = freshness
		}
		if freshness, ok := config.Extra["freshness"].(float64); ok {
			qwenConfig.Freshness = int(freshness)
		}
		if promptIntervene, ok := config.Extra["prompt_intervene"].(string); ok && promptIntervene != "" {
			qwenConfig.PromptIntervene = promptIntervene
		}
		if prependSearchResult, ok := config.Extra["prepend_search_result"].(bool); ok {
			qwenConfig.PrependSearchResult = prependSearchResult
		}
		if assignedSiteList, ok := config.Extra["assigned_site_list"].([]string); ok {
			qwenConfig.AssignedSiteList = assignedSiteList
		}
		if assignedSiteList, ok := config.Extra["assigned_site_list"].([]interface{}); ok {
			var sites []string
			for _, item := range assignedSiteList {
				if site, ok := item.(string); ok && site != "" {
					sites = append(sites, site)
				}
			}
			if len(sites) > 0 {
				qwenConfig.AssignedSiteList = sites
			}
		}
	}

	response, err := NewQwenSearchClient(qwenConfig).Search(query)
	if err != nil {
		return nil, err
	}

	searchResults := response.SearchResults
	if config.PageSize > 0 {
		startIdx := 0
		if config.Page > 1 {
			startIdx = (config.Page - 1) * config.PageSize
		}
		if startIdx < len(searchResults) {
			searchResults = searchResults[startIdx:]
		} else {
			searchResults = nil
		}
		if len(searchResults) > config.PageSize {
			searchResults = searchResults[:config.PageSize]
		}
	}

	var results []*ostype.OmniSearchResult
	for _, sr := range searchResults {
		content := buildQwenOfficialContent(sr, response)
		results = append(results, &ostype.OmniSearchResult{
			Title:      sr.Title,
			URL:        sr.URL,
			Content:    content,
			FaviconURL: sr.Icon,
			Source:     c.GetType().String(),
			Summary:    response.Summary,
			Data: map[string]any{
				"index":           sr.Index,
				"site_name":       sr.SiteName,
				"content":         sr.Content,
				"display_content": content,
				"extra_tools":     response.ExtraToolInfo,
			},
		})
	}

	// Preserve the synthesized answer even when DashScope returns no search_results.
	if len(results) == 0 && response.Summary != "" {
		results = append(results, &ostype.OmniSearchResult{
			Source:  c.GetType().String(),
			Summary: response.Summary,
		})
	}

	return results, nil
}
