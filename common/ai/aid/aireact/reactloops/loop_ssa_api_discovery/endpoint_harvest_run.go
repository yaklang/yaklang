package loop_ssa_api_discovery

import (
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// RunEndpointHarvestForSession 按会话 language 运行已注册的静态 HTTP 抽取器并合并入 http_endpoints。
func RunEndpointHarvestForSession(rt *Runtime) (*EndpointHarvestReport, error) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return nil, utils.Error("nil runtime")
	}
	sess := rt.Session
	rep := &EndpointHarvestReport{
		GeneratedAt:           time.Now().UTC(),
		Language:              sess.Language,
		SourcesRun:            []string{},
		BySourceCount:         map[string]int{},
		StaticHarvestBySource: map[string]int{},
	}

	langEnum, lerr := ssaconfig.ValidateLanguage(sess.Language)
	if lerr != nil || langEnum == "" {
		rep.Warnings = append(rep.Warnings, "skip static harvest: invalid or empty language")
		return rep, nil
	}

	harvesters := staticHarvestersFor(langEnum)
	if len(harvesters) == 0 {
		rep.Warnings = append(rep.Warnings, "skip static harvest: no static harvester for language")
		rep.Notes = "该语言尚未注册静态 HTTP 路由抽取器；请使用 discovery_upsert_http_endpoint 等由模型写入。"
		return rep, nil
	}

	if !sess.CodePathOK || strings.TrimSpace(sess.CodeRootPath) == "" {
		rep.Warnings = append(rep.Warnings, "skip static harvest: code path invalid")
		return rep, nil
	}

	var allStatic []HarvestedEndpoint
	for _, h := range harvesters {
		rows, herr := h.fn(sess.CodeRootPath)
		if herr != nil {
			return nil, herr
		}
		rep.SourcesRun = append(rep.SourcesRun, h.sourceKey)
		rep.StaticHarvestBySource[h.sourceKey] = len(rows)
		allStatic = append(allStatic, rows...)
	}

	if k := "static_java_spring_annotations"; rep.StaticHarvestBySource[k] > 0 {
		rep.StaticSpringEndpoints = rep.StaticHarvestBySource[k]
	}

	ins, upd, merr := MergeHarvestedHttpEndpoints(rt.Repo, sess.ID, allStatic)
	if merr != nil {
		return nil, merr
	}
	rep.InsertedRows = ins
	rep.UpdatedRows = upd

	all, _ := rt.Repo.ListHttpEndpoints(sess.ID)
	rep.TotalHttpEndpoints = len(all)
	for _, e := range all {
		src := e.Source
		if src == "" {
			src = "(empty)"
		}
		rep.BySourceCount[src]++
	}

	staticKeys := make(map[string]struct{})
	for _, h := range allStatic {
		staticKeys[routeKey(h.Method, h.PathPattern)] = struct{}{}
	}
	for _, e := range all {
		if !strings.EqualFold(e.Source, "ai") {
			continue
		}
		k := routeKey(e.Method, e.PathPattern)
		if _, ok := staticKeys[k]; !ok {
			rep.AIOrphanHints = append(rep.AIOrphanHints, EndpointOrphan{
				ID: e.ID, Method: e.Method, PathPattern: e.PathPattern, HandlerClass: e.HandlerClass,
				Reason: "ai row not matched by any static harvest route key",
			})
		}
	}

	if len(allStatic) == 0 {
		rep.Warnings = append(rep.Warnings, "static harvest produced 0 endpoints (framework patterns missed or non-web project)")
	}
	return rep, nil
}
