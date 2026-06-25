package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

func parseRoutingProfileFromAgentJSON(raw string, rt *Runtime) (*RoutingProfileV1, error) {
	raw = stripAITaggedJSONPayload(raw)
	if raw == "" {
		return nil, utils.Error("empty routing profile json")
	}
	var direct RoutingProfileV1
	if err := json.Unmarshal([]byte(raw), &direct); err == nil {
		if err := normalizeRoutingProfileV1(&direct, rt); err == nil {
			return &direct, nil
		}
	}
	var flex map[string]any
	if err := json.Unmarshal([]byte(raw), &flex); err != nil {
		return nil, utils.Wrapf(err, "routing profile json")
	}
	return routingProfileFromFlexibleMap(flex, rt)
}

func routingProfileFromFlexibleMap(m map[string]any, rt *Runtime) (*RoutingProfileV1, error) {
	if m == nil {
		return nil, utils.Error("nil routing map")
	}
	p := &RoutingProfileV1{
		SchemaVersion: routingProfileSchemaVersion,
		ValidationStatus: "confirmed",
		Target: RoutingProfileTarget{
			ContextPath: "/",
		},
	}
	if rt != nil && rt.Session != nil {
		p.Target.Raw = rt.Session.TargetRaw
		p.Target.EffectiveOrigin = EffectiveTargetBaseURL(rt.Session)
	}
	if v, ok := m["validation_status"].(string); ok && strings.TrimSpace(v) != "" {
		p.ValidationStatus = strings.TrimSpace(v)
	}
	if spaces, ok := m["url_spaces"].([]any); ok {
		for i, item := range spaces {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			sp := RoutingURLSpace{
				ID:          fmtAnyString(row["id"]),
				Label:       fmtAnyString(row["label"]),
				MountPrefix: normURLPath(fmtAnyString(row["mount_prefix"])),
				Confidence:  parseFlexibleConfidence(row["confidence"]),
			}
			if sp.ID == "" {
				sp.ID = fmt.Sprintf("space_%d", i)
			}
			if sp.MountPrefix == "" {
				sp.MountPrefix = "/"
			}
			p.URLSpaces = append(p.URLSpaces, sp)
		}
	}
	baseOrigin := ""
	if rt != nil && rt.Session != nil {
		baseOrigin = strings.TrimSuffix(EffectiveTargetBaseURL(rt.Session), "/")
	}
	if eb, ok := m["effective_bases"].([]any); ok {
		for i, item := range eb {
			switch t := item.(type) {
			case string:
				s := strings.TrimSpace(t)
				if s == "" {
					continue
				}
				sid := fmt.Sprintf("space_%d", i)
				if strings.HasPrefix(s, "/") {
					p.EffectiveBases = append(p.EffectiveBases, RoutingEffectiveBase{
						SpaceID: sid,
						BaseURL: baseOrigin + s,
					})
				} else if strings.HasPrefix(s, "http") {
					p.EffectiveBases = append(p.EffectiveBases, RoutingEffectiveBase{
						SpaceID: sid,
						BaseURL: s,
					})
				}
			case map[string]any:
				p.EffectiveBases = append(p.EffectiveBases, RoutingEffectiveBase{
					SpaceID: fmtAnyString(t["space_id"]),
					BaseURL: fmtAnyString(t["base_url"]),
				})
			}
		}
	}
	if len(p.EffectiveBases) == 0 && len(p.URLSpaces) > 0 {
		for _, sp := range p.URLSpaces {
			p.EffectiveBases = append(p.EffectiveBases, RoutingEffectiveBase{
				SpaceID: sp.ID,
				BaseURL: baseOrigin + sp.MountPrefix,
			})
		}
	}
	if err := normalizeRoutingProfileV1(p, rt); err != nil {
		return nil, err
	}
	return p, nil
}

func parseFlexibleConfidence(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		switch s {
		case "high", "confirmed":
			return 0.9
		case "medium", "provisional":
			return 0.7
		case "low":
			return 0.4
		default:
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				return f
			}
		}
	}
	return 0.85
}

