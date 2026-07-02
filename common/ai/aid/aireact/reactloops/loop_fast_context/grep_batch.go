package loop_fast_context

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// grepBatchSearch is one inline grep invocation inside grep_files_batch.
type grepBatchSearch struct {
	ID     string
	Params aitool.InvokeParams
}

type grepBatchRunResult struct {
	BatchAdded int
	Total      int
	BatchPaths []string
	ExecErrors []string
	FirstFatal error
}

func parseGrepBatchSearches(action *aicommon.Action) ([]grepBatchSearch, error) {
	if action == nil {
		return nil, utils.Error("action is nil")
	}

	items := action.GetInvokeParamsArray("searches")
	raw := strings.TrimSpace(action.GetString("searches"))
	if raw == "" {
		raw = strings.TrimSpace(action.GetInvokeParams("next_action").GetString("searches"))
	}
	if strings.HasPrefix(raw, "[") {
		parsed, err := parseJSONArrayOfObjects(raw)
		if err != nil {
			return nil, utils.Wrap(err, "searches must be a JSON array of objects")
		}
		items = parsed
	}
	if len(items) == 0 && raw != "" {
		parsed, err := parseJSONArrayOfObjects(raw)
		if err != nil {
			return nil, utils.Wrap(err, "searches must be a JSON array of objects")
		}
		items = parsed
	}

	if len(items) == 0 {
		return nil, utils.Error("searches is required: provide a non-empty array of {path, pattern, ...} objects")
	}

	searches := make([]grepBatchSearch, 0, len(items))
	for i, item := range items {
		params := normalizeGrepSearchParams(item)
		if err := validateGrepSearchParams(params); err != nil {
			return nil, utils.Wrapf(err, "searches[%d]", i)
		}
		id := strings.TrimSpace(item.GetString("id"))
		if id == "" {
			id = fmt.Sprintf("search_%d", i+1)
		}
		searches = append(searches, grepBatchSearch{
			ID:     id,
			Params: params,
		})
	}
	return searches, nil
}

func parseJSONArrayOfObjects(raw string) ([]aitool.InvokeParams, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, utils.Error("empty JSON array")
	}

	var items []map[string]any
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		var unquoted string
		if err2 := json.Unmarshal([]byte(raw), &unquoted); err2 == nil {
			if err3 := json.Unmarshal([]byte(unquoted), &items); err3 == nil {
				return mapsToInvokeParamsArray(items), nil
			}
		}
		return nil, err
	}
	return mapsToInvokeParamsArray(items), nil
}

func mapsToInvokeParamsArray(items []map[string]any) []aitool.InvokeParams {
	out := make([]aitool.InvokeParams, 0, len(items))
	for _, item := range items {
		out = append(out, aitool.InvokeParams(item))
	}
	return out
}

func normalizeGrepSearchParams(params aitool.InvokeParams) aitool.InvokeParams {
	if params == nil {
		params = aitool.InvokeParams{}
	} else {
		copied := make(aitool.InvokeParams, len(params))
		for k, v := range params {
			copied[k] = v
		}
		params = copied
	}
	delete(params, "id")

	params["output-mode"] = "files_with_matches"
	if params.GetInt("limit") <= 0 {
		if max := params.GetInt("max"); max > 0 {
			params["limit"] = max
			delete(params, "max")
		} else {
			params["limit"] = grepFilesWithMatchesLimit
		}
	}
	return params
}

func validateGrepSearchParams(params aitool.InvokeParams) error {
	if strings.TrimSpace(params.GetString("path")) == "" {
		return utils.Error("path is required (absolute directory or file)")
	}
	if strings.TrimSpace(params.GetString("pattern")) == "" {
		return utils.Error("pattern is required")
	}
	return nil
}

func runGrepBatch(
	loop interface {
		Get(string) string
		Set(string, any)
	},
	invoker aicommon.AIInvokeRuntime,
	ctx context.Context,
	searches []grepBatchSearch,
) grepBatchRunResult {
	concurrency := grepBatchConcurrency
	if cfg := invoker.GetConfig(); cfg != nil && cfg.GetToolComposeConcurrency() > 0 {
		concurrency = cfg.GetToolComposeConcurrency()
	}
	if concurrency <= 0 {
		concurrency = 1
	}

	var result grepBatchRunResult
	var mu sync.Mutex
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, search := range searches {
		search := search
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			select {
			case <-ctx.Done():
				mu.Lock()
				result.ExecErrors = append(result.ExecErrors, fmt.Sprintf("%s: %v", search.ID, ctx.Err()))
				mu.Unlock()
				return
			default:
			}

			toolResult, _, execErr := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, "grep", search.Params)
			if execErr != nil {
				errMsg := fmt.Sprintf("%s pattern=%q: %v", search.ID, search.Params.GetString("pattern"), execErr)
				mu.Lock()
				result.ExecErrors = append(result.ExecErrors, errMsg)
				if result.FirstFatal == nil {
					result.FirstFatal = execErr
				}
				mu.Unlock()
				return
			}

			paths := parseGrepFilesWithMatchesOutput(utils.InterfaceToString(toolResult.Data))
			added := mergePathsIntoFileIndex(loop, paths...)
			mu.Lock()
			result.BatchAdded += added
			result.BatchPaths = append(result.BatchPaths, paths...)
			mu.Unlock()

			invoker.AddToTimeline(
				fmt.Sprintf("[FASTCONTEXT_GREP_BATCH:%s]", search.ID),
				fmt.Sprintf("grep +%d paths pattern=%q", added, search.Params.GetString("pattern")),
			)
		}()
	}
	wg.Wait()

	result.Total = len(listFileIndex(loop))
	return result
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
