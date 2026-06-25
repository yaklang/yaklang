package loop_ssa_api_discovery

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// SupplementStaticRouteHints merges static hints into http_endpoints without overwriting AI routes.
func SupplementStaticRouteHints(ctx context.Context, invoker aicommon.AIInvokeRuntime, rt *Runtime) (inserted, merged int, err error) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return 0, 0, utils.Error("nil runtime")
	}

	hints, herr := readStaticRouteHintsReport(rt.WorkDir)
	if herr != nil {
		if rep, cerr := CollectStaticRouteHints(ctx, invoker, rt); cerr == nil && rep != nil {
			hints = rep
		} else if cerr != nil {
			return 0, 0, cerr
		}
	}
	if hints == nil || len(hints.Hints) == 0 {
		log.Infof("ssa_api_discovery: supplement static hints: none available")
		return 0, 0, nil
	}

	var harvested []HarvestedEndpoint
	for _, h := range hints.Hints {
		harvested = append(harvested, HarvestedEndpoint{
			Method:        h.Method,
			PathPattern:   h.PathPattern,
			HandlerClass:  h.HandlerClass,
			HandlerMethod: h.HandlerMethod,
			Provenance:    SourceStaticHint,
			FileRelPath:   h.FileRelPath,
		})
	}
	ins, upd, merr := MergeHarvestedHttpEndpointsSupplement(rt.Repo, rt.Session.ID, harvested)
	if merr != nil {
		return 0, 0, merr
	}
	log.Infof("ssa_api_discovery: supplement static hints inserted=%d merged=%d", ins, upd)
	return ins, upd, nil
}

// MergeHarvestedHttpEndpointsSupplement merges static hints; never overwrites AI-primary routes.
func MergeHarvestedHttpEndpointsSupplement(repo *store.Repository, sessionID uint, rows []HarvestedEndpoint) (inserted, updated int, err error) {
	if repo == nil {
		return 0, 0, utils.Error("nil repo")
	}
	existing, err := repo.ListHttpEndpoints(sessionID)
	if err != nil {
		return 0, 0, err
	}
	byKey := make(map[string]*store.HttpEndpoint)
	for i := range existing {
		e := &existing[i]
		byKey[routeKey(e.Method, e.PathPattern)] = e
	}
	for _, h := range rows {
		k := routeKey(h.Method, h.PathPattern)
		if cur, ok := byKey[k]; ok {
			if IsAIPrimaryEndpointSource(cur.Source) {
				continue
			}
			need := false
			if cur.HandlerClass == "" && h.HandlerClass != "" {
				cur.HandlerClass = h.HandlerClass
				need = true
			}
			if cur.HandlerMethod == "" && h.HandlerMethod != "" {
				cur.HandlerMethod = h.HandlerMethod
				need = true
			}
			if need {
				if err := repo.UpdateHttpEndpoint(cur); err != nil {
					log.Warnf("ssa_api_discovery: supplement update endpoint: %v", err)
				} else {
					updated++
				}
			}
			continue
		}
		row := &store.HttpEndpoint{
			SessionID:     sessionID,
			Method:        h.Method,
			PathPattern:   normURLPath(h.PathPattern),
			HandlerClass:  h.HandlerClass,
			HandlerMethod: h.HandlerMethod,
			Source:        SourceStaticHint,
			Status:        store.EndpointStatusPendingValidation,
		}
		if row.PathPattern == "" {
			row.PathPattern = "/"
		}
		if reason := NormalizeAndValidateEndpoint(row); reason != "" {
			log.Warnf("ssa_api_discovery: supplement rejected %s %s: %s", row.Method, row.PathPattern, reason)
			continue
		}
		if err := repo.CreateHttpEndpoint(row); err != nil {
			return inserted, updated, err
		}
		byKey[k] = row
		inserted++
	}
	return inserted, updated, nil
}