func normalizeRoutingProfileV1(p *RoutingProfileV1, rt *Runtime) error {
	if p == nil {
		return utils.Error("nil profile")
	}
	if p.SchemaVersion == 0 {
		p.SchemaVersion = routingProfileSchemaVersion
	}
	if strings.TrimSpace(p.ValidationStatus) == "" {
		p.ValidationStatus = "confirmed"
	}
	baseOrigin := ""
	if rt != nil && rt.Session != nil {
		baseOrigin = strings.TrimSuffix(EffectiveTargetBaseURL(rt.Session), "/")
		if p.Target.Raw == "" {
			p.Target.Raw = rt.Session.TargetRaw
		}
		if p.Target.EffectiveOrigin == "" {
			p.Target.EffectiveOrigin = EffectiveTargetBaseURL(rt.Session)
		}
	}
	for i := range p.URLSpaces {
		if p.URLSpaces[i].MountPrefix == "" {
			p.URLSpaces[i].MountPrefix = "/"
		}
		p.URLSpaces[i].MountPrefix = normURLPath(p.URLSpaces[i].MountPrefix)
		if p.URLSpaces[i].ID == "" {
			p.URLSpaces[i].ID = fmt.Sprintf("space_%d", i)
		}
	}
	if len(p.EffectiveBases) == 0 && len(p.URLSpaces) > 0 {
		for _, sp := range p.URLSpaces {
			p.EffectiveBases = append(p.EffectiveBases, RoutingEffectiveBase{
				SpaceID: sp.ID,
				BaseURL: baseOrigin + sp.MountPrefix,
			})
		}
	}
	for i := range p.EffectiveBases {
		if p.EffectiveBases[i].SpaceID == "" {
			p.EffectiveBases[i].SpaceID = fmt.Sprintf("space_%d", i)
		}
		if p.EffectiveBases[i].BaseURL != "" && !strings.HasPrefix(p.EffectiveBases[i].BaseURL, "http") {
			p.EffectiveBases[i].BaseURL = baseOrigin + normURLPath(p.EffectiveBases[i].BaseURL)
		}
	}
	return nil
}

func mountPrefixForControllerLayer(layer string) string {
	switch strings.ToLower(strings.TrimSpace(layer)) {
	case "admin":
		return "/admin"
	case "api":
		return "/api"
	case "web":
		return "/"
	default:
		return ""
	}
}

func bootstrapRoutingProfileFromComponentMap(rt *Runtime) error {
	if rt == nil || rt.Session == nil {
		return utils.Error("nil runtime")
	}
	comp, err := loadComponentPackageMap(rt.WorkDir)
	if err != nil || comp == nil || len(comp.Components) == 0 {
		return bootstrapRoutingProfileFromConfig(rt)
	}
	base := strings.TrimSuffix(EffectiveTargetBaseURL(rt.Session), "/")
	rp := &RoutingProfileV1{
		SchemaVersion:    routingProfileSchemaVersion,
		ValidationStatus: "provisional",
		Target: RoutingProfileTarget{
			Raw:             rt.Session.TargetRaw,
			EffectiveOrigin: EffectiveTargetBaseURL(rt.Session),
			ContextPath:     "/",
		},
	}
	seen := map[string]struct{}{}
	for _, c := range comp.Components {
		mp := mountPrefixForControllerLayer(c.ControllerLayer)
		if mp == "" {
			continue
		}
		if _, ok := seen[mp]; ok {
			continue
		}
		seen[mp] = struct{}{}
		rp.URLSpaces = append(rp.URLSpaces, RoutingURLSpace{
			ID:          strings.TrimPrefix(mp, "/"),
			Label:       c.Label,
			MountPrefix: mp,
			Confidence:  0.75,
		})
	}
	if len(rp.URLSpaces) == 0 {
		return bootstrapRoutingProfileFromConfig(rt)
	}
	for _, sp := range rp.URLSpaces {
		rp.EffectiveBases = append(rp.EffectiveBases, RoutingEffectiveBase{
			SpaceID: sp.ID,
			BaseURL: base + sp.MountPrefix,
		})
	}
	canonical, err := CanonicalRoutingProfileJSON(rp)
	if err != nil {
		return err
	}
	if rt.Repo != nil && rt.Session != nil && strings.TrimSpace(rt.Session.UUID) != "" {
		_ = rt.Repo.UpdateSessionFields(rt.Session.UUID, map[string]interface{}{
			"routing_profile_json": canonical,
		})
	}
	return WriteRoutingProfileFile(rt.WorkDir, canonical)
}
