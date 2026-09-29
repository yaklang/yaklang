package loop_risk_enrich

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const searchPageSize = 300

type flowCandidate struct {
	Flow  *schema.HTTPFlow
	Score int
	Why   string
}

// AI picks discriminating evidence clues, but cannot change the risk ID or
// runtime/session boundary bound during InitTask. BeforeID is a stable keyset
// cursor: old evidence does not require walking thousands of irrelevant rows.
type searchCriteria struct {
	URLContains       string
	RequestContains   string
	ResponseContains  string
	Method            string
	StatusCode        int
	TimeWindowMinutes int // ± minutes around the attached risk creation time
	BeforeID          int64
	Limit             int // number of representative endpoint groups in feedback
}

func (q searchCriteria) normalize() (searchCriteria, error) {
	q.URLContains = strings.TrimSpace(q.URLContains)
	q.RequestContains = strings.TrimSpace(q.RequestContains)
	q.ResponseContains = strings.TrimSpace(q.ResponseContains)
	q.Method = strings.ToUpper(strings.TrimSpace(q.Method))
	if q.URLContains == "" && q.RequestContains == "" && q.ResponseContains == "" && q.Method == "" && q.StatusCode == 0 && q.TimeWindowMinutes == 0 {
		return q, fmt.Errorf("provide a discriminating URL, request, response, method, status or risk time-window clue")
	}
	for _, term := range []string{q.URLContains, q.RequestContains, q.ResponseContains} {
		if len(term) > 256 {
			return q, fmt.Errorf("search clue exceeds 256 bytes")
		}
	}
	if len(q.Method) > 16 || strings.ContainsAny(q.Method, " \t\n\r") {
		return q, fmt.Errorf("invalid HTTP method")
	}
	if q.StatusCode != 0 && (q.StatusCode < 100 || q.StatusCode > 599) {
		return q, fmt.Errorf("invalid HTTP status code")
	}
	if q.TimeWindowMinutes < 0 || q.TimeWindowMinutes > 10080 {
		return q, fmt.Errorf("time_window_minutes must be between 0 and 10080")
	}
	if q.BeforeID < 0 {
		return q, fmt.Errorf("before_id must not be negative")
	}
	if q.Limit <= 0 {
		q.Limit = 12
	}
	if q.Limit > 30 {
		q.Limit = 30
	}
	return q, nil
}

func containsEvidence(haystack, needle string, decodeURL bool) bool {
	if needle == "" {
		return true
	}
	if strings.Contains(strings.ToLower(haystack), strings.ToLower(needle)) {
		return true
	}
	if decodeURL {
		decoded, err := url.QueryUnescape(haystack)
		return err == nil && strings.Contains(strings.ToLower(decoded), strings.ToLower(needle))
	}
	return false
}

// An encoded payload need not appear verbatim in the stored request. The
// longest readable token is a safe SQL *superset* prefilter; Go-side matching
// verifies the complete decoded clue before reporting a hit.
func readableAnchor(raw string) string {
	var best, current strings.Builder
	flush := func() {
		if current.Len() > best.Len() {
			best.Reset()
			best.WriteString(current.String())
		}
		current.Reset()
	}
	for _, ch := range raw {
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) {
			current.WriteRune(ch)
		} else {
			flush()
		}
	}
	flush()
	if best.Len() >= 3 {
		return best.String()
	}
	return ""
}

func cluePrefilterTerms(clue string, request bool) []string {
	if clue == "" {
		return nil
	}
	terms := []string{clue}
	if request {
		terms = append(terms, url.QueryEscape(clue), url.PathEscape(clue))
		if anchor := readableAnchor(clue); anchor != "" {
			terms = append(terms, anchor)
		}
	}
	result := make([]string, 0, len(terms))
	seen := map[string]bool{}
	for _, term := range terms {
		if term == "" || seen[term] {
			continue
		}
		seen[term] = true
		result = append(result, term)
	}
	return result
}

