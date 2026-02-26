package loop_smart_qa

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const memoryMaxBytesLimit = 10240

func makeMemorySearchAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	desc := "Search the AI persistent memory system bound to the current session. " +
		"Supports semantic (embedding-based), BM25 (keyword relevance), keyword (tag-based), or combined search. " +
		"Results include timestamps."

	toolOpts := []aitool.ToolOption{
		aitool.WithStringParam("query",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("The search query to find relevant memories.")),
		aitool.WithStringParam("search_mode",
			aitool.WithParam_Description("Search mode: 'semantic', 'bm25', 'keyword', or 'all'. Default: 'all'."),
			aitool.WithParam_Default("all")),
		aitool.WithIntegerParam("limit",
			aitool.WithParam_Description("Maximum number of results. Default: 10."),
			aitool.WithParam_Default(10)),
		aitool.WithIntegerParam("bytes_limit",
			aitool.WithParam_Description("Maximum content size in bytes (max 10240). Default: 4096."),
			aitool.WithParam_Default(4096)),
	}

	return reactloops.WithRegisterLoopAction(
		"search_persistent_memory",
		desc, toolOpts,
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if strings.TrimSpace(action.GetString("query")) == "" {
				return utils.Error("query is required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			query := strings.TrimSpace(action.GetString("query"))
			searchMode := strings.ToLower(strings.TrimSpace(action.GetString("search_mode")))
			if searchMode == "" {
				searchMode = "all"
			}
			limit := int(action.GetInt("limit"))
			if limit <= 0 {
				limit = 10
			}
			bytesLimit := int(action.GetInt("bytes_limit"))
			if bytesLimit <= 0 {
				bytesLimit = 4096
			}
			if bytesLimit > memoryMaxBytesLimit {
				bytesLimit = memoryMaxBytesLimit
			}

			invoker := loop.GetInvoker()
			loop.LoadingStatus(fmt.Sprintf("searching persistent memory: %s (mode: %s)", query, searchMode))

			memTriage := loop.GetMemoryTriage()
			if utils.IsNil(memTriage) {
				op.Feedback("no memory triage available for this session")
				op.Continue()
				return
			}

			var content string
			var err error

			if triage, ok := memTriage.(*aimem.AIMemoryTriage); ok {
				content, err = doMemorySearch(triage, query, searchMode, limit, bytesLimit)
			} else {
				var result *aicommon.SearchMemoryResult
				result, err = memTriage.SearchMemoryWithoutAI(query, bytesLimit)
				if err == nil && result != nil {
					content = result.TotalContent
				}
			}

			if err != nil {
				log.Warnf("memory search failed: %v", err)
				op.Feedback(fmt.Sprintf("memory search failed: %v", err))
				op.Continue()
				return
			}

			if strings.TrimSpace(content) == "" {
				op.Feedback("no relevant memories found")
				op.Continue()
				return
			}

			appendMemoryResults(loop, content)
			invoker.AddToTimeline("memory_search_result",
				fmt.Sprintf("Memory search (%s): %s\n\n%s", searchMode, query, utils.ShrinkString(content, 2048)))

			op.Feedback(fmt.Sprintf("memory search completed for: '%s'", query))
			op.Continue()
		},
	)
}

func doMemorySearch(triage *aimem.AIMemoryTriage, query, searchMode string, limit, bytesLimit int) (string, error) {
	switch searchMode {
	case "semantic":
		results, err := triage.SearchBySemantics(query, limit)
		if err != nil {
			return "", err
		}
		return fmtSearchResults(results, bytesLimit, "semantic"), nil
	case "bm25":
		result, err := triage.SearchMemoryWithoutAI(query, bytesLimit)
		if err != nil {
			return "", err
		}
		if result == nil || len(result.Memories) == 0 {
			return "", nil
		}
		return fmtMemoryEntities(result.Memories, bytesLimit, "bm25"), nil
	case "keyword":
		keywords := strings.Fields(query)
		if len(keywords) == 0 {
			keywords = []string{query}
		}
		entities, err := triage.SearchByTags(keywords, false, limit)
		if err != nil {
			return "", err
		}
		return fmtMemoryEntities(entities, bytesLimit, "keyword"), nil
	default:
		result, err := triage.SearchMemoryWithoutAI(query, bytesLimit)
		if err != nil {
			return "", err
		}
		if result == nil || len(result.Memories) == 0 {
			return "", nil
		}
		return fmtMemoryEntities(result.Memories, bytesLimit, "all"), nil
	}
}

func fmtSearchResults(results []*aicommon.SearchResult, bytesLimit int, mode string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== Memory Search (mode: %s) ===\n\n", mode))

	totalBytes := 0
	count := 0
	for _, r := range results {
		if r == nil || r.Entity == nil {
			continue
		}
		entry := fmt.Sprintf("- [%s] (score: %.3f)\n  %s\n\n",
			r.Entity.CreatedAt.Format("2006-01-02 15:04:05"),
			r.Score, r.Entity.Content)
		entryBytes := len([]byte(entry))
		if totalBytes+entryBytes > bytesLimit {
			break
		}
		sb.WriteString(entry)
		totalBytes += entryBytes
		count++
	}
	sb.WriteString(fmt.Sprintf("--- %d memories, %d bytes ---\n", count, totalBytes))
	return sb.String()
}

func fmtMemoryEntities(entities []*aicommon.MemoryEntity, bytesLimit int, mode string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== Memory Search (mode: %s) ===\n\n", mode))

	totalBytes := 0
	count := 0
	for _, e := range entities {
		if e == nil {
			continue
		}
		entry := fmt.Sprintf("- [%s]", e.CreatedAt.Format("2006-01-02 15:04:05"))
		if len(e.Tags) > 0 {
			for _, t := range e.Tags {
				entry += " #" + t
			}
		}
		entry += "\n  " + e.Content + "\n\n"
		entryBytes := len([]byte(entry))
		if totalBytes+entryBytes > bytesLimit {
			break
		}
		sb.WriteString(entry)
		totalBytes += entryBytes
		count++
	}
	sb.WriteString(fmt.Sprintf("--- %d memories, %d bytes ---\n", count, totalBytes))
	return sb.String()
}

var memorySearchAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return makeMemorySearchAction(r)
}
