package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/utils"
)

// Phase1PrepBundle is the merged output of Phase1A parallel prep tasks.
type Phase1PrepBundle struct {
	Version        int                       `json:"version"`
	GeneratedAt    string                    `json:"generated_at"`
	SessionUUID    string                    `json:"session_uuid"`
	Tasks          map[string]Phase1PrepTask `json:"tasks"`
	Warnings       []string                  `json:"warnings"`
	EffectiveBases []string                  `json:"effective_bases"`
	RouteCount     int                       `json:"route_count"`
	AuthStatus     string                    `json:"auth_status"`
}

type Phase1PrepTask struct {
	OK      bool   `json:"ok"`
	Source  string `json:"source,omitempty"`
	Summary string `json:"summary"`
	Error   string `json:"error,omitempty"`
}

// writeMinimalPhase1PrepBundle records Phase1 pipeline prep summary for contract checks.
func writeMinimalPhase1PrepBundle(rt *Runtime, source string) error {
	if rt == nil || rt.Session == nil {
		return utils.Error("nil runtime")
	}
	routeCount := 0
	if rt.Repo != nil {
		if eps, err := rt.Repo.ListHttpEndpoints(rt.Session.ID); err == nil {
			routeCount = len(eps)
		}
	}
	reconOK := false
	if _, err := os.Stat(store.Phase1ReconPath(rt.WorkDir)); err == nil {
		reconOK = true
	}
	if source == "" {
		source = "phase1_recon"
	}
	bundle := &Phase1PrepBundle{
		Version:        1,
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		SessionUUID:    rt.Session.UUID,
		RouteCount:     routeCount,
		EffectiveBases: parseEffectiveBasesFromSession(rt),
		Tasks: map[string]Phase1PrepTask{
			source: {OK: reconOK, Source: "react", Summary: "ReAct recon replaced parallel Yak prep"},
		},
	}
	if err := writePhase1PrepBundle(rt.WorkDir, bundle); err != nil {
		return err
	}
	if rt.Repo != nil {
		b, _ := json.MarshalIndent(bundle, "", "  ")
		_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, store.ArtifactPhase1PrepBundle, string(b))
	}
	return nil
}

func writePhase1PrepBundle(workDir string, bundle *Phase1PrepBundle) error {
	path := store.Phase1PrepBundlePath(workDir)
	b, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}
	return writeJSONFile(path, b)
}

func writeJSONFile(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func writeRouteCandidatesFromDB(rt *Runtime) (string, error) {
	eps, err := rt.Repo.ListHttpEndpoints(rt.Session.ID)
	if err != nil {
		return "", err
	}
	type cand struct {
		ID            uint   `json:"id"`
		Method        string `json:"method"`
		PathPattern   string `json:"path_pattern"`
		HandlerClass  string `json:"handler_class"`
		HandlerMethod string `json:"handler_method"`
		Source        string `json:"source"`
		Status        string `json:"status"`
	}
	out := make([]cand, 0, len(eps))
	for _, e := range eps {
		out = append(out, cand{
			ID: e.ID, Method: e.Method, PathPattern: e.PathPattern,
			HandlerClass: e.HandlerClass, HandlerMethod: e.HandlerMethod,
			Source: e.Source, Status: e.Status,
		})
	}
	payload := map[string]any{"candidates": out, "count": len(out)}
	b, _ := json.MarshalIndent(payload, "", "  ")
	if err := writeJSONFile(store.RouteCandidatesPath(rt.WorkDir), b); err != nil {
		return "", err
	}
	return store.RouteCandidatesPath(rt.WorkDir), nil
}

func parseEffectiveBasesFromSession(rt *Runtime) []string {
	if rt == nil || rt.Session == nil {
		return []string{"/"}
	}
	p, err := RoutingProfileFromSession(rt.Session)
	if err != nil || p == nil || len(p.EffectiveBases) == 0 {
		if base := strings.TrimSpace(EffectiveTargetBaseURL(rt.Session)); base != "" {
			return []string{strings.TrimRight(base, "/")}
		}
		return []string{"/"}
	}
	out := make([]string, 0, len(p.EffectiveBases))
	for _, eb := range p.EffectiveBases {
		if u := strings.TrimSpace(eb.BaseURL); u != "" {
			out = append(out, strings.TrimRight(u, "/"))
		}
	}
	if len(out) == 0 {
		return []string{"/"}
	}
	return out
}

func writeForwardingProfileFromSession(rt *Runtime) (string, error) {
	prof := map[string]any{
		"source":            "session_routing_profile",
		"effective_bases":   parseEffectiveBasesFromSession(rt),
		"routing_excerpt":   utils.ShrinkString(rt.Session.RoutingProfileJSON, 4000),
		"base_calibration": rt.Session.ApiBaseCalibrationMetaJSON,
	}
	b, _ := json.MarshalIndent(prof, "", "  ")
	if err := writeJSONFile(store.ForwardingProfilePath(rt.WorkDir), b); err != nil {
		return "", err
	}
	return store.ForwardingProfilePath(rt.WorkDir), nil
}

func writeForwardingProfileFromBaseCal(rt *Runtime, rep *BaseCalibrationReport) (string, error) {
	bases := []string{}
	for _, v := range rep.Variants {
		if v.Base != "" {
			bases = append(bases, v.Base)
		}
	}
	prof := map[string]any{
		"source":          "api_base_calibrator",
		"effective_bases": bases,
		"report":          rep,
	}
	b, _ := json.MarshalIndent(prof, "", "  ")
	if err := writeJSONFile(store.ForwardingProfilePath(rt.WorkDir), b); err != nil {
		return "", err
	}
	return store.ForwardingProfilePath(rt.WorkDir), nil
}