// searchFlows filters on the database BEFORE paging. One old matching packet
// is found directly even if 10,000 newer unrelated flows share the session.
// The returned page is grouped by the action before reaching the model.
func searchFlows(db *gorm.DB, r *schema.Risk, env *riskEnvironment, criteria searchCriteria) ([]flowCandidate, int64, bool, error) {
	q, err := criteria.normalize()
	if err != nil {
		return nil, 0, false, err
	}
	if db == nil || env == nil || r == nil || env.RiskID != int64(r.ID) || len(env.RuntimeIDs) == 0 {
		return nil, 0, false, fmt.Errorf("attached risk runtime environment is unavailable")
	}
	request := &ypb.QueryHTTPFlowRequest{
		RuntimeIDs:       env.RuntimeIDs,
		BeforeId:         q.BeforeID,
		Methods:          q.Method,
		IncludeInUrl:     cluePrefilterTerms(q.URLContains, false),
		RequestContains:  cluePrefilterTerms(q.RequestContains, true),
		ResponseContains: cluePrefilterTerms(q.ResponseContains, false),
		Full:             true,
		SkipTotal:        true,
		Pagination: &ypb.Paging{
			Page: 1, Limit: searchPageSize, OrderBy: "id", Order: "desc",
		},
	}
	if q.StatusCode != 0 {
		request.StatusCode = strconv.Itoa(q.StatusCode)
	}
	if q.TimeWindowMinutes > 0 {
		if r.CreatedAt.IsZero() {
			return nil, 0, false, fmt.Errorf("attached risk has no creation time for a time-window search")
		}
		window := time.Duration(q.TimeWindowMinutes) * time.Minute
		request.AfterCreatedAt = r.CreatedAt.Add(-window).Unix()
		request.BeforeCreatedAt = r.CreatedAt.Add(window).Unix()
	}
	_, rows, err := yakit.QueryHTTPFlow(db, request)
	if err != nil {
		return nil, 0, false, err
	}
	var matches []flowCandidate
	for _, f := range rows {
		if !containsEvidence(f.Url, q.URLContains, true) ||
			!containsEvidence(f.GetRequest(), q.RequestContains, true) ||
			!containsEvidence(f.GetResponse(), q.ResponseContains, false) {
			continue
		}
		sameTarget, exactPath := flowMatchesTarget(env.Scope, f)
		item := flowCandidate{Flow: f, Score: 2, Why: "attached risk/session runtime"}
		if sameTarget {
			item.Score += 2
			item.Why += ", same host/port"
		}
		if exactPath {
			item.Score += 3
			item.Why += ", exact endpoint"
		}
		if r.Parameter != "" && containsEvidence(f.GetRequest(), r.Parameter, true) {
			item.Score += 2
			item.Why += ", risk parameter"
		}
		if r.Payload != "" && containsEvidence(f.GetRequest(), unquoteRiskField(r.Payload), true) {
			item.Score += 3
			item.Why += ", risk payload"
		}
		matches = append(matches, item)
	}
	if len(rows) == 0 {
		return matches, 0, false, nil
	}
	return matches, int64(rows[len(rows)-1].ID), len(rows) == searchPageSize, nil
}

type evidenceGroup struct {
	Key            string
	Representative flowCandidate
	Count          int
}

