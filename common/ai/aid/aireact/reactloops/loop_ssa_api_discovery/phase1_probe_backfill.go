package loop_ssa_api_discovery

import (
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
)

func canonicalPathFromFullSampleURL(fullURL string) string {
	fullURL = strings.TrimSpace(fullURL)
	if fullURL == "" {
		return ""
	}
	u, err := url.Parse(fullURL)
	if err != nil {
		return ""
	}
	return normURLPath(u.Path)
}

func endpointStatusFromVerifiedRow(row *store.VerifiedHttpApi) string {
	if row == nil {
		return store.EndpointStatusPendingValidation
	}
	if row.Verified {
		return store.EndpointStatusAlive
	}
	switch row.ProbeStatusCode {
	case 401, 403:
		return store.EndpointStatusAuthFailed
	case 0:
		return store.EndpointStatusRejected
	default:
		return store.EndpointStatusRejected
	}
}

// ApplyVerifiedHttpApiProbeBackfill syncs a probed verified_http_apis row into http_endpoints and code_reading_plan.
func ApplyVerifiedHttpApiProbeBackfill(rt *Runtime, row *store.VerifiedHttpApi) error {
	if rt == nil || rt.Repo == nil || rt.Session == nil || row == nil {
		return nil
	}
	if !store.VerifiedHttpApiHasProbeEvidence(row) {
		return nil
	}

	originalPath := strings.TrimSpace(row.PathPattern)
	correctedPath := canonicalPathFromFullSampleURL(row.FullSampleURL)
	if correctedPath == "" {
		correctedPath = originalPath
	}

	if correctedPath != "" && correctedPath != originalPath {
		row.PathPattern = correctedPath
		if err := rt.Repo.UpdateVerifiedHttpApi(row); err != nil {
			return err
		}
		if err := updateCodeReadingPlanPath(rt, row.Method, originalPath, correctedPath); err != nil {
			log.Warnf("ssa_api_discovery: backfill code_reading_plan path %s -> %s: %v", originalPath, correctedPath, err)
		}
	}

	if err := backfillHttpEndpointFromVerifiedRow(rt, row, originalPath); err != nil {
		return err
	}
	return nil
}

func backfillHttpEndpointFromVerifiedRow(rt *Runtime, row *store.VerifiedHttpApi, lookupPath string) error {
	eps, err := rt.Repo.ListHttpEndpoints(rt.Session.ID)
	if err != nil {
		return err
	}
	method := strings.ToUpper(strings.TrimSpace(row.Method))
	path := strings.TrimSpace(row.PathPattern)
	lookup := normURLPath(lookupPath)
	if lookup == "" {
		lookup = path
	}

	var target *store.HttpEndpoint
	for i := range eps {
		ep := &eps[i]
		if strings.ToUpper(strings.TrimSpace(ep.Method)) != method {
			continue
		}
		epPath := normURLPath(ep.PathPattern)
		if epPath == lookup || epPath == path {
			target = ep
			break
		}
	}
	if target == nil {
		now := time.Now()
		target = &store.HttpEndpoint{
			SessionID:       rt.Session.ID,
			Method:          method,
			PathPattern:     path,
			HandlerClass:    row.HandlerFile,
			HandlerMethod:   row.HandlerSymbol,
			Source:          SourceAICodeRead,
			Status:          endpointStatusFromVerifiedRow(row),
			LastProbedAt:    &now,
			ProbeStatusCode: row.ProbeStatusCode,
			RejectReason:    row.RejectReason,
		}
		if row.Verified {
			target.RejectReason = ""
		}
		if ev, err := json.Marshal(map[string]any{
			"full_sample_url": row.FullSampleURL,
			"verdict_reason":  row.VerdictReason,
			"verified":        row.Verified,
		}); err == nil {
			target.ProbeEvidence = string(ev)
		}
		_, err := EndpointInsertionGateway(rt, target)
		return err
	}

	now := time.Now()
	target.PathPattern = path
	target.Status = endpointStatusFromVerifiedRow(row)
	target.LastProbedAt = &now
	target.ProbeStatusCode = row.ProbeStatusCode
	if row.Verified {
		target.RejectReason = ""
	} else if strings.TrimSpace(row.RejectReason) != "" {
		target.RejectReason = row.RejectReason
	}
	if row.HandlerFile != "" {
		target.HandlerClass = row.HandlerFile
	}
	if row.HandlerSymbol != "" {
		target.HandlerMethod = row.HandlerSymbol
	}
	if ev, err := json.Marshal(map[string]any{
		"full_sample_url": row.FullSampleURL,
		"verdict_reason":  row.VerdictReason,
		"verified":        row.Verified,
	}); err == nil {
		target.ProbeEvidence = string(ev)
	}
	return rt.Repo.UpdateHttpEndpoint(target)
}

func updateCodeReadingPlanPath(rt *Runtime, method, oldPath, newPath string) error {
	plan, err := LoadCodeReadingPlan(rt.WorkDir)
	if err != nil {
		return err
	}
	api := LookupDiscoveredAPI(plan, method, oldPath)
	if api == nil {
		return nil
	}
	api.PathPattern = normURLPath(newPath)
	return PersistCodeReadingPlan(rt, plan)
}

// BackfillAllVerifiedHttpApisCatalog applies probe backfill for every row with HTTP evidence.
func BackfillAllVerifiedHttpApisCatalog(rt *Runtime) error {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return nil
	}
	rows, err := rt.Repo.ListVerifiedHttpApis(rt.Session.ID)
	if err != nil {
		return err
	}
	for i := range rows {
		row := rows[i]
		if !store.VerifiedHttpApiHasProbeEvidence(&row) {
			continue
		}
		if err := ApplyVerifiedHttpApiProbeBackfill(rt, &row); err != nil {
			log.Warnf("ssa_api_discovery: probe backfill id=%d %s %s: %v", row.ID, row.Method, row.PathPattern, err)
		}
	}
	return nil
}
