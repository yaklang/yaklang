package loop_risk_enrich

import (
	"fmt"
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

type portSearchCriteria struct {
	HostContains        string
	Port                int
	ServiceContains     string
	FingerprintContains string
	BeforeID            int64
}

// Search port fingerprints in the same immutable runtime boundary. This is
// useful for non-HTTP risks and for old port-only results absent from survey.
func searchPorts(db *gorm.DB, env *riskEnvironment, q portSearchCriteria) ([]*schema.Port, int64, bool, error) {
	if db == nil || env == nil || len(env.RuntimeIDs) == 0 {
		return nil, 0, false, fmt.Errorf("attached risk runtime environment is unavailable")
	}
	q.HostContains = strings.TrimSpace(q.HostContains)
	q.ServiceContains = strings.TrimSpace(q.ServiceContains)
	q.FingerprintContains = strings.TrimSpace(q.FingerprintContains)
	if q.HostContains == "" && q.Port == 0 && q.ServiceContains == "" && q.FingerprintContains == "" {
		return nil, 0, false, fmt.Errorf("provide a host, port, service or fingerprint clue")
	}
	if q.Port < 0 || q.Port > 65535 || q.BeforeID < 0 || len(q.HostContains) > 128 || len(q.ServiceContains) > 128 || len(q.FingerprintContains) > 128 {
		return nil, 0, false, fmt.Errorf("invalid port search clue or cursor")
	}
	query := db.Model(&schema.Port{}).Where("runtime_id IN (?)", env.RuntimeIDs)
	if q.BeforeID > 0 {
		query = query.Where("id < ?", q.BeforeID)
	}
	if q.Port > 0 {
		query = query.Where("port = ?", q.Port)
	}
	query = addClueFilter(query, "host", q.HostContains, false)
	query = addClueFilter(query, "service_type", q.ServiceContains, false)
	query = addClueFilter(query, "fingerprint", q.FingerprintContains, false)
	var rows []*schema.Port
	if err := query.Order("id desc").Limit(100).Find(&rows).Error; err != nil {
		return nil, 0, false, err
	}
	var matches []*schema.Port
	for _, row := range rows {
		if containsEvidence(row.Host, q.HostContains, false) &&
			containsEvidence(row.ServiceType, q.ServiceContains, false) &&
			containsEvidence(row.Fingerprint, q.FingerprintContains, false) {
			matches = append(matches, row)
		}
	}
	if len(rows) == 0 {
		return matches, 0, false, nil
	}
	return matches, int64(rows[len(rows)-1].ID), len(rows) == 100, nil
}

func searchPortAction(invoker aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction("search_risk_ports",
		"Search existing port/service/fingerprint evidence in the attached risk's fixed runtime boundary, including older records.",
		[]aitool.ToolOption{
			aitool.WithStringParam("host_contains", aitool.WithParam_Description("Hostname or IP clue")),
			aitool.WithIntegerParam("port", aitool.WithParam_Description("Specific port, if known")),
			aitool.WithStringParam("service_contains", aitool.WithParam_Description("Service keyword e.g. ssh or http")),
			aitool.WithStringParam("fingerprint_contains", aitool.WithParam_Description("Software or fingerprint clue")),
			aitool.WithIntegerParam("before_id", aitool.WithParam_Description("next_before_id from the previous port search")),
		}, nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			env, err := environment(loop)
			if err != nil {
				op.Fail(err.Error())
				return
			}
			q := portSearchCriteria{
				HostContains: action.GetString("host_contains"), Port: action.GetInt("port"),
				ServiceContains: action.GetString("service_contains"), FingerprintContains: action.GetString("fingerprint_contains"),
				BeforeID: int64(action.GetInt("before_id")),
			}
			results, next, more, err := searchPorts(projectDB(invoker), env, q)
			if err != nil {
				op.Fail(err.Error())
				return
			}
			var b strings.Builder
			fmt.Fprintf(&b, "Port evidence: %d matches within the fixed boundary (next_before_id=%d, has_more=%t). Only matching risk targets can be written back.\n", len(results), next, more)
			for _, port := range results[:min(len(results), 25)] {
				fmt.Fprintf(&b, "- port_id=%d %s:%d/%s service=%s state=%s runtime=%s\n", port.ID, port.Host, port.Port, port.Proto, port.ServiceType, port.State, port.RuntimeId)
			}
			if len(results) > 25 {
				b.WriteString("Showing 25 representatives; add another clue to reduce noise.\n")
			}
			op.Feedback(b.String())
		})
}