func groupFlowCandidates(candidates []flowCandidate, limit int) []evidenceGroup {
	groups := map[string]*evidenceGroup{}
	for _, item := range candidates {
		u, err := url.Parse(item.Flow.Url)
		if err != nil {
			continue
		}
		key := fmt.Sprintf("%s %s://%s%s %d", item.Flow.Method, u.Scheme, u.Host, u.EscapedPath(), item.Flow.StatusCode)
		group, ok := groups[key]
		if !ok {
			groups[key] = &evidenceGroup{Key: key, Representative: item, Count: 1}
			continue
		}
		group.Count++
		if item.Score > group.Representative.Score {
			group.Representative = item
		}
	}
	result := make([]evidenceGroup, 0, len(groups))
	for _, group := range groups {
		result = append(result, *group)
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.Representative.Score != b.Representative.Score {
			return a.Representative.Score > b.Representative.Score
		}
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		return a.Key < b.Key
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

func searchHistoryPrompt(loop *reactloops.ReActLoop) string {
	if loop == nil {
		return ""
	}
	history, _ := loop.GetVariable("risk_enrich_searches").([]string)
	return strings.Join(history, "\n")
}

func searchFlowAction(invoker aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction("search_risk_flows",
		"Search the initialized risk's runtime boundary with a discriminating clue. Database filtering finds old hits directly; similar flows are grouped before presentation.",
		[]aitool.ToolOption{
			aitool.WithStringParam("url_contains", aitool.WithParam_Description("Endpoint path or URL fragment from the risk")),
			aitool.WithStringParam("request_contains", aitool.WithParam_Description("Parameter, payload or distinctive request fragment; URL-encoded matches are considered")),
			aitool.WithStringParam("response_contains", aitool.WithParam_Description("Response marker/error/observed proof")),
			aitool.WithStringParam("method", aitool.WithParam_Description("Exact HTTP method")),
			aitool.WithIntegerParam("status_code", aitool.WithParam_Description("Exact HTTP status code")),
			aitool.WithIntegerParam("time_window_minutes", aitool.WithParam_Description("Optional ± minutes around this attached risk's database creation time; use if payload/path clues are weak")),
			aitool.WithIntegerParam("before_id", aitool.WithParam_Description("Use next_before_id to continue to older *matching* evidence")),
			aitool.WithIntegerParam("limit", aitool.WithParam_Description("Representative endpoint groups, default 12, max 30")),
		}, nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			env, r, err := loadBoundRisk(loop, invoker)
			if err != nil {
				op.Fail(err.Error())
				return
			}
			q := searchCriteria{
				URLContains: action.GetString("url_contains"), RequestContains: action.GetString("request_contains"),
				ResponseContains: action.GetString("response_contains"), Method: action.GetString("method"),
				StatusCode: action.GetInt("status_code"), TimeWindowMinutes: action.GetInt("time_window_minutes"), BeforeID: int64(action.GetInt("before_id")), Limit: action.GetInt("limit"),
			}
			q, err = q.normalize()
			if err != nil {
				op.Fail(err.Error())
				return
			}
			matches, next, more, err := searchFlows(projectDB(invoker), r, env, q)
			if err != nil {
				op.Fail(err.Error())
				return
			}
			groups := groupFlowCandidates(matches, q.Limit)
			var b strings.Builder
			fmt.Fprintf(&b, "Risk #%d: %d verified clue matches in this page, %d representative clusters shown (next_before_id=%d, has_more=%t). These are candidates, not confirmed proof.\n", env.RiskID, len(matches), len(groups), next, more)
			for _, group := range groups {
				item := group.Representative
				fmt.Fprintf(&b, "- flow_id=%d group_count=%d %s score=%d (%s)\n", item.Flow.ID, group.Count,
					utils.ShrinkString(group.Key, 180), item.Score, item.Why)
			}
			if more {
				b.WriteString("Continue with next_before_id to inspect older matches, or refine the clue.\n")
			}
			history, _ := loop.GetVariable("risk_enrich_searches").([]string)
			history = append(history, fmt.Sprintf("url=%q req=%q rsp=%q method=%s status=%d window=%dm before=%d -> matches=%d groups=%d next=%d", q.URLContains, q.RequestContains, q.ResponseContains, q.Method, q.StatusCode, q.TimeWindowMinutes, q.BeforeID, len(matches), len(groups), next))
			if len(history) > 8 {
				history = history[len(history)-8:]
			}
			loop.Set("risk_enrich_searches", history)
			op.Feedback(b.String())
		})
}
