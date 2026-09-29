package loop_risk_enrich

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// targetScope narrows the strict runtime/session boundary to the risk's target.
// Sharing a runtime still does not prove a flow is evidence for this risk.
type targetScope struct {
	host string
	port int
	path string
}

func scopeForRisk(r *schema.Risk) targetScope {
	s := targetScope{host: strings.ToLower(strings.TrimSpace(r.Host)), port: r.Port}
	if s.host == "" {
		s.host = strings.ToLower(strings.TrimSpace(r.IP))
	}
	raw := strings.TrimSpace(r.Url)
	if raw == "" && r.Details != "" {
		var details map[string]any
		unquoted, err := strconv.Unquote(r.Details)
		if err != nil {
			unquoted = r.Details
		}
		if json.Unmarshal([]byte(unquoted), &details) == nil {
			raw, _ = details["target_input"].(string)
		}
	}
	if raw == "" && strings.Contains(r.Host, "/") {
		// Older AI risks stored a normalized host/path as Host, not Url.
		raw = r.Host
	}
	if raw != "" {
		hasScheme := strings.Contains(raw, "://")
		if !hasScheme {
			raw = "http://" + raw
		}
		if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
			s.host = strings.ToLower(u.Hostname())
			s.path = u.EscapedPath()
			if s.path == "/" {
				s.path = ""
			}
			if s.port == 0 && hasScheme {
				s.port, _ = strconv.Atoi(u.Port())
			}
			if s.port == 0 && hasScheme {
				if u.Scheme == "https" {
					s.port = 443
				} else if u.Scheme == "http" {
					s.port = 80
				}
			}
		}
	}
	return s
}

func flowMatchesTarget(s targetScope, f *schema.HTTPFlow) (bool, bool) {
	if s.host == "" || f == nil {
		return false, false
	}
	u, err := url.Parse(f.Url)
	if err != nil || !strings.EqualFold(u.Hostname(), s.host) {
		return false, false
	}
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		if u.Scheme == "https" {
			port = 443
		} else if u.Scheme == "http" {
			port = 80
		}
	}
	if s.port != 0 && port != s.port {
		return false, false
	}
	return true, s.path != "" && s.path == u.EscapedPath()
}

func runtimeIDsForRisk(db *gorm.DB, r *schema.Risk) ([]string, error) {
	seen := make(map[string]struct{})
	ids := make([]string, 0, 32)
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id != "" {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				ids = append(ids, id)
			}
		}
	}
	add(r.RuntimeId)
	if r.AISessionID == "" {
		return ids, nil
	}
	session, err := yakit.GetAISessionMetaBySessionID(db, r.AISessionID)
	if err != nil && !gorm.IsRecordNotFoundError(err) {
		return nil, err
	}
	if err == nil && session.RelatedRuntimeIDS != "" {
		var related []string
		if err := json.Unmarshal([]byte(session.RelatedRuntimeIDS), &related); err != nil {
			return nil, err
		}
		for _, id := range related {
			add(id)
		}
	}
	// Older sessions may only have AIAgentRuntime rows, not RelatedRuntimeIDS.
	coordinatorIDs, err := yakit.QueryAgentRuntimeUUIDsBySessionID(db, r.AISessionID)
	if err != nil {
		return nil, err
	}
	for _, id := range coordinatorIDs {
		add(id)
	}
	if len(ids) > 500 {
		return nil, fmt.Errorf("AI session has %d runtime IDs; bounded enrichment supports at most 500", len(ids))
	}
	return ids, nil
}

func getScopedFlow(db *gorm.DB, flowID int64, ids []string) (*schema.HTTPFlow, error) {
	if flowID <= 0 || len(ids) == 0 {
		return nil, fmt.Errorf("flow is outside the risk/AI session boundary or does not exist")
	}
	_, flows, err := yakit.QueryHTTPFlow(db, &ypb.QueryHTTPFlowRequest{
		RuntimeIDs: ids,
		IncludeId:  []int64{flowID},
		Full:       true,
		SkipTotal:  true,
		Pagination: &ypb.Paging{Page: 1, Limit: 1, OrderBy: "id", Order: "desc"},
	})
	if err != nil {
		return nil, fmt.Errorf("flow is outside the risk/AI session boundary or does not exist: %w", err)
	}
	if len(flows) != 1 {
		return nil, fmt.Errorf("flow is outside the risk/AI session boundary or does not exist")
	}
	return flows[0], nil
}
