package loop_risk_enrich

import (
	"fmt"
	"strconv"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const maxPacketBytes = 256 * 1024

// attachFlow never creates another risk or overwrites an existing request or
// response. The selected flow must belong to the risk/session runtime boundary
// AND have the same target. Endpoint checks prevent confusing sibling findings.
func attachFlow(db *gorm.DB, env *riskEnvironment, flowID int) error {
	if env == nil || len(env.RuntimeIDs) == 0 {
		return fmt.Errorf("flow is outside the initialized risk runtime boundary")
	}
	r, err := loadRisk(db, int(env.RiskID))
	if err != nil {
		return err
	}
	if flowID <= 0 {
		return fmt.Errorf("flow_id must be positive")
	}
	f, err := getScopedFlow(db, int64(flowID), env.RuntimeIDs)
	if err != nil {
		return err
	}
	s := env.Scope
	matched, exactPath := flowMatchesTarget(s, f)
	if !matched {
		return fmt.Errorf("flow host/port does not match the risk target")
	}
	if s.path != "" && !exactPath {
		return fmt.Errorf("flow is not from the risk endpoint; leave it as a candidate")
	}
	req, rsp := f.GetRequest(), f.GetResponse()
	if req == "" && rsp == "" {
		return fmt.Errorf("flow has no saved request/response evidence")
	}
	if len(req) > maxPacketBytes || len(rsp) > maxPacketBytes {
		return fmt.Errorf("flow packet exceeds snapshot limit; inspect the HTTP flow directly")
	}
	for _, pair := range r.PacketPairs {
		if pair != nil && pair.HTTPFlowId == int64(f.ID) {
			return nil // idempotent
		}
	}
	pairs := append(r.PacketPairs, &schema.PacketPair{
		HTTPFlowId: int64(f.ID), Url: f.Url, Request: req, Response: rsp,
	})
	update := &yakit.RiskHTTPFlowEvidenceUpdate{PacketPairs: pairs}
	// The legacy top-level request/response fields form a pair. Do not combine
	// a new response with a different pre-existing request (or vice versa).
	oldReq, err := strconv.Unquote(r.QuotedRequest)
	if err != nil {
		oldReq = r.QuotedRequest
	}
	oldRsp, err := strconv.Unquote(r.QuotedResponse)
	if err != nil {
		oldRsp = r.QuotedResponse
	}
	if r.QuotedRequest == "" && req != "" && (r.QuotedResponse == "" || oldRsp == rsp) {
		quoted := strconv.Quote(req)
		update.QuotedRequest = &quoted
	}
	if r.QuotedResponse == "" && rsp != "" && (r.QuotedRequest == "" || oldReq == req) {
		quoted := strconv.Quote(rsp)
		update.QuotedResponse = &quoted
	}
	return yakit.UpdateRiskHTTPFlowEvidence(db, int64(r.ID), update)
}
