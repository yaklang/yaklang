package loop_risk_enrich

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// surveyEnvironment exposes the size and shape of the fixed evidence set.
// It reads only compact metadata, never thousands of packet bodies into AI
// context. Counts are exact; endpoint clusters summarize a recent sample.
func surveyEnvironment(db *gorm.DB, env *riskEnvironment) (string, error) {
	if db == nil || env == nil || len(env.RuntimeIDs) == 0 {
		return "", fmt.Errorf("risk runtime environment unavailable")
	}
	flowPage, sample, err := yakit.QueryHTTPFlow(db, &ypb.QueryHTTPFlowRequest{
		RuntimeIDs:         env.RuntimeIDs,
		ExcludeRequestRaw:  true,
		ExcludeResponseRaw: true,
		Pagination:         &ypb.Paging{Page: 1, Limit: 1500, OrderBy: "id", Order: "desc"},
	})
	if err != nil {
		return "", err
	}
	totalFlows := int64(flowPage.TotalRecord)
	type bucket struct {
		name  string
		count int
	}
	counts := make(map[string]int)
	targetCount := 0
	for _, f := range sample {
		matched, _ := flowMatchesTarget(env.Scope, f)
		if matched {
			targetCount++
		}
		u, err := url.Parse(f.Url)
		if err != nil || u.Hostname() == "" {
			continue
		}
		path := u.EscapedPath()
		if path == "" {
			path = "/"
		}
		counts[fmt.Sprintf("%s %s%s (%d)", f.Method, u.Hostname(), path, f.StatusCode)]++
	}
	buckets := make([]bucket, 0, len(counts))
	for name, count := range counts {
		buckets = append(buckets, bucket{name: name, count: count})
	}
	sort.Slice(buckets, func(i, j int) bool {
		if buckets[i].count == buckets[j].count {
			return buckets[i].name < buckets[j].name
		}
		return buckets[i].count > buckets[j].count
	})
	var b strings.Builder
	fmt.Fprintf(&b, "Runtime boundary: %d IDs. Total HTTP flows: %d.\n", len(env.RuntimeIDs), totalFlows)
	fmt.Fprintf(&b, "Recent metadata sample: %d flows; %d match the risk host/port. Top endpoint/status clusters (sample counts, not totals):\n", len(sample), targetCount)
	for _, item := range buckets[:min(len(buckets), 12)] {
		fmt.Fprintf(&b, "- %d × %s\n", item.count, item.name)
	}
	if totalFlows > int64(len(sample)) {
		b.WriteString("The inventory only samples recent metadata. Targeted searches query the entire runtime boundary, including older flows.\n")
	}
	return b.String(), nil
}

func surveyAction(invoker aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction("survey_risk_evidence",
		"Review the exact scoped flow total and recent endpoint clusters before choosing a discriminating search hypothesis.",
		nil, nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			env, err := environment(loop)
			if err != nil {
				op.Fail(err.Error())
				return
			}
			summary, err := surveyEnvironment(projectDB(invoker), env)
			if err != nil {
				op.Fail(err.Error())
				return
			}
			loop.Set("risk_enrich_inventory", summary)
			op.Feedback(summary)
		})
}
